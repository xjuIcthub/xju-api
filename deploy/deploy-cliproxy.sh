#!/usr/bin/env bash
# deploy/deploy-cliproxy.sh — claude-tri 上一键构建、升级和回滚动态 CLIProxyAPI 池。
#
# 默认镜像: winbeau/cli-proxy-api:deploy-<当前 main 的 7 位提交 SHA>
# 本脚本只构建 Go/CLIProxyAPI，不构建前端，也不替换 new-api。
set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT_DIR="$REPO_ROOT/deploy"
BRANCH="${BRANCH:-main}"
PULL="${PULL:-1}"
PRUNE="${PRUNE:-0}"
BACKUP="${BACKUP:-1}"
HEALTH_RETRIES="${HEALTH_RETRIES:-30}"
HEALTH_INTERVAL="${HEALTH_INTERVAL:-2}"
CLIPROXY_DIR="${CLIPROXY_DIR:-/opt/cli-proxy-api}"
PROVISION_DIR="${PROVISION_DIR:-/opt/xju-api/provision}"
POOL_REGISTRY_FILE="${POOL_REGISTRY_FILE:-/opt/new-api/data/xju-pools.json}"
IMAGE_ENV_FILE="${CLIPROXY_IMAGE_ENV_FILE:-$CLIPROXY_DIR/.cliproxy-image.env}"
DEPLOY_LOCK="${CLIPROXY_DEPLOY_LOCK:-$CLIPROXY_DIR/.deploy.lock}"
MAINTENANCE_FILE="${POOL_MAINTENANCE_FILE:-$PROVISION_DIR/.maintenance}"
PROVISION_LOCK="${POOL_PROVISION_LOCK:-$PROVISION_DIR/.provision.lock}"
DRY_RUN=0
ROLLBACK_MODE=0
ROLLBACK_TARGET=""
DEPLOY_ARGS=()

# shellcheck source=deploy/cliproxy-pool-runtime.sh
source "$SCRIPT_DIR/cliproxy-pool-runtime.sh"

usage() {
	cat <<'EOF'
用法:
  bash deploy/deploy-cliproxy.sh
  bash deploy/deploy-cliproxy.sh --dry-run
  bash deploy/deploy-cliproxy.sh --rollback
  bash deploy/deploy-cliproxy.sh --rollback winbeau/cli-proxy-api:deploy-123abcd

默认流程:
  fast-forward origin/main → 护栏 → 构建 deploy-<当前提交短SHA>
  → 动态池备份 → canary/其余池/main 顺序升级 → 失败自动整批回滚
  → 固化未来新池镜像 → 重启 provision watcher → 全量验活。

环境变量:
  PULL=0       跳过 git fetch/merge
  BACKUP=0     跳过部署前动态备份
  PRUNE=1      成功后清理旧镜像/构建缓存（默认保留回滚镜像）
  BRANCH=main  要部署的远端分支
EOF
}

while (($# > 0)); do
	case "$1" in
	--dry-run)
		DRY_RUN=1
		DEPLOY_ARGS+=(--dry-run)
		shift
		;;
	--rollback)
		ROLLBACK_MODE=1
		DEPLOY_ARGS+=(--rollback)
		if (($# > 1)) && [[ "$2" != --* ]]; then
			ROLLBACK_TARGET="$2"
			DEPLOY_ARGS+=("$2")
			shift
		fi
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "未知参数: $1" >&2
		usage >&2
		exit 2
		;;
	esac
done

require_command() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "缺少命令: $1" >&2
		exit 1
	}
}

is_commit_image() {
	[[ "$1" =~ ^winbeau/cli-proxy-api:deploy-[0-9a-f]{7}$ ]]
}

