#!/usr/bin/env bash
# One-time rollout helper for converting users.quota into the paid Default-pool
# balance. Dry-run by default; pass --apply only after stopping new-api writes.
set -euo pipefail

usage() {
  echo "usage: $0 [--apply] [--force] [--allow-active-funding] /absolute/path/to/one-api.db" >&2
  exit 2
}

apply=false
force=false
allow_active_funding=false
db_path=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --apply) apply=true ;;
    --force) force=true ;;
    --allow-active-funding) allow_active_funding=true ;;
    -h|--help) usage ;;
    --*) usage ;;
    *)
      [ -z "$db_path" ] || usage
      db_path=$1
      ;;
  esac
  shift
done

[ -n "$db_path" ] || usage
case "$db_path" in
  /*) ;;
  *)
    echo "database path must be absolute: $db_path" >&2
    exit 1
    ;;
esac
if [[ "$db_path" == *$'\n'* ]]; then
  echo "database path must not contain a newline" >&2
  exit 1
fi
[ -f "$db_path" ] || { echo "database not found: $db_path" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || {
  echo "python3 with the standard sqlite3 module is required; no changes were made" >&2
  exit 1
}

python3 - "$db_path" "$apply" "$force" "$allow_active_funding" <<'PY'
from __future__ import annotations

import datetime as dt
import hashlib
import os
from pathlib import Path
import sqlite3
import sys
import time


db_path = Path(sys.argv[1])
apply = sys.argv[2] == "true"
force = sys.argv[3] == "true"
allow_active_funding = sys.argv[4] == "true"
reset_marker_key = "XjuDefaultBalanceResetAt"


def readonly_connection(path: Path) -> sqlite3.Connection:
    connection = sqlite3.connect(path.resolve().as_uri() + "?mode=ro", uri=True)
    connection.execute("PRAGMA query_only = ON")
    return connection


def integrity_result(connection: sqlite3.Connection) -> str:
    rows = [str(row[0]) for row in connection.execute("PRAGMA integrity_check")]
    return "\n".join(rows)


def table_exists(connection: sqlite3.Connection, name: str) -> bool:
    row = connection.execute(
        "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)
    ).fetchone()
    return row is not None


def table_columns(connection: sqlite3.Connection, name: str) -> set[str]:
    return {str(row[1]) for row in connection.execute(f'PRAGMA table_info("{name}")')}


def scalar(connection: sqlite3.Connection, sql: str, params: tuple[object, ...] = ()) -> int:
    row = connection.execute(sql, params).fetchone()
    if row is None or row[0] is None:
        return 0
    return int(row[0])


def option_value(connection: sqlite3.Connection, key: str) -> str:
    row = connection.execute("SELECT value FROM options WHERE key = ?", (key,)).fetchone()
    return "" if row is None or row[0] is None else str(row[0])


def rollout_state(connection: sqlite3.Connection) -> dict[str, int | str]:
    now = int(time.time())
    active_redemptions = 0
    active_redemption_quota = 0
    pending_topups = 0
    active_subscriptions = 0

    if table_exists(connection, "redemptions"):
        redemption_columns = table_columns(connection, "redemptions")
        required = {"quota", "status", "expired_time", "deleted_at"}
        if required.issubset(redemption_columns):
            active_redemptions = scalar(
                connection,
                """
                SELECT COUNT(*) FROM redemptions
                WHERE deleted_at IS NULL AND status = 1
                  AND (expired_time = 0 OR expired_time >= ?)
                """,
                (now,),
            )
            active_redemption_quota = scalar(
                connection,
                """
                SELECT COALESCE(SUM(quota), 0) FROM redemptions
                WHERE deleted_at IS NULL AND status = 1
                  AND (expired_time = 0 OR expired_time >= ?)
                """,
                (now,),
            )
    if table_exists(connection, "top_ups") and "status" in table_columns(connection, "top_ups"):
        pending_topups = scalar(
            connection, "SELECT COUNT(*) FROM top_ups WHERE status = 'pending'"
        )
    if table_exists(connection, "user_subscriptions"):
        subscription_columns = table_columns(connection, "user_subscriptions")
        if {"status", "end_time"}.issubset(subscription_columns):
            active_subscriptions = scalar(
                connection,
                """
                SELECT COUNT(*) FROM user_subscriptions
                WHERE status = 'active' AND end_time > ?
                """,
                (now,),
            )
    return {
        "active_redemptions": active_redemptions,
        "active_redemption_quota": active_redemption_quota,
        "pending_topups": pending_topups,
        "active_subscriptions": active_subscriptions,
        "reset_marker": option_value(connection, reset_marker_key),
    }


def report_database(connection: sqlite3.Connection) -> dict[str, int | str]:
    integrity = integrity_result(connection)
    if integrity != "ok":
        raise RuntimeError(f"database integrity check failed: {integrity}")
    if not table_exists(connection, "users") or not table_exists(connection, "options"):
        raise RuntimeError("the target is not a compatible new-api database")

    user_columns = table_columns(connection, "users")
    required_columns = {"quota", "used_quota"}
    missing = required_columns - user_columns
    if missing:
        raise RuntimeError(f"users table is missing required columns: {', '.join(sorted(missing))}")

    user_count = scalar(connection, "SELECT COUNT(*) FROM users")
    nonzero_count = scalar(connection, "SELECT COUNT(*) FROM users WHERE quota != 0")
    quota_sum = scalar(connection, "SELECT COALESCE(SUM(quota), 0) FROM users")
    if "aff_quota" in user_columns:
        aff_nonzero_count = scalar(connection, "SELECT COUNT(*) FROM users WHERE aff_quota != 0")
        aff_quota_sum = scalar(connection, "SELECT COALESCE(SUM(aff_quota), 0) FROM users")
    else:
        aff_nonzero_count = 0
        aff_quota_sum = 0

    print(f"database: {db_path}")
    print(f"users: {user_count}")
    print(f"users with non-zero Default balance: {nonzero_count}")
    print(f"quota units to clear: {quota_sum}")
    print(f"users with legacy transferable invite quota: {aff_nonzero_count}")
    print(f"legacy invite quota units to clear: {aff_quota_sum}")

    state = rollout_state(connection)
    print(
        "active redemption codes still able to add balance: "
        f"{state['active_redemptions']} ({state['active_redemption_quota']} quota units)"
    )
    print(f"pending online top-up orders: {state['pending_topups']}")
    print(f"active subscriptions (separate funding source): {state['active_subscriptions']}")
    if state["reset_marker"]:
        print(f"previous reset marker: {state['reset_marker']}")
    return state


read_connection = readonly_connection(db_path)
try:
    state = report_database(read_connection)
finally:
    read_connection.close()

if not apply:
    print("dry run only; rerun with --apply after stopping new-api writes")
    raise SystemExit(0)

if state["reset_marker"] and not force:
    raise RuntimeError(
        "this database already has a Default-balance reset marker; "
        "use --force only after reviewing the existing reset and new balances"
    )
active_funding = (
    int(state["active_redemptions"])
    + int(state["pending_topups"])
    + int(state["active_subscriptions"])
)
if active_funding > 0 and not allow_active_funding:
    raise RuntimeError(
        "active redemption codes, pending top-ups, or subscriptions still exist; "
        "resolve them first or explicitly pass --allow-active-funding"
    )

timestamp = dt.datetime.now().strftime("%Y%m%d_%H%M%S_%f")
backup_path = Path(f"{db_path}.pre-default-balance-reset.{timestamp}.bak")
if backup_path.exists():
    raise RuntimeError(f"refusing to overwrite existing backup: {backup_path}")

write_connection = sqlite3.connect(str(db_path), timeout=30, isolation_level=None)
write_connection.execute("PRAGMA busy_timeout = 30000")
try:
    integrity = integrity_result(write_connection)
    if integrity != "ok":
        raise RuntimeError(f"database integrity check failed before backup: {integrity}")

    backup_connection = sqlite3.connect(str(backup_path))
    try:
        write_connection.backup(backup_connection)
    finally:
        backup_connection.close()
    os.chmod(backup_path, 0o600)

    backup_check = readonly_connection(backup_path)
    try:
        backup_integrity = integrity_result(backup_check)
    finally:
        backup_check.close()
    if backup_integrity != "ok":
        raise RuntimeError(
            f"backup integrity check failed: {backup_integrity}; no balances were changed"
        )

    latest_state = rollout_state(write_connection)
    if latest_state["reset_marker"] and not force:
        raise RuntimeError("a reset marker appeared after the dry-run; no balances were changed")
    latest_active_funding = (
        int(latest_state["active_redemptions"])
        + int(latest_state["pending_topups"])
        + int(latest_state["active_subscriptions"])
    )
    if latest_active_funding > 0 and not allow_active_funding:
        raise RuntimeError("active funding appeared after the dry-run; no balances were changed")

    user_columns = table_columns(write_connection, "users")
    assignments = ["quota = 0"]
    predicates = ["quota != 0"]
    if "aff_quota" in user_columns:
        assignments.append("aff_quota = 0")
        predicates.append("aff_quota != 0")

    write_connection.execute("BEGIN IMMEDIATE")
    try:
        write_connection.execute(
            f"UPDATE users SET {', '.join(assignments)} WHERE {' OR '.join(predicates)}"
        )
        for key, value in (
            ("QuotaForNewUser", "0"),
            ("QuotaForInviter", "0"),
            ("QuotaForInvitee", "0"),
            ("checkin_setting.enabled", "true"),
            ("payment_setting.online_payment_enabled", "false"),
            (reset_marker_key, dt.datetime.now(dt.timezone.utc).isoformat()),
        ):
            write_connection.execute(
                "INSERT OR IGNORE INTO options(key, value) VALUES(?, ?)", (key, value)
            )
            write_connection.execute(
                "UPDATE options SET value = ? WHERE key = ?", (value, key)
            )
        write_connection.commit()
    except Exception:
        write_connection.rollback()
        raise

    remaining = scalar(write_connection, "SELECT COUNT(*) FROM users WHERE quota != 0")
    remaining_aff = 0
    if "aff_quota" in user_columns:
        remaining_aff = scalar(
            write_connection, "SELECT COUNT(*) FROM users WHERE aff_quota != 0"
        )
    if remaining != 0 or remaining_aff != 0:
        raise RuntimeError(
            "verification failed: "
            f"{remaining} users still have a balance; "
            f"{remaining_aff} still have transferable invite quota"
        )
finally:
    write_connection.close()

digest = hashlib.sha256()
with backup_path.open("rb") as backup_file:
    for chunk in iter(lambda: backup_file.read(1024 * 1024), b""):
        digest.update(chunk)

print("reset complete")
print(f"backup: {backup_path}")
print(f"backup sha256: {digest.hexdigest()}")
print("used_quota, aff_history, token usage, and usage logs were preserved")
print("start the new new-api version only after this reset succeeds")
PY
