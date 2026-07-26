<!-- v1 由 Workflow xju-refactor-plan 生成(understand 5×Sonnet → design 3×Opus → synthesize 1×Opus);2026-07 -->
<!-- v2 2026-07-14 按 owner 指示校准:不 fork、不建 vendor 分支/子仓库,就地大刀阔斧重构,版权仅主 README 声明;并同步 d35fb67..583fec6 落地事实。 -->
<!-- v3 2026-07-14 按 owner 第二轮指示:顶层重组为统一的 xju-api 总体——new-api 与 newapi-customization 两目录务必合并、前后端尽量分离、CLIProxyAPI 移入服务端目录。 -->
<!-- v3.1 2026-07-14 按 owner 第三轮指示:文件命名规范化,scripts/ 与 deploy/ 统一命名规范(见 §5.4)。 -->

# xju-api 重构计划(v3.1)

> 历史说明：本文记录重组当时的资源条件，其中“tri 前端构建会 OOM”的限制已失效。当前生产构建口径以 `CLAUDE.md` 与 `docs/runbook.md` 为准：tri 默认运行 `SKIP_WEB=0 bash deploy/deploy.sh` 完整构建。

> 目标:一个规范、统一、高内聚低耦合的 **xju-api 总体**——顶层即读得出"前端 / 服务端 / 部署 / 脚本 / 文档"五件事,而不是"两个 vendored 仓库 + 一本改造说明书"。
> 方针(owner 已拍板,两轮指示):**就地大刀阔斧重构**——不 fork 上游、不建 vendor 分支、不用 git submodule/subtree;「上游可升级性」不再是设计目标;版权在**主 README 许可小节声明**即可;**顶层重组**:`new-api` 与 `newapi-customization` 合并、前后端尽量分离、CLIProxyAPI 移入 `server/`。
> 硬护栏不变:绝不删除/修改 new-api / QuantumNous / router-for-me 的品牌、页脚归属、版权头、LICENSE/NOTICE、go module 路径(`github.com/QuantumNous/new-api`)。**目录搬迁不触护栏**——归属文件随目录整体走、逐字不动。
> 本文是计划,尚未执行;方向已获 owner 确认,执行时按 §7 分阶段落地。

---

## 0. 版本演进(v1 → v2 → v3 改了什么)

**v2(砍机制)**:砍掉 vendor 分支 + 三方合并 + tag/VERSION 锚点、MANIFEST.yaml + CI、根 LICENSE/NOTICE、双版权头模板 + 6 文件改头闸门 + SPDX 头、i18n 前缀方案(与上游「英文原文即 key」约定冲突);并校准 d35fb67 之后 8 个提交的事实——多池注册表 `ResolvePoolMgmt/ListConfiguredPools`、pool_auth 扩至 7 个池感知 handler(含 zip 批量导入)、池选择 Tabs、channel-test chat/image 双延迟拆分、已有 3 个测试文件("零测试"论断过时)、scripts/deploy 的 K12 与 prebuilt 新增件。

**v3(重组顶层)**:在 v2 的全部动作之上,新增**一次性顶层搬迁**——
- `new-api/web/default/` 上移为顶层 `web/`(前端独立);`web/classic` 主题连同 bun workspace 壳**删除**(未使用,接线面已核实极窄);
- `new-api/`(Go 后端)迁入 `server/newapi/`;`CLIProxyAPI/` 迁入 `server/cliproxy/`;
- `newapi-customization/` 撤销顶层目录,记录并入 `docs/`;
- 前后端分离的边界:**源码目录级分离,交付形态不变**——仍是 go:embed 单二进制,前端构建产物拷入 `server/newapi/web/dist` 接收槽。

