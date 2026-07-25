# xju-api 实施计划

> 三层 AI API 代理平台 · 面向下游用户的「日卡 / 三天卡 / 周卡」发卡系统
> 仓库：`winbeau/xju-api`（公开）｜部署机：`claude-tri`｜状态：**✅ 已上线运行中**（[api.selab.top](https://api.selab.top)）
>
> **本文是设计与实施的事实来源(source of truth)。Phase 0–5 已全部完成、平台已上线;上线后的功能迭代(号池管理页 / 邀请码系统 / 用量看板 / 品牌换新等)记录在 [CHANGELOG.md](./CHANGELOG.md),不在此重复展开。下文保留作为架构与机制的权威说明。**
>
> ⚠️ 本仓库为**公开仓库**。全文所有密钥、Token、api-key、secret 一律使用 `__PLACEHOLDER__` 占位符，**严禁提交任何真实凭证**。真实值只写在部署机本地、被 `.gitignore` 排除的文件里。

---

## 目录

1. [项目背景与目标](#1-项目背景与目标)
2. [总体架构](#2-总体架构)
3. [服务器与域名](#3-服务器与域名)
4. [日卡系统设计](#4-日卡系统设计)
5. [前端改造](#5-前端改造new-api-webdefault)
6. [仓库结构](#6-仓库结构xju-api-放什么)
7. [分阶段实施计划](#7-分阶段实施计划)
8. [安全基线](#8-安全基线)
9. [风险与待确认项](#9-风险与待确认项)

---

## 1. 项目背景与目标

**一句话定位**：xju-api 是一套自建的三层 AI API 代理平台，用「时间卡」（日卡 / 三天卡 / 周卡，月卡留位）的形式，把上游多个 AI 账号（号池）的能力打包售卖给下游用户。

### 1.1 三层职责

| 层 | 名称 | 承载软件 | 域名 / 端口 | 职责 |
|---|---|---|---|---|
| **L1 用户配置层** | 发卡与统计前台 | **new-api** | `api.selab.top` → `127.0.0.1:3000` | 面向下游用户与运营：发卡 / 续卡 / 开闭 / 用量统计；前端仿 xju-feiyue 的 Notion 风格并裁掉无用功能 |
| **L2 中转胶水** | 协议中转 | **CLIProxyAPI** | `codex.selab.top` → `127.0.0.1:8317` | 把 L1 下发的 OpenAI 兼容请求，转成上游各家协议 |
| **L3 号源号池** | 凭证池 | **CLIProxyAPI**（同进程） | `auths/*.json` | 承载上游账号凭证，负载轮换。**默认零改动**（按需可删减/升级适配），主要搬运号池 |

- **入口**：`claude-tri` 上新装 **Caddy**，两个子域各自 TLS/ACME 自动签证，反代到两个只绑定 `127.0.0.1` 的后端。
- **L1 → L2 接线**：在 new-api 后台新增一个「OpenAI 兼容」渠道，Base URL 填 **`http://cli-proxy-api:8317`**（容器名），Key = CLIProxyAPI `config.yaml` 里一条常驻内部 `api-key`。
  > ⚠️ **实测修正**（2026-07-13 部署时发现）：**不能填 `http://127.0.0.1:8317`**。new-api 跑在容器里，`127.0.0.1` 指向的是它自己的回环，而 CLIProxyAPI 的 `8317` 只发布在**宿主**的 `127.0.0.1` 上——容器间不通，请求会报 `do request failed`。
  > 正确做法：两个容器接入同一 docker 网络 `xju-net`（见 `deploy/`），用**容器名**互访。该网络不发布端口，公网暴露面不变。

### 1.2 日卡产品

一张卡 = 一段有效时长。用户拿到一把**稳定不变的 Token** 配一次即可长期使用，运营通过写 Token 的到期时间戳来「充值时长」。四个档位：**日卡（1 天）/ 三天卡（3 天）/ 周卡（7 天）/ 月卡（30 天，留位待定）**。机制细节见 [§4](#4-日卡系统设计)。

### 1.3 核心设计原则

- **后端最小改动优先**：CLIProxyAPI 默认不改，**按需可做删减 / 升级适配**（源码已内置本仓，可直接改）；new-api 只改前端（视觉 + 裁剪），业务逻辑复用其原生令牌体系。
- **单文件优先**：new-api 用 SQLite 单文件落库，不起 postgres/redis 容器（部署机资源紧张）。
- **公开仓库安全第一**：全文占位符，真实值不入库。

---

## 2. 总体架构

### 2.1 数据流图

```
                                 ┌──────────────────────────────────────┐
   下游用户 / 运营 ──HTTPS 443──▶ │            Caddy (claude-tri)          │
   (浏览器 / SDK)                │  api.selab.top    → 127.0.0.1:3000     │
                                 │  codex.selab.top  → 127.0.0.1:8317     │
                                 └───────────┬───────────────┬────────────┘
                                             │               │
                            ┌────────────────▼──┐      ┌──────▼─────────────────┐
                            │   L1  new-api      │      │  L2/L3  CLIProxyAPI     │
                            │   :3000 (SQLite)   │      │  :8317                  │
                            │  发卡/统计/鉴权    │      │  协议中转 + 号池        │
                            └────────┬───────────┘      └──────────┬─────────────┘
                                     │  OpenAI 兼容渠道               │  轮换凭证
                                     │  Base=127.0.0.1:8317          │
                                     └───────────────────────────────┘
                                                                    │
                                                          ┌─────────▼──────────┐
                                                          │  auths/*.json 号池  │
                                                          │  → 上游各家 AI API  │
                                                          └────────────────────┘

   一次业务请求路径：
   用户 SDK ──(Bearer=用户日卡Token)──▶ api.selab.top(Caddy) ──▶ new-api
     new-api 校验 Token 有效期/额度 ──(Bearer=内部api-key)──▶ 127.0.0.1:8317 CLIProxyAPI
       CLIProxyAPI 选号池凭证 ──▶ 上游 AI ──▶ 原路返回，new-api 记账写 logs 表
```

### 2.2 组件清单

| 组件 | 版本 / 镜像 | 部署方式 | 监听 | 数据卷 / 关键文件 | 是否改造 |
|---|---|---|---|---|---|
| Caddy | 官方最新 | 系统服务（apt / 二进制） | `0.0.0.0:80,443` | `/etc/caddy/Caddyfile`、`caddy_data/`（ACME 证书） | 新写配置 |
| new-api (L1) | `winbeau/xju-newapi:<tag>`（**自建定制镜像**,见 [deploy/build-newapi.sh](./deploy/build-newapi.sh);前端有换肤/裁剪,不能用上游镜像） | Docker 单容器 | `127.0.0.1:3000` | `/opt/new-api/data`（SQLite）、`/opt/new-api/logs` | **前端改造** |
| CLIProxyAPI (L2/L3) | `eceasy/cli-proxy-api:latest`（建议 pin） | Docker 单容器 | `127.0.0.1:8317` | `config.yaml`、`auths/`、`logs/` | **默认零改动（按需适配）** |

> 说明：new-api 不起自带 `docker-compose.yml` 的 `postgres:15` + `redis:latest`（部署机内存/磁盘紧张，且不设 `SQL_DSN` 即自动落 SQLite、不设 `REDIS_CONN_STRING` 即退化内存缓存）。

---

## 3. 服务器与域名

### 3.1 部署机 claude-tri

- **地址**：`ssh -p 48687 winbeau@70.39.193.15`
- **首连规格实测**（部署前以现场 `Phase 0` 复核为准）：

| 项 | 实测 | 影响 |
|---|---|---|
| OS | Ubuntu 24.04.4 LTS / x86_64 | — |
| CPU | 4 vCPU | 够用 |
| 内存 | 3.8Gi 总，**空闲仅 ~126Mi**（available ~2.2Gi） | **偏紧** → 不起多余容器 |
| 磁盘 | 49G 总，已用 78%，**剩 ~11G** | **偏紧** → 控日志/镜像膨胀，加磁盘告警 |
| Docker | v29.6.1 + Compose v5.3.1 已装 | 直接用 Docker，不本地编译 |
| Caddy / Nginx | **均未装** | 需新装 Caddy |
| 已占端口 | `0.0.0.0:6379/5432/8000/5173`、`127.0.0.1:8099`、`:2022`、`:48687(ssh)` | **多项目共用机**，新增端口须避开，`ufw` 增量加规则不可 reset |

> ⚠️ claude-tri 是多项目共用机，已有别的项目在跑 redis/postgres/dev-server。**不能假设"净土"，不能无脑 `ufw enable` 清规则**，一切端口/防火墙操作走增量。

### 3.2 域名与 DNS（部署前置，否则 Caddy 拿不到证书）

- `selab.top` 托管在 **Cloudflare**（当前主域解析到 GitHub Pages）。
- 需**新增两条 A 记录**指向 `70.39.193.15`：
  - `api.selab.top`（L1 前台）
  - `codex.selab.top`（L2 后端代理）
- 实测这两个子域当前**无 A/AAAA 记录**，必须先加。
- **首次签证建议先设「仅 DNS / 灰云」**，避免 CF 橙云代理拦截 Caddy 的 HTTP-01 / TLS-ALPN-01 挑战；证书签发成功后再决定是否开代理。

### 3.3 TLS

- 由 Caddy 统一做 ACME（Let's Encrypt），一份 Caddyfile 两个 site block，各自独立 `tls` 块。
- 两个后端**不开自己的 TLS**（CLIProxyAPI `tls.enable: false`），TLS 只在 Caddy 这层终止。
- Caddyfile 骨架见 [§6](#6-仓库结构xju-api-放什么) 的 `deploy/Caddyfile`，要点：
  - `api.selab.top` 反代 `127.0.0.1:3000`，健康检查 `health_uri /api/status`（new-api 原生健康端点）。
  - `codex.selab.top` 反代 `127.0.0.1:8317`，CLIProxyAPI 无专用健康端点，可探 `/v1/models` 替代或不设。
  - 统一加 `header_up X-Real-IP / X-Forwarded-*`、`encode zstd gzip`、JSON 滚动日志（`roll_size 50mb / roll_keep 10 / roll_keep_for 720h`）。

---

## 4. 日卡系统设计

> ✅ 本节机制已在 new-api 源码层核实，直接照做，无需再论证。核心：**日卡不是新功能，而是 new-api 原生令牌 `ExpiredTime` 字段的产品化用法。**

### 4.1 卡档位表

一张卡 = 给用户那把常驻 Token 写一个**绝对秒级到期时间戳**：

```
新 expired_time = max(原到期时间, now) + N * 86400        // N ∈ {1, 3, 7, 30}
```

| 卡种 | N（天） | 秒数 | 说明 |
|---|---|---|---|
| 日卡 | 1 | 86400 | — |
| 三天卡 | 3 | 259200 | — |
| 周卡 | 7 | 604800 | — |
| 月卡 | 30 | 2592000 | **留位**，是否上架见 [§9](#9-风险与待确认项) |

- **`max(原到期, now)` 的意义**：未过期时续卡在原到期基础上叠加（不亏用户）；已过期时从当下起算。
- **到期即时生效、零延迟、无 cron**：每次业务请求走 `ValidateUserToken` 现算当前时间是否超过 `ExpiredTime`，超过直接 `401`。不需要定时任务扫描。
- **Token 稳定**：Key 有 `uniqueIndex` + 软删，值稳定不变，**用户配一次，长期不用重配**，运营只改到期时间。

### 4.2 三个核心接口

均为 new-api 原生 `/api/token/` 接口，用运营/用户的 `access_token` 调用。

**① 建卡（发新卡）** — `POST /api/token/`
```jsonc
{
  "name": "user-alice-daycard",   // 令牌名（模型B 靠这个区分用户，见 §4.4）
  "expired_time": 1752505200,     // 绝对秒级时间戳 = max(原到期, now)+N*86400
  "unlimited_quota": true,        // 时间控开闭，用量仍照常记账
  "group": "default"              // 分组，对应 Group Pricing
}
```

**② 续卡 / 复活（关键坑）** — **完整 PUT + `status_only` 两步**（实现即 [`scripts/renew-card.sh`](./scripts/renew-card.sh)，curl 全文见 [docs/daycard-api.md ②](./docs/daycard-api.md#-续卡--复活--完整-put--status_only-两步)）
```jsonc
// 第 1 步:完整 PUT 只写新 expired_time(不带 status;完整体根本不更新 status 字段;
//          业务字段须先 GET 原样带回 —— 完整 PUT 是全量覆盖)
{ "id": 123, "name": "user-alice-daycard", "expired_time": 1752591600, "unlimited_quota": true, "group": "default", /* …其余业务字段原样带回 */ }
// 第 2 步:PUT /api/token/?status_only=true 置回启用 —— 此时库里 expired_time 已是未来,守卫放行
{ "id": 123, "status": 1 }
```
> ⚠️ **坑（`controller/token.go` UpdateToken 源码级修正）**：「置 `status=1`」的守卫检查发生在**字段更新之前**、对照库里**旧的** `expired_time` —— 对已标记 `status=3(过期)` 的令牌，连「完整 PUT 携带 `status:1`」也会被拒。先写未来 `expired_time`、再 `status_only` 置启用的两步顺序，对「未过期叠加 / 自然过期 / 已标记过期 / 手动禁用」四种起点均正确。

**③ 临时关卡 / 开卡** — `PUT /api/token/?status_only=true`
```jsonc
{ "id": 123, "status": 2 }   // 2=禁用（临时关）；1=启用（仅对未过期令牌有效，过期复活见②）
```

### 4.3 统计与对账

| 需求 | 数据来源 | 前提 |
|---|---|---|
| 按用户聚合用量/花费 | `GET /api/data/users`（AdminAuth） | 每用户一账号（模型A） |
| 明细流水（每次调用） | `logs` 表：`prompt_tokens` / `completion_tokens` | — |
| Token 消耗量 | 看 `token_used` 字段 | — |
| 花费（美元） | 看 `quota` 字段，**1 美元 = 500,000 quota** | — |

> `unlimited_quota: true` 只是让「时间」成为唯一开闭闸门，**用量依然全额记账**，统计/对账不受影响。

### 4.4 账号模型 A / B（二选一，影响统计粒度）

| 模型 | 结构 | 统计 | 建卡权限 | 推荐度 |
|---|---|---|---|---|
| **模型 A** | **每个下游用户一个 new-api 账号** | `GET /api/data/users` **原生按用户聚合**，最干净 | 各用户用自己的 `access_token` 给自己名下建卡（自服务） | ✅ **推荐** |
| **模型 B** | 全部令牌挂在**运营一个账号**下 | 只能靠令牌 `name` 前缀区分用户，聚合需自己写 | 运营 `access_token` 统一建卡 | 备选 |

> 关键约束（auth.go:116）：**一个 `access_token` 只能给自己名下建卡**。这是模型 A 天然按用户统计、也是模型 B 必须全挂运营账号的原因。

### 4.5 要不要自建发卡脚本？

- **非必需**。三个核心接口（建/续/关）用 new-api 后台 UI 手动点即可满足初期运营。
- **需要时才写**（脚本级 glue，见 [§6](#6-仓库结构xju-api-放什么) 的 `scripts/`）：
  - 发卡自动化（批量建卡 / 定时续卡）；
  - 卡密自助激活（用户输卡密 → 脚本预存 `access_token` 调上述接口写 `expired_time`）。
- **明确不能用兑换码当日卡**：new-api 兑换码只加 `quota` 额度、**不加时间**，与日卡机制无关（[§5](#5-前端改造new-api-webdefault) 中 redemption-codes 因此删除）。
- 每用户令牌默认上限 **1000** 把，日卡场景绰绰有余。

---

## 5. 前端改造（new-api `web`）

> 目标：把 new-api 默认前台改造成 **xju-feiyue 的 Notion 风格**，并**裁掉与「发卡/统计」无关的功能**。技术栈现状：Tailwind **v4**（`@theme inline`）+ shadcn `base-nova` + Base UI + hugeicons，且**已有 `[data-theme-preset='xxx']` 覆盖机制**——这是最小改动挂载点。

### 5.1 Notion 风格规范（量化，来自 xju-feiyue）

**核心结论**：Notion 观感来自「衬线标题 + 无衬线正文」的字体混排 + 暖色中性灰 + 极浅边框分层（靠 border 不靠阴影），**不是任何主题包**。落地方式与 new-api 现有 token 体系**同构**（都是「CSS 变量单一色源 → Tailwind 消费」），是换皮不换骨。

**中性色阶 + 主色**（浅色）：

| 语义 | Hex | 映射到 new-api 槽位 |
|---|---|---|
| 背景 | `#ffffff` | `--background` |
| 次背景（Notion 招牌暖米） | `#f7f6f3` | `--muted` / `--sidebar` |
| hover 背景 | `#f1f1ef` | hover 态 |
| 正文字（Notion 招牌暖黑，非纯黑） | `#37352f` | `--foreground` |
| 次要字 | `#787774` | `--muted-foreground` |
| 弱字 | `#9b9a97` | 时间戳/占位 |
| 分割线（极浅） | `#edece9` | `--border` |
| 强分割线 | `#dcdad4` | focus ring |
| 链接蓝 | `#2383e2` | `--primary` 或 info |

**字体三栈**：
```
--font-sans:  'Inter Tight', 'PingFang SC', -apple-system, sans-serif   // UI 骨架/按钮/表格
--font-serif: 'Source Serif 4', 'Noto Serif SC', Georgia, serif          // 标题/卡片大数字
--font-mono:  'JetBrains Mono', 'SF Mono', Menlo, monospace              // 代码
```
> 关键观感规则：**UI 用无衬线，标题/卡片标题/dashboard 大数字用衬线**（`font-serif font-semibold tabular-nums`）。这比配色更能还原 Notion 质感。new-api 已有 `--font-serif`（当前 Lora，中文链已配好），**只替换主字体名为 Source Serif 4，复用其中文栈**。

**圆角 / 阴影 / 分层**：
- 圆角三档 6/8/12px。new-api 全站圆角由单一 `--radius` 派生，**设 `--radius: 0.5rem`（8px）一处即改全站**。
- 阴影极轻：`--shadow-card: 0 1px 2px rgba(0,0,0,.04)`；**卡片用 border 分层不用阴影**，hover 时**背景整体变浅**（`hover:bg-muted`）而非加深阴影。
- 表格：圆角在**外层容器**（`rounded-xl border overflow-hidden`），表头 `bg-muted text-xs text-muted-foreground`，行 hover `hover:bg-muted/60`，行分割 `border-b border-border last:border-0`。
- 徽章：**12% alpha 同色 tint 底 + 同色文字**（非纯色底白字），用于 Token 状态（正常/已过期/已禁用）。
- 加 `.scrollbar-notion` 细滚动条工具类（性价比最高的精致度细节）。

### 5.2 保留 / 删除 / 改造清单（route/feature 级）

> **关键修正**：`/models`（后台模型元数据/部署）**保留**；真正该删的公开「模型商城/定价」是 `/pricing`。

| Feature | 判定 | 理由 / 操作要点 |
|---|---|---|
| `auth` | **保留** | L1 门禁核心。可选：后台关 OAuth/Passkey 只留账号密码 |
| **`keys`** | **保留（核心 + 改造）** | 日卡本体（令牌 CRUD）。**改造重点见 §5.4** |
| `dashboard` | **保留** | Overview/Model/Flow/User Analytics（对应 `GET /api/data/users`），天然贴合按用户统计 |
| `usage-logs` | **保留** | 明细对账。Task/Drawing 子 tab 用 config 隐藏而非删代码 |
| `users` | **保留** | 一账号一用户入口。摘掉行操作「查看订阅」（依赖已删的 subscriptions） |
| `system-settings` | **保留（精修子面板，见 §5.3）** | L1 加「OpenAI 兼容」渠道就在这 |
| **`channels`** | **保留（核心）** | 就是要在这加指向 codex 的渠道，不改 |
| `models`（后台） | **保留** | 模型元数据/部署管理，非用户商城，别跟 pricing 混 |
| `system-info` | **保留** | SUPER_ADMIN 运维信息 |
| `setup` | **保留** | 首次初始化向导 |
| `profile` | **保留** | Wallet/Chat 相关死开关随对应 feature 删除一并摘掉 |
| `errors` / `legal` / `performance-metrics` | **保留** | 框架错误边界 / footer 条件渲染依赖 / dashboard 图表底层 lib；删了会编译报错，保留成本为零 |
| `home` | **改造** | Notion 风格首页主战场；`hero.tsx`/`cta.tsx` 里指向 `/pricing` 的 `<Link>` 必须同步改 |
| `chat` | **删除** | 内置 Web Chat，日卡平台不需要 |
| `playground` | **删除**（可选留 1 个自测渠道用） | Prompt 调试台，内部工具 |
| `wallet` | **删除** | 自助充值，发卡是后台开票不是自助支付 |
| `subscriptions` | **删除** | 独立 Stripe/Creem 订阅体系，与 `token.ExpiredTime` 完全不是一回事 |
| `pricing` | **删除路由，保留共享 lib** | 公开定价商城页；但 `lib/billing-expr.ts`/`tier-expr.ts`/`dynamic-pricing-breakdown.tsx` 被 usage-logs 和 system-settings 复用，**不能整目录删** |
| `rankings` | **删除** | 营销排行榜，孤立无引用，最省事 |
| `redemption-codes` | **删除** | 兑换码只加额度不加时间，与日卡无关 |
| `about` | **删除** | 营销页，无引用 |

### 5.3 system-settings 子面板精修

| 区 | 子面板 | 判定 |
|---|---|---|
| Billing & Payment | Quota / Currency / Model Pricing / **Group Pricing** | 保留（Group Pricing 对应 token 的 `group` 字段） |
| Billing & Payment | **Payment Gateway**（creem/waffo） | **删**（配合 wallet/subscriptions） |
| Billing & Payment | **Check-in Rewards**（每日签到送额度） | **删** |
| Console Content | Announcements / FAQ / API Addresses / Data Dashboard | 保留（公告/FAQ 可复用为 Notion 首页内容源） |
| Console Content | **Chat Presets** / **Drawing** | **删** |
| Models&Routing / Security / Operations | 全部 | 保留（基础设施） |

### 5.4 从哪下手（改造重点两处）

1. **`keys/components/api-keys-mutate-drawer.tsx`（日卡实际落地点）**：在建卡/续卡表单加「1天 / 3天 / 7天」快捷按钮（30 天月卡档先隐藏，见 §9），点击按 `max(原到期, now) + N*86400` 回填 `expired_time`；续卡走**完整 PUT**（带 `status:1` + 新 `expired_time`），不能只 `status_only`（对齐 token.go:280 坑）。同时 `data-table-row-actions.tsx` 摘掉「发到 Chat」行操作（否则残留 `useChatPresets` 死引用）。
2. **`routes/index.tsx` + `features/home/`**：仿 xju-feiyue 重做首页，替换 `Stats`/`Features`/`HowItWorks` 视觉但保留 `PublicLayout` 骨架和 `Footer` 归属。

### 5.5 换肤最小路径（优先级）

1. **换色板**（~10min）：`theme-presets.css` 新增 `[data-theme-preset='notion']`，只填 background/foreground/card/muted/border/primary 六槽为上面 6 个 hex（转 oklch），其余 success/warning/chart-* 先继承 default。
2. **圆角+阴影**（同预设块两行）：`--radius: 0.5rem` + 阴影降级 `0 1px 2px rgba(0,0,0,.04)`。
3. **字体**：`--font-sans` 换 Inter Tight；标题类选择性加 `font-serif`。遵循 new-api 现有自托管字体约定，不直接 CDN import。
4. **卡片/表格 hover 语义**：把「阴影加深」批量换成「背景变浅」`hover:bg-muted`——单一改动即从「通用 SaaS 后台」变「Notion 风」。
5. **滚动条**：抄 `.scrollbar-notion` 挂主内容容器。
6. **不迁移**：xju-feiyue 顶栏 + MegaMenu 导航（内容社区范式），new-api 保留自己的后台侧栏骨架，只置换皮。

### 5.6 删除操作纪律（硬约束）

- **删除顺序**：路由文件 → 侧栏菜单配置（`use-sidebar-data.ts` + 各 `section-registry.tsx`）→ 摘跨 feature 耦合引用（各删除包的「必须改」项）→ 删 feature 目录 → 跑 `bun run typecheck`（`tsgo -b`）+ `bun run lint` 清零 → `bun run knip` 扫孤儿。
- **不手改 `src/routeTree.gen.ts`**（自动生成，删路由后 `bun run dev/build` 会重建）。
- **推荐先灰度再物理删**：两层 `sidebar_modules` 配置可先把 chat/wallet/subscription/redemption 隐藏，观察无访问再删代码。
- 🚫 **护栏（不可绕过，AGENTS.md Project Governance）**：**禁止删除/修改 new-api / QuantumNous 的品牌、版权、归属**——`footer.tsx` 的 `ProjectAttribution`（GitHub 链接 + "New API"）、每个文件头 Copyright 注释块。新增/改造文件也要保留标准版权头（`scripts/add-copyright.mjs` 会检查）。**只重做视觉 + 裁业务，归属文本原样保留。**

---

## 6. 仓库结构（`xju-api` 放什么）

> `xju-api` 采用**单仓（monorepo）**开发：顶层五件事 —— 前端 `web/`、服务端 `server/`（内置 new-api 与 CLIProxyAPI 源码，已去除各自 `.git`）、部署 `deploy/`、脚本 `scripts/`、文档 `docs/`（2026-07 顶层重组，见 docs/REFACTOR-PLAN.md §5.0）。各子项目保留自带 `LICENSE` 与 `.gitignore`；顶层 `.gitignore` 只做全仓密钥保护（`config.yaml`/`auths/`/真实 `.env` 永不入库，`*.example.*` 模板保留）。

```
xju-api/
├── PLAN.md                      # 本文档
├── README.md                    # 项目速览 + 快速上手（指向 PLAN.md 各节）
├── .gitignore                   # 全仓密钥保护（config.yaml/auths/真实.env 永不入库；*.example.* 保留）
│
├── web/                         # 【前端】React 19 + Rsbuild 独立 bun 应用（原 new-api/web/default；换肤+裁剪在此改）
├── server/
│   ├── newapi/                  # 【L1 源码·已内置，去 .git】QuantumNous/new-api（Go 后端；go:embed web/dist 接收槽）
│   └── cliproxy/                # 【L2/L3 源码·已内置，去 .git】router-for-me/CLIProxyAPI；默认零改动，按需可删减/升级适配
│
├── deploy/                      # 部署脚手架
│   ├── Caddyfile                # 两 site block（api/codex），占位邮箱，见 §3.3
│   ├── run-newapi.sh           # docker run 模板（127.0.0.1:3000，SESSION_SECRET 用 openssl 生成）
│   ├── docker-compose.cliproxy.yml   # 改端口绑定 127.0.0.1，OAuth 回调口默认注释
│   ├── config.example.yaml      # CLIProxyAPI 配置样板，全占位符（真实 config.yaml 被 gitignore）
│   ├── Dockerfile.newapi.prebuilt  # 唯一镜像构建路径（Go-only；前端产物预构建 COPY 进来）
│   └── backup.sh                # SQLite .backup + auths/ + Caddyfile 滚动备份
│
├── scripts/                     # 发卡 glue（非必需，需要自动化时才用）
│   ├── issue-card.sh            # 建卡：POST /api/token/
│   ├── renew-card.sh            # 续卡：完整 PUT /api/token/（带 status:1 + 新 expired_time）
│   ├── toggle-card.sh           # 临时开闭：PUT /api/token/?status_only=true
│   └── .env.example             # NEWAPI_BASE / ACCESS_TOKEN 占位（真实 .env 被 gitignore）
│
└── docs/                        # 文档
    ├── daycard-api.md           # §4 三接口的 curl 示例（占位 token）
    ├── runbook.md               # 升级/回滚/排障速查
    ├── newapi-customization.md  # §5 换肤 + 裁剪的落地记录（原 newapi-customization/README.md）
    ├── theme-notion.md          # [data-theme-preset='notion'] 色板/圆角/字体/滚动条规范
    ├── prune-checklist.md       # 删除包逐项清单（含 2026-07 classic 删除 + 单主题化）
    └── REFACTOR-PLAN.md         # 2026-07 顶层重组计划（已执行）
```

> 顶层 `.gitignore` 覆盖：`**/config.yaml` / `config.local.yaml` / `**/.env` / `auth.json` / `**/auths/` / `*.pem` / `*.key` / `*.db` / `*.sqlite*` / `logs/` / `node_modules/`；**保留** `*.example.*` 模板与 lockfile（vendored 源码开发需要）。新增敏感类型时同步补充。

---

## 7. 分阶段实施计划

> 每阶段：**任务 → 验收标准 → 预估**。Phase 之间串行，Phase 内部分任务可并行。

### Phase 0 — 环境确认与前置（预估 0.5 天）

| 任务 | 说明 |
|---|---|
| 复核 claude-tri 规格 | `ssh` 上机复查内存/磁盘/已占端口，确认 8317/3000 未被占 |
| DNS 加记录 | CF 面板给 `api.selab.top` / `codex.selab.top` 各加 A → `70.39.193.15`，先「仅 DNS/灰云」 |
| 确认 `:2022` 用途 | 未知监听服务，上 ufw 前查明避免误伤 |
| 定账号模型 A/B | 见 §4.4，推荐 A（影响后续建卡流程） |

**验收**：`dig api.selab.top` / `dig codex.selab.top` 均返回 `70.39.193.15`；ufw 现状已知；端口无冲突。

---

### Phase 1 — 后端部署（预估 1 天）

| 任务 | 关键点 |
|---|---|
| 装 Caddy | apt / 官方二进制；落 `/etc/caddy/Caddyfile`（先只留 api/codex 两 block），`caddy validate` 通过 |
| ufw 增量放行 | `48687/tcp`(ssh，先加!) + `80/tcp` + `443/tcp`，确认后再 enable，**别锁死自己** |
| 起 CLIProxyAPI | docker-compose，`127.0.0.1:8317:8317`，OAuth 回调口注释；`config.yaml` 填占位 `api-keys` |
| rsync 号池 | 从现跑号池的机器 `rsync -e "ssh -p 48687"` 拉 `auths/*.json` 到 `/opt/cli-proxy-api/auths/` |
| 起 new-api | `deploy/run-newapi.sh`：绑 `127.0.0.1:3000:3000` + 接入 `xju-net`，挂 `/opt/new-api/data`，设 `SESSION_COOKIE_SECURE=true` / `SESSION_COOKIE_TRUSTED_URL=https://api.selab.top` / `SESSION_SECRET`（首次生成后持久化复用，否则重启全员掉登录） |
| 初始化管理员 | ⚠️ **实测修正**：本版**不再自动建 `root/123456`**，改走初始化向导 —— `POST /api/setup {username,password,confirmPassword}`（或浏览器 `/setup`）。**直接设强密码**，省掉「先弱密码再改密」的窗口期 |

**验收**：`https://api.selab.top` 出登录页且 TLS 有效；`https://codex.selab.top` 反代通（`/v1/models` 或返回预期）；两后端只在 `127.0.0.1` 可见（外网 `curl :3000/:8317` 不通）；号池 `auths/` 已就位。

---

### Phase 2 — L1 接 L2（预估 0.5 天）

| 任务 | 关键点 |
|---|---|
| 加 OpenAI 兼容渠道 | new-api 后台 → 渠道 → 添加，类型 `OpenAI`，Base URL `http://127.0.0.1:8317`，Key 填 CLIProxyAPI 常驻 `api-key` |
| 对齐模型名 | 分组/模型列表按 CLIProxyAPI 实际暴露的模型对齐 |
| 建测试令牌 | 建一把带 `expired_time`（近未来）+ `unlimited_quota:true` 的 Token |
| 端到端打通 | 用测试 Token 请求 `api.selab.top/v1/chat/completions` 拿到真实回复 |

**验收**：一次完整请求（用户 Token → api → new-api → 127.0.0.1:8317 → 号池 → 上游 → 回）成功返回；`logs` 表出现该次记账（`prompt/completion_tokens`、`quota`）。

---

### Phase 3 — 前端裁剪 + 换肤（预估 3–5 天）

> 在本地对顶层 `web/` 改；`scripts/package-web-dist.sh` 本机构建并生成带 commit + SHA-256 的发布物，Codex-tri 用 `deploy/install-web-dist.sh` 安装到 `server/newapi/prebuilt/current/dist`，再由 `deploy/build-newapi.sh` 仅编 Go 并把产物嵌入镜像。改动步骤沉淀进 `docs/newapi-customization.md`。

| 任务 | 关键点 |
|---|---|
| 换肤 | 按 §5.5 加 `[data-theme-preset='notion']`：色板/圆角/阴影/字体/滚动条 |
| 裁剪删除包 | 按 §5.2/§5.6 顺序删 A(chat/playground) / B(drawing) / C(wallet) / D(subscriptions) / E(pricing) / F(rankings) / G(redemption) / I(about)，每步跑 typecheck |
| 日卡快捷按钮 | 改 `api-keys-mutate-drawer.tsx` 加 1/3/7/30 天快捷 + 完整 PUT 续卡逻辑 |
| 首页改造 | `routes/index.tsx` + `features/home/` Notion 风格，改 hero/cta 的 `/pricing` 链接 |
| 质量闸 | `bun run typecheck` + `bun run lint` + `bun run knip` 全清零；版权头保留 |

**验收**：`tsgo -b` / lint / knip 无错；被删入口在 UI 消失且无死链；日卡快捷按钮实测能建/续卡；首页/后台呈 Notion 观感；footer 归属与文件版权头完整保留。

---

### Phase 4 — 日卡脚本（预估 1 天，按需）

| 任务 | 关键点 |
|---|---|
| 三脚本 | `issue-card.sh` / `renew-card.sh` / `toggle-card.sh`，读 `.env`（占位）里的 `ACCESS_TOKEN` |
| 续卡正确性 | 严格走完整 PUT（`status:1` + 未来 `expired_time`），覆盖「已过期复活」用例 |
| （可选）卡密自助激活 | 用户输卡密 → 脚本调建/续接口写 `expired_time` |

**验收**：脚本对未过期/已过期 Token 分别正确续时；关卡后请求 401、开卡后恢复；脚本内无硬编码真实 token（全走 `.env`）。

---

### Phase 5 — 上线验收（预估 0.5 天）

| 任务 | 关键点 |
|---|---|
| 备份 | `backup.sh`：SQLite `.backup` + `auths/` + `Caddyfile`/`caddy_data`，cron 滚动保 7 份 |
| 监控告警 | new-api healthcheck；**磁盘阈值告警**（df cron + notify-win，剩 11G 是最大风险）；三处日志纳入监控 |
| 升级/回滚演练 | 镜像 pin 具体 tag；`docker pull → 停旧 → 同 volume 起新 → 验 /api/status`；回滚换回旧 tag（数据在宿主 volume 不丢） |
| 灰度发几张真卡 | 给内测用户发日卡，验证配一次长期可用 + 到期 401 + 续卡复活 |

**验收**：备份产物可恢复；磁盘告警可触达；一次升级+回滚演练成功；内测卡全链路正常。

---

## 8. 安全基线

1. **公网暴露面最小**：仅 Caddy 的 `80/443` 对外；new-api `3000`、CLIProxyAPI `8317` **只绑 `127.0.0.1`**（`docker run -p 127.0.0.1:3000:3000`，compose 加 `127.0.0.1:` 前缀，避开默认 `0.0.0.0` 语法坑）。
2. **OAuth 回调口不对公网**：`1455`(codex)、`54545`(claude)、`51121`(antigravity)、`8085`/`11451`——常驻期全部注释/不放行。普通用户在「我的号池」使用登录导入：浏览器完成 OpenAI 登录后停在本机 `localhost:1455`，用户把完整回调地址粘回页面；new-api 校验固定 origin/path + owner-bound state 后，仅把一次性 code 转交该用户的 CLIProxyAPI。SSH `-L` 仅保留为管理员运维回退。
3. **ufw 增量**：多项目共用机，只 `allow` 新增（48687/80/443）不 reset；enable 前务必确认 48687 已放行。
4. **密钥占位**：`config.yaml` 的 `secret-key`/`api-keys`、`docker run` 的 `SESSION_SECRET` 全用 `__PLACEHOLDER__` 或 `$(openssl rand -hex 32)`；真实值只在部署机本地、`.gitignore` 覆盖，不入库。
5. **鉴权分层**：下游用户持日卡 Token；new-api → CLIProxyAPI 用内部 `api-key`；CLIProxyAPI `remote-management.allow-remote: false`（管理接口仅 localhost）。
6. **初始口令**：new-api `root/123456` 部署后**立即改密** + 视需求关注册/OAuth。
7. **HTTPS cookie**：`SESSION_COOKIE_SECURE=true` + `SESSION_COOKIE_TRUSTED_URL=https://api.selab.top`，否则跨子域信任校验挡登录。

---

## 9. 风险与待确认项

| # | 项 | 现状 / 风险 | 待决策 |
|---|---|---|---|
| 1 | **仓库归属** | ✅ 已定 `winbeau/xju-api`（公开）；**不转 XjuSelab**，维持 winbeau 个人仓 | 无（全程 winbeau 账号 commit + push） |
| 2 | **账号模型 A/B** | ✅ 已定 **模型 A**：每个下游用户一个 new-api 账号，`GET /api/data/users` 原生按用户聚合 | 无（发卡流程 / 统计按模型 A 落地） |
| 3 | **月卡是否上架** | ✅ 已定 **暂不上架**（机制留位 N=30） | 前端日卡快捷按钮只做 1/3/7 天，30 天档先隐藏 |
| 4 | **claude-tri 资源紧张** | 内存空闲 ~126Mi、磁盘剩 ~11G，日志/镜像易撑爆 | 必上磁盘告警；镜像 pin tag 控层数；日志滚动已配 |
| 5 | **多项目共用机干扰** | 已有 redis/postgres/dev-server 占 0.0.0.0；`:2022` 用途未明 | Phase 0 查明 `:2022`；所有端口/防火墙走增量 |
| 6 | **CF 橙云拦 ACME** | 首签期橙云代理可能拦 HTTP-01/TLS-ALPN | 首签先灰云，签发后再评估是否开代理 |
| 7 | **号池搬运** | 真实 `auths/*.json` 在现跑号池的机器上，本仓 `auths/` 只有 `.gitkeep` | Phase 1 rsync 搬运，勿从代码仓找 |
| 8 | **new-api 上游升级** | 源码已内化进本仓（脱离上游 git），前端定制直接改顶层 `web/` | 2026-07 起上游可升级性**非目标**（REFACTOR-PLAN §6）：与上游永久分叉，安全修复人工阅读、手工移植 |
| 9 | **自助支付边界** | 现阶段发卡=后台开票，不做自助支付；subscriptions 已删 | 未来若要自助购卡，正确路径是「支付成功 → 调 token API 写 expired_time」的后端改造，非当前范围 |

---

> 文档版本：v2（已上线）｜Phase 0–5 全部完成,平台运行中｜上线后迭代见 [CHANGELOG.md](./CHANGELOG.md)｜所有凭证均为占位符,真实值不入库。