is_cliproxy_image() {
	[[ "$1" =~ ^winbeau/cli-proxy-api:[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]
}

target_commit_for_image() {
	local image="$1"
	if is_commit_image "$image"; then
		printf '%s\n' "${image##*-}"
		return
	fi
	# Legacy semantic images embed their actual Git commit independently of Version.
	printf '\n'
}

read_image_env() {
	CURRENT_DESIRED_IMAGE=""
	STORED_ROLLBACK_IMAGE=""
	if [[ -f "$IMAGE_ENV_FILE" ]]; then
		CURRENT_DESIRED_IMAGE="$(sed -n 's/^CLIPROXY_IMAGE=//p' "$IMAGE_ENV_FILE" | tail -n 1)"
		STORED_ROLLBACK_IMAGE="$(sed -n 's/^CLIPROXY_ROLLBACK_IMAGE=//p' "$IMAGE_ENV_FILE" | tail -n 1)"
	fi
}

write_image_env() {
	local current="$1" rollback="$2" temp
	temp="$IMAGE_ENV_FILE.tmp.$$"
	umask 077
	printf 'CLIPROXY_IMAGE=%s\nCLIPROXY_ROLLBACK_IMAGE=%s\n' "$current" "$rollback" >"$temp"
	chmod 600 "$temp"
	mv "$temp" "$IMAGE_ENV_FILE"
}

registry_json() {
	if [[ -r "$POOL_REGISTRY_FILE" ]]; then
		cat "$POOL_REGISTRY_FILE"
		return
	fi
	if docker inspect new-api --format '{{.State.Running}}' 2>/dev/null | grep -Fx true >/dev/null; then
		docker exec new-api cat /data/xju-pools.json
		return
	fi
	echo "dynamic pool registry is not readable: $POOL_REGISTRY_FILE" >&2
	return 1
}

registry_entry() {
	local id="$1" entry
	entry="$(registry_json | jq -c --arg id "$id" '.[] | select(.id == $id)' | head -n 1)"
	[[ -n "$entry" ]] || return 1
	printf '%s\n' "$entry"
}

pool_kind() {
	local id="$1" entry owner kind
	case "$id" in
	main | k12-pool)
		printf 'admin\n'
		return
		;;
	esac
	entry="$(registry_entry "$id")" || {
		echo "运行中的池不在 registry 中: $id" >&2
		return 1
	}
	owner="$(jq -r '.owner_user_id // 0' <<<"$entry")"
	kind="$(jq -r '.kind // ""' <<<"$entry")"
	if [[ "$kind" == private || "$kind" == "" && "$owner" =~ ^[0-9]+$ && "$owner" -gt 0 ]]; then
		printf 'private\n'
	elif [[ "$kind" == admin || -z "$kind" ]]; then
		printf 'admin\n'
	else
		echo "不支持的池类型: $id kind=$kind" >&2
		return 1
	fi
}

validate_pool_contract() {
	local id="$1" port="$2" kind="$3" expected_image="${4:-}" expected_image_id="${5:-}"
	local name inspect config_port registry expected_port
	name="$(cliproxy_container_name "$id")"
	inspect="$(docker inspect "$name")"
	config_port="$(cliproxy_pool_port_from_config "$CLIPROXY_DIR/config.$id.yaml")"
	[[ "$config_port" == "$port" ]] || {
		echo "$name 配置端口 $config_port 与部署端口 $port 不一致" >&2
		return 1
	}
	if registry="$(registry_entry "$id" 2>/dev/null)"; then
		expected_port="$(jq -r '.port // 0' <<<"$registry")"
		if [[ "$expected_port" != 0 && "$expected_port" != "$port" ]]; then
			echo "$name registry 端口 $expected_port 与配置端口 $port 不一致" >&2
			return 1
		fi
	fi

	jq -e --arg network "${XJU_NET:-xju-net}" --arg port "$port" \
		--arg config "$CLIPROXY_DIR/config.$id.yaml" \
		--arg auths "$CLIPROXY_DIR/auths-$id" \
		--arg logs "$CLIPROXY_DIR/logs-$id" '
		.[0] as $c |
		$c.State.Running == true and
		$c.HostConfig.RestartPolicy.Name == "unless-stopped" and
		($c.NetworkSettings.Networks | has($network)) and
		($c.HostConfig.PortBindings[$port + "/tcp"] | length == 1) and
		$c.HostConfig.PortBindings[$port + "/tcp"][0].HostIp == "127.0.0.1" and
		$c.HostConfig.PortBindings[$port + "/tcp"][0].HostPort == $port and
		([$c.Mounts[] | select(.Source == $config and .Destination == "/CLIProxyAPI/config.yaml")] | length == 1) and
		([$c.Mounts[] | select(.Source == $auths and .Destination == "/root/.cli-proxy-api")] | length == 1) and
		([$c.Mounts[] | select(.Source == $logs and .Destination == "/CLIProxyAPI/logs")] | length == 1)
	' <<<"$inspect" >/dev/null || {
		echo "$name 运行契约校验失败" >&2
		return 1
	}
	if [[ "$kind" == private ]]; then
		jq -e '.[0].HostConfig.Memory == 268435456 and
			.[0].HostConfig.MemoryReservation == 67108864 and
			.[0].HostConfig.NanoCpus == 750000000 and
			.[0].HostConfig.PidsLimit == 128 and
			.[0].HostConfig.LogConfig.Type == "json-file" and
			.[0].HostConfig.LogConfig.Config["max-size"] == "10m" and
			.[0].HostConfig.LogConfig.Config["max-file"] == "2"' <<<"$inspect" >/dev/null || {
			echo "$name 私有池资源限制校验失败" >&2
			return 1
		}
	fi
	if [[ -n "$expected_image" ]]; then
		[[ "$(jq -r '.[0].Config.Image' <<<"$inspect")" == "$expected_image" ]] || {
			echo "$name 未运行目标镜像 $expected_image" >&2
			return 1
		}
	fi
	if [[ -n "$expected_image_id" ]]; then
		[[ "$(jq -r '.[0].Image' <<<"$inspect")" == "$expected_image_id" ]] || {
			echo "$name 的 image ID 与 $expected_image 不一致" >&2
			return 1
		}
	fi
}