**v3.1(命名规范化)**:v1/v2 的「发卡脚本 snake_case 与 deploy/ 命名保持现状 + README 免责说明」边界项**作废翻转**——owner 指示全仓文件命名规范化,scripts/ 与 deploy/ 收敛到同一套规则(§5.4):shell 入口脚本一律 kebab-case 动词-宾语式(`issue_card.sh`→`issue-card.sh` 等 3 处、`new-api.run.sh`→`run-newapi.sh`、compose 文件对齐 `docker-compose.<变体>.yml` 惯例);各生态钦定名(Caddyfile / Dockerfile.* / *.service / Go snake_case)优先于仓库统一规则,不强扭。改名与 P1 顶层搬迁同批落地,tri 只迁移一次。

---

## 1. 现状诊断(校准后)

- **(a) 版权虚标 → 降级**。上游 `add-copyright.mjs` 给无头新前端文件盖 QuantumNous 头,自有原创已有 ≥7 个被盖。owner 口径:主 README 声明兜底,存量不纠正。机制备忘:脚本对已带任意非 QuantumNous `/*…Copyright…*/` 前导块的文件自动跳过(已读源码确认),新自有文件可自愿挂自有头防误盖,不强制、不 fork 脚本。
- **(b) 合规文本缺根级权威源 → 降级**。README 已有「🙏 构建于」+「📄 许可」两节,补一句 CLIProxyAPI = MIT 即达标;不新增根 LICENSE/NOTICE。
- **(c) 自有↔上游边界与内聚问题(仍成立,动机=可读性)**:自有键写死上游共享文件(`use-sidebar-*` 的 `/pool` 字面量)、`routeTree.gen.ts` 生成物入库;号池前端 API client 放错 feature(`features/pool/index.tsx` 从 `features/channels/pool/pool-api.ts` import,横跨两个 feature);管理 API 的 HTTP round-trip 双份近似重复(`controller/pool_auth.go` 的 `poolMgmtRoundTrip` vs `service/pool_cleanup.go` 的 `poolMgmtRequest`);`pool_auth.go` 7 个 root handler 无 `recordManageAudit`(`invite_code.go` 已有 ×4 规范);小时级自动清理只扫 default 池,**K12 池不被自动清理**(有意 or 遗漏,需决断);`pool_cleanup` 无 `IsMasterNode` 守卫;~16 个文件的 `// xju-api:` 标记写法不一。
- **(d) 顶层形态问题(v3 新增)**。仓库顶层读出来是「`new-api/`(内嵌前端)+ `CLIProxyAPI/` 两个外来仓库 + `newapi-customization/` 一本说明书」,而非一个产品:前端源码藏在 `new-api/web/default/` 三层深处;`web/` 还挂着一个从未使用的 `classic` 主题和为它而生的 bun workspace 壳(本机开发被迫 `--filter './default'` 绕行);说明书目录与被说明的代码分居两个顶层目录。

**已实测的搬迁约束(决定 §5.0 怎么做)**:
- `main.go` 以 `//go:embed web/default/dist` + `web/classic/dist` 嵌入双主题——go:embed 只能引用包目录树内的文件,**前端源码外迁后,构建产物必须拷回 Go 树内**;
- classic 接线面极窄:`main.go`(2 组 embed 变量 + 传参)+ `router/web-router.go`(`FrontendAssets` 结构、`NewThemeAwareFS`、`GetTheme()=="classic"` 分支),删除是小改动;
- `web/package.json` 是 bun workspace 壳(members: default、classic + catalog 版本表),拆平后 `--filter` 怪癖消失;
- 两个 Dockerfile 的 build context = `new-api/`,COPY 路径需随迁改写;`cli-proxy.docker-compose.yml` 用预构建镜像、不引用 CLIProxyAPI 源码路径,**搬它零影响**;
- 本机 docker build 已坏 → 实际主路径是「本机 bun build + tri 用 `Dockerfile.newapi.prebuilt` 编 Go」,全量版 `Dockerfile.newapi`(含双前端构建阶段)当前两台机都跑不动。

---

## 2. 方针

