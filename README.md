<div align="center">

# <img src="web/public/logo.png" width="56" align="absmiddle" alt="" /> XJU API

新疆大学 · 软件开发实验室<br />
<sub>Software Engineering Lab · Xinjiang University</sub>

> 兼容 OpenAI 的三层 AI API 代理平台

[![api.selab.top](https://img.shields.io/badge/api.selab.top-live-0f7b6c?style=flat-square)](https://api.selab.top)
[![API](https://img.shields.io/badge/API-OpenAI%20compatible-2383e2?style=flat-square)](#支持的客户端)
[![built on](https://img.shields.io/badge/built%20on-New%20API%20·%20CLIProxyAPI-d9730d?style=flat-square)](#构建于)
[![license](https://img.shields.io/badge/license-AGPL--3.0-9065b0?style=flat-square)](#许可)

</div>

---

## 这是什么

新疆大学 Se Lab 自建的 AI API 代理平台。

## 特性

- **时间卡** —— 日 / 三天 / 周卡,机制见 [docs/daycard-api.md](./docs/daycard-api.md)。
- **号池管理页** —— 导入 / 验活 / 额度 / 一键开独立号池。
- **仅邀请注册** —— 一次性邀请码,一码一用。
- **Codex 一键配置** —— 一键复制 `config.toml` / `auth.json`。
- **用量看板** —— USD 与 token 双显。
- **可靠用量记账** —— Codex stream/non-stream exactly-once 收口,中断与零 usage 不再从 CPA/Redis 漏记。
- **Notion 风格前端** —— 极简换肤,裁掉用不上的功能。

## 进行中

- **单账号周限额 + 重置券** —— 号池页逐账号 5h / 周窗口用量与重置券。
- **单账号订阅期限** —— 让每个号显示 ChatGPT 订阅到期,方案见 [docs/pool-enrichment-design.md](./docs/pool-enrichment-design.md)。
- **统一号池搭建模式** —— 建池双模:CLIProxy enriched 登录 / go-pool 批量。

> 系统怎么跑、号池技术全解见 [docs/architecture-and-pool-tech.md](./docs/architecture-and-pool-tech.md)。

## 支持的客户端

任意 OpenAI 兼容客户端:Codex（CLI / GUI）、Cherry Studio、CC Switch,以及 curl / SDK / 其它 CLI。

## 架构

| 层 | 承载 | 域名 | 职责 |
|---|---|---|---|
| L1 用户配置层 | new-api | `api.selab.top` | 发卡 / 续卡 / 开闭 / 用量统计 |
| L2 中转胶水 | CLIProxyAPI | `codex.selab.top` | OpenAI 兼容请求转上游各家协议 |
| L3 号源号池 | CLIProxyAPI | — | 上游账号凭证 `auths/*.json`,负载轮换 |

入口 Caddy 两个子域各自 ACME/TLS,反代到只绑 `127.0.0.1` 的后端。

```
用户 SDK ──(Bearer=时间卡 Token)──▶ api.selab.top(Caddy) ──▶ new-api(:3000)
   校验到期/额度 ──(Bearer=内部 api-key)──▶ 127.0.0.1:8317 CLIProxyAPI ──▶ 号池 ──▶ 上游 AI
```

## 部署

开发改动与前端发布物在本机完成；`claude-tri` 只拉取已提交代码并部署。New API 与
CLIProxyAPI 使用独立入口：

```bash
# New API：先安装与 HEAD 匹配的前端发布物，再只编 Go 并换容器
cd /home/winbeau/opt/xju-api
PULL=0 SKIP_WEB=1 bash deploy/deploy.sh

# CLIProxyAPI：只构建 Go，镜像固定为 deploy-<当前 7 位提交 SHA>
bash deploy/deploy-cliproxy.sh --dry-run
PRUNE=0 bash deploy/deploy-cliproxy.sh
```

CLIProxyAPI 入口会逐池验活、失败自动回滚，并让未来新建号池使用同一 commit 镜像。
编排见 [`deploy/`](./deploy/);升级、回滚、排障见 [docs/runbook.md](./docs/runbook.md)。

## 目录结构

```
xju-api/
├── web/              前端
├── server/
│   ├── newapi/       L1 发卡 / 统计
│   └── cliproxy/     L2/L3 号池
├── deploy/           部署脚手架
├── scripts/          发卡 glue + 护栏自检
├── docs/             架构 / 号池技术 / 接口 / runbook
├── PLAN.md           完整实施计划
└── CHANGELOG.md      上线后迭代记录
```

## 安全

公开仓库,密钥一律占位符,真实凭证只在部署机本地、被 `.gitignore` 排除。仅 Caddy `80/443` 对外,后端只绑 `127.0.0.1`。详见 [PLAN.md §8](./PLAN.md#8-安全基线)。

## 构建于

本平台基于两个开源项目构建,谨此致谢,其归属与版权均予保留:

- **[New API](https://github.com/QuantumNous/new-api)** —— by QuantumNous,本平台 L1 基座,AGPL-3.0。
- **[CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)** —— by router-for-me,本平台 L2/L3 基座,MIT。

本仓库对 new-api 做了换肤、裁剪与若干功能增强(见 [docs/newapi-customization.md](./docs/newapi-customization.md)),但未删除或修改 New API 与 QuantumNous 的品牌、页脚归属与版权头。

## 许可

本仓库整体以 AGPL-3.0 发布,全文见根 [LICENSE](./LICENSE)。

`server/cliproxy/` 子树为 MIT(router-for-me),随附分发、以独立进程运行,许可原文见 [server/cliproxy/LICENSE](./server/cliproxy/LICENSE)。
