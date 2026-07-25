#!/usr/bin/env bash
set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNTIME="$REPO_ROOT/deploy/cliproxy-pool-runtime.sh"
BUILD="$REPO_ROOT/deploy/build-cliproxy.sh"
DEPLOY="$REPO_ROOT/deploy/deploy-cliproxy.sh"

fail() {
	echo "not ok - $*" >&2
	exit 1
}

assert_contains() {
	local haystack="$1" needle="$2"
	[[ "$haystack" == *"$needle"* ]] || fail "missing: $needle"
}

assert_not_contains() {
	local haystack="$1" needle="$2"
	[[ "$haystack" != *"$needle"* ]] || fail "unexpected: $needle"
}

SHORT_SHA="$(git -C "$REPO_ROOT" rev-parse --short=7 HEAD)"
assert_contains "$(grep -F 'EXPECTED_TAG="deploy-$SHORT_SHA"' "$BUILD")" 'EXPECTED_TAG="deploy-$SHORT_SHA"'
assert_contains "$(grep -F 'TAG="${1:-$EXPECTED_TAG}"' "$BUILD")" 'TAG="${1:-$EXPECTED_TAG}"'
assert_contains "$(grep -F 'if [[ "$TAG" != "$EXPECTED_TAG" ]]' "$BUILD")" 'if [[ "$TAG" != "$EXPECTED_TAG" ]]'

# shellcheck source=deploy/cliproxy-pool-runtime.sh
source "$RUNTIME"

calls=""
docker() {
	calls+="docker $*\n"
	if [[ "$1 $2" == "run -d" ]]; then
		printf 'container-id\n'
	fi
}
export CLIPROXY_DIR="$(mktemp -d)"
trap 'rm -rf "$CLIPROXY_DIR"' EXIT
mkdir -p "$CLIPROXY_DIR/auths-7" "$CLIPROXY_DIR/logs-7"
printf 'port: 8324\n' >"$CLIPROXY_DIR/config.7.yaml"
printf 'MANAGEMENT_PASSWORD=x\n' >"$CLIPROXY_DIR/.pool-mgmt-7.env"
cliproxy_start_pool 7 8324 private winbeau/cli-proxy-api:deploy-123abcd >/dev/null
assert_contains "$calls" '--memory=256m'
assert_contains "$calls" '--memory-reservation=64m'
assert_contains "$calls" '--cpus=0.75'
assert_contains "$calls" '--pids-limit=128'
assert_contains "$calls" '--log-opt max-size=10m'
assert_contains "$calls" '127.0.0.1:8324:8324'

calls=""
mkdir -p "$CLIPROXY_DIR/auths-main" "$CLIPROXY_DIR/logs-main"
printf 'port: 8317\n' >"$CLIPROXY_DIR/config.main.yaml"
printf 'MANAGEMENT_PASSWORD=x\n' >"$CLIPROXY_DIR/.pool-mgmt-main.env"
cliproxy_start_pool main 8317 admin winbeau/cli-proxy-api:deploy-123abcd >/dev/null
assert_not_contains "$calls" '--memory=256m'
assert_contains "$calls" '127.0.0.1:8317:8317'

DEPLOY_TEXT="$(<"$DEPLOY")"
assert_contains "$DEPLOY_TEXT" 'docker exec new-api cat /data/xju-pools.json'
assert_contains "$(<"$REPO_ROOT/deploy/backup.sh")" 'find "$path" -type f ! -readable'
assert_contains "$(<"$REPO_ROOT/deploy/backup.sh")" 'sudo -n tar czf "$archive"'
assert_contains "$(<"$REPO_ROOT/deploy/backup.sh")" 'sudo -n chown "$(id -u):$(id -g)" "$archive"'
assert_contains "$DEPLOY_TEXT" '[[ -r "$POOL_REGISTRY_FILE" ]]'
assert_contains "$DEPLOY_TEXT" 'TARGET_IMAGE="$HEAD_IMAGE"'
assert_contains "$DEPLOY_TEXT" 'dry-run 完成,未修改 Docker/systemd/运行文件'
assert_contains "$DEPLOY_TEXT" 'rollback_changed_pools'
assert_contains "$DEPLOY_TEXT" 'write_image_env "$TARGET_IMAGE" "$PREVIOUS_FLEET_IMAGE"'
assert_contains "$DEPLOY_TEXT" 'CLIPROXY_IMAGE=$TARGET_IMAGE'
assert_contains "$DEPLOY_TEXT" 'for ((index = ${#UPGRADED_IDS[@]} - 1; index >= 0; index--))'
assert_contains "$DEPLOY_TEXT" '[[ "$id" == main ]] && ROLLOUT_IDS+=("$id")'
assert_contains "$DEPLOY_TEXT" 'touch "$MAINTENANCE_FILE"'
assert_contains "$DEPLOY_TEXT" 'flock "$provision_fd"'
assert_not_contains "$DEPLOY_TEXT" 'bun '
assert_not_contains "$DEPLOY_TEXT" 'deploy-newapi'

echo "ok - CLIProxyAPI deploy invariants"
