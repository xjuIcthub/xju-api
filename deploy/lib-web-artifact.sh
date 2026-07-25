#!/usr/bin/env bash
# Shared lock helpers for frontend artifact installation and image builds.

xju_acquire_web_artifact_lock() {
	local parent="$1"
	XJU_WEB_ARTIFACT_LOCK="$parent/.web-artifact.lock"
	mkdir -p "$parent"

	if ! mkdir "$XJU_WEB_ARTIFACT_LOCK" 2>/dev/null; then
		if [[ ! -d "$XJU_WEB_ARTIFACT_LOCK" || -L "$XJU_WEB_ARTIFACT_LOCK" ]]; then
			echo "invalid frontend artifact lock: $XJU_WEB_ARTIFACT_LOCK" >&2
			return 1
		fi
		local holder=
		if [[ -f "$XJU_WEB_ARTIFACT_LOCK/pid" ]]; then
			holder="$(tr -d '\n' <"$XJU_WEB_ARTIFACT_LOCK/pid")"
		fi
		if [[ "$holder" =~ ^[0-9]+$ ]] && kill -0 "$holder" 2>/dev/null; then
			echo "frontend artifact operation already running as PID $holder" >&2
			return 1
		fi
		if [[ -n "$(find "$XJU_WEB_ARTIFACT_LOCK" -mindepth 1 -maxdepth 1 ! -name pid -print -quit)" ]]; then
			echo "stale frontend lock contains unexpected files; inspect manually: $XJU_WEB_ARTIFACT_LOCK" >&2
			return 1
		fi
		rm -f "$XJU_WEB_ARTIFACT_LOCK/pid"
		rmdir "$XJU_WEB_ARTIFACT_LOCK"
		mkdir "$XJU_WEB_ARTIFACT_LOCK"
	fi

	printf '%s\n' "$$" >"$XJU_WEB_ARTIFACT_LOCK/pid"
}

xju_release_web_artifact_lock() {
	if [[ -z "${XJU_WEB_ARTIFACT_LOCK:-}" || ! -d "$XJU_WEB_ARTIFACT_LOCK" ]]; then
		return 0
	fi
	local holder=
	if [[ -f "$XJU_WEB_ARTIFACT_LOCK/pid" ]]; then
		holder="$(tr -d '\n' <"$XJU_WEB_ARTIFACT_LOCK/pid")"
	fi
	if [[ "$holder" == "$$" ]]; then
		rm -f "$XJU_WEB_ARTIFACT_LOCK/pid"
		rmdir "$XJU_WEB_ARTIFACT_LOCK" 2>/dev/null || true
	fi
}