**就地大刀阔斧**:new-api 与 CLIProxyAPI 视为**已内化的代码**,不再为「未来 merge 上游」保留任何机制或克制——可直接改上游文件、激进裁剪、按产品形态重排目录。约束只有两条:

1. **硬护栏**(文首):品牌/归属/版权头/LICENSE/go module 路径不动;目录搬迁时归属文件随目录整体走。
2. **产品在线**(v0.5.15+):每 Phase 可交付、可回滚;前端 build 与质量闸一律在本机跑(tri 会 OOM);顶层搬迁在独立分支完成、全绿后合入,tri 重新 clone 部署。

---

## 3. 目标目录结构

标注:**【上游】**=保留归属、可就地改;**【自有】**=xju-api 原创;**★**=本次新增/移动/改动。

```
xju-api/
├── README.md                    ★ 许可小节补 CLIProxyAPI=MIT 一句;仓库结构节刷新
├── PLAN.md · CHANGELOG.md · CLAUDE.md   ★ 路径/命令全面对齐新布局(cd web、server/newapi …)
├── .gitignore                   ★ web/dist · server/newapi/web/dist · routeTree.gen.ts · i18n _reports/*.untranslated.json
│
├── web/                         ★【前端】原 new-api/web/default 整体上移;bun workspace 拆平,classic 删除
│   ├── package.json             ★ 由 workspace member 转独立应用,catalog 版本表内联,重生 bun.lock
│   ├── rsbuild.config.ts · knip.config.ts · AGENTS.md · scripts/(i18n 工具、add-copyright.mjs 随迁)
│   └── src/
│       ├── registry/xju-modules.ts       ★【自有·新增】自有侧栏/路由/section 键收敛中心
│       ├── features/
│       │   ├── pool/                     【自有整目录】index.tsx(Tabs+zip 导入,已落地)+ ★api.ts(由 channels/pool/pool-api.ts 迁入)
│       │   ├── invite-codes/             【自有整目录】★横切接入改走 section-registry
│       │   ├── channels/                 【上游·edit】渠道测试双延迟 UI(已落地);★pool/ 迁空后删除
│       │   └── keys/components/pool-integration/   ★ cc-switch / codex-config 迁入
│       ├── hooks/{use-sidebar-config,use-sidebar-data}.ts  【上游·edit】★import xju-modules 后 merge
│       └── i18n/locales/*.json           沿用「英文原文即 key」,不加前缀
│
├── server/                      ★【服务端】
│   ├── newapi/                  【上游·AGPL-3.0】原 new-api/ 整体迁入(去前端源码);go module 路径不变(护栏)
│   │   ├── go.mod               module github.com/QuantumNous/new-api —— 逐字不动
│   │   ├── LICENSE · NOTICE · THIRD-PARTY-LICENSES.md   随迁、原样(Docker /licenses 权威源)
│   │   ├── main.go              ★ embed 收敛单主题:`//go:embed web/dist`(classic 两组 embed 变量删除)
│   │   ├── common/xju_pool_registry.go   【自有】★改名;ResolvePoolMgmt/ListConfiguredPools(已落地)
│   │   ├── controller/
│   │   │   ├── xju_invite_code.go · xju_pool_auth.go    【自有】★改名;7 handler ★补 recordManageAudit
│   │   │   ├── channel-test.go   【上游·edit】chat/image 拆分(已落地)
│   │   │   └── user.go           【上游·edit】★Register 邀请码收口
│   │   ├── model/xju_invite_code.go · model/channel.go(ResponseTimeImage,已落地)
│   │   ├── service/
│   │   │   ├── xju_pool_client.go        ★【自有·新增】唯一 HTTP round-trip helper
│   │   │   └── xju_pool_cleanup.go       【自有】★改名;★IsMasterNode 守卫 + 多池清理决断
│   │   ├── router/web-router.go  【上游·edit】★单主题化(去 ThemeAwareFS/classic 分支)
│   │   └── web/dist/            ★ 前端构建产物接收槽(gitignore;占位 index.html 保 go build 可编译)
│   └── cliproxy/                【上游·MIT】原 CLIProxyAPI/ 整体迁入;LICENSE 随迁;默认零改动、按需删减
│
├── deploy/                      【自有·运维】★路径改写 + 命名规范化(规则见 §5.4)
│   ├── build-newapi.sh          ★ 流程改:cd web && bun run build → 拷 dist 入 server/newapi/web/dist → docker build
│   ├── run-newapi.sh            ★ 由 new-api.run.sh 改名(对齐 build-newapi.sh 的动词-宾语式)
│   ├── Dockerfile.newapi.prebuilt  ★ context=server/newapi;prebuilt 只剩单 dist;/licenses COPY 路径随迁
│   ├── Dockerfile.newapi        ★ 决断:改造(context=仓库根,COPY web/ + server/newapi/)或删除——两台机现都跑不动全量构建,prebuilt 是主路径
│   ├── docker-compose.cliproxy.yml  ★ 由 cli-proxy.docker-compose.yml 改名(compose 惯例 docker-compose.<变体>.yml)
│   ├── setup-pool-mgmt.sh · setup-pool-mgmt-k12.sh · backup.sh   (已合规)
│   ├── config.example.yaml · config.k12.example.yaml   (已合规:目标名插 .example)
│   ├── Caddyfile                生态钦定名,不动
│   └── cli-proxy-api.service    ★ 决断:compose 若已是唯一部署路径则删除;保留则不改名(unit 名=线上服务名)
│
├── scripts/                     【自有·发卡+运维】★命名规范化(规则见 §5.4)
│   ├── _common.sh               ★ 新增:被 source 的库,抽发卡三件套公共段(下划线前缀=非入口标记)
│   ├── issue-card.sh · renew-card.sh · toggle-card.sh   ★ 由 snake_case 改名
│   └── import-pool-zip.sh · create-k12-channel.sh   (已合规)
│
└── docs/
    ├── daycard-api.md · REFACTOR-PLAN.md(本文)
    ├── runbook.md               ★ 补「双池密钥」节 + 「新布局部署」节(tri 重新 clone 的迁移步骤)
    ├── newapi-customization.md  ★ 原 newapi-customization/README.md 并入:自有文件/注入点清单 + 构建耗时唯一出处;「升级重放顺序」标历史参考
    ├── prune-checklist.md · theme-notion.md   ★ 由 newapi-customization/ 迁入并刷新
    └── superpowers/{plans,specs}/   设计文档存档
