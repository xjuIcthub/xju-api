#!/usr/bin/env bash
# Verify and atomically install a locally built frontend artifact on Codex-tri.
set -euo pipefail

usage() {
	cat <<'EOF' >&2
usage: deploy/install-web-dist.sh /path/xju-web-dist-*.tar.gz [/path/file.sha256]

The artifact must match the current repository HEAD and must not be marked dirty.
Set ALLOW_DIRTY_ARTIFACT=1 only for a non-production test. PREBUILT_DIR may be
set to an absolute alternate destination for isolated testing.
EOF
	exit 2
}

if [[ "$#" -lt 1 || "$#" -gt 2 || "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
	usage
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARCHIVE="$1"
CHECKSUM="${2:-$1.sha256}"
PREBUILT_DIR="${PREBUILT_DIR:-$REPO_ROOT/server/newapi/prebuilt/current}"

case "$PREBUILT_DIR" in
	/*) ;;
	*) echo "PREBUILT_DIR must be absolute: $PREBUILT_DIR" >&2; exit 1 ;;
esac

[[ -f "$ARCHIVE" ]] || { echo "artifact not found: $ARCHIVE" >&2; exit 1; }
[[ -f "$CHECKSUM" ]] || { echo "checksum not found: $CHECKSUM" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || {
	echo "python3 is required to validate the artifact safely" >&2
	exit 1
}
command -v git >/dev/null 2>&1 || { echo "missing command: git" >&2; exit 1; }

cd "$REPO_ROOT"
CURRENT_COMMIT="$(git rev-parse --verify HEAD)"
if { ! git diff --quiet || ! git diff --cached --quiet; } && \
	[[ "${ALLOW_DIRTY_WORKTREE:-0}" != 1 ]]; then
	echo "tracked worktree is dirty; refusing to install a production frontend artifact" >&2
	exit 1
fi
PREBUILT_PARENT="$(dirname "$PREBUILT_DIR")"
mkdir -p "$PREBUILT_PARENT"
if [[ -L "$PREBUILT_DIR" ]]; then
	echo "refusing to replace symlink destination: $PREBUILT_DIR" >&2
	exit 1
fi
if [[ -e "$PREBUILT_DIR" && ! -d "$PREBUILT_DIR" ]]; then
	echo "prebuilt destination exists but is not a directory: $PREBUILT_DIR" >&2
	exit 1
fi

# shellcheck source=deploy/lib-web-artifact.sh
source "$REPO_ROOT/deploy/lib-web-artifact.sh"
xju_acquire_web_artifact_lock "$PREBUILT_PARENT"
JOURNAL="$PREBUILT_PARENT/.web-install-journal"
TARGET_NAME="$(basename "$PREBUILT_DIR")"
STAGE_DIR=
INSTALL_ACTIVE=0
BACKUP_DIR=

cleanup() {
	local status=$?
	trap - EXIT INT TERM HUP
	if [[ "$status" -ne 0 && "$INSTALL_ACTIVE" == 1 ]]; then
		local failed="$PREBUILT_PARENT/$TARGET_NAME.failed.$(date -u +%Y%m%d_%H%M%S).$$"
		if [[ -n "$BACKUP_DIR" ]]; then
			if [[ -d "$BACKUP_DIR" ]]; then
				if [[ -e "$PREBUILT_DIR" ]]; then
					mv "$PREBUILT_DIR" "$failed" || true
				fi
				if [[ ! -e "$PREBUILT_DIR" ]]; then
					mv "$BACKUP_DIR" "$PREBUILT_DIR" || true
				fi
			fi
		elif [[ -e "$PREBUILT_DIR" ]]; then
			mv "$PREBUILT_DIR" "$failed" || true
		fi
		if [[ -e "$PREBUILT_DIR" || -z "$BACKUP_DIR" ]]; then
			rm -f "$JOURNAL"
		fi
	fi
	if [[ -n "$STAGE_DIR" && -d "$STAGE_DIR" ]]; then
		rm -rf "$STAGE_DIR"
	fi
	xju_release_web_artifact_lock
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM HUP

recover_interrupted_install() {
	[[ -f "$JOURNAL" ]] || return 0
	local recorded_backup
	recorded_backup="$(sed -n '1p' "$JOURNAL")"
	case "$recorded_backup" in
		NONE|"$PREBUILT_PARENT/$TARGET_NAME.previous."*) ;;
		*)
			echo "invalid frontend install journal; inspect manually: $JOURNAL" >&2
			return 1
			;;
	esac
	local interrupted="$PREBUILT_PARENT/$TARGET_NAME.interrupted.$(date -u +%Y%m%d_%H%M%S).$$"
	if [[ "$recorded_backup" != NONE ]]; then
		if [[ -d "$recorded_backup" ]]; then
			if [[ -e "$PREBUILT_DIR" ]]; then
				mv "$PREBUILT_DIR" "$interrupted"
				echo "unverified interrupted bundle kept at: $interrupted" >&2
			fi
			mv "$recorded_backup" "$PREBUILT_DIR"
		elif [[ ! -e "$PREBUILT_DIR" ]]; then
			echo "frontend install journal backup and current bundle are both missing" >&2
			return 1
		fi
	elif [[ -e "$PREBUILT_DIR" ]]; then
		mv "$PREBUILT_DIR" "$interrupted"
		echo "unverified interrupted bundle kept at: $interrupted" >&2
	fi
	rm -f "$JOURNAL"
}

recover_interrupted_install

STAGE_DIR="$(mktemp -d "$PREBUILT_PARENT/.web-install.XXXXXX")"

python3 - "$ARCHIVE" "$CHECKSUM" "$STAGE_DIR/artifact" <<'PY'
from __future__ import annotations

import hashlib
from pathlib import Path, PurePosixPath
import re
import shutil
import sys
import tarfile


archive = Path(sys.argv[1]).resolve()
checksum_file = Path(sys.argv[2]).resolve()
destination = Path(sys.argv[3])
checksum_lines = checksum_file.read_text(encoding="utf-8").splitlines()
if len(checksum_lines) != 1:
    raise RuntimeError(f"invalid SHA-256 sidecar: {checksum_file}")
checksum_match = re.fullmatch(
    r"([0-9a-fA-F]{64})[ \t]+\*?([^ \t\r\n]+)[ \t]*", checksum_lines[0]
)
if checksum_match is None or checksum_match.group(2) != archive.name:
    raise RuntimeError(f"invalid SHA-256 sidecar: {checksum_file}")
expected_digest = checksum_match.group(1).lower()
digest = hashlib.sha256()
with archive.open("rb") as stream:
    for chunk in iter(lambda: stream.read(1024 * 1024), b""):
        digest.update(chunk)
if digest.hexdigest() != expected_digest:
    raise RuntimeError("frontend artifact SHA-256 mismatch")

destination.mkdir(mode=0o700)
seen: set[str] = set()
total_size = 0
has_index = False
has_static_asset = False

with tarfile.open(archive, mode="r:gz") as bundle:
    members = bundle.getmembers()
    if len(members) > 10000:
        raise RuntimeError("frontend artifact contains too many entries")
    for member in members:
        name = member.name.rstrip("/")
        path = PurePosixPath(name)
        if (
            not name
            or path.is_absolute()
            or ".." in path.parts
            or any(ord(character) < 32 or ord(character) == 127 for character in name)
        ):
            raise RuntimeError(f"unsafe artifact path: {member.name!r}")
        normalized = path.as_posix()
        if name != normalized:
            raise RuntimeError(f"non-canonical artifact path: {member.name!r}")
        if path.parts[0] not in {"manifest.json", "dist"}:
            raise RuntimeError(f"unexpected artifact path: {member.name!r}")
        if path.parts[0] == "manifest.json" and len(path.parts) != 1:
            raise RuntimeError(f"unexpected artifact path: {member.name!r}")
        if normalized in seen:
            raise RuntimeError(f"duplicate artifact path: {member.name!r}")
        seen.add(normalized)
        if not (member.isdir() or member.isreg()):
            raise RuntimeError(f"unsupported artifact entry type: {member.name!r}")
        if member.isreg():
            total_size += member.size
            if total_size > 200 * 1024 * 1024:
                raise RuntimeError("frontend artifact expands beyond 200 MiB")
            has_index = has_index or (name == "dist/index.html" and member.size > 0)
            has_static_asset = has_static_asset or (
                len(path.parts) > 2 and path.parts[:2] == ("dist", "static") and member.size > 0
            )

    if "manifest.json" not in seen or not has_index or not has_static_asset:
        raise RuntimeError("artifact is missing manifest.json, dist/index.html, or static assets")

    for member in members:
        relative = PurePosixPath(member.name.rstrip("/"))
        target = destination.joinpath(*relative.parts)
        if member.isdir():
            target.mkdir(parents=True, exist_ok=True, mode=0o755)
            continue
        target.parent.mkdir(parents=True, exist_ok=True, mode=0o755)
        source = bundle.extractfile(member)
        if source is None:
            raise RuntimeError(f"cannot read artifact member: {member.name!r}")
        with source, target.open("wb") as output:
            shutil.copyfileobj(source, output)
        target.chmod(0o644)

PY

validate_artifact() {
	local dist="$1"
	local manifest="$2"
	local args=(
		validate
		--dist "$dist"
		--manifest "$manifest"
		--expected-commit "$CURRENT_COMMIT"
	)
	if [[ "${ALLOW_DIRTY_ARTIFACT:-0}" == 1 ]]; then
		args+=(--allow-dirty)
	fi
	python3 "$REPO_ROOT/scripts/web-artifact-manifest.py" "${args[@]}"
}
validate_artifact "$STAGE_DIR/artifact/dist" "$STAGE_DIR/artifact/manifest.json"
CHECKSUM_DIGEST="$(awk 'NR == 1 { print $1 }' "$CHECKSUM" | tr 'A-F' 'a-f')"
printf '%s\n' "$CHECKSUM_DIGEST" >"$STAGE_DIR/artifact/artifact.sha256"

TIMESTAMP="$(date -u +%Y%m%d_%H%M%S)"
if [[ -d "$PREBUILT_DIR" ]]; then
	BACKUP_DIR="$PREBUILT_PARENT/$TARGET_NAME.previous.$TIMESTAMP.$$"
	[[ ! -e "$BACKUP_DIR" ]] || {
		echo "prebuilt backup path already exists: $BACKUP_DIR" >&2
		exit 1
	}
fi

JOURNAL_TMP="$JOURNAL.tmp.$$"
printf '%s\n' "${BACKUP_DIR:-NONE}" >"$JOURNAL_TMP"
mv "$JOURNAL_TMP" "$JOURNAL"
INSTALL_ACTIVE=1
if [[ -n "$BACKUP_DIR" ]]; then
	mv "$PREBUILT_DIR" "$BACKUP_DIR"
fi

if ! mv "$STAGE_DIR/artifact" "$PREBUILT_DIR"; then
	echo "failed to install frontend artifact; previous bundle restored when possible" >&2
	exit 1
fi
validate_artifact "$PREBUILT_DIR/dist" "$PREBUILT_DIR/manifest.json"
rm -f "$JOURNAL"
INSTALL_ACTIVE=0

echo "==> frontend artifact installed"
echo "destination: $PREBUILT_DIR"
echo "source commit: $CURRENT_COMMIT"
if [[ -n "$BACKUP_DIR" ]]; then
	echo "previous bundle: $BACKUP_DIR"
fi