discover_fleet() {
	mapfile -t ALL_POOL_CONTAINERS < <(docker ps -a --format '{{.Names}}' | grep '^cli-proxy-api-' | sort -V || true)
	mapfile -t RUNNING_POOL_CONTAINERS < <(docker ps --format '{{.Names}}' | grep '^cli-proxy-api-' | sort -V || true)
	((${#RUNNING_POOL_CONTAINERS[@]} > 0)) || {
		echo "没有运行中的动态 CLIProxyAPI 池" >&2
		return 1
	}
	if docker ps -a --format '{{.Names}}' | grep -Eq '^(cli-proxy-api|cli-proxy-api-k12)$'; then
		echo "检测到退役静态 CLIProxyAPI 容器,拒绝部署" >&2
		return 1
	fi
	if ((${#ALL_POOL_CONTAINERS[@]} != ${#RUNNING_POOL_CONTAINERS[@]})); then
		echo "存在 stopped 或 rollback CLIProxyAPI 容器,请先人工处理:" >&2
		comm -23 <(printf '%s\n' "${ALL_POOL_CONTAINERS[@]}") <(printf '%s\n' "${RUNNING_POOL_CONTAINERS[@]}") >&2
		return 1
	fi

	POOL_IDS=()
	declare -gA POOL_PORTS=() POOL_KINDS=() POOL_PREVIOUS_IMAGES=() POOL_ANCHORS=()
	local container id port kind image
	for container in "${RUNNING_POOL_CONTAINERS[@]}"; do
		id="${container#cli-proxy-api-}"
		cliproxy_require_pool_runtime "$id"
		port="$(cliproxy_pool_port_from_config "$CLIPROXY_DIR/config.$id.yaml")"
		kind="$(pool_kind "$id")"
		image="$(docker inspect "$container" --format '{{.Config.Image}}')"
		POOL_IDS+=("$id")
		POOL_PORTS["$id"]="$port"
		POOL_KINDS["$id"]="$kind"
		POOL_PREVIOUS_IMAGES["$id"]="$image"
		validate_pool_contract "$id" "$port" "$kind" "$image"
		cliproxy_wait_for_health "$container" "$port" || {
			echo "部署前健康检查失败: $container" >&2
			return 1
		}
	done

	mapfile -t current_images < <(printf '%s\n' "${POOL_PREVIOUS_IMAGES[@]}" | sort -u)
	if ((${#current_images[@]} != 1)); then
		echo "现役池镜像不一致,拒绝整批部署:" >&2
		printf '  %s\n' "${current_images[@]}" >&2
		return 1
	fi
	PREVIOUS_FLEET_IMAGE="${current_images[0]}"
	if [[ -n "${EXPECTED_PREVIOUS_FLEET_IMAGE:-}" && "$PREVIOUS_FLEET_IMAGE" != "$EXPECTED_PREVIOUS_FLEET_IMAGE" ]]; then
		echo "maintenance 前后 fleet 镜像发生变化: $EXPECTED_PREVIOUS_FLEET_IMAGE -> $PREVIOUS_FLEET_IMAGE" >&2
		return 1
	fi

	ROLLOUT_IDS=()
	for id in "${POOL_IDS[@]}"; do
		[[ "$id" == main ]] || ROLLOUT_IDS+=("$id")
	done
	for id in "${POOL_IDS[@]}"; do
		[[ "$id" == main ]] && ROLLOUT_IDS+=("$id")
	done
}

print_plan() {
	echo "==> 部署提交: $(git log -1 --pretty='%h %s')"
	echo "==> 目标镜像: $TARGET_IMAGE"
	echo "==> 当前镜像: $PREVIOUS_FLEET_IMAGE"
	if ((${#ROLLOUT_IDS[@]} > 0)); then
		if [[ "${ROLLOUT_IDS[0]}" == main ]]; then
			echo "==> 升级顺序（仅 main）: main"
		else
			echo "==> 升级顺序（首池 canary, main 最后）: ${ROLLOUT_IDS[*]}"
		fi
	fi
	local id
	for id in "${ROLLOUT_IDS[@]}"; do
		printf '  %-16s port=%-5s kind=%s\n' "$id" "${POOL_PORTS[$id]}" "${POOL_KINDS[$id]}"
	done
}

install_provision_unit() {
	sudo install -m 0644 "$SCRIPT_DIR/xju-provision.service" /etc/systemd/system/xju-provision.service
	sudo systemctl daemon-reload
}

verify_watcher_image() {
	local pid
	sudo systemctl is-active --quiet xju-provision
	pid="$(systemctl show xju-provision --property MainPID --value)"
	[[ "$pid" =~ ^[0-9]+$ && "$pid" -gt 0 ]] || return 1
	sudo tr '\0' '\n' <"/proc/$pid/environ" | grep -Fx "CLIPROXY_IMAGE=$TARGET_IMAGE" >/dev/null
}

rollback_changed_pools() {
	local failed=0 index id name anchor
	if [[ -n "${CURRENT_POOL_ID:-}" ]]; then
		name="$(cliproxy_container_name "$CURRENT_POOL_ID")"
		anchor="${POOL_ANCHORS[$CURRENT_POOL_ID]:-}"
		if [[ -z "$anchor" ]] || ! docker container inspect "$anchor" >/dev/null 2>&1; then
			echo "缺少 rollback anchor: ${anchor:-$CURRENT_POOL_ID}" >&2
			failed=1
		else
			docker rm -f "$name" >/dev/null 2>&1 || true
			docker rename "$anchor" "$name" || failed=1
			docker start "$name" >/dev/null || failed=1
			cliproxy_wait_for_health "$name" "${POOL_PORTS[$CURRENT_POOL_ID]}" || failed=1
		fi
	fi
	for ((index = ${#UPGRADED_IDS[@]} - 1; index >= 0; index--)); do
		id="${UPGRADED_IDS[$index]}"
		[[ "$id" == "${CURRENT_POOL_ID:-}" ]] && continue
		name="$(cliproxy_container_name "$id")"
		anchor="${POOL_ANCHORS[$id]}"
		if ! docker container inspect "$anchor" >/dev/null 2>&1; then
			echo "缺少 rollback anchor: $anchor" >&2
			failed=1
			continue
		fi
		docker rm -f "$name" >/dev/null 2>&1 || true
		docker rename "$anchor" "$name" || failed=1
		docker start "$name" >/dev/null || failed=1
		cliproxy_wait_for_health "$name" "${POOL_PORTS[$id]}" || failed=1
	done
	return "$failed"
}

FAILURE_HANDLED=0
MAINTENANCE_ACTIVE=0
WATCHER_STOPPED=0
CURRENT_POOL_ID=""
UPGRADED_IDS=()
ORIGINAL_DESIRED_IMAGE=""
ORIGINAL_ROLLBACK_IMAGE=""
ENV_MIGRATED=0

restore_watcher_configuration() {
	local desired rollback
	desired="${ORIGINAL_DESIRED_IMAGE:-${PREVIOUS_FLEET_IMAGE:-}}"
	rollback="${ORIGINAL_ROLLBACK_IMAGE:-}"
	if [[ -n "$desired" ]]; then
		write_image_env "$desired" "$rollback" || return 1
	elif ((ENV_MIGRATED == 1)); then
		rm -f "$IMAGE_ENV_FILE"
	fi
	install_provision_unit || return 1
	sudo systemctl start xju-provision || return 1
}

handle_failure() {
	local rc="$1"
	trap - ERR INT TERM
	((FAILURE_HANDLED == 0)) || exit "$rc"
	FAILURE_HANDLED=1
	if ((MAINTENANCE_ACTIVE == 0)); then
		exit "$rc"
	fi
	echo "==> CLIProxyAPI 部署失败,开始恢复" >&2
	sudo systemctl stop xju-provision >/dev/null 2>&1 || true
	WATCHER_STOPPED=1
	if rollback_changed_pools; then
		CURRENT_POOL_ID=""
		if restore_watcher_configuration && sudo systemctl is-active --quiet xju-provision; then
			WATCHER_STOPPED=0
			rm -f "$MAINTENANCE_FILE"
			MAINTENANCE_ACTIVE=0
			echo "==> 已恢复部署前池与 provision watcher" >&2
		else
			echo "==> 池已恢复,但 watcher 未能启动;maintenance 保留" >&2
		fi
	else
		echo "==> 自动回滚未完整成功;maintenance 保留,watcher 保持停止,请立即人工处理" >&2
	fi
	exit "$rc"
}
trap 'handle_failure $?' ERR
trap 'handle_failure 130' INT TERM

for command in git docker curl jq flock systemctl sudo sed; do
	require_command "$command"
done

cd "$REPO_ROOT"
if ! git diff --quiet || ! git diff --cached --quiet; then
	echo "tracked 工作区存在本地修改,停止部署:" >&2
	git status --short --untracked-files=no >&2
	exit 1
fi
if [[ -n "$(git status --porcelain --untracked-files=all -- server/cliproxy)" ]]; then
	echo "server/cliproxy 存在未提交文件,不能用 HEAD commit tag 构建:" >&2
	git status --short --untracked-files=all -- server/cliproxy >&2
	exit 1
fi
if [[ "$PULL" == 1 && "${XJU_CLIPROXY_DEPLOY_AFTER_PULL:-0}" != 1 ]]; then
	CURRENT_BRANCH="$(git branch --show-current)"
	if [[ "$CURRENT_BRANCH" != "$BRANCH" ]]; then
		echo "当前分支是 $CURRENT_BRANCH,目标分支是 $BRANCH" >&2
		exit 1
	fi
	git fetch --prune origin "$BRANCH"
	git merge --ff-only "origin/$BRANCH"
	exec env XJU_CLIPROXY_DEPLOY_AFTER_PULL=1 PULL="$PULL" PRUNE="$PRUNE" \
		BACKUP="$BACKUP" BRANCH="$BRANCH" HEALTH_RETRIES="$HEALTH_RETRIES" \
		HEALTH_INTERVAL="$HEALTH_INTERVAL" CLIPROXY_DIR="$CLIPROXY_DIR" \
		PROVISION_DIR="$PROVISION_DIR" POOL_REGISTRY_FILE="$POOL_REGISTRY_FILE" \
		CLIPROXY_IMAGE_ENV_FILE="$IMAGE_ENV_FILE" bash "$SCRIPT_DIR/deploy-cliproxy.sh" \
		"${DEPLOY_ARGS[@]}"
fi

mkdir -p "$CLIPROXY_DIR" "$PROVISION_DIR"
exec {deploy_fd}>"$DEPLOY_LOCK"
if ! flock -n "$deploy_fd"; then
	echo "另一个 CLIProxyAPI 部署正在运行" >&2
	exit 1
fi

bash "$REPO_ROOT/scripts/check-guardrails.sh"
docker network inspect "${XJU_NET:-xju-net}" >/dev/null
SHORT_SHA="$(git rev-parse --short=7 HEAD)"
[[ "$SHORT_SHA" =~ ^[0-9a-f]{7}$ ]] || {
	echo "无法取得合法的 7 位 Git 提交 SHA: $SHORT_SHA" >&2
	exit 1
}
HEAD_IMAGE="winbeau/cli-proxy-api:deploy-$SHORT_SHA"
read_image_env
ORIGINAL_DESIRED_IMAGE="$CURRENT_DESIRED_IMAGE"
ORIGINAL_ROLLBACK_IMAGE="$STORED_ROLLBACK_IMAGE"

if [[ "$ROLLBACK_MODE" == 1 ]]; then
	TARGET_IMAGE="${ROLLBACK_TARGET:-$STORED_ROLLBACK_IMAGE}"
	if [[ -n "$ROLLBACK_TARGET" ]]; then
		is_commit_image "$TARGET_IMAGE" || {
			echo "显式回滚镜像必须是 commit tag: $TARGET_IMAGE" >&2
			exit 1
		}
	else
		is_cliproxy_image "$TARGET_IMAGE" || {
			echo "没有可用的回滚镜像: ${TARGET_IMAGE:-<empty>}" >&2
			exit 1
		}
	fi
	docker image inspect "$TARGET_IMAGE" >/dev/null
else
	TARGET_IMAGE="$HEAD_IMAGE"
fi

discover_fleet
is_cliproxy_image "$PREVIOUS_FLEET_IMAGE" || {
	echo "当前 fleet 镜像不是受支持的自建 CLIProxyAPI 镜像: $PREVIOUS_FLEET_IMAGE" >&2
	exit 1
}
EXPECTED_PREVIOUS_FLEET_IMAGE="$PREVIOUS_FLEET_IMAGE"
print_plan

if [[ "$DRY_RUN" == 1 ]]; then
	if [[ "$ROLLBACK_MODE" != 1 ]]; then
		if docker image inspect "$TARGET_IMAGE" >/dev/null 2>&1; then
			echo "==> dry-run: 目标镜像已存在,正式部署将验证其 image ID 与当前容器"
		else
			echo "==> dry-run: 将构建 $TARGET_IMAGE"
		fi
	fi
	echo "==> dry-run 完成,未修改 Docker/systemd/运行文件"
	exit 0
fi

sudo -v
TARGET_IMAGE_PREEXISTED=0
if docker image inspect "$TARGET_IMAGE" >/dev/null 2>&1; then
	TARGET_IMAGE_PREEXISTED=1
fi
if [[ "$ROLLBACK_MODE" != 1 ]]; then
	if ((TARGET_IMAGE_PREEXISTED == 1)); then
		echo "==> 目标 commit 镜像已存在,跳过重建: $TARGET_IMAGE"
	else
		bash "$SCRIPT_DIR/build-cliproxy.sh" "deploy-$SHORT_SHA"
	fi
fi
docker image inspect "$TARGET_IMAGE" >/dev/null
TARGET_IMAGE_ID="$(docker image inspect "$TARGET_IMAGE" --format '{{.Id}}')"
TARGET_COMMIT="$(target_commit_for_image "$TARGET_IMAGE")"

# 先让 systemd 具备重启旧 watcher 的条件。首轮迁移时 env 文件可能尚不存在。
if [[ ! -f "$IMAGE_ENV_FILE" ]]; then
	write_image_env "$PREVIOUS_FLEET_IMAGE" ""
	ORIGINAL_DESIRED_IMAGE="$PREVIOUS_FLEET_IMAGE"
	ORIGINAL_ROLLBACK_IMAGE=""
	ENV_MIGRATED=1
fi
install_provision_unit

touch "$MAINTENANCE_FILE"
MAINTENANCE_ACTIVE=1
exec {provision_fd}>"$PROVISION_LOCK"
flock "$provision_fd"
sudo systemctl stop xju-provision
WATCHER_STOPPED=1

# watcher 停止后再次发现,避免 preflight 后恰好新增/删除池。
discover_fleet
print_plan
if [[ "$BACKUP" == 1 ]]; then
	echo "==> 备份动态池"
	BACKUP_CADDY=0 bash "$SCRIPT_DIR/backup.sh"
fi
if [[ "$PREVIOUS_FLEET_IMAGE" == "$TARGET_IMAGE" ]]; then
	TARGET_ALREADY_DEPLOYED=1
	for id in "${POOL_IDS[@]}"; do
		validate_pool_contract "$id" "${POOL_PORTS[$id]}" "${POOL_KINDS[$id]}" "$TARGET_IMAGE" "$TARGET_IMAGE_ID"
	done
	echo "==> 所有现役池已运行目标镜像及同一 image ID,跳过容器替换"
else
	TARGET_ALREADY_DEPLOYED=0
	for id in "${ROLLOUT_IDS[@]}"; do
		CURRENT_POOL_ID="$id"
		name="$(cliproxy_container_name "$id")"
		anchor="$name-rollback-$SHORT_SHA"
		if docker container inspect "$anchor" >/dev/null 2>&1; then
			echo "rollback anchor 已存在: $anchor" >&2
			false
		fi
		POOL_ANCHORS["$id"]="$anchor"
		echo "==> 升级 $name -> $TARGET_IMAGE"
		docker stop "$name" >/dev/null
		docker rename "$name" "$anchor"
		cliproxy_start_pool "$id" "${POOL_PORTS[$id]}" "${POOL_KINDS[$id]}" "$TARGET_IMAGE" >/dev/null
		cliproxy_wait_for_health "$name" "${POOL_PORTS[$id]}"
		validate_pool_contract "$id" "${POOL_PORTS[$id]}" "${POOL_KINDS[$id]}" "$TARGET_IMAGE" "$TARGET_IMAGE_ID"
		if [[ -n "$TARGET_COMMIT" ]]; then
			log_output="$(docker logs "$name" 2>&1)"
			grep -F "Commit: $TARGET_COMMIT" <<<"$log_output" >/dev/null || {
				echo "$name 启动日志未报告目标 commit" >&2
				false
			}
		fi
		UPGRADED_IDS+=("$id")
		CURRENT_POOL_ID=""
	done
fi

if ((TARGET_ALREADY_DEPLOYED == 1)); then
	if [[ -z "$CURRENT_DESIRED_IMAGE" || "$CURRENT_DESIRED_IMAGE" != "$TARGET_IMAGE" ]]; then
		rollback_pointer="$STORED_ROLLBACK_IMAGE"
		if [[ -z "$rollback_pointer" || "$rollback_pointer" == "$TARGET_IMAGE" ]]; then
			rollback_pointer="$PREVIOUS_FLEET_IMAGE"
		fi
		write_image_env "$TARGET_IMAGE" "$rollback_pointer"
	fi
else
	write_image_env "$TARGET_IMAGE" "$PREVIOUS_FLEET_IMAGE"
fi
install_provision_unit
sudo systemctl start xju-provision
WATCHER_STOPPED=0
verify_watcher_image

for id in "${POOL_IDS[@]}"; do
	name="$(cliproxy_container_name "$id")"
	cliproxy_wait_for_health "$name" "${POOL_PORTS[$id]}"
	validate_pool_contract "$id" "${POOL_PORTS[$id]}" "${POOL_KINDS[$id]}" "$TARGET_IMAGE" "$TARGET_IMAGE_ID"
done

for id in "${UPGRADED_IDS[@]}"; do
	docker rm "${POOL_ANCHORS[$id]}" >/dev/null
done
rm -f "$MAINTENANCE_FILE"
MAINTENANCE_ACTIVE=0
exec {provision_fd}>&-

trap - ERR INT TERM
echo "==> CLIProxyAPI 部署成功: $TARGET_IMAGE"
echo "==> 现役池: ${POOL_IDS[*]}"
echo "==> 回滚镜像: $PREVIOUS_FLEET_IMAGE"

if [[ "$PRUNE" == 1 ]]; then
	KEEP=2 bash "$SCRIPT_DIR/prune-docker.sh" || echo "WARN: Docker 清理未完全成功" >&2
else
	echo "==> 未清理镜像;稳定后可运行 KEEP=2 bash deploy/prune-docker.sh"
fi