```

**边界判据(写进 web/AGENTS.md 与 server/newapi 注释,三层可 grep)**:(1) **目录归属**——`web/src/features/pool`、`web/src/features/invite-codes`、`web/src/registry/`、`pool-integration/`、`deploy/`、`scripts/`、`docs/` 整目录自有;(2) **受控标记词表**——全仓只允许 `// xju-api:{new|edit|prune|inject}` 四标签;(3) **文件命名**——自有 Go 文件统一 `xju_` 前缀。并写明:**版权头不作来源判据**。

---

## 4. 版权(极简口径)

- **主 README 一处声明,即唯一权威口径**:「🙏 构建于」节保留双致谢;「📄 许可」节补一句:`server/cliproxy/ 子树为 MIT(router-for-me),随附分发、独立进程运行,许可见 server/cliproxy/LICENSE`。
- **不新增**根 LICENSE/NOTICE,**不做**版权头纠错/双模板/SPDX 工程,**不 fork** `add-copyright.mjs`。
- **护栏照旧且随迁**:`server/newapi/{LICENSE,NOTICE,THIRD-PARTY-LICENSES.md}`、`server/cliproxy/LICENSE`、footer.tsx ProjectAttribution、go module 路径逐字不动;`Dockerfile.newapi.prebuilt` 的 `/licenses` COPY 链路随 context 迁移同步改路径、内容不变。
- 机制备忘:`add-copyright.mjs`(随迁至 `web/scripts/`)对已带任意非 QuantumNous Copyright 前导块的文件自动跳过;新自有前端文件可挂 `Copyright (C) 2026 xju-api contributors` 头防误盖,不强制。

