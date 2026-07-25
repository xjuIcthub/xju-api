#!/usr/bin/env bash
# deploy/build-cliproxy.sh — 构建与当前 Git 提交一一对应的 CLIProxyAPI 镜像。
#
# 镜像固定命名为 winbeau/cli-proxy-api:deploy-<7位提交短SHA>。构建只涉及
# server/cliproxy 的 Go 服务，不构建前端，也不替换任何运行中容器。
set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHORT_SHA="$(git -C "$REPO_ROOT" rev-parse --short=7 HEAD)"
[[ "$SHORT_SHA" =~ ^[0-9a-f]{7}$ ]] || {
	echo "无法取得合法的 7 位 Git 提交 SHA: $SHORT_SHA" >&2
	exit 1
}
EXPECTED_TAG="deploy-$SHORT_SHA"
TAG="${1:-$EXPECTED_TAG}"

if [[ "$TAG" != "$EXPECTED_TAG" ]]; then
	echo "CLIProxyAPI 镜像 tag 必须对应当前提交: $EXPECTED_TAG（收到: $TAG）" >&2
	exit 1
fi
if [[ ! "$TAG" =~ ^deploy-[0-9a-f]{7}$ ]]; then
	echo "CLIProxyAPI 镜像 tag 不合法: $TAG" >&2
	exit 1
fi

IMAGE="winbeau/cli-proxy-api:$TAG"
CTX="$REPO_ROOT/server/cliproxy"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "==> 构建 $IMAGE（Go-only，context=server/cliproxy）"
DOCKER_BUILDKIT=1 docker build \
	-f "$CTX/Dockerfile" \
	--build-arg VERSION="$TAG" \
	--build-arg COMMIT="$SHORT_SHA" \
	--build-arg BUILD_DATE="$BUILD_DATE" \
	-t "$IMAGE" \
	"$CTX"

IMAGE_ID="$(docker image inspect --format '{{.Id}}' "$IMAGE")"
echo "==> 构建完成: $IMAGE"
printf 'CLIPROXY_IMAGE=%s\n' "$IMAGE"
printf 'CLIPROXY_IMAGE_ID=%s\n' "$IMAGE_ID"
