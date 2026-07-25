#!/usr/bin/env bash
# deploy/provision-poold.sh — 号池开通 watcher(#4 Phase B,B2 宿主 helper)
#
# 架构:new-api(容器内,不碰 docker socket)把开通/删除请求写进
#   $PROVISION_DIR/requests/<id>.json;本 daemon(宿主,以有 docker 权限的用户跑)
#   接单——起/删独立 cliproxy 实例、回写 $PROVISION_DIR/results/<id>.json。
# 由 deploy/xju-provision.service 常驻(Restart=always)。
#
# 请求(new-api 写):{"action":"create","pool_id":"edu","label":"Edu","port":8319,
#                    "mode":"cliproxy","owner_user_id":42,"kind":"private","group_key":"private-42"}
#                    {"action":"delete","pool_id":"edu"}
# 结果(本脚本写):create → {pool_id,status:ok,mgmt_url,mgmt_secret,port,internal_key,error}
#                  delete → {pool_id,status:ok,error}
#
# 安全:secret / internal_key 由本脚本生成;结果文件 600;pool_id 严格校验防路径穿越。
set -uo pipefail

PROVISION_DIR="${PROVISION_DIR:-/opt/xju-api/provision}"
CLIPROXY_DIR="${CLIPROXY_DIR:-/opt/cli-proxy-api}"
IMAGE="${CLIPROXY_IMAGE:?CLIPROXY_IMAGE must be configured}"
TEMPLATE="${CONFIG_TEMPLATE:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/config.k12.example.yaml}"
PRIVATE_TEMPLATE="${PRIVATE_CONFIG_TEMPLATE:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/config.private.example.yaml}"
POLL_INTERVAL="${POLL_INTERVAL:-3}"
MAINTENANCE_FILE="${POOL_MAINTENANCE_FILE:-$PROVISION_DIR/.maintenance}"
PROVISION_LOCK="${POOL_PROVISION_LOCK:-$PROVISION_DIR/.provision.lock}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=deploy/cliproxy-pool-runtime.sh
source "$SCRIPT_DIR/cliproxy-pool-runtime.sh"

REQ="$PROVISION_DIR/requests"
RES="$PROVISION_DIR/results"
DONE="$PROVISION_DIR/processed"

log() { echo "[provision-poold] $(date '+%F %T') $*"; }

# valid_pool_id: lowercase alnum + dash, ≤31 chars, not a reserved (env) pool.
valid_pool_id() {
	[[ "$1" =~ ^[a-z0-9][a-z0-9-]{0,30}$ ]] && [[ "$1" != default && "$1" != k12 ]]
}

write_result() { # id json
	umask 077
	printf '%s\n' "$2" >"$RES/$1.json.tmp" && mv "$RES/$1.json.tmp" "$RES/$1.json"
}

err_result() { # id message
	write_result "$1" "$(jq -nc --arg id "$1" --arg e "$2" '{pool_id:$id,status:"error",error:$e}')"
}

provision_create() { # id label port mode owner_user_id kind group_key
	local id="$1" label="$2" port="$3" mode="$4" owner_user_id="$5" kind="$6" group_key="$7"
	if ! [[ "$port" =~ ^[0-9]+$ ]] || ((port < 1024 || port > 65535)); then
		err_result "$id" "invalid port: $port"
		return
	fi
	if ! [[ "$owner_user_id" =~ ^[0-9]+$ ]]; then
		err_result "$id" "invalid owner_user_id"
		return
	fi
	case "$kind" in
	admin)
		if ((owner_user_id != 0)) || [[ -n "$group_key" && "$group_key" != "$id" ]]; then
			err_result "$id" "invalid admin pool ownership metadata"
			return
		fi
		;;
	private)
		if ((owner_user_id <= 0)) || [[ "$group_key" != "private-$owner_user_id" ]]; then
			err_result "$id" "invalid private pool ownership metadata"
			return
		fi
		;;
	*)
		err_result "$id" "invalid pool kind: $kind"
		return
		;;
	esac
	local mgmt key cfg envf url template_source
	mgmt="$(openssl rand -hex 32)"
	key="$(openssl rand -hex 32)"
	cfg="$CLIPROXY_DIR/config.$id.yaml"
	envf="$CLIPROXY_DIR/.pool-mgmt-$id.env"
	template_source="$TEMPLATE"
	if [[ "$kind" == private ]]; then
		template_source="$PRIVATE_TEMPLATE"
	fi
	if [[ ! -f "$template_source" ]]; then
		err_result "$id" "config template not found"
		return
	fi

	# Clone the isolated-pool template, substituting port + secrets.
	if ! sed -e "s/^port: .*/port: $port/" \
		-e "s|__MANAGEMENT_SECRET_K12__|$mgmt|" \
		-e "s|__INTERNAL_API_KEY_K12__|$key|" \
		"$template_source" >"$cfg"; then
		err_result "$id" "failed to write config"
		return
	fi
	umask 077
	printf 'MANAGEMENT_PASSWORD=%s\n' "$mgmt" >"$envf"
	mkdir -p "$CLIPROXY_DIR/auths-$id" "$CLIPROXY_DIR/logs-$id"

	docker rm -f "cli-proxy-api-$id" >/dev/null 2>&1 || true
	if ! cliproxy_start_pool "$id" "$port" "$kind" "$IMAGE" >/dev/null; then
		err_result "$id" "docker run failed"
		return
	fi

	# Wait for the instance to answer before reporting success.
	if ! cliproxy_wait_for_health "cli-proxy-api-$id" "$port"; then
		err_result "$id" "instance did not become healthy"
		return
	fi

	# new-api reaches the instance by container name over xju-net.
	url="http://cli-proxy-api-$id:$port"
	write_result "$id" "$(jq -nc \
		--arg id "$id" --arg label "$label" --arg url "$url" --arg mgmt "$mgmt" \
		--arg key "$key" --arg mode "$mode" --arg kind "$kind" --arg group "$group_key" \
		--argjson port "$port" --argjson owner "$owner_user_id" \
		'{pool_id:$id,label:$label,action:"create",status:"ok",mgmt_url:$url,mgmt_secret:$mgmt,port:$port,internal_key:$key,error:"",mode:$mode,owner_user_id:$owner,kind:$kind,group_key:$group}')"
	log "created pool $id on port $port"
}

