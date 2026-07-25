#!/usr/bin/env bash
# deploy/deploy-newapi.sh — claude-tri 上一键更新、构建并部署 New API。
#
# 默认流程:
#   1) 检查 tracked 工作区无本地修改;
#   2) fast-forward 到 origin/main,并重新执行更新后的脚本;
#   3) 校验本机传入的前端发布物,在服务器只构建 Go 镜像;
#   4) 替换 new-api 容器并检查 /api/status;
#   5) 失败时尝试恢复部署前镜像。
#
# 用法:
#   bash deploy/deploy-newapi.sh
#   bash deploy/deploy-newapi.sh announcements-20260724
#
# 可选环境变量:
#   PULL=0         已手工拉取代码时跳过 git fetch/merge
#   SKIP_WEB=1     使用已安装的 prebuilt/current(默认且生产必须保持为 1)
#   SKIP_WEB=0     仅限非 tri 环境,允许在当前机器重新构建前端
#   ROLLBACK=0     健康检查失败时不自动恢复旧镜像
#   PRUNE=1        成功后运行 deploy/prune-docker.sh
#   BRANCH=main    要部署的远端分支
#   HEALTH_RETRIES=30 HEALTH_INTERVAL=2
set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BRANCH="${BRANCH:-main}"
PULL="${PULL:-1}"
ROLLBACK="${ROLLBACK:-1}"
PRUNE="${PRUNE:-0}"
HEALTH_RETRIES="${HEALTH_RETRIES:-30}"
HEALTH_INTERVAL="${HEALTH_INTERVAL:-2}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:3000/api/status}"
SKIP_WEB="${SKIP_WEB:-1}"

usage() {
	cat <<'EOF'
用法: bash deploy/deploy-newapi.sh [镜像 tag]

默认会拉取 origin/main、校验预装前端产物、只构建 Go、替换容器并验活。
前端必须先在 Codex-vps 构建并用 deploy/install-web-dist.sh 安装。
未指定 tag 时自动使用 deploy-<当前提交短 SHA>。

常用示例:
  bash deploy/deploy-newapi.sh
  PULL=0 bash deploy/deploy-newapi.sh test-tag
  SKIP_WEB=1 bash deploy/deploy-newapi.sh release-tag
  PRUNE=1 bash deploy/deploy-newapi.sh
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
	usage
	exit 0
fi

require_command() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "缺少命令: $1" >&2
		exit 1
	}
}

wait_for_health() {
	local attempt
	for ((attempt = 1; attempt <= HEALTH_RETRIES; attempt++)); do
		if curl -fsS --max-time 5 "$HEALTH_URL" >/dev/null 2>&1; then
			return 0
		fi
		if [[ "$(docker inspect new-api --format '{{.State.Running}}' 2>/dev/null || true)" != true ]]; then
			break
		fi
		echo "==> 等待健康检查 ($attempt/$HEALTH_RETRIES)"
		sleep "$HEALTH_INTERVAL"
	done
	return 1
}

rollback_to_previous() {
	if [[ "$ROLLBACK" != 1 ]]; then
		echo "==> ROLLBACK=0,不自动恢复旧镜像" >&2
		return 1
	fi
	if [[ -z "$PREVIOUS_IMAGE" ]]; then
		echo "==> 部署前没有 new-api 容器,无旧镜像可恢复" >&2
		return 1
	fi
	if ! docker image inspect "$PREVIOUS_IMAGE" >/dev/null 2>&1; then
		echo "==> 旧镜像不存在,无法恢复: $PREVIOUS_IMAGE" >&2
		return 1
	fi

	echo "==> 尝试恢复部署前镜像: $PREVIOUS_IMAGE" >&2
	if ! IMAGE="$PREVIOUS_IMAGE" bash "$REPO_ROOT/deploy/run-newapi.sh"; then
		echo "==> 旧镜像启动失败,请立即人工检查" >&2
		return 1
	fi
	if wait_for_health; then
		echo "==> 已恢复旧镜像: $PREVIOUS_IMAGE" >&2
		return 0
	fi

	echo "==> 旧镜像恢复后仍未通过健康检查" >&2
	return 1
}

require_command git
require_command docker
require_command curl

cd "$REPO_ROOT"

if [[ "$PULL" == 1 && "${XJU_DEPLOY_AFTER_PULL:-0}" != 1 ]]; then
	if ! git diff --quiet || ! git diff --cached --quiet; then
		echo "tracked 工作区存在本地修改,为避免覆盖已停止部署:" >&2
		git status --short --untracked-files=no >&2
		exit 1
	fi

	CURRENT_BRANCH="$(git branch --show-current)"
	if [[ "$CURRENT_BRANCH" != "$BRANCH" ]]; then
		echo "当前分支是 $CURRENT_BRANCH,目标分支是 $BRANCH;请先切换分支" >&2
		exit 1
	fi

	echo "==> 更新代码: origin/$BRANCH"
	git fetch --prune origin "$BRANCH"
	git merge --ff-only "origin/$BRANCH"

	# git 更新可能同时更新本脚本;重新执行一次,确保后续逻辑使用最新版本。
	exec env XJU_DEPLOY_AFTER_PULL=1 PULL="$PULL" ROLLBACK="$ROLLBACK" \
		PRUNE="$PRUNE" BRANCH="$BRANCH" HEALTH_RETRIES="$HEALTH_RETRIES" \
		HEALTH_INTERVAL="$HEALTH_INTERVAL" HEALTH_URL="$HEALTH_URL" \
		SKIP_WEB="$SKIP_WEB" bash "$REPO_ROOT/deploy/deploy-newapi.sh" "$@"
fi

if [[ "$SKIP_WEB" != 1 ]]; then
	require_command bun
fi

COMMIT_SHA="$(git rev-parse --short=7 HEAD)"
TAG="${1:-deploy-$COMMIT_SHA}"
if [[ ! "$TAG" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]; then
	echo "镜像 tag 不合法: $TAG" >&2
	exit 1
fi

IMAGE="winbeau/xju-newapi:$TAG"
PREVIOUS_IMAGE="$(docker inspect new-api --format '{{.Config.Image}}' 2>/dev/null || true)"

echo "==> 部署提交: $(git log -1 --pretty='%h %s')"
echo "==> 新镜像: $IMAGE"
if [[ -n "$PREVIOUS_IMAGE" ]]; then
	echo "==> 当前镜像: $PREVIOUS_IMAGE"
fi

SKIP_WEB="$SKIP_WEB" bash "$REPO_ROOT/deploy/build-newapi.sh" "$TAG"

echo "==> 替换 new-api 容器"
if ! IMAGE="$IMAGE" bash "$REPO_ROOT/deploy/run-newapi.sh"; then
	echo "==> 新容器启动命令失败" >&2
	rollback_to_previous || true
	exit 1
fi

if ! wait_for_health; then
	echo "==> 新版本未通过健康检查: $HEALTH_URL" >&2
	docker logs --tail 120 new-api >&2 2>/dev/null || true
	rollback_to_previous || true
	exit 1
fi

echo "==> 部署成功"
docker inspect new-api --format '容器镜像: {{.Config.Image}}'
curl -fsS "$HEALTH_URL"
echo ""

if [[ "$PRUNE" == 1 ]]; then
	echo "==> 清理旧镜像与构建缓存"
	bash "$REPO_ROOT/deploy/prune-docker.sh"
else
	echo "==> 未清理旧镜像;确认稳定后可运行: bash deploy/prune-docker.sh"
fi
