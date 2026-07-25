# Claude Code × Codex 兼容链路 Bug 审计与修复里程碑

> 日期：2026-07-25
> 范围：`cc-switch → codex.selab.top → CLIProxyAPI → OpenAI Codex Responses`
> 状态：**M1 已实现并于 2026-07-25 部署；M2/M3 待后续设计与实施**
> 线上验证：`claude-tri` 的 `main` + 数字池 `4–12` 共 10 个现役容器均运行 `winbeau/cli-proxy-api:deploy-43116d2`，逐池 `/healthz` 正常，`xju-provision` active，maintenance gate 已清除。
> 说明：若 cc-switch 直接连接 `codex.selab.top/v1/messages`，问题位于 `server/cliproxy`，new-api L1 不参与该请求链路。

## 1. 背景与审计结论

实际使用 Claude Code 时出现了三个相关现象：

1. 主 agent 的上下文/token 数持续变化，但 Workflow、subagent 对应的 CPA 用量或额度像“卡住”一样，部分请求显示为 0；
2. 长对话、Workflow 和多 agent 场景容易直接收到：

   ```text
   API Error: 400 Your input exceeds the context window of this model
   ```

3. WebSearch、长工具调用和并发 subagent 偶尔长时间无响应。

为避免单个审计 agent 自身超出上下文，最终采用了细粒度 Workflow：19 个定向 scout、4 个归并 agent、4 个对抗核验 agent 和 1 个最终汇总 agent，共 28 个 agent。28/28 成功，0 个上下文错误。

本轮 Workflow 记录约 430 万 subagent tokens。该数字不等于最终上游账单，但能证明：**Workflow/subagent 确实产生了大量模型用量；CPA 页面不更新并不是因为它们没有消费 token。**

审计确认需要分成两条独立修复主线：

- **主线 A：usage accounting**——请求已实际执行，但 usage 没有稳定、准确地发布到 CPA/Redis usage sink；
- **主线 B：重试与上下文恢复**——context overflow 目前是终止错误，工具链中的终止等待和 cooldown 重试又会放大“卡住”的体感。

这不是单纯的 Claude Code UI 显示问题。客户端本地 token 估算、CLIProxy 内部 UsageReporter、CPA/Redis 队列并非同一实时数据源，因此主界面 token 在跳，不代表 CPA 已收到最终 usage。

---

## 2. 主线 A：Usage accounting

### 2.1 用户侧症状

- 主 agent token 数正常变化，但 Workflow/subagent 的账号用量迟迟不更新；
- 流式请求比非流式请求更容易出现 token 为 0；
- 并发任务被取消、提前结束或工具链收束后，CPA 请求数和 token 数可能都没有变化；
- 实际产生大量上游调用，但管理页看起来像没有消耗额度。

### 2.2 已确认根因

#### A1. 部分 Codex 流结束时没有发布任何主 usage 记录

当前 Codex 流主要在 `response.completed` 中成功解析到 usage 时才调用 UsageReporter。以下路径可能直接结束而不发布成功或失败记录：

- `response.completed` 存在，但没有可解析的 usage/service tier；
- clean EOF；
- context cancellation；
- 下游停止消费流，goroutine 在发送阶段退出；
- 部分提前结束路径。

相关位置：

- `server/cliproxy/internal/runtime/executor/codex_executor.go:898-905`
- `server/cliproxy/internal/runtime/executor/codex_executor.go:1145-1215`
- `server/cliproxy/internal/runtime/executor/codex_executor.go:1838-1845`

Workflow/subagent 并发多，取消和提前收束更常见，因此问题在多 agent 场景中更明显。

#### A2. `TrackFailure` 不能覆盖异步 goroutine 的终止结果

`ExecuteStream` 在返回 `StreamResult` 后，外层函数已经结束；此时 deferred `TrackFailure` 无法感知之后发生在流 goroutine 内的 EOF、取消或 scanner error。异步阶段必须自行完成 usage finalization。

相关位置：

- `server/cliproxy/internal/runtime/executor/codex_executor.go:1054-1055`
- `server/cliproxy/internal/runtime/executor/codex_executor.go:1145-1216`
- `server/cliproxy/internal/runtime/executor/helps/usage_helpers.go:186-207`