provision_delete() { # id
	local id="$1"
	docker rm -f "cli-proxy-api-$id" >/dev/null 2>&1 || true
	rm -f "$CLIPROXY_DIR/config.$id.yaml" "$CLIPROXY_DIR/.pool-mgmt-$id.env"
	# Keep auths-$id/ so the pool's accounts survive a pool delete (recoverable).
	write_result "$id" "$(jq -nc --arg id "$id" '{pool_id:$id,action:"delete",status:"ok",error:""}')"
	log "deleted pool $id (accounts kept in auths-$id/)"
}

process_one() { # request-file
	local f="$1" id action label port pid mode owner_user_id kind group_key
	id="$(basename "$f" .json)"
	if ! action="$(jq -r '.action // "create"' "$f" 2>/dev/null)"; then
		err_result "$id" "unreadable request"
		return
	fi
	pid="$(jq -r '.pool_id // ""' "$f")"
	label="$(jq -r '.label // .pool_id' "$f")"
	port="$(jq -r '.port // 0' "$f")"
	mode="$(jq -r '.mode // "cliproxy"' "$f")"
	owner_user_id="$(jq -r '.owner_user_id // 0' "$f")"
	kind="$(jq -r '.kind // "admin"' "$f")"
	group_key="$(jq -r '.group_key // ""' "$f")"
	if [[ "$pid" != "$id" ]] || ! valid_pool_id "$pid"; then
		err_result "$id" "invalid or mismatched pool id"
		return
	fi
	case "$action" in
	delete) provision_delete "$pid" ;;
	create) provision_create "$pid" "$label" "$port" "$mode" "$owner_user_id" "$kind" "$group_key" ;;
	*) err_result "$id" "unknown action: $action" ;;
	esac
}

process_one_locked() { # request-file
	local f="$1" lock_fd
	exec {lock_fd}>"$PROVISION_LOCK"
	flock "$lock_fd"
	process_one "$f"
	exec {lock_fd}>&-
}

main() {
	command -v jq >/dev/null || {
		echo "need jq" >&2
		exit 1
	}
	command -v docker >/dev/null || {
		echo "need docker" >&2
		exit 1
	}
	command -v flock >/dev/null || {
		echo "need flock" >&2
		exit 1
	}
	docker image inspect "$IMAGE" >/dev/null 2>&1 || {
		echo "configured CLIProxyAPI image not found: $IMAGE" >&2
		exit 1
	}
	mkdir -p "$REQ" "$RES" "$DONE"
	log "watching $REQ (image $IMAGE, interval ${POLL_INTERVAL}s, admin template $TEMPLATE, private template $PRIVATE_TEMPLATE)"
	while true; do
		if [[ -e "$MAINTENANCE_FILE" ]]; then
			sleep "$POLL_INTERVAL"
			continue
		fi
		for f in "$REQ"/*.json; do
			[[ -e "$f" ]] || continue
			process_one_locked "$f"
			mv "$f" "$DONE/$(basename "$f" .json).$(date +%s).json" 2>/dev/null || rm -f "$f"
		done
		sleep "$POLL_INTERVAL"
	done
}

main "$@"
