#!/usr/bin/env python3
"""Create and validate the manifest used by xju-api frontend artifacts."""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys


FORMAT = "xju-web-dist-v1"
MAX_FILES = 10_000
MAX_TOTAL_SIZE = 200 * 1024 * 1024


def fail(message: str) -> None:
    raise RuntimeError(message)


def validate_relative_path(path: str) -> bytes:
    if not path or any(ord(character) < 32 or ord(character) == 127 for character in path):
        fail(f"unsafe frontend path: {path!r}")
    try:
        return path.encode("utf-8")
    except UnicodeEncodeError as error:
        fail(f"frontend path is not valid UTF-8: {path!r}: {error}")


def inspect_dist(dist: Path) -> tuple[str, int, int]:
    if not dist.is_dir() or dist.is_symlink():
        fail(f"frontend dist is not a normal directory: {dist}")

    entries: list[tuple[str, Path, int]] = []
    for root, directory_names, file_names in os.walk(dist, followlinks=False):
        root_path = Path(root)
        for directory_name in directory_names:
            directory_path = root_path / directory_name
            if directory_path.is_symlink():
                fail(f"frontend dist contains a symlink: {directory_path}")
        for file_name in file_names:
            file_path = root_path / file_name
            file_stat = file_path.lstat()
            if not stat.S_ISREG(file_stat.st_mode):
                fail(f"frontend dist contains a non-regular file: {file_path}")
            relative = file_path.relative_to(dist).as_posix()
            validate_relative_path(relative)
            entries.append((relative, file_path, file_stat.st_size))

    entries.sort(key=lambda item: item[0].encode("utf-8"))
    if not entries:
        fail("frontend dist contains no files")
    if len(entries) > MAX_FILES:
        fail(f"frontend dist contains more than {MAX_FILES} files")

    index_size = next((size for name, _, size in entries if name == "index.html"), 0)
    if index_size <= 0:
        fail("frontend dist is missing a non-empty index.html")
    if not any(name.startswith("static/") and size > 0 for name, _, size in entries):
        fail("frontend dist is missing non-empty static assets")

    total_size = sum(size for _, _, size in entries)
    if total_size > MAX_TOTAL_SIZE:
        fail("frontend dist exceeds the 200 MiB safety limit")

    tree_digest = hashlib.sha256()
    for relative, file_path, size in entries:
        path_bytes = validate_relative_path(relative)
        file_digest = hashlib.sha256()
        with file_path.open("rb") as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                file_digest.update(chunk)
        tree_digest.update(len(path_bytes).to_bytes(8, "big"))
        tree_digest.update(path_bytes)
        tree_digest.update(size.to_bytes(8, "big"))
        tree_digest.update(file_digest.digest())

    return tree_digest.hexdigest(), len(entries), total_size


def load_manifest(path: Path) -> dict[str, object]:
    def strict_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
        result: dict[str, object] = {}
        for key, value in pairs:
            if key in result:
                fail(f"duplicate frontend artifact manifest field: {key}")
            result[key] = value
        return result

    try:
        manifest = json.loads(
            path.read_text(encoding="utf-8"), object_pairs_hook=strict_object
        )
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as error:
        fail(f"cannot read frontend artifact manifest: {error}")
    if not isinstance(manifest, dict):
        fail("frontend artifact manifest must be a JSON object")
    return manifest


def create_manifest(args: argparse.Namespace) -> None:
    commit = args.source_commit.lower()
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
        fail("source commit must be a full 40-character Git SHA")
    if args.source_dirty not in {"0", "1"}:
        fail("source dirty must be 0 or 1")

    tree_digest, file_count, total_size = inspect_dist(args.dist)
    manifest = {
        "format": FORMAT,
        "source_commit": commit,
        "source_dirty": args.source_dirty == "1",
        "created_at": args.created_at
        or dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat(),
        "bun_version": args.bun_version,
        "dist_tree_sha256": tree_digest,
        "dist_file_count": file_count,
        "dist_total_size": total_size,
    }
    args.output.write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    print(f"dist_tree_sha256={tree_digest}")
    print(f"dist_file_count={file_count}")


def validate_manifest(args: argparse.Namespace) -> None:
    manifest = load_manifest(args.manifest)
    if manifest.get("format") != FORMAT:
        fail("unsupported frontend artifact manifest format")

    commit = manifest.get("source_commit")
    if not isinstance(commit, str) or not re.fullmatch(r"[0-9a-f]{40}", commit):
        fail("invalid source_commit in frontend artifact manifest")
    if commit != args.expected_commit.lower():
        fail(
            f"frontend artifact commit {commit} does not match repository "
            f"{args.expected_commit.lower()}"
        )

    source_dirty = manifest.get("source_dirty")
    if not isinstance(source_dirty, bool):
        fail("invalid source_dirty in frontend artifact manifest")
    if source_dirty and not args.allow_dirty:
        fail("frontend artifact was built from a dirty source tree")

    expected_tree_digest = manifest.get("dist_tree_sha256")
    expected_file_count = manifest.get("dist_file_count")
    expected_total_size = manifest.get("dist_total_size")
    if not isinstance(expected_tree_digest, str) or not re.fullmatch(
        r"[0-9a-f]{64}", expected_tree_digest
    ):
        fail("invalid dist_tree_sha256 in frontend artifact manifest")
    if type(expected_file_count) is not int or expected_file_count < 1:
        fail("invalid dist_file_count in frontend artifact manifest")
    if type(expected_total_size) is not int or expected_total_size < 1:
        fail("invalid dist_total_size in frontend artifact manifest")

    tree_digest, file_count, total_size = inspect_dist(args.dist)
    if tree_digest != expected_tree_digest:
        fail("frontend dist tree hash does not match its manifest")
    if file_count != expected_file_count or total_size != expected_total_size:
        fail("frontend dist size/count does not match its manifest")

    print(f"source_commit={commit}")
    print(f"source_dirty={int(source_dirty)}")
    print(f"dist_tree_sha256={tree_digest}")
    print(f"dist_file_count={file_count}")


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser()
    commands = root.add_subparsers(dest="command", required=True)

    create = commands.add_parser("create")
    create.add_argument("--dist", type=Path, required=True)
    create.add_argument("--output", type=Path, required=True)
    create.add_argument("--source-commit", required=True)
    create.add_argument("--source-dirty", required=True)
    create.add_argument("--created-at", default="")
    create.add_argument("--bun-version", default="unknown")
    create.set_defaults(handler=create_manifest)

    validate = commands.add_parser("validate")
    validate.add_argument("--dist", type=Path, required=True)
    validate.add_argument("--manifest", type=Path, required=True)
    validate.add_argument("--expected-commit", required=True)
    validate.add_argument("--allow-dirty", action="store_true")
    validate.set_defaults(handler=validate_manifest)
    return root


def main() -> int:
    args = parser().parse_args()
    try:
        args.handler(args)
    except RuntimeError as error:
        print(str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
