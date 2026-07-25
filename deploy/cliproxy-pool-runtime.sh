#!/usr/bin/env bash
# deploy/cliproxy-pool-runtime.sh — 动态 CLIProxyAPI 池的统一运行契约。
# 供 provision-poold.sh 与 deploy-cliproxy.sh source；本文件自身不执行命令。

cliproxy_validate_pool_id() {
	[[ "${1:-}" =~ ^[a-z0-9][a-z0-9-]{0,30}$ ]]
}

cliproxy_container_name() {
	printf 'cli-proxy-api-%s\n' "$1"
}

cliproxy_pool_port_from_config() {
	local config="$1"
	local port
	port="$(sed -nE 's/^[[:space:]]*port:[[:space:]]*([0-9]+)[[:space:]]*$/\1/p' "$config" | head -n 1)"
	[[ "$port" =~ ^[0-9]+$ ]] && ((port >= 1024 && port <= 65535)) || return 1
	printf '%s\n' "$port"
}

cliproxy_require_pool_runtime() {
	local id="$1"
	local root="${CLIPROXY_DIR:-/opt/cli-proxy-api}"
	cliproxy_validate_pool_id "$id" || {
		echo "invalid pool id: $id" >&2
		return 1
	}
	[[ -f "$root/config.$id.yaml" ]] || {
		echo "missing pool config: $root/config.$id.yaml" >&2
		return 1
	}
	[[ -d "$root/auths-$id" ]] || {
		echo "missing pool auth directory: $root/auths-$id" >&2
		return 1
	}
	[[ -d "$root/logs-$id" ]] || {
		echo "missing pool log directory: $root/logs-$id" >&2
		return 1
	}
	[[ -f "$root/.pool-mgmt-$id.env" ]] || {
		echo "missing pool environment: $root/.pool-mgmt-$id.env" >&2
		return 1
	}
}

cliproxy_start_pool() {
	local id="$1" port="$2" kind="$3" image="$4"
	local root="${CLIPROXY_DIR:-/opt/cli-proxy-api}"
	local network="${XJU_NET:-xju-net}"
	local name
	name="$(cliproxy_container_name "$id")"

	cliproxy_require_pool_runtime "$id"
	[[ "$port" =~ ^[0-9]+$ ]] && ((port >= 1024 && port <= 65535)) || {
		echo "invalid pool port: $port" >&2
		return 1
	}
	case "$kind" in
	admin | private) ;;
	*)
		echo "invalid pool kind: $kind" >&2
		return 1
		;;
	esac

	local -a run_args=(
		-d --name "$name" --restart unless-stopped
		--network "$network" -p "127.0.0.1:$port:$port"
		-v "$root/config.$id.yaml:/CLIProxyAPI/config.yaml"
		-v "$root/auths-$id:/root/.cli-proxy-api"
		-v "$root/logs-$id:/CLIProxyAPI/logs"
		--env-file "$root/.pool-mgmt-$id.env"
	)
	if [[ "$kind" == private ]]; then
		run_args+=(
			--memory=256m --memory-reservation=64m --cpus=0.75 --pids-limit=128
			--log-opt max-size=10m --log-opt max-file=2
		)
	fi

	docker run "${run_args[@]}" "$image"
}

cliproxy_wait_for_health() {
	local name="$1" port="$2"
	local retries="${HEALTH_RETRIES:-30}"
	local interval="${HEALTH_INTERVAL:-2}"
	local attempt
	for ((attempt = 1; attempt <= retries; attempt++)); do
		if curl -fsS --max-time 5 "http://127.0.0.1:$port/healthz" >/dev/null 2>&1; then
			return 0
		fi
		if [[ "$(docker inspect "$name" --format '{{.State.Running}}' 2>/dev/null || true)" != true ]]; then
			break
		fi
		echo "==> 等待 $name 健康检查 ($attempt/$retries)"
		sleep "$interval"
	done
	return 1
}
