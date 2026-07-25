#!/usr/bin/env bash
# deploy/backup.sh — claude-tri 动态部署滚动备份。
# 内容: New API SQLite + 动态池 registry/config/env/auths + CLIProxy 镜像状态 + Caddy。
set -Eeuo pipefail
umask 077

KEEP="${KEEP:-7}"
BACKUP_ROOT="${BACKUP_ROOT:-/opt/backups/xju-api}"
NEWAPI_DATA="${NEWAPI_DATA:-/opt/new-api/data}"
CLIPROXY_DIR="${CLIPROXY_DIR:-/opt/cli-proxy-api}"
BACKUP_CADDY="${BACKUP_CADDY:-1}"
STAMP="$(date +%Y%m%d-%H%M%S)"
DEST="$BACKUP_ROOT/$STAMP"
mkdir -p "$DEST"

# 1) New API SQLite 热备。
[[ -f "$NEWAPI_DATA/one-api.db" ]] || {
	echo "missing New API database: $NEWAPI_DATA/one-api.db" >&2
	exit 1
}
[[ -f "$NEWAPI_DATA/xju-pools.json" ]] || {
	echo "missing dynamic pool registry: $NEWAPI_DATA/xju-pools.json" >&2
	exit 1
}
[[ -d "$CLIPROXY_DIR" ]] || {
	echo "missing CLIProxyAPI directory: $CLIPROXY_DIR" >&2
	exit 1
}

if docker exec new-api sh -c 'command -v sqlite3' >/dev/null 2>&1; then
	docker exec new-api sqlite3 /data/one-api.db ".backup /data/.backup-tmp.db"
	mv "$NEWAPI_DATA/.backup-tmp.db" "$DEST/one-api.db"
elif command -v sqlite3 >/dev/null 2>&1; then
	sqlite3 "$NEWAPI_DATA/one-api.db" ".backup '$DEST/one-api.db'"
else
	echo "WARN: 未找到 sqlite3,退化为直接 cp" >&2
	cp "$NEWAPI_DATA/one-api.db" "$DEST/one-api.db"
fi

# 2) 动态池 registry 与运行数据。日志不进入备份。
cp -a "$NEWAPI_DATA/xju-pools.json" "$DEST/"
mapfile -d '' pool_paths < <(
	find "$CLIPROXY_DIR" -mindepth 1 -maxdepth 1 \
		\( -name 'config.*.yaml' -o -name '.pool-mgmt-*.env' -o -name 'auths-*' -o -name '.cliproxy-image.env' \) \
		-print0 | sort -z
)
((${#pool_paths[@]} > 0)) || {
	echo "no dynamic pool files found in $CLIPROXY_DIR" >&2
	exit 1
}
tar czf "$DEST/cli-proxy-dynamic.tar.gz" --absolute-names "${pool_paths[@]}"
tar tzf "$DEST/cli-proxy-dynamic.tar.gz" >/dev/null

# 3) 不含密钥的 fleet manifest，便于恢复时核对镜像/端口/挂载。
{
	printf 'created_at=%s\n' "$(date -Is)"
	for container in $(docker ps -a --format '{{.Names}}' | grep '^cli-proxy-api-' | sort -V); do
		docker inspect "$container" | jq -c '.[0] | {
			name: .Name,
			image: .Config.Image,
			status: .State.Status,
			networks: (.NetworkSettings.Networks | keys),
			ports: .HostConfig.PortBindings,
			mounts: [.Mounts[] | {source: .Source, destination: .Destination}],
			memory: .HostConfig.Memory,
			memory_reservation: .HostConfig.MemoryReservation,
			nano_cpus: .HostConfig.NanoCpus,
			pids_limit: .HostConfig.PidsLimit,
			log_config: .HostConfig.LogConfig
		}'
	done
} >"$DEST/cliproxy-fleet.jsonl"

# 4) Caddy 配置与证书可按部署场景跳过。
if [[ "$BACKUP_CADDY" == 1 ]]; then
	CADDY_DATA=""
	for d in /var/lib/caddy /root/.local/share/caddy; do
		[[ -d "$d" ]] && CADDY_DATA="$d" && break
	done
	tar czf "$DEST/caddy.tar.gz" /etc/caddy/Caddyfile ${CADDY_DATA:+"$CADDY_DATA"} 2>/dev/null
fi

find "$DEST" -maxdepth 1 -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 -r sha256sum >"$DEST/SHA256SUMS"
ls -1dt "$BACKUP_ROOT"/*/ 2>/dev/null | tail -n +$((KEEP + 1)) | xargs -r rm -rf

echo "[$(date '+%F %T')] backup ok -> $DEST ($(du -sh "$DEST" | cut -f1))"