---

## 5. 具体动作

### 5.0 顶层重组(v3 新增,一次性搬迁)

全部用 `git mv` 保留历史(`git log --follow` 可追溯),在独立分支完成:

1. **前端上移**:`git mv new-api/web/default web`;删除 `new-api/web/classic` 与 workspace 壳(`new-api/web/package.json`、`bun.lock`、`node_modules`——顺带回收本机紧张磁盘);`web/package.json` 从 workspace member 转独立应用,catalog 版本内联,重生 `bun.lock`;`--filter './default'` 怪癖就此消失。
2. **单主题化**:`main.go` 删 `classicBuildFS/classicIndexPage` 两组 embed 变量,embed 路径改 `//go:embed web/dist`;`router/web-router.go` 的 `FrontendAssets` 去 Classic 字段、`NewThemeAwareFS` 折叠为 defaultFS、删 `GetTheme()=="classic"` 分支(核对 GetTheme 默认回退,防线上 theme option 残值);Dockerfile 双前端构建阶段删 classic 一半。
3. **服务端归位**:`git mv new-api server/newapi`;`git mv CLIProxyAPI server/cliproxy`。go.mod 的 module 路径与包内 import 全部**不需要动**(Go 按 module 路径解析,目录位置无关);两个独立 module 平级共存,无嵌套问题。
4. **产物流**:`server/newapi/web/dist/` 作 go:embed 接收槽——gitignore 产物、入库一个占位 `index.html`(保证不跑前端也能 `go build`);`deploy/build-newapi.sh` 改为「`cd web && bun run build` → 拷 `web/dist` → `server/newapi/web/dist` → docker build(context=`server/newapi`)」;prebuilt 流单 dist 化(tar/scp → tri 的 `prebuilt/` → COPY 覆盖)。
5. **说明书并入**:`newapi-customization/` 三个 md 迁入 `docs/`(README.md → `docs/newapi-customization.md`),`patches/` 按 §5.3 决断后删目录。
6. **命名规范化同批落地**(§5.4 改名表):与搬迁合成**一次迁移事件**——tri 只重新 clone 一次、cron(backup.sh 绝对路径、jercy 定时任务)与 systemd/compose 引用只改一轮。
7. **全仓路径修复**:CLAUDE.md(前端命令改 `cd web`)、PLAN.md、README 结构节、`.gitignore`、runbook、两个 Dockerfile;`Dockerfile.newapi` 全量版决断(改造或删除,理由见 §3 树注);旧文件名/旧路径全仓 grep 清零(现存引用约 30 处:PLAN.md ×8、runbook ×5、脚本自引注释、README、daycard-api.md)。

### 5.1 Frontend(`web/src`)

- **注册反转**:新建 `registry/xju-modules.ts` 收敛自有 nav 项、`DEFAULT_SIDEBAR_MODULES.admin.pool`、`URL_TO_CONFIG_MAP['/pool']`、section 定义;`use-sidebar-config.ts` / `use-sidebar-data.ts` / `nav-modules.ts` 只留 import + merge。
- **修 feature 越界**:`features/channels/pool/pool-api.ts` → `features/pool/api.ts`,`channels/pool/` 删除。
- **invite-codes 横切收口**:改走上游既有 `section-registry.tsx` 扩展点。
- **keys 自有件内聚**:`cc-switch-dialog`、`codex-config-dialog` 迁 `keys/components/pool-integration/`。
- **生成物出库**:`routeTree.gen.ts` 与 i18n `_reports/*.untranslated.json` gitignore。
- **i18n**:沿用「英文原文即 key」扁平约定 + `i18n:sync`。
- 纯上游 UI kit(`components/ui`、`components/layout`、`lib`、`stores`)只消费不改;裁剪可更激进,每轮后 typecheck/lint/knip/build 四闸清零。

