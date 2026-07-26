# CLAUDE.md — xju-api 项目工作指引

> 新会话请**先完整读 [PLAN.md](./PLAN.md)**,它是本项目的唯一事实来源(架构 / 日卡系统 / 前端改造 / 部署 / Phase 0-5)。本文件只给"怎么在这个仓库里干活"的约束。

## 这是什么

三层 AI API 代理平台,用「日卡 / 三天卡 / 周卡」时间卡形式把上游 AI 号池打包分发给下游用户。
`server/newapi`(L1 发卡/统计) → `server/cliproxy`(L2/L3 号池)。详见 PLAN.md。

## 开发与部署分工(当前口径)

- **本机 = claude-vps**:用于写代码、运行开发测试、按需本地构建、`git commit`(winbeau 身份)+`git push`;不直接操作生产容器或生产数据。
- **claude-tri = 70.39.193.15:48687(user winbeau)**:生产构建与部署主机,资源已可完整运行 Bun + Docker 构建。标准入口是 `bash deploy/deploy.sh`(默认 `SKIP_WEB=0`),会拉取代码、构建前端、构建 Go 镜像、换容器并验活。
- `SKIP_WEB=1` + `deploy/install-web-dist.sh` 仍作为可选的 prebuilt 发布物路径,用于复用外部构建或紧急降载,不再是 tri 的硬性要求。
- 生产部署、容器重建、数据库操作仍只在 claude-tri 执行。

## 已定决策(勿再动摇/重新讨论)

- L1 账号模型 = **模型 A**(每个下游用户一个 new-api 账号,`GET /api/data/users` 原生按用户统计)
- 卡档位:**日 / 三天 / 周**;**月卡(30天)机制留位但暂不上架**,前端快捷按钮只做 1/3/7 天
- 前端风格 = **仿 xju-feiyue 的 Notion 风格**(=界面 UI 风格),参照本机 `~/wenbiao_zhao/xju-feiyue`
- 仓库 = **winbeau/xju-api 公开**,提交/推送**一律用 winbeau**;不转 XjuSelab
- `server/cliproxy/` 默认零改动,**按需可删减/升级适配**
- 顶层布局 = **web / server / deploy / scripts / docs 五件事**(2026-07 REFACTOR-PLAN §5.0 重组,不再回退双 vendored 目录形态);上游可升级性**不是**设计目标
- 🚫 护栏:**禁止删除/修改 new-api / QuantumNous 的品牌、版权、归属**(footer 归属、文件版权头);删功能时保留(见 PLAN.md §5.6)。自检:`./scripts/check-guardrails.sh`

## 当前主攻(待解决)

> 方案与背景机制见 `docs/pool-enrichment-design.md` 与 `docs/architecture-and-pool-tech.md`。

1. **单账号周限额 + 重置券** —— 号池页逐账号 5h/周窗口用量 + 重置券统计,手动/自动刷新(基础已落地,持续打磨)。
2. **单账号订阅期限(外部阻塞,非待编码项)** —— 让每个号显示 ChatGPT 订阅到期。**这不是仓里缺代码**:精简号 token 里没有订阅日期,refresh 被 OpenAI 挡(400)、主动拉取被 Cloudflare 挡(403),**唯一产出路径是 enriched authorize 重新登录**(见 `docs/pool-enrichment-design.md` §Honest limits / §Existing lean accounts)。仓内该做的已做:JWT 有日期就显示、access_token 兜底、P2 top-level plan 兜底、前端数值 epoch 防护。剩下的是**运营动作**(对存量精简号走 enriched 重新登录),不是编码任务。
3. **统一的号池模式** —— 建池选双模:**CLIProxy enriched 登录(默认)** / **go-pool 批量**;go-pool 分支需在自己的 authorize 请求补三参数(`id_token_add_organizations=true` + `codex_cli_simplified_flow=true` + scope 加 `offline_access`)且用 OpenAI 已注册的 `redirect_uri`,导出才带订阅日期。

