# 运维速查（claude-tri）

> 部署全流程见 [PLAN.md §7](../PLAN.md#7-分阶段实施计划)；本文只放上线后的升级 / 回滚 / 排障。

## 组件与落位

| 组件 | 落位 | 监听 | 数据 |
|---|---|---|---|
| Caddy | 系统服务 `/etc/caddy/Caddyfile` | `0.0.0.0:80/443` | 证书 `/var/lib/caddy` |
| new-api | docker `new-api`（[deploy/run-newapi.sh](../deploy/run-newapi.sh)） | `127.0.0.1:3000` | `/opt/new-api/data/one-api.db` |
| CLIProxyAPI 号池 | **动态池容器** `cli-proxy-api-<id>`（provision watcher 起）——现役 `main`(=default 组,8317) / `k12-pool`(=k12 组,8318) + 任意一键开的池 | `127.0.0.1:<port>` | `config.<id>.yaml` + `auths-<id>/` |

> **2026-07-16 起:default / k12 也已并入动态池统一管理**(见下「统一动态池」节)。原 compose 管的 `cli-proxy-api` / `cli-proxy-api-k12` 两静态容器已删除,`docker-compose.cliproxy.yml` 退役——**切勿再 `docker compose up`**,否则会重建 8317/8318 静态容器与动态池 `cli-proxy-api-main` / `cli-proxy-api-k12-pool` 抢端口。env 播种的 default/k12 池已通过清空 `POOL_MGMT_SECRET`/`POOL_K12_MGMT_SECRET`(即把 `.pool-mgmt.env` / `.pool-mgmt-k12.env` 改名 `.retired`)从前端隐藏。

## 升级（先 pin tag，勿追 latest）

> ⚠️ new-api 前端已做换肤 + 裁剪 + 功能增强,**不能 `docker pull` 上游镜像**(会丢定制),必须**自建镜像** `winbeau/xju-newapi:<tag>`。

### New API 标准发布链路（前端只在 Codex-vps 构建）

Default 付费池**首次上线**还有一次性数据步骤，必须先完整执行
[default-paid-pool.md](./default-paid-pool.md)：停止旧版 new-api 后先 dry-run，再运行
`scripts/reset-default-balances.sh --apply /opt/new-api/data/one-api.db`；只有脚本成功、
备份与 SHA-256 已记录后才能启动新镜像。不能先启新版本再清零。脚本会同时把在线
支付总开关写为关闭，支付渠道和法币比例未确定前不得开启。

先在 Codex-vps 的干净、已提交工作树中构建并打包。打包脚本会重新执行
`bun run typecheck` 与 `bun run build`，把当前完整 Git SHA 和静态文件树哈希写入
manifest，并生成 SHA-256 sidecar。应先 commit + push，保证 tri 能取得完全相同的提交：

```bash
cd /Users/jacksonhuang/project/xju-api
git status --short
./scripts/check-guardrails.sh
./scripts/package-web-dist.sh /private/tmp/xju-web-artifacts

# 取脚本刚输出的两个绝对路径；以下文件名仅作示例。
rsync -a \
  /private/tmp/xju-web-artifacts/xju-web-dist-<sha>-<timestamp>.tar.gz \
  /private/tmp/xju-web-artifacts/xju-web-dist-<sha>-<timestamp>.tar.gz.sha256 \
  claude-tri:/home/winbeau/opt/xju-artifacts/
```

再到 Codex-tri 先更新到**同一个提交**、安装发布物，最后只编 Go。安装器会校验
SHA-256、拒绝路径穿越/链接/超大归档、核对 manifest 与当前 `HEAD`，并把旧 bundle
保留为 `server/newapi/prebuilt/current.previous.<timestamp>`：

```bash
cd /home/winbeau/opt/xju-api

# 必须在 main；空输出表示 detached HEAD，此时先停下核对，不要强制 reset。
test "$(git branch --show-current)" = main
git status --short --untracked-files=no
git pull --ff-only origin main

artifact=/home/winbeau/opt/xju-artifacts/xju-web-dist-<sha>-<timestamp>.tar.gz
bash deploy/install-web-dist.sh "$artifact" "$artifact.sha256"

# 已在上一步更新代码，因此显式 PULL=0；SKIP_WEB=1 也是 tri 默认值。
PULL=0 SKIP_WEB=1 bash deploy/deploy.sh release-<sha>
```

`deploy/build-newapi.sh` 会再次检查 `prebuilt/current/dist/index.html`、静态资源、
manifest 提交号、dirty 标记、文件树哈希及安装 journal；安装和构建共用互斥锁。
任一不匹配都会在 Docker 构建和容器替换前停止。
不要在 tri 使用 `SKIP_WEB=0`，它会违反两机分工并可能因内存不足 OOM。
新镜像同时写入后端提交、前端提交和发布物 SHA 标签，可在替换容器前检查：

```bash
docker image inspect winbeau/xju-newapi:<tag> --format '{{json .Config.Labels}}'
```

若 tri 当前处于 detached HEAD，先只读确认 `git status --short --untracked-files=no`
没有 tracked 改动，再显式 `git switch main`；未跟踪的旧 `new-api/prebuilt/` 是历史构建
产物，不属于新输入路径，切换前不要自动删除。新版本稳定后再单独归档或清理。

```bash
# new-api:仓库在 /home/winbeau/opt/xju-api;数据在宿主 volume,换镜像不丢。
# 在已按上文安装匹配前端发布物后,总入口跑护栏、只构建 Go、换容器、
# 清理 Docker,再检查本地/公网 API 与 xju-provision;新版失败时尝试回滚。
cd /home/winbeau/opt/xju-api
PULL=0 SKIP_WEB=1 bash deploy/deploy.sh

# 指定镜像 tag:
PULL=0 SKIP_WEB=1 bash deploy/deploy.sh announcements-20260724

# deploy.sh 默认 SKIP_WEB=1；若代码在安装发布物后未再变化,也可省略显式值:
PULL=0 bash deploy/deploy.sh release-tag

# CLIProxyAPI（自建镜像，commit 与镜像一一对应；只构建 Go，不构建前端）
cd /home/winbeau/opt/xju-api

# 只预检：打印 main HEAD、目标 deploy-<7位SHA>、现役池和 canary/main 顺序，零变更
bash deploy/deploy-cliproxy.sh --dry-run

# 一键拉取 main、构建、备份、逐池升级、失败自动回滚、同步未来新池镜像并验活
PRUNE=0 bash deploy/deploy-cliproxy.sh

# 部署后发现回归时，使用 .cliproxy-image.env 中记录的精确上一镜像整批回滚
PULL=0 bash deploy/deploy-cliproxy.sh --rollback

# 也可显式指定 commit 镜像
PULL=0 bash deploy/deploy-cliproxy.sh \
  --rollback winbeau/cli-proxy-api:deploy-123abcd
```

CLIProxyAPI 镜像固定为 `winbeau/cli-proxy-api:deploy-<当前 main 的 7 位提交短 SHA>`，
脚本拒绝 tag/commit 不一致的构建。动态 registry 若因宿主权限不可读，脚本会通过运行中的
`new-api` 容器读取 `/data/xju-pools.json`，因此 `--dry-run` 不需要修改数据库文件权限。它只管理**当前正在运行**的
`cli-proxy-api-<id>`，不会复活故意停用或不存在的 `k12-pool`。升级顺序为一个非
`main` 池作 canary、其余池、`main` 最后；私有池的 256 MiB/0.75 CPU/PID/日志限制
由 `deploy/cliproxy-pool-runtime.sh` 统一保留。

部署期间会创建 provision maintenance gate，等待在途开池完成后停止 watcher；所有池
通过后才原子更新 `/opt/cli-proxy-api/.cliproxy-image.env` 并重启 watcher，因此现役池与
未来新池始终使用同一 commit 镜像。任一池失败会逆序恢复本轮已升级池；若无法完整恢复，
maintenance 会保留且 watcher 不启动，避免在不一致 fleet 上继续开池。

默认 `PRUNE=0`，上一镜像作为回滚锚保留。确认稳定后运行：

```bash
KEEP=2 bash deploy/prune-docker.sh
```

> `deploy/deploy.sh` 仍是 New API（前端 + Go）部署入口；只更新 CLIProxyAPI 时不要运行它。
> `deploy/docker-compose.cliproxy.yml` 是退役静态拓扑的历史/破玻璃参考，常规升级严禁
> `docker compose up`。一键部署也会在发现静态 `cli-proxy-api`/`cli-proxy-api-k12`
> 容器时 fail closed，防止抢占 8317/8318。

### Anthropic 兼容升级说明（2026-07-24）

New API 启动时会自动 reconcile `cliproxy-pool` / `cliproxy-pool-*` 存量渠道：原地升级为
Advanced Custom 并补齐 Anthropic / OpenAI 六条原样路由和 Claude Code Header 白名单。
升级保留渠道 ID、Group、Key、BaseURL、Models、启停状态及已有额外 Advanced Custom 路由；
重复启动不会产生第二渠道。日志只输出渠道 ID 与成功/失败数，不输出号池内部 Key。

上线前照常备份 `one-api.db`；启动后可在管理员渠道页确认同一渠道 ID 的类型已变为
Advanced Custom，再分别用 `/v1/messages/count_tokens`、`/v1/messages` 和 `/v1/responses`
做冒烟测试。用户端 Claude Endpoint 必须填写 `https://api.selab.top`（不带 `/v1`），
不要让用户直连 `codex.selab.top`。

**回滚** = 用上一版镜像 tag 重跑 `IMAGE=winbeau/xju-newapi:<旧tag> bash deploy/run-newapi.sh`(旧镜像仍在本机;数据在宿主 volume 不受影响)。升级前记下当前 tag。

## 统一动态池（2026-07-16 迁移完成）

原来 default / k12 是 **env 播种的静态池**(compose 起 `cli-proxy-api` / `cli-proxy-api-k12`,new-api 靠 `POOL_MGMT_SECRET` / `POOL_K12_MGMT_SECRET` 识别)。现已并入**动态池**统一管理:

| 池 id | 前端 label | 承载组(卡路由) | 容器 / 端口 | 号数(迁移时) |
|---|---|---|---|---|
| `main` | Default | `default` | `cli-proxy-api-main` / 8317 | 5(3 停用/2 活) |
| `k12-pool` | K12 | `k12` | `cli-proxy-api-k12-pool` / 8318 | 501(**全部 disabled**——源文件即如此,非迁移所致) |

- **迁移动作**(已完成,勿重复):停旧静态容器 → provision watcher 建 `main`/`k12-pool` 动态池(端口沿用 8317/8318)→ `cp -a` 旧 `auths/`→`auths-main/`、`auths-k12/`→`auths-k12-pool/`(号原样搬,disabled 位保真)→ 新 channel 3/4 建成后**改回 `default`/`k12` 组**(存量卡不用换发)→ 停旧 channel 1/2 → 删 GroupRatio/UserUsableGroups 里多余的 `main`/`k12-pool` 组 → 清空 env 密钥隐藏静态池 → 删旧静态容器。
- **channel 1 必须留(即便 disabled)**:`createPoolChannel` 克隆 channel id 1 的 models 当模板,删了则一键开新池会报 "cannot read primary channel"。
- **回滚(破玻璃)**:旧号在 `auths/` + `auths-k12/` 原样保留;旧 config(`config.yaml`/`config.k12.yaml`)、旧密钥(`.pool-mgmt.env.retired` / `.pool-mgmt-k12.env.retired`)、旧镜像 `winbeau/cli-proxy-api:v0.8.6` 均在位。正常版本回滚优先运行 `deploy/deploy-cliproxy.sh --rollback`;只有该脚本及动态备份都不可用时,才停 `main`/`k12-pool` → 把 `.retired` 改回 → 参照 git 历史中的退役 compose(或等价 docker run)起旧静态容器 → 前端 channel 3/4 停、1/2 启 → 重跑 `run-newapi.sh`(注入回旧密钥)。
- **k12 池 501 号全 disabled** 是迁移前就有的存量状态(旧 k12 channel 同样不出模型),不是本次迁移引入;要激活需对号做验活/enriched 重登。

## Codex 账号登录（claude-tri）

从 WSL 一键发起目标池的 enriched OAuth 登录：

```bash
./scripts/login-codex-via-tri.sh main
# 其他动态池：./scripts/login-codex-via-tri.sh <pool-id>
```

脚本在 claude-tri 发起 OAuth、保存 Token、确认落盘并做 405 轻量验活；WSL 只负责
`1455 + 池端口` 两条 SSH 转发并打开 OpenAI 官方页面。密码/MFA 只在 OpenAI 页面输入，
不进入脚本。启动时会自动清理端口与转发方向相符的旧 WSL SSH 隧道，不会终止其他监听
进程。完整实测链路、WSL mirrored networking 注意事项与安全边界见
[docs/codex-pool-login.md](./codex-pool-login.md)。

## Codex Responses WebSocket

日卡用户继续使用 L1 地址 `https://api.selab.top/v1`。Codex 配置须包含：

```toml
model_provider = "OpenAI"

[model_providers.OpenAI]
name = "OpenAI"
base_url = "https://api.selab.top/v1"
wire_api = "responses"
requires_openai_auth = true
supports_websockets = true
```

请求链路为：Codex `GET /v1/responses` Upgrade → new-api 校验日卡 → 首个
`response.create` 读取模型并按 token group 选池 → 对所选 CLIProxyAPI 渠道发起
上游 Upgrade。连接内只允许一个 in-flight response；每个 turn 都重新校验令牌，
并走与 HTTP Responses 相同的模型映射、字段过滤、预扣费、usage 结算和日志。
`generate=false` 预热会退回预扣费，不产生零 token 消费日志。

Caddy `reverse_proxy` 原生支持 WebSocket，无需 Nginx 风格的显式
`Upgrade`/`Connection` 配置。上线后用一把有效日卡做短时握手检查：

```bash
curl --http1.1 -i -N --max-time 3 \
  -H "Authorization: Bearer __DAY_CARD_TOKEN__" \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  https://api.selab.top/v1/responses
```

首行应为 `HTTP/1.1 101 Switching Protocols`；随后连接因测试命令的 3 秒上限
退出是正常的。若返回 401，先查日卡；若握手后收到
`do_request_failed`，说明选中的渠道上游没有实现 Responses WebSocket，或
CLIProxyAPI 容器仍是旧镜像。临时回退可设 `supports_websockets = false`。

## 双池密钥（default + K12）

两把**独立**的明文管理密钥,同机同目录、互不通用,均被 `.gitignore` 挡、600 权限:

| 池 | env 文件(tri) | 注入谁 | 变量 |
|---|---|---|---|
| default | `/opt/cli-proxy-api/.pool-mgmt.env` | `cli-proxy-api` 容器 + new-api | `MANAGEMENT_PASSWORD` 与 `POOL_MGMT_SECRET`(同值) |
| K12 | `/opt/cli-proxy-api/.pool-mgmt-k12.env` | `cli-proxy-api-k12` 容器 + new-api | `MANAGEMENT_PASSWORD` 与 `POOL_K12_MGMT_SECRET`(同值) |

- **生成**:[deploy/setup-pool-mgmt.sh](../deploy/setup-pool-mgmt.sh) / [setup-pool-mgmt-k12.sh](../deploy/setup-pool-mgmt-k12.sh)。幂等——已存在非空文件不覆盖。
- **轮换(退役静态池破玻璃路径)**:`bash deploy/setup-pool-mgmt.sh --force`(K12 同理)→ 按旧静态容器参数重建对应 CLIProxyAPI → 重跑 `deploy/run-newapi.sh`(重注入 new-api 侧密钥)。现役动态池使用各自 `.pool-mgmt-<id>.env`,不走 compose。
- **为什么走明文 env**:`config.yaml` 里的 `secret-key` 是 bcrypt 哈希,不能当 Bearer 用;`MANAGEMENT_PASSWORD` 走 ConstantTimeCompare,且会自动解除 `allow-remote:false` 让 new-api 从 docker 内网调管理 API。
- **xju-net 互访契约**:管理 API 只在 docker 内网,new-api 用容器名访问——`http://cli-proxy-api:8317` / `http://cli-proxy-api-k12:8318`(env:`POOL_MGMT_URL` / `POOL_K12_MGMT_URL`)。某池密钥留空 = 该池端点 503、前端自动隐藏该池 Tab,其余部署不受影响。

## 新布局部署（2026-07 顶层重组迁移,一次性）

> 仓库已重组:前端上移 `web/`、Go 后端 `server/newapi/`、CLIProxyAPI `server/cliproxy/`;
> `new-api.run.sh→run-newapi.sh`、`cli-proxy.docker-compose.yml→docker-compose.cliproxy.yml`、
> 发卡脚本 snake→kebab;全量 `Dockerfile.newapi` 与 `cli-proxy-api.service` 已删除。

tri 上迁移步骤:

1. **备份先行**:`bash deploy/backup.sh`。
2. **更新仓库**:`cd /home/winbeau/opt/xju-api && git pull --ff-only origin main`(git 自动应用 rename;或干脆删掉重 clone——仓库无状态,数据都在 `/opt` 宿主卷)。
3. **prebuilt 新路径**:旧 `new-api/prebuilt/{default-dist,classic-dist}` 已作废;先保留到新镜像验活完成,再单独归档/清理。本机发布物经安装器落到 `server/newapi/prebuilt/current/dist`(单产物),tri 不构建前端。
4. **引用检查**:backup cron 走 `deploy/backup.sh` 相对仓库路径未变;旧的 `docker compose -f` 命令/别名应删除,CLIProxyAPI 统一改用 `deploy/deploy-cliproxy.sh`。`deploy/docker-compose.cliproxy.yml` 只作退役静态拓扑的历史/破玻璃参考。
5. **完整部署**:严格按本页「New API 标准发布链路」先传并安装前端发布物,再运行 `PULL=0 SKIP_WEB=1 bash deploy/deploy.sh <tag>`。
6. **回滚**:布局回滚 = `git checkout d02c62c`(重组前最后一个 commit,旧脚本名照旧用)+ 旧镜像 tag 重跑;数据不涉及。

## 号池一键开池 host helper(#4 Phase B,一次性安装)

前端「新建号池」→ new-api(容器内,**不碰 docker socket**)写开通请求 → 宿主 watcher 接单起独立 cliproxy 实例。安全边界:new-api 只读写共享目录,docker 操作全在宿主 watcher(以有 docker 权限的 winbeau 跑)。

**安装(在 claude-tri 上,一次性)**:
```bash
# 1) 共享目录(watcher 属主,new-api 容器 root 写请求进来它也能 mv)
sudo install -d -o winbeau -g winbeau /opt/xju-api/provision/{requests,results,processed}
# 2) 构建当前 commit 镜像、初始化 desired image 并安装 systemd unit
short_sha="$(git rev-parse --short=7 HEAD)"
bash deploy/build-cliproxy.sh "deploy-$short_sha"
printf 'CLIPROXY_IMAGE=winbeau/cli-proxy-api:deploy-%s\nCLIPROXY_ROLLBACK_IMAGE=\n' "$short_sha" \
  > /opt/cli-proxy-api/.cliproxy-image.env
chmod 600 /opt/cli-proxy-api/.cliproxy-image.env
sudo install -m 0644 /home/winbeau/opt/xju-api/deploy/xju-provision.service \
  /etc/systemd/system/xju-provision.service
sudo systemctl daemon-reload && sudo systemctl enable --now xju-provision.service
systemctl status xju-provision.service          # 应 active (running)
# 3) new-api 容器要挂 /provision(run-newapi.sh 已含 -v /opt/xju-api/provision:/provision
#    + POOL_PROVISION_DIR=/provision),重跑一次换上即可
IMAGE=winbeau/xju-newapi:<tag> bash deploy/run-newapi.sh
```

**契约**:请求 `provision/requests/<id>.json`(new-api 写,644,无密钥)→ watcher 起 `cli-proxy-api-<id>` 容器(新端口、接 xju-net、config 克隆自 `config.k12.example.yaml`)→ 结果 `provision/results/<id>.json`(watcher 写,600,含 mgmt_secret/internal_key)→ new-api 轮询后写动态注册表 `/opt/new-api/data/xju-pools.json`。

**排障**:`journalctl -u xju-provision -f` 看 watcher 日志;`docker ps | grep cli-proxy-api-` 看新实例;`docker logs cli-proxy-api-<id>` 看 cliproxy 起没起。开通卡在 provisioning:多半 watcher 没跑或 xju-net/端口冲突。**删池**:前端删或写 `{"action":"delete","pool_id":"<id>"}` 到 requests/(停容器+删 config/env,保留 auths-<id>/ 号不丢)。

- **区域代理(可选)**:若池要做 enriched 登录/在非受支持区域跑,给该池 live `config.<id>.yaml` 填
  `proxy-url: "socks5://…"`(模板 `config.example.yaml`/`config.k12.example.yaml` 已留注释占位),重建容器生效。

## 维护清理(定期在 tri 跑,腾磁盘)

> 原则:上线部署尽管供应资源;维护时清掉重复构建的垃圾。tri/vps 磁盘紧的主因是 docker
> 重复构建的旧镜像 / dangling 层 / build cache。

```bash
# 安全:只清 dangling + 超"当前+回滚"的旧 tag + 超量 build cache;运行中镜像与回滚锚不动
bash /home/winbeau/opt/xju-api/deploy/prune-docker.sh
# 临时调参:KEEP=3 CACHE_KEEP=5GB bash deploy/prune-docker.sh
docker system df    # 看回收效果
```
- 升级后新 tag verify 通过即可跑一次,回收被取代的旧构建。CLIProxyAPI 的一键部署默认不清理;确认稳定后显式运行 `KEEP=2 bash deploy/prune-docker.sh`。
- 前端只在 Codex-vps 构建;tri 对 New API 和 CLIProxyAPI 都只做 Go-only 镜像构建。CLIProxyAPI 镜像 tag 为 `deploy-<提交SHA>`;重复构建后可运行 `bash deploy/prune-docker.sh` 回收旧镜像与 build cache。
- `docker system df` 若因 containerd 遗留的缺失 snapshot 报错,清理脚本会记录警告并让总部署继续做 API/服务验活;按错误中的容器 ID 定位后再单独清理。

## 备份 / 恢复

- 备份：[deploy/backup.sh](../deploy/backup.sh)，cron 每日 04:30，滚动保 7 份于
  `/opt/backups/xju-api/`。当前版本会备份 SQLite、`xju-pools.json`、全部
  `config.<id>.yaml`、`.pool-mgmt-<id>.env`、`auths-<id>/`、
  `.cliproxy-image.env` 及不含密钥的 fleet manifest；默认不备份池日志。
- CLIProxy 部署器在 watcher 停止并取得 provision lock 后运行
  `BACKUP_CADDY=0 bash deploy/backup.sh`，不读取 Caddy 私有目录；定时完整备份仍默认包含
  Caddy。
- 恢复 New API：停容器 → 用备份的 `one-api.db` 覆盖
  `/opt/new-api/data/one-api.db` → 起容器。
- 恢复动态 CLIProxy 数据：先停 `xju-provision` 并建立 maintenance，恢复 `xju-pools.json` 与
  `cli-proxy-dynamic.tar.gz`，确认 `.cliproxy-image.env` 指向已存在的镜像，再按备份中的
  `cliproxy-fleet.jsonl` 重建丢失的容器。若现役容器仍完整，只需运行
  `PULL=0 BACKUP=0 bash deploy/deploy-cliproxy.sh --rollback <镜像>` 做整批版本回滚。
  `deploy-cliproxy.sh` 出于安全考虑不会凭空重建一个完全丢失的 fleet。**不要**使用退役
  compose 恢复动态池。
- 恢复 Caddy：解包 `caddy.tar.gz` → `systemctl reload caddy`（证书目录一并恢复可免重签）。

## 排障速查

| 症状 | 先查 | 常见原因 |
|---|---|---|
| 登录后立即掉登录 / 登录不上 | 容器 env | `SESSION_COOKIE_SECURE` / `SESSION_COOKIE_TRUSTED_URL=https://api.selab.top` 未设或不匹配（PLAN.md §8-7）；`SESSION_SECRET` 变了会全员失效 |
| 证书签不下来 | `journalctl -u caddy` | Cloudflare 橙云拦 ACME 挑战 → 先切「仅 DNS/灰云」（PLAN.md §9-6）；80/443 未放行 |
| 用户请求 401 | 令牌状态/到期 | 日卡到期即时 401 属正常；复活走 `scripts/renew-card.sh`（两步，见 docs/daycard-api.md ②） |
| 渠道测试失败 | new-api 渠道配置 | Base URL 应为 `http://127.0.0.1:8317`，Key= CLIProxyAPI `config.yaml` 的 `api-keys` 之一 |
| 上游全部报错 | `docker logs cli-proxy-api-main` | 号池凭证过期 → 重新 OAuth（临时开回调口走 SSH 隧道，PLAN.md §8-2）；配额耗尽等冷却 |
| 机器变慢 / OOM | `free -h`、`docker stats` | 本机内存只有 3.8Gi 且多项目共用 —— 不要再起新容器 |
| 磁盘告警 | `df -h`、`docker system df` | 日志/旧镜像膨胀：`docker image prune`、查三处日志滚动是否生效（剩 ~11G 是最大风险，PLAN.md §9-4） |

## 部署实测踩坑（2026-07-13 首次上线，全部已验证）

> 这些是 PLAN.md 规划时未预见、在真实部署中撞到的，**照做可避免重复踩**。

| # | 坑 | 现象 | 正解 |
|---|---|---|---|
| 1 | **容器间回环不通** | 渠道 Base URL 填 `http://127.0.0.1:8317`，请求报 `upstream error: do request failed` | new-api 在容器内，`127.0.0.1` 是它自己的回环；CLIProxyAPI 的 8317 只发布在**宿主**回环上。两容器接入同一网络 `xju-net`，Base URL 改用**容器名** `http://cli-proxy-api:8317` |
| 2 | **不再有 root/123456** | 用 `root/123456` 登录返回「用户名或密码错误」；日志显示 `system is not initialized and no root user exists` | 走初始化向导 `POST /api/setup {username,password,confirmPassword}`，**一步到位设强密码** |
| 3 | **建渠道 payload 要包信封** | `POST /api/channel/` 平铺字段 → 服务端 **panic**（nil 指针，`validateChannel` 在 nil 判断前解引用） | 必须包一层：`{"mode":"single","channel":{...}}` |
| 4 | **改渠道不能带 `status`** | `PUT /api/channel/` 返回 `Invalid parameters` | `controller/channel.go:931` 显式拒绝含 `status` 的请求（status 有独立端点）。用最小 patch：`{"id":1,"base_url":"...","key":"..."}` |
| 5 | **读回渠道时 key 被屏蔽** | 读回改一改再 PUT，会把密钥**擦成空** | `GET /api/channel/:id` 返回 `"key":""`。PUT 时必须**显式补回真实 key** |
| 6 | **模型未配价直接拒绝请求** | 报「模型 xxx 的价格未配置」，请求 400 | 开 `SelfUseModeEnabled=true`（`PUT /api/option/`）。实测**不影响记账**：`logs` 里 prompt/completion tokens 和 quota 依然全额记录 |

## 硬约束提醒

- 端口/防火墙一律**增量**操作，严禁 `ufw reset` / 无脑 `ufw enable`（多项目共用机，PLAN.md §3.1）。
- OAuth 回调口（1455/54545/51121/8085/11451）常驻期保持注释，仅登录号池时临时开。
- 真实 `config.yaml` / `auths/` / `.env` 永不入库；仓库里只有 `*.example.*`。