#### A3. UsageReporter 是 first-write-wins，但终止状态没有统一收口

`Publish`、`PublishFailure` 和 `EnsurePublished` 共用一个 `sync.Once`。这一机制适合保证 exactly-once，但前提是每条终止路径都必须明确选择：

- 带 usage 的成功；
- 无 usage 的零 token 成功；
- 带已知部分 usage 的失败。

当前没有统一 finalizer，因此部分路径根本没有触发这三者中的任何一个。

相关位置：

- `server/cliproxy/internal/runtime/executor/helps/usage_helpers.go:93-103`
- `server/cliproxy/internal/runtime/executor/helps/usage_helpers.go:186-240`

#### A4. Codex 缺失 `total_tokens` 时可能重复计算 reasoning

当前通用 fallback 使用：

```text
input + output + reasoning
```

但 OpenAI/Codex 的 reasoning 通常已经是 output 的子集，cache 通常已经是 input 的子集。再次相加会高估总量；反过来，如果只返回 cache/reasoning 细分字段，又可能留下 `TotalTokens=0`。

相关位置：

- `server/cliproxy/internal/runtime/executor/helps/usage_helpers.go:199-226`
- `server/cliproxy/internal/runtime/executor/helps/usage_helpers.go:586-625`

修复原则：

- 有上游 `total_tokens` 时优先保留；
- Codex 父级 input/output 齐全时，总量按 `input + output`；
- reasoning/cache 作为细分维度展示，不与父级重复相加；
- 上游完全不提供 usage 时，不用本地 tokenizer 伪造精确数字，只记零 token 成功或失败。

#### A5. Claude SSE 的 usage 在终端事件才出现

Codex 转 Anthropic 流时：

- `message_start` 的 input/output 为 0；
- 最终 `message_delta` 才带 input、output 和 cache。

相关位置：

- `server/cliproxy/internal/translator/codex/claude/codex_claude_response.go:94-102`
- `server/cliproxy/internal/translator/codex/claude/codex_claude_response.go:136-152`
- `server/cliproxy/internal/translator/codex/claude/codex_claude_response.go:781-798`

这是因为 Codex 通常直到 `response.completed` 才提供精确 usage。若不预估 token、也不缓存整条响应，就不能在早期 `message_start` 提供真实 input usage。因此本次不强行伪造早期 usage，而是确保终端 usage 和 CPA 内部 accounting 都可靠到达。

### 2.3 Usage 修复落地（2026-07-25）

M1 已由提交 `add7f2c` 实现，并随 commit 镜像 `deploy-43116d2` 部署到全部现役
CLIProxyAPI 动态池。实际落地内容：
1. 保留 `UsageReporter.once`，建立“每个 reporter 恰好一条主记录”的硬约束；
2. 增加“失败但保留已知部分 usage”的发布接口；
3. 非流式 `response.completed` 无 usage 时发布零 token 成功；
4. 流式 goroutine 增加统一 accounting finalizer；
5. clean EOF、scanner error、completion 前 cancellation 都发布失败，不再静默关闭；
6. completion 到达后先完成 accounting，再翻译和发送终止事件；
7. `response.completed` 作为 accounting 终止点，避免其后的错误覆盖已完成请求；
8. Codex total 使用 OpenAI 子集语义，避免 reasoning/cache 双重计算；
9. 增加 stream/non-stream、EOF、取消、零 usage、partial usage 和 exactly-once 回归测试；
10. 增加真实 Redis queue plugin 测试，确认 CPA 最终 payload 不再漏记。

详细实现与部署入口：