## 仓库结构

- `web/` — 前端(React 19 + Rsbuild,独立 bun 应用),**换肤+裁剪**主战场。原 `new-api/web/default/`。
- `server/newapi/` — L1 Go 后端(AGPL-3.0,上游归属保留)。原 `new-api/`;go module 路径不变。
- `server/cliproxy/` — L2/L3 号池(MIT)。原 `CLIProxyAPI/`。默认不改,按需适配。
- `deploy/` — 部署面:Caddyfile、`Dockerfile.newapi.prebuilt`(唯一镜像路径)、`build-newapi.sh`、`run-newapi.sh`、compose、配置样板。
- `scripts/` — 发卡/运维 glue(kebab-case 动词-宾语式)+ `check-guardrails.sh` 护栏自检。
- `docs/` — 见下「docs 速览」。
- `PLAN.md` — 实施计划(9 节 + Phase 0-5)。

## docs 速览

- **[PLAN.md](./PLAN.md)** — 唯一事实来源:架构 / 日卡系统 / 前端改造 / 部署 / Phase 0-5。
- **[docs/architecture-and-pool-tech.md](./docs/architecture-and-pool-tech.md)** — 🌟**新人先读**:系统架构(New API vs CLIProxyAPI 谁是号池)、账号存储与调度、cpa/sub2 格式、OAuth/JWT 鉴权、token 生命周期、订阅 + 5h/周限额原理、default 5 号实况。
- **[docs/pool-enrichment-design.md](./docs/pool-enrichment-design.md)** — 让号显示 plan + 订阅日期的**双模方案设计**(CLIProxy enriched 登录 / go-pool 补参数),含仓内改动清单、部署缺口、诚实边界。
- **[docs/runbook.md](./docs/runbook.md)** — 上线后升级 / 回滚 / 排障 / 双池管理密钥。
- **[docs/daycard-api.md](./docs/daycard-api.md)** — 时间卡三接口速查。
- **[docs/newapi-customization.md](./docs/newapi-customization.md)** — new-api 换肤 + 裁剪 + 功能增强改造记录。
- **[docs/prune-checklist.md](./docs/prune-checklist.md)** — 前端裁剪逐包清单。
- **[docs/theme-notion.md](./docs/theme-notion.md)** — Notion 换肤记录。
- **[docs/REFACTOR-PLAN.md](./docs/REFACTOR-PLAN.md)** — 2026-07 顶层重组计划(已收官,历史参考)。
- **[docs/superpowers/](./docs/superpowers/)** — 设计文档存档(brainstorm / plan 一次性产物)。
- **[CHANGELOG.md](./CHANGELOG.md)** — 上线后迭代记录。

## 前端开发命令

```bash
cd web
bun install
bun run dev        # 本地开发服
bun run build      # 本地验证构建；生产也可由 tri 的 deploy.sh 完整构建
bun run typecheck  # tsgo -b,必须清零
bun run lint
bun run knip       # 扫删除后的孤儿引用
```

## 后端构建(server/newapi)

```bash
cd server/newapi && go build .   # 裸编译走 web/dist 占位 index.html
./deploy/build-newapi.sh <tag>   # 完整镜像:bun build → 拷 dist → docker build
```

## 安全(公开仓)

- **绝不提交真实密钥**:`config.yaml` / `auths/` / 真实 `.env` / `*.key` 已被 `.gitignore` 挡;文档/样板一律用 `__PLACEHOLDER__` 或 `$(openssl rand -hex 32)`。
- 真实凭证只存在于 claude-tri 本地被 gitignore 的文件里。

## 建议起手式

新会话在本机接手时,通常从 PLAN.md §7 拆 todolist。代码与测试在本机推进并推送；生产部署在 claude-tri 运行 `bash deploy/deploy.sh`,默认由服务器完成前端与 Go 镜像构建。
