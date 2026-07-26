# 迭代记录 · CHANGELOG

> 上线后的功能迭代与修复,按主题归档(非严格时序)。架构与机制的权威说明见 [PLAN.md](./PLAN.md) 与 [docs/architecture-and-pool-tech.md](./docs/architecture-and-pool-tech.md);enriched / 订阅日期方案见 [docs/pool-enrichment-design.md](./docs/pool-enrichment-design.md);逐条提交见 `git log`。
> 当前线上镜像：**`winbeau/xju-newapi:deploy-8b8bdfb`** + **`winbeau/cli-proxy-api:deploy-43116d2`**（部署机 `claude-tri`，仓库 `/home/winbeau/opt/xju-api`；CLIProxyAPI 于 2026-07-25 完成全部现役动态池升级）。

---

## 公告

- **公告历史时间线** —— 保存顶部通知公告时，后端会在同一事务中把公告正文与服务器发布时间追加到时间线，旧公告不再被下一次发布覆盖；启动时会幂等迁移已有 `Notice`，优先从管理审计日志恢复其真实发布时间。普通用户可在通知中心滚动查看全部已保存历史，不再只显示最近 20 条。

## 号池管理

- **共享号池分级权限** —— 普通用户可从个人侧栏进入共享号池，但仅能查看脱敏后的账号状态、逐号测速和逐号查询额度；新增 / 导入 / 登录 / 启停 / 删除 / 重置 / 批量检测 / 自动维护均由管理员或 root 操作。普通用户自己的私人号池继续保留完整管理能力，role=10 管理员不能跨入他人私人池，root 保留全局排障权限。
- **Anthropic / Claude Code 原生兼容** —— 现有 `cliproxy-pool-*` 渠道启动时原地、幂等升级为 Advanced Custom，保留渠道 ID、Group、Key、BaseURL、Models 与启停状态；`/v1/messages`（含 SSE、thinking、tool_use/tool_result）和 `/v1/messages/count_tokens` 保持 Anthropic 协议直达 CLIProxyAPI，同时保留 Chat Completions、Completions、Responses 与 Responses Compact。Claude Code 所需 `Anthropic-*`、`X-Claude-*`、`X-Stainless-*` 和 `User-Agent` 安全透传，用户鉴权头会被号池内部 Key 替换。
- **私人号池完整工作台 + Web 登录导入** —— 「我的号池」保留四步引导，同时同步账号列表、上传 / ZIP / 粘贴、验活、启停删除、额度 / 重置券、自动清理等单池管理能力；用户和管理员号池工作台都提供「登录」按钮，可直接发起 OpenAI OAuth，复制浏览器失败页中的 localhost 回调地址即可入池，无需 SSH `-L`。管理员登录会绑定当前选中的号池。
- **私人号池免平台余额限制** —— 私人号池请求继续按与公用号池相同的模型价格计算并写用量日志、用户 `used_quota`、请求数和渠道用量，但不校验、不扣减用户钱包或订阅额度；平台用户额度只限制公用号池。API Key 自身的可选额度仍可作为单 Key 安全阀，默认无限额度的私人 Key 可持续使用。
- **一键导入号池认证** —— 粘贴 codex `auth.json` 即加号,无需 scp + 重启。
- **号池独立管理页** —— 从渠道页弹窗提为 admin 侧栏独立页;状态徽章、启用 / 禁用 / 删除 / 刷新。
- **文件上传 + .zip 批量导入** —— 「新增账号」支持上传 `.json`;或一次导入整包 `.zip`(K12 的 501 号即此路)。
- **K12 独立号池** —— 第二 CLIProxyAPI 实例(`8318` / `auths-k12`)真隔离,渠道分组路由。
- **一键开池(#4)** —— 号池页「新建号池」只填名字 → 宿主 watcher 自动起隔离 `cli-proxy-api-<id>` 实例 + 建渠道 + 注册进池;配套一键删池。
- **主动验活(号池验活)** —— 逐号用 `api-call` 钉定该号凭证发探针(`GET /codex/responses`,405=活 / 401=死),单号 / 全池后台批量、可选自动禁用死号。
- **每账号额度** —— 5h / 周窗口用量 + 重置券统计,手动 / 定向刷新(只刷已用尽 / 未知额度的号),可自动刷新与自动重置。
- **跨池导入护栏** —— `foreignPoolMarker` 拒绝把带其它池标记的号导入本池,防污染。
- **自动清理欠费号** —— 每小时把超过 24h 不可用的号自动禁用(可开关 + 立即清理)。
- 后端管理密钥基石:注入明文 `MANAGEMENT_PASSWORD`(CLIProxyAPI 管理接口)。

## 注册 / 邀请码

- **邀请好礼活动页** —— 个人侧栏在「额度充值」下新增独立「邀请好礼」入口，原充值页的邀请码、邀请链接、成功人数与阶梯奖励统一迁入新页；新增红金节庆活动主视觉、动态礼盒与奖励特效、「参与活动」交互及里程碑进度。`$10,000` 大奖当前仅作活动预告，助力与发放规则待运营方案确定后再接后端结算。
- **个人推荐奖励** —— 个人推荐码可重复分享；每成功邀请一人，邀请人与被邀请人各得 `$5` Default 余额，邀请人累计达 3 / 5 / 10 人再一次性额外得 `$10` / `$20` / `$50`。密码、OAuth、微信新注册统一门禁，账号创建、一次性码消费、OAuth 绑定和奖励同事务提交；独立奖励记录保证并发不重复发放且不追发历史邀请。管理员一次性准入码不参与奖励。
- **固定每日签到** —— 每日签到固定发 `$0.10` Default 余额，不再随机。
- **仅邀请注册** —— `InviteCodeRequired=true`;注册必须填有效邀请码(后端 CAS 原子消费,一码一用)。
- **邀请码系统** —— 新增 `invite_codes` 表 + 生成 / 列表 / 启停 / 删除 API;管理员「用户」页「生成邀请码」弹窗(批量 + 有效天数 + 状态管理)。
- **关自用模式** —— `SelfUseModeEnabled=false`(默认 true 会隐藏全部注册入口);登录页 Sign in 旁加白底「Sign up」按钮。
- 当前 3 个超管:`winbeau` / `candyman` / `hyyyyyyz`。

## Codex 配置 / 模型

- **CC Switch Claude 一键配置** —— API Token 行在 Codex 图标右侧新增 CC Switch 官方 Logo；配置弹窗默认 Claude 模式，端点固定 `https://api.selab.top`、Full URL 为否，并预设 XJU 三档映射：主模型/Opus → `gpt-5.6-sol`、Sonnet → `gpt-5.6-terra`、Haiku → `gpt-5.6-luna`。可复制标准 Config JSON 或 Deep Link；Token 默认遮罩，配置只在浏览器本地生成。
- **Codex 一键配置** —— API 密钥操作列直达按钮,一键复制 `config.toml` / `auth.json`,去掉 CLI 字样,ChatGPT 花瓣图标。
- **Responses WebSocket** —— `api.selab.top/v1/responses` 支持 `101 Switching Protocols`;L1 先验日卡、首帧按模型/分组选池,再保持到 CLIProxyAPI 的持久 WebSocket,每个 `response.create` 继续预扣费、按 usage 结算并写用量日志;一键配置默认生成 `supports_websockets = true`。
- 修 base_url 变 localhost、key 变 `sk-sk` 两个 bug;默认模型改 `gpt-5.6-sol`。
- **渠道测试识别图像模型** —— `gpt-image*` 走 `/v1/images/generations` 探测,不再误判不可用;移除号池不提供的 `gpt-5.3-codex-spark`。
- 现役号池模型:`gpt-5.6-sol/terra/luna`、`gpt-5.5`、`gpt-5.4(-mini)`、`codex-auto-review`、`gpt-image-2/1.5`。

## 用量看板

- **兑换码充值与双路线教程** —— Default 额度当前只通过兑换码充值，运营口径固定为 `¥1 = $100`；在线支付继续关闭。教程“推荐配置流程”拆为 Default 共享号池与私人号池两条路线，Default 路线置顶并明确兑换额度、选择 Default 分组、配置客户端和查看消耗，私人号池继续不扣减 Default 余额。
- **Default 付费池** —— 用户 `quota` 明确为 Default 共享池余额，私人池继续免余额校验和扣减；管理员用户页拆分「Default 余额 / 全部池累计用量」，并新增全站累计用量汇总。用户侧新增额度充值、邀请规则与身份门槛入口；管理员账号池下新增 Default 独立倍率页。
- **余额身份视觉** —— 当前余额满 `$50` 显示流光香槟金名字，满 `$100` 加银皇冠，满 `$1,000` 加金皇冠；纯视觉且支持 reduced-motion。
- **安全余额迁移** —— 新增默认 dry-run 的一次性清零工具，使用 Python SQLite backup API 生成并校验 `0600` 备份后才清除旧余额；无需生产宿主机安装 `sqlite3`，并保留总用量、邀请历史和日志。清零时显式关闭服务端在线支付总开关，旧支付凭证和 pending 回调不能把余额重新加回。
- **前端发布物校验** —— 前端固定在 Codex-vps 构建，打包时写入 Git SHA 并生成 SHA-256；Codex-tri 安装器安全解包、核对当前提交并原子替换，生产部署默认只编 Go，杜绝旧前端或占位页混入新镜像。
- 概览「近 24h 消耗 / 历史使用」**同时显示 USD 与 token**(此前只有 USD)。
- token 数**全语种统一显示**:< 10M 千分位整数、≥ 10M 两位小数 M;适度放大并上主色。
- 历史 token 查询改 29 天窗口(self data 接口限 1 个月,超范围被拒返回 0)。

## 品牌 / 前端

- **品牌标 = 黑白 Gateway app-icon**(白色网关标 + 黑圆底,X 加长加大);`logo.png` + `favicon.ico` 统一,带版本号破缓存。
- 标签页标题首屏即 `XJU API`(内联脚本消除 `New API → XJU API` 闪烁;`<title>` 与页脚归属保留不动)。
- **登录页极简** —— 只留「XJU API + 3 个客户端(Codex / Cherry Studio / CC Switch)」,去营销腔。
- **首页话术降 AI 味** —— 删编造统计;去掉平台不提供的 Claude / Gemini 虚指;feature 改真实(GPT-5.x / Codex / gpt-image)。
- 移除管理员「模型」页(账号经渠道 / 号池管理);删设置向导 / 绘图·任务日志;API 密钥表列重构。

## 部署 / 构建

- **Codex usage accounting 已上线**（2026-07-25）—— Codex 非流式/流式请求统一 exactly-once 收口：无 usage 的正常 completion 记零 token 成功，clean EOF、scanner error 和 completion 前取消记失败，失败时保留已观察到的部分 usage；缺失 `total_tokens` 时按 OpenAI 子集语义计算，不重复累加 reasoning/cache。补齐 helper、executor、Claude translator、race 与 Redis queue 回归，详见 [docs/bug-fix-July25.md](./docs/bug-fix-July25.md)。
- **CLIProxyAPI commit 镜像一键部署** —— 新增 `deploy/deploy-cliproxy.sh`，镜像固定为 `deploy-<7位 Git SHA>`；自动发现现役动态池、非 main canary → 其余池 → main 顺序升级，保留私有池资源限制，部署期间暂停 provision，失败逆序回滚，成功后同步未来新池镜像。动态备份支持从 new-api 容器读取权限受限 registry，并用缓存 sudo 归档 root-owned OAuth 文件而不改原文件权限。
- **2026-07-25 生产验收** —— `cli-proxy-api-main` 与数字池 `4–12` 共 10 个现役容器全部运行 `winbeau/cli-proxy-api:deploy-43116d2`；端口 `8317`、`8321–8329` 的 `/healthz` 全部正常，`xju-provision` 为 active，`.maintenance` 不存在；`.cliproxy-image.env` 记录当前镜像与 `v0.9.1` 回滚锚。
- **构建加速** —— BuildKit 缓存挂载(`go build` 40s → 7s);去掉前端 build 的 cache mount(它会让旧 bundle 静默上线)。
- **固定 `NODE_NAME`** —— 否则每次重部署在系统信息页留一个僵尸节点。
- dev server 支持 HTTPS(否则本地登录失败)。

---

> 护栏:以上所有改动**均未删除 / 修改 New API 与 QuantumNous 的品牌、页脚归属与版权头**(见 [README §构建于](./README.md#-构建于))。