- usage helper：`server/cliproxy/internal/runtime/executor/helps/usage_helpers.go`
- Codex executor：`server/cliproxy/internal/runtime/executor/codex_executor.go`
- executor 集成回归：`server/cliproxy/internal/runtime/executor/usage_accounting_test.go`
- Redis queue 回归：`server/cliproxy/test/usage_logging_test.go`
- 一键部署：`deploy/deploy-cliproxy.sh`
- 运维说明：[docs/runbook.md](./runbook.md#升级先-pin-tag勿追-latest)

原始实施计划存档：

- `/home/winbeau/.claude/plans/sparkling-conjuring-anchor.md`

---

## 3. 主线 B：重试与上下文恢复

### 3.1 用户侧症状

- 长对话或大 Workflow 直接返回 400 context overflow；
- Claude Code 没有在该请求内自动恢复，必须人工 compact、开新会话或缩小 agent 输入；
- WebSearch 和长工具调用偶尔像卡死，尤其在多 subagent 并发时明显；
- 账号明明还有池资源，请求却可能长时间等待 cooldown 或终止事件。

### 3.2 已确认根因

#### B1. 当前 Codex Responses 路径没有自动 compact-and-retry

context overflow 在当前执行器中是终止错误：

- HTTP 非 2xx 直接返回；
- 终端 SSE 中识别到的 context overflow 映射为 400；
- 只有调用方明确设置 `opts.Alt == "responses/compact"` 时才访问 `/responses/compact`；
- streaming compact 当前会被拒绝；
- 没有 `overflow → compact → 重试原请求` 的编排。

相关位置：

- `server/cliproxy/internal/runtime/executor/codex_executor.go:110-153`
- `server/cliproxy/internal/runtime/executor/codex_executor.go:754-756`
- `server/cliproxy/internal/runtime/executor/codex_executor.go:846-881`
- `server/cliproxy/internal/runtime/executor/codex_executor.go:941-977`
- `server/cliproxy/internal/runtime/executor/codex_executor.go:1040-1042`

现有测试只证明显式、非流式 compact 透传，不覆盖自动触发、压缩后历史替换和继续生成：

- `server/cliproxy/internal/runtime/executor/codex_executor_compact_test.go:17-78`

#### B2. `/count_tokens` 路由并不缺失

以下接口已经注册：

- `POST /v1/messages`
- `POST /v1/messages/count_tokens`

相关位置：

- `server/cliproxy/internal/api/server.go:533-534`
- `server/cliproxy/sdk/api/handlers/claude/code_handlers.go:208-216`
- `server/cliproxy/sdk/api/handlers/claude/code_handlers.go:257-279`

因此该问题不能通过“补一个 count_tokens 接口”解决。真正缺失的是溢出后的恢复策略，以及对模型窗口/错误语义的端到端一致性验证。

#### B3. Claude Code 客户端 compact 与 CPA 服务端 compact 是两层机制

Claude Code 客户端会根据自己的 token 估算、模型上下文窗口和阈值决定何时 compact。CPA 可以通过准确的 token count、模型元数据和标准错误帮助它判断，但不能简单返回一个字段就强制客户端刷新上下文。

CPA 服务端可以实现独立的安全网：

```text
普通请求
→ 检测 context overflow
→ 调用 /responses/compact
→ 使用压缩结果重建请求
→ 最多重试原请求一次
```

这不是刷新 OAuth token、账号或配额能解决的问题。

#### B4. Codex WebSocket 可能长期等待终止事件

当账号启用 Codex WebSocket 路径时，执行器会持续等待：

- `response.completed`；
- `response.done`；
- error。

如果上游已经产生工具事件，却迟迟不发终止事件，单次静默读取可能等到约 5 分钟 idle timeout。读失败后没有安全的重连/resume 或 HTTP fallback。

相关位置：

- `server/cliproxy/internal/runtime/executor/codex_websockets_executor.go:607-699`
- `server/cliproxy/internal/runtime/executor/codex_websockets_executor.go:760-790`
- `server/cliproxy/internal/runtime/executor/codex_websockets_executor.go:1722-1739`

该问题只影响启用了 WebSocket 的账号路径，HTTP/SSE 不走这里。

#### B5. 号池 cooldown 会放大并发请求延迟

账号选择本身不会主动 sleep；但当一轮凭证尝试后可用账号都处于 cooldown，且允许 retry 时，每个并发调用方可能同步等待最早 cooldown。等待受 `maxRetryInterval` 控制，并带少量 jitter。

相关位置：

- `server/cliproxy/sdk/cliproxy/auth/selector.go:199-282`
- `server/cliproxy/sdk/cliproxy/auth/conductor.go:2315-2425`
- `server/cliproxy/sdk/cliproxy/auth/conductor.go:3481-3657`

`maxRetryInterval=0` 可以作为诊断手段关闭等待、改为快速失败，但不是最终产品策略。

#### B6. HTTP SSE forwarder 不是主要缓冲点

HTTP SSE forwarder 每收到一个上游 chunk 就立即写出并 flush，没有主动批处理。延迟更可能来自：

- 上游迟迟不产生 chunk/terminal；
- WebSocket 终止等待；
- cooldown/backoff；
- 中间反代缓冲；
- WebSearch 上游事件 shape 不完整。

相关位置：

- `server/cliproxy/sdk/api/handlers/stream_forwarder.go:52-119`

WebSearch 转换器还有缺失 ID、action-only 事件和 block index 的潜在正确性问题，但目前只能确认是兼容风险，不能直接认定为主要延迟根因：

- `server/cliproxy/internal/translator/codex/claude/codex_claude_response_web_search.go:13-88`
- `server/cliproxy/internal/translator/codex/claude/codex_claude_response_web_search.go:155-188`

### 3.3 重试与上下文修复点

1. 统一识别 HTTP 和 SSE 两条路径的 context overflow；
2. 设计一次性 `compact-and-retry` 状态机，严格限制最多自动恢复一次；
3. compact 使用独立超时和独立错误分类，避免原请求无限挂起；
4. 压缩后保留 system instructions、tool call/tool result 配对、reasoning replay 所需数据；
5. 明确流式请求的恢复方式：先完成非流式 compact，再重新建立流式原请求；
6. 确认客户端 model alias、上下文窗口和 `/count_tokens` 结果一致；
7. 给 overflow、compact 失败、重试成功、二次 overflow 和取消增加端到端测试；
8. 给 WebSocket terminal wait 设置明确上限并输出可识别的超时错误；
9. 评估只在安全条件下进行一次 HTTP/SSE fallback，不做无限重连；
10. 将账号 failover 与 cooldown sleep 分开观测，记录每次等待原因和持续时间；
11. 为 WebSearch/Workflow 场景记录首字节、首 tool event、terminal event 和每次 retry/cooldown 时间；
12. 通过关闭 WS、设置 `maxRetryInterval=0` 的 A/B 测试定位实际延迟来源。

---

## 4. 修复里程碑

| 里程碑 | 范围 | 关键产物 | 验收标准 | 状态 |
|---|---|---|---|---|
| **M0：问题审计** | usage、context、retry、WebSearch 延迟 | 28-agent 细粒度审计、对抗核验、根因清单 | 28/28 agent 成功；区分已确认缺陷、潜在风险和正常客户端行为 | ✅ 已完成 |
| **M1：Usage accounting P0** | Codex 非流式/流式记账 | exactly-once finalizer、零 usage 成功、EOF/取消失败、Codex total 修正、Redis queue 回归测试 | 每个 Codex 请求恰好一条主记录；正常完成不漏记；中断不静默；reasoning/cache 不重复计数 | ✅ 已实现并部署 |
| **M2：Context recovery P0** | context overflow 与 compact | overflow 分类、一次性 compact-and-retry、历史重建、独立超时、端到端测试 | 可恢复的 overflow 自动重试一次；不可恢复时快速、明确失败；无无限循环 | ⏳ 待设计/实施 |
| **M3：Retry/terminal latency P1** | WebSocket、cooldown、并发工具调用 | terminal deadline、retry/cooldown 可观测性、安全 fallback 策略 | WebSearch/工具调用不再无提示等待数分钟；每次等待可定位到上游、WS 或 cooldown | ⏳ 待设计/实施 |
| **M4：Claude Code 全链路验收** | 主 agent、Workflow、subagent、WebSearch | stream/non-stream A/B、长上下文、并发 agent、CPA 对账报告 | Claude Code token 与 CPA 最终 usage 可解释；Workflow 不漏请求；overflow 可恢复或明确失败；WebSearch 延迟有界 | ⏳ 待验收 |

### 4.1 M1：Usage accounting P0 任务拆分

- **M1.1**：扩展 UsageReporter，支持失败记录携带已知 usage；
- **M1.2**：修正 Codex provider 的 total fallback；
- **M1.3**：非流式 completion 无 usage 时记零 token 成功；
- **M1.4**：流式 goroutine 增加统一 finalizer；
- **M1.5**：EOF、scanner error、取消和 completion 后取消测试；
- **M1.6**：Claude stream/non-stream 终端 usage 一致性测试；
- **M1.7**：Redis queue/CPA payload 集成测试；
- **M1.8**：race、相关包、全仓构建和 guardrails 验证。

### 4.2 M2：Context recovery P0 任务拆分

- **M2.1**：统一 HTTP/SSE overflow 分类和可重试判定；
- **M2.2**：抽取 compact 请求构造与结果解析接口；
- **M2.3**：实现最多一次的 compact-and-retry 状态机；
- **M2.4**：保留 system、tools、tool result 和必要 reasoning 状态；
- **M2.5**：处理 compact 超时、compact 失败、重试取消和二次 overflow；
- **M2.6**：验证 `/count_tokens`、模型窗口和 alias 元数据一致性；
- **M2.7**：补非流式和流式恢复的端到端测试；
- **M2.8**：确认自动恢复不会造成重复计费、重复 tool call 或无限循环。

### 4.3 M3：Retry/terminal latency P1 任务拆分

- **M3.1**：记录账号选择、failover、cooldown、重试开始/结束时间；
- **M3.2**：缩短并显式配置 WebSocket terminal wait；
- **M3.3**：为缺失 terminal 的连接返回明确错误；
- **M3.4**：评估一次性 HTTP/SSE fallback 的安全条件；
- **M3.5**：修复 SSE error channel 关闭后的潜在 busy-loop；
- **M3.6**：补 WebSearch 缺失 ID、action-only 和未知 event shape 测试；
- **M3.7**：用 WS on/off、`maxRetryInterval` on/off 和不同池状态做延迟矩阵测试。

---

## 5. 实施顺序与边界

当前进度：`M1 Usage accounting` 已完成并上线；接下来按以下顺序推进：

```text
M2 Context recovery
→ M3 Retry/terminal latency
→ M4 Claude Code 全链路验收
```

原因：

1. usage 是观测与对账基础；若请求数和 token 仍会漏记，后续无法准确判断 compact/retry 是否造成重复请求或重复计费；
2. context recovery 会引入额外 compact 请求和一次重试，必须先建立可靠的 exactly-once accounting；
3. WebSocket/cooldown 优化涉及超时和 fallback 策略，应在记账与恢复状态机稳定后实施；
4. 两条主线应分别提交和测试，避免 usage 修复与请求行为变更混在同一个补丁中。

本轮审计与后续 M2/M3 里程碑不包含：

- new-api L1 修改；
- 部署机操作；
- OAuth/账号 token 刷新；
- 用本地 tokenizer 伪造上游缺失的精确 usage；
- 未经验证的无限重试、无限 WebSocket 重连或多次自动 compact；
- accounting v2 公共 SDK/plugin ABI/Redis schema 迁移。该迁移应作为后续独立里程碑处理。

---

## 6. 最终验收场景

至少覆盖以下真实 Claude Code 使用方式：

1. 短对话，stream/non-stream usage 对账；
2. 主 agent 连续多轮对话；
3. 10+ subagent 并发 Workflow；
4. subagent 正常完成、取消、提前失败；
5. 长上下文首次 overflow 后 compact-and-retry 成功；
6. compact 失败或二次 overflow 时快速终止；
7. WebSearch 走 HTTP/SSE 与 WebSocket 的 A/B；
8. 号池正常、部分 cooldown、全部 cooldown 三种状态；
9. Claude Code 本地 token、CLIProxy usage record、Redis/CPA payload 三方对账；
10. 全仓测试、race 检查、Go build 和 `./scripts/check-guardrails.sh` 全部通过。