### 5.2 Backend(`server/newapi`,复用原生横切机制)

- **HTTP round-trip 单一来源**:新增 `service/xju_pool_client.go`,收敛 controller 的 `poolMgmtClient`+`poolMgmtRoundTrip` 与 service 的 `poolCleanupClient`+`poolMgmtRequest`(env 解析已由 `common.ResolvePoolMgmt` 统一)。
- **审计对齐**:`pool_auth` 7 个 root handler(ListPools 除外可议)补 `recordManageAudit`(pool_auth.add/import/delete/status/clean)。
- **多实例守卫**:`pool_cleanup` 补 `if !common.IsMasterNode { return }`。
- **auto-clean 多池决断**:改为遍历 `ListConfiguredPools()` 逐池 sweep,或注释 + runbook 明确「K12 只手动清」——二选一,不留隐性行为。
- **Register 收口**:抽 `service.ConsumeInviteCodeForRegistration(affCode) (release func(), err error)`,`user.go` 只留调用 + defer;**单测先行**(消费/回滚并发原子性,testify),邀请码泄漏/永久占用是最大风险;延续 `pool_registry_test.go`/`pool_auth_test.go` 已建立的测试风格。
- **provenance 改名**:`invite_code.go`(controller/model)、`pool_auth.go`、`pool_cleanup.go`、`pool_registry.go` 加 `xju_` 前缀(Go 改名零 import 影响)。
- **Option 契约显式化**:`option.go` 旁挂「xju Option key + 类型」清单注释(`InviteCodeRequired`/`PoolAutoCleanEnabled` 走 `*Enabled` 后缀、`PoolAutoCleanHours` 走特判)。
- **AffCode 双语义**:补注释或请求体单列 `invite_code` 字段。

### 5.3 Scripts / Deploy / Docs

- `scripts/_common.sh` 抽发卡三件套公共段;`import-pool-zip.sh`/`create-k12-channel.sh` 走 MGMT/admin API、按需接入不强求。
- runbook 补「双池密钥」节(`POOL_MGMT_SECRET`+`POOL_K12_MGMT_SECRET` 生成/轮换/`--force`/`xju-net` 互访契约)+「新布局部署」节(tri 重新 clone、prebuilt 新路径、cron/compose 引用更新、回滚到旧布局的方法)。
- PLAN.md §4.2 续卡两步 PUT 对齐 `renew-card.sh` 并链接 `daycard-api.md ②`;§6 树、§7 完成态刷新;构建耗时数字只留 `docs/newapi-customization.md` 一处。
- `docs/newapi-customization.md` 补「自有文件/注入点清单」(prose、人读);「升级重放顺序」标历史参考;`prune-checklist.md`/`theme-notion.md` 刷新至现状(含 classic 删除、单主题化)。
- `patches/` 决断:落真实 patch 或删除(记录并入 newapi-customization.md 一句话)。

### 5.4 文件命名规范(全仓统一,scripts/ 与 deploy/ 同一套规则)

**规则(六条,生态钦定约定优先于仓库统一规则)**:

