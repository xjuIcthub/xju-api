#!/usr/bin/env bash
# Build and package the web bundle on Codex-vps for transfer to Codex-tri.
# The resulting archive contains dist/ plus a source manifest and SHA-256 sidecar.
set -euo pipefail

usage() {
	cat <<'EOF' >&2
usage: scripts/package-web-dist.sh [output-directory]

By default this runs `bun run typecheck` and `bun run build` in web/, then
writes a versioned .tar.gz and matching .sha256 file. The tracked worktree and
web/ source must be clean.

Test/emergency overrides:
  SKIP_BUILD=1  package the existing web/dist without rebuilding
  SKIP_TYPECHECK=1 skip typecheck but still perform a fresh production build
  ALLOW_DIRTY=1 allow packaging an artifact marked source_dirty=1
EOF
	exit 2
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || "$#" -gt 1 ]]; then
	usage
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="$REPO_ROOT/web/dist"
OUTPUT_DIR="${1:-/private/tmp/xju-web-artifacts}"

case "$OUTPUT_DIR" in
	/*) ;;
	*) OUTPUT_DIR="$(pwd)/$OUTPUT_DIR" ;;
esac

require_command() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "missing command: $1" >&2
		exit 1
	}
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		require_command shasum
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

create_manifest() {
	python3 "$REPO_ROOT/scripts/web-artifact-manifest.py" create \
		--dist "$DIST_DIR" \
		--output "$STAGE_DIR/manifest.json" \
		--source-commit "$SOURCE_COMMIT" \
		--source-dirty "$SOURCE_DIRTY" \
		--created-at "$CREATED_AT" \
		--bun-version "$BUN_VERSION"
}

detect_source_dirty() {
	SOURCE_DIRTY=0
	if ! git diff --quiet || ! git diff --cached --quiet; then
		SOURCE_DIRTY=1
	fi
	if [[ -n "$(git ls-files --others --exclude-standard | awk '$0 != "AGENTS.md"')" ]]; then
		SOURCE_DIRTY=1
	fi
}

require_clean_source() {
	if [[ "$SOURCE_DIRTY" == 1 && "${ALLOW_DIRTY:-0}" != 1 ]]; then
		echo "tracked worktree or web/ source is dirty; commit the release first" >&2
		echo "use ALLOW_DIRTY=1 only for a non-production test artifact" >&2
		exit 1
	fi
}

validate_staged_dist() {
	local args=(
		validate
		--dist "$STAGE_DIR/dist"
		--manifest "$STAGE_DIR/manifest.json"
		--expected-commit "$SOURCE_COMMIT"
	)
	if [[ "$SOURCE_DIRTY" == 1 ]]; then
		args+=(--allow-dirty)
	fi
	python3 "$REPO_ROOT/scripts/web-artifact-manifest.py" "${args[@]}"
}

require_command git
require_command tar
require_command awk
require_command python3

cd "$REPO_ROOT"
SOURCE_COMMIT="$(git rev-parse --verify HEAD)"
SOURCE_SHORT="$(git rev-parse --short=12 HEAD)"
detect_source_dirty
require_clean_source
if [[ "${SKIP_BUILD:-0}" != 1 ]]; then
	require_command bun
	echo "==> building frontend on Codex-vps"
	if [[ "${SKIP_TYPECHECK:-0}" != 1 ]]; then
		(cd "$REPO_ROOT/web" && bun run typecheck)
	fi
	(cd "$REPO_ROOT/web" && bun run build)
else
	echo "==> SKIP_BUILD=1, packaging existing $DIST_DIR"
fi

detect_source_dirty
require_clean_source

mkdir -p "$OUTPUT_DIR"
STAGE_DIR="$(mktemp -d "${TMPDIR:-/private/tmp}/xju-web-package.XXXXXX")"
cleanup() {
	rm -rf "$STAGE_DIR"
}
trap cleanup EXIT

CREATED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
BUILD_STAMP="$(date -u +%Y%m%d_%H%M%S)"
BUILD_ID="$SOURCE_SHORT-$BUILD_STAMP"
BUN_VERSION="$(bun --version 2>/dev/null || echo unknown)"
ARCHIVE="$OUTPUT_DIR/xju-web-dist-$BUILD_ID.tar.gz"
CHECKSUM="$ARCHIVE.sha256"

[[ ! -e "$ARCHIVE" && ! -L "$ARCHIVE" && ! -e "$CHECKSUM" && ! -L "$CHECKSUM" ]] || {
	echo "artifact path already exists: $ARCHIVE" >&2
	exit 1
}

create_manifest
cp -R "$DIST_DIR" "$STAGE_DIR/dist"
validate_staged_dist

# macOS bsdtar otherwise emits AppleDouble `._*` entries for extended attributes.
COPYFILE_DISABLE=1 tar -czf "$ARCHIVE" -C "$STAGE_DIR" manifest.json dist
DIGEST="$(sha256_file "$ARCHIVE")"
printf '%s  %s\n' "$DIGEST" "$(basename "$ARCHIVE")" >"$CHECKSUM"

echo "==> frontend artifact ready"
echo "archive: $ARCHIVE"
echo "checksum: $CHECKSUM"
echo "source commit: $SOURCE_COMMIT"
echo "source dirty: $SOURCE_DIRTY"
