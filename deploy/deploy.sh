#!/usr/bin/env bash
# deploy/deploy.sh — claude-tri 完整部署总入口。
#
# 顺序:
#   拉取 main → 项目护栏 → 校验预装前端 + Go 构建 → 换容器/失败回滚
#   → Docker 清理 → 本地/公网/API/服务检查。
#
# 用法:
#   bash deploy/deploy.sh
#   bash deploy/deploy.sh release-20260724
#
# 可选环境变量:
#   PULL=0             跳过 git 更新
#   PRUNE=0            跳过 Docker 清理(默认执行)
#   CHECK_PUBLIC=0     跳过公网 API 检查
#   CHECK_PROVISION=0  跳过 xju-provision 服务检查
#   其余 SKIP_WEB / ROLLBACK / HEALTH_* 透传给 deploy-newapi.sh。
set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BRANCH="${BRANCH:-main}"
PULL="${PULL:-1}"
PRUNE="${PRUNE:-1}"
CHECK_PUBLIC="${CHECK_PUBLIC:-1}"
CHECK_PROVISION="${CHECK_PROVISION:-1}"
LOCAL_HEALTH_URL="${LOCAL_HEALTH_URL:-http://127.0.0.1:3000/api/status}"
PUBLIC_HEALTH_URL="${PUBLIC_HEALTH_URL:-https://api.selab.top/api/status}"
SKIP_WEB="${SKIP_WEB:-1}"

usage() {
	cat <<'EOF'
用法: bash deploy/deploy.sh [镜像 tag]

完整执行:
  1. fast-forward origin/main
  2. scripts/check-guardrails.sh
  3. deploy/deploy-newapi.sh(校验预装前端 + Go + 换容器 + 健康检查/失败回滚)
  4. deploy/prune-docker.sh
  5. 检查本地 API、公网 API、new-api 镜像与 xju-provision

默认 tag: deploy-<当前提交短 SHA>
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

require_command git
require_command bash
require_command docker
require_command curl

cd "$REPO_ROOT"

if [[ "$PULL" == 1 && "${XJU_DEPLOY_ALL_AFTER_PULL:-0}" != 1 ]]; then
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

	echo "==> [1/5] 更新代码: origin/$BRANCH"
	git fetch --prune origin "$BRANCH"
	git merge --ff-only "origin/$BRANCH"

	# 使用刚拉取到的最新版总入口继续执行。
	exec env XJU_DEPLOY_ALL_AFTER_PULL=1 PULL="$PULL" PRUNE="$PRUNE" \
		BRANCH="$BRANCH" CHECK_PUBLIC="$CHECK_PUBLIC" \
		CHECK_PROVISION="$CHECK_PROVISION" LOCAL_HEALTH_URL="$LOCAL_HEALTH_URL" \
		PUBLIC_HEALTH_URL="$PUBLIC_HEALTH_URL" SKIP_WEB="$SKIP_WEB" \
		ROLLBACK="${ROLLBACK:-1}" HEALTH_RETRIES="${HEALTH_RETRIES:-30}" \
		HEALTH_INTERVAL="${HEALTH_INTERVAL:-2}" \
		bash "$REPO_ROOT/deploy/deploy.sh" "$@"
fi

echo "==> [2/5] 检查项目护栏"
bash "$REPO_ROOT/scripts/check-guardrails.sh"

echo "==> [3/5] 构建并部署 New API"
PULL=0 PRUNE=0 HEALTH_URL="$LOCAL_HEALTH_URL" \
	SKIP_WEB="$SKIP_WEB" ROLLBACK="${ROLLBACK:-1}" \
	HEALTH_RETRIES="${HEALTH_RETRIES:-30}" \
	HEALTH_INTERVAL="${HEALTH_INTERVAL:-2}" \
	bash "$REPO_ROOT/deploy/deploy-newapi.sh" "$@"

echo "==> [4/5] 清理 Docker 构建垃圾"
if [[ "$PRUNE" == 1 ]]; then
	if ! bash "$REPO_ROOT/deploy/prune-docker.sh"; then
		echo "WARN: Docker 清理未完全成功,继续执行部署后健康检查。" >&2
	fi
else
	echo "==> PRUNE=0,跳过 Docker 清理"
fi

echo "==> [5/5] 部署后检查"
curl -fsS --max-time 10 "$LOCAL_HEALTH_URL" >/dev/null
echo "ok   本地 API: $LOCAL_HEALTH_URL"

if [[ "$CHECK_PUBLIC" == 1 ]]; then
	curl -fsS --max-time 15 "$PUBLIC_HEALTH_URL" >/dev/null
	echo "ok   公网 API: $PUBLIC_HEALTH_URL"
else
	echo "skip 公网 API 检查(CHECK_PUBLIC=0)"
fi

docker inspect new-api --format 'ok   new-api: {{.State.Status}} | {{.Config.Image}}'

if [[ "$CHECK_PROVISION" == 1 ]]; then
	require_command systemctl
	systemctl is-active --quiet xju-provision
	echo "ok   xju-provision: active"
else
	echo "skip xju-provision 检查(CHECK_PROVISION=0)"
fi

echo "==> 全部部署步骤完成: $(git rev-parse --short=7 HEAD)"