1. **shell 入口脚本**:kebab-case 动词-宾语式 `<verb>-<object>.sh`,scripts/ 与 deploy/ 无差别适用;变体用连字符后缀(`setup-pool-mgmt-k12.sh`)。
2. **被 source 的库**:下划线前缀(`_common.sh`)——标记「非入口、不可直接执行」,且目录内排序置顶。
3. **配置样板**:目标文件名中插 `.example`(`config.example.yaml` → 部署时落为 `config.yaml`),变体用点分段(`config.k12.example.yaml`)。
4. **生态钦定名不动**:`Caddyfile`、`Dockerfile.<target>[.<variant>]`、`*.service`(unit 名=线上服务名)、`docker-compose.<variant>.yml`、README/PLAN/CHANGELOG/AGENTS/CLAUDE 等全大写仓库元文档。
5. **Go 自有文件**:Go 生态 snake_case + `xju_` 前缀(与 shell 的 kebab 不冲突——各生态各自的约定优先)。
6. **docs 与前端**:小写 kebab-case(`daycard-api.md`、上游前端文件命名),已一致,沿用。

**改名表(P1 同批执行,git mv 保历史)**:

| 现名 | 新名 | 依据 |
|---|---|---|
| `scripts/issue_card.sh` | `scripts/issue-card.sh` | 规则 1 |
| `scripts/renew_card.sh` | `scripts/renew-card.sh` | 规则 1 |
| `scripts/toggle_card.sh` | `scripts/toggle-card.sh` | 规则 1 |
| `deploy/new-api.run.sh` | `deploy/run-newapi.sh` | 规则 1,对齐 `build-newapi.sh` |
| `deploy/cli-proxy.docker-compose.yml` | `deploy/docker-compose.cliproxy.yml` | 规则 4 compose 惯例 |
| `deploy/cli-proxy-api.service` | 决断:删除(compose 已是唯一部署路径时)或保留原名 | 规则 4 |

已合规无需动:`import-pool-zip.sh`、`create-k12-channel.sh`、`build-newapi.sh`、`setup-pool-mgmt{,-k12}.sh`、`backup.sh`、`config{,.k12}.example.yaml`、`Caddyfile`、`Dockerfile.newapi{,.prebuilt}`。

**引用修复**:全仓 grep 旧名清零(~30 处:PLAN.md ×8、runbook ×5、脚本自引注释与互引、README、daycard-api.md);tri 侧 cron(backup.sh 绝对路径、jercy 定时任务)与 compose 命令行 `-f` 参数在「新布局部署」迁移时一并更新——与 P1 顶层搬迁合成一次迁移事件。

---

## 6. 上游升级 = 非目标

- **new-api**:顶层重组 + 大刀阔斧后与上游永久分叉,不再追新版本;上游安全修复人工阅读、手工移植。
- **CLIProxyAPI**(`server/cliproxy/`):唯一保留的可升级面——独立进程 + 纯 HTTP 契约,如换新版本整目录替换,回归 `/v0/management/auth-files*` 契约字段(`unavailable`/`updated_at`/`last_refresh`/`files`),**default 与 k12 两池各跑一遍**。
- 唯一保留的机器校验:~10 行**护栏自检脚本**(grep footer ProjectAttribution、`server/newapi/{LICENSE,NOTICE,THIRD-PARTY-LICENSES.md}` 未改动、go module 路径)——大幅搬迁/裁剪时误伤品牌归属的风险真实存在,这条保险便宜且必要。

---

## 7. 分阶段路线(P0 → P1 → P2 ‖ P3 ‖ P4,总量 ~4.5–6 人日)

> 全程不阻断线上;P1 在独立分支、全绿后合入;前端 build 一律本机(tri OOM)。**P1 先行**,让后续所有工作落在最终路径上。

### P0 · 仓库卫生 + 轻量合规(~0.5 天)
- **动作**:README 许可小节补 CLIProxyAPI=MIT;`.gitignore` 加 routeTree.gen.ts + i18n _reports;统一 ~16 处标记为四标签词表;落护栏自检脚本。
- **验收**:README 双许可如实;`git status` 干净;自检脚本通过。
- **风险**:近零。

