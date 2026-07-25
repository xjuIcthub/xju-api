#!/usr/bin/env bash
# deploy/build-newapi.sh — 构建定制 new-api 镜像(prebuilt 流,唯一构建路径)
#
# 本机流程:cd web && bun run build → 拷 dist → server/newapi/prebuilt/current/dist
#       → docker build -f deploy/Dockerfile.newapi.prebuilt(context = server/newapi)
# tri 流程:先用 deploy/install-web-dist.sh 安装本机产物,再以 SKIP_WEB=1 只编 Go。
#
# 用法(仓库根目录):
#   ./deploy/build-newapi.sh              # tag 默认 winbeau/xju-newapi:latest
#   ./deploy/build-newapi.sh v0.6.0       # 指定 tag
#   SKIP_WEB=1 ./deploy/build-newapi.sh   # 跳过前端构建,校验并复用 prebuilt/current
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAG="${1:-latest}"
IMAGE="winbeau/xju-newapi:${TAG}"
PREBUILT_ROOT="$REPO_ROOT/server/newapi/prebuilt"
PREBUILT="${PREBUILT_DIR:-$PREBUILT_ROOT/current}"
PREBUILT_PARENT="$(dirname "$PREBUILT")"
case "$PREBUILT" in
	/*) ;;
	*) echo "PREBUILT_DIR 必须是绝对路径: $PREBUILT" >&2; exit 1 ;;
esac

# shellcheck source=deploy/lib-web-artifact.sh
source "$REPO_ROOT/deploy/lib-web-artifact.sh"
xju_acquire_web_artifact_lock "$PREBUILT_PARENT"
cleanup() {
	local status=$?
	trap - EXIT INT TERM HUP
	xju_release_web_artifact_lock
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM HUP

if [[ -e "$PREBUILT_PARENT/.web-install-journal" ]]; then
	echo "检测到未完成的前端安装 journal;拒绝构建: $PREBUILT_PARENT/.web-install-journal" >&2
	exit 1
fi

validate_dist() {
	local dist_dir="$1"
	local allow_dirty=()
	if [[ "${ALLOW_DIRTY_WEB_ARTIFACT:-0}" == 1 ]]; then
		allow_dirty+=(--allow-dirty)
	fi
	python3 "$REPO_ROOT/scripts/web-artifact-manifest.py" validate \
		--dist "$dist_dir" \
		--manifest "$PREBUILT/manifest.json" \
		--expected-commit "$(git -C "$REPO_ROOT" rev-parse --verify HEAD)" \
		"${allow_dirty[@]}"
}

if [[ "${SKIP_WEB:-0}" != 1 ]]; then
	echo "==> 前端构建(web/)"
	(cd "$REPO_ROOT/web" && bun install --frozen-lockfile && bun run build)
	rm -rf "$PREBUILT"
	mkdir -p "$PREBUILT"
	SOURCE_DIRTY=0
	if ! git -C "$REPO_ROOT" diff --quiet || ! git -C "$REPO_ROOT" diff --cached --quiet; then
		SOURCE_DIRTY=1
	fi
	if [[ -n "$(git -C "$REPO_ROOT" ls-files --others --exclude-standard -- web)" ]]; then
		SOURCE_DIRTY=1
	fi
	python3 "$REPO_ROOT/scripts/web-artifact-manifest.py" create \
		--dist "$REPO_ROOT/web/dist" \
		--output "$PREBUILT/manifest.json" \
		--source-commit "$(git -C "$REPO_ROOT" rev-parse --verify HEAD)" \
		--source-dirty "$SOURCE_DIRTY" \
		--bun-version "$(bun --version)"
	cp -R "$REPO_ROOT/web/dist" "$PREBUILT/dist"
	ALLOW_DIRTY_WEB_ARTIFACT=1 validate_dist "$PREBUILT/dist"
	python3 - "$PREBUILT/manifest.json" >"$PREBUILT/artifact.sha256" <<'PY'
import json
from pathlib import Path
import sys

manifest = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
print(manifest["dist_tree_sha256"])
PY
else
	[[ -f "$PREBUILT/manifest.json" ]] || {
		echo "SKIP_WEB=1 但 $PREBUILT/manifest.json 不存在" >&2
		echo "先用 deploy/install-web-dist.sh 安装本机构建的前端发布物" >&2
		exit 1
	}
	[[ -s "$PREBUILT/artifact.sha256" ]] || {
		echo "前端发布物缺少 artifact.sha256 安装标记" >&2
		exit 1
	}
	[[ "$(tr -d '\n' <"$PREBUILT/artifact.sha256")" =~ ^[0-9a-f]{64}$ ]] || {
		echo "前端发布物 artifact.sha256 安装标记无效" >&2
		exit 1
	}
	validate_dist "$PREBUILT/dist"
	echo "==> 跳过前端构建,使用已校验的 $PREBUILT/dist"
fi

if [[ "${VALIDATE_ONLY:-0}" == 1 ]]; then
	echo "==> 前端发布物校验通过;VALIDATE_ONLY=1,不构建镜像"
	exit 0
fi

SOURCE_REVISION="$(git -C "$REPO_ROOT" rev-parse --verify HEAD)"
WEB_REVISION="$(python3 - "$PREBUILT/manifest.json" <<'PY'
import json
from pathlib import Path
import sys

print(json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))["source_commit"])
PY
)"
WEB_ARTIFACT_SHA256="$(tr -d '\n' <"$PREBUILT/artifact.sha256")"

echo "==> 构建 $IMAGE(Go-only,-p 2 压内存峰值)"
DOCKER_BUILDKIT=1 docker build \
	-f "$REPO_ROOT/deploy/Dockerfile.newapi.prebuilt" \
	--build-arg "XJU_SOURCE_REVISION=$SOURCE_REVISION" \
	--build-arg "XJU_WEB_REVISION=$WEB_REVISION" \
	--build-arg "XJU_WEB_ARTIFACT_SHA256=$WEB_ARTIFACT_SHA256" \
	-t "$IMAGE" \
	"$REPO_ROOT/server/newapi"

echo ""
echo "==> 完成: $IMAGE"
echo "    部署: IMAGE=$IMAGE bash $REPO_ROOT/deploy/run-newapi.sh"