### P1 · 顶层重组 + 命名规范化(~1–1.5 天,全案最大动作,独立分支)
- **动作**:§5.0 全部七步(git mv 三大搬迁、classic 删除 + 单主题化、workspace 拆平、embed/产物流改造、说明书并入、§5.4 改名表同批执行、全仓路径修复)。
- **验收**:`cd web && bun run build` 绿;dist 拷入后 `cd server/newapi && go build` 绿 + 既有单测全过;prebuilt 镜像流程在 tri 完整演练一遍并起服务;`/pool`(双池 Tabs/zip 导入)、发卡(新名脚本)、渠道测试手工回归;全仓 grep 旧文件名/旧路径零残留;`git log --follow` 能追溯迁移文件历史;护栏自检通过。
- **风险**:高(一次性大搬迁)——独立分支隔离;tri 按 runbook「新布局部署」节重新 clone;失败整支废弃,main 不受影响。

### P2 · 前端内聚(~1–1.5 天)
- **动作**:§5.1——xju-modules.ts 注册反转、pool api 迁移、section-registry、pool-integration。
- **验收**:typecheck/lint/knip/build 四闸清零;dev 下 /pool、邀请码、渠道测试手工回归;上游常量对象无 `/pool` 字面量;`features/channels/pool/` 已删除。
- **风险**:中(触路由/侧栏骨架 + routeTree 再生),单独可回滚。

### P3 · 后端收口(~1–1.5 天,可与 P2 并行)
- **动作**:§5.2 全部;先补邀请码消费/回滚单测再做 Register 收口。
- **验收**:`go build` + 全部单测过;发卡链路 + 双池 auth-files 增删/导入/清理回归;号池操作出审计日志。
- **风险**:中;auto-clean 多池决断改行为前与 owner 确认 K12 是否纳入自动清理。

### P4 · scripts/deploy/docs 标准化(~0.5–1 天,可与 P2/P3 并行)
- **动作**:§5.3 全部。
- **验收**:三发卡脚本 source `_common.sh` 且发卡回归通过;runbook 有双池密钥 + 新布局部署两节;PLAN §4.2 与 `renew-card.sh` 一致。
- **风险**:低。

---

## 8. 明确不做什么

1. **不 fork 上游、不建 vendor 分支、不用 git submodule/subtree、不做三方合并机制**(owner 指示)。
2. **不建 MANIFEST.yaml、不做重量级 CI**——自有清单在 `docs/newapi-customization.md` 人读维护;唯一机器校验是护栏自检脚本。
3. **不新增根 LICENSE/NOTICE、不做版权头纠错/双模板/SPDX 工程**——版权口径 = 主 README 声明 + 子目录上游许可文件原样随迁。
4. **不动硬护栏**:品牌、页脚归属、版权头、LICENSE/NOTICE、**go module 路径 `github.com/QuantumNous/new-api`**(改它 = 全量 import 重写 + 违护栏,目录迁移完全不需要动它)。
5. **前后端分离止于目录级**——不拆独立 git 仓库、不上 monorepo 工具(nx/turbo);交付形态仍是 go:embed 单二进制、new-api 单进程托管前端,Caddy/部署拓扑不变。
6. **CLIProxyAPI 默认零改动**(位置迁移 ≠ 代码改动),按需可删减/升级适配;不为 L1 便利反向改其 management API 形状。
7. **不把后端自有文件搬独立子包**(`controller/xju` 等)——失去 `recordManageAudit` 等未导出 helper 的访问;`xju_` 前缀即可。
8. **命名统一不越过生态钦定约定**——`Caddyfile`、`Dockerfile.<x>`、`*.service`(若保留)、全大写仓库元文档、Go snake_case 不强扭 kebab;统一规则只管自有 shell 脚本/配置样板/文档(v1/v2「snake_case 保持现状 + 免责说明」边界项作废,由 §5.4 取代)。
9. **不折腾 i18n 结构**:「英文原文即 key」扁平约定 + `i18n:sync`,不加前缀、不拆 namespace。
10. **不给 Go 文件逐一加许可头**——README 总声明已覆盖。
