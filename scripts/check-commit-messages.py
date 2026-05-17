#!/usr/bin/env python3
"""Validate Conventional Commit headers for a git revision range."""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from dataclasses import dataclass

ALLOWED_TYPES = {
    "feat",
    "fix",
    "docs",
    "style",
    "refactor",
    "perf",
    "test",
    "chore",
    "ci",
    "build",
    "revert",
}
HEADER_RE = re.compile(
    r"^(?P<type>[a-z]+)(?:\([^)\r\n]+\))?(?P<breaking>!)?: (?P<subject>\S.*)$"
)
MAX_HEADER_LENGTH = 100


@dataclass(frozen=True)
class Commit:
    sha: str
    header: str


class GitCommandError(RuntimeError):
    """Raised when git cannot read the requested commit range."""


def run_git(args: list[str]) -> str:
    try:
        completed = subprocess.run(
            ["git", *args],
            check=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
    except subprocess.CalledProcessError as exc:
        stderr = (exc.stderr or "").strip()
        detail = stderr or f"git exited with status {exc.returncode}"
        raise GitCommandError(detail) from exc
    return completed.stdout


def commits_in_range(rev_from: str, rev_to: str) -> list[Commit]:
    raw = run_git(
        ["log", "--no-merges", "--format=%H%x00%s%x00", f"{rev_from}..{rev_to}"]
    )
    if not raw:
        return []

    parts = raw.rstrip("\0\n").split("\0")
    commits: list[Commit] = []
    for index in range(0, len(parts), 2):
        try:
            sha = parts[index].strip()
            header = parts[index + 1].strip()
        except IndexError as exc:
            raise ValueError("unexpected git log output while reading commits") from exc
        if sha:
            commits.append(Commit(sha=sha, header=header))
    return commits


def validate_header(header: str) -> list[str]:
    errors: list[str] = []
    if len(header) > MAX_HEADER_LENGTH:
        errors.append(
            f"header length {len(header)} exceeds {MAX_HEADER_LENGTH} characters"
        )

    match = HEADER_RE.match(header)
    if not match:
        errors.append(
            "header must match '<type>(optional-scope)!: <subject>' Conventional Commit format"
        )
        return errors

    commit_type = match.group("type")
    if commit_type not in ALLOWED_TYPES:
        allowed = ", ".join(sorted(ALLOWED_TYPES))
        errors.append(f"type '{commit_type}' is not allowed; expected one of: {allowed}")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--from", dest="rev_from", required=True)
    parser.add_argument("--to", dest="rev_to", required=True)
    args = parser.parse_args()

    try:
        commits = commits_in_range(args.rev_from, args.rev_to)
    except GitCommandError as exc:
        print(
            f"Unable to read commit range {args.rev_from}..{args.rev_to}: {exc}",
            file=sys.stderr,
        )
        return 2

    if not commits:
        print(f"No commits to lint in range {args.rev_from}..{args.rev_to}")
        return 0

    failures = 0
    for commit in commits:
        errors = validate_header(commit.header)
        if errors:
            failures += 1
            print(f"✖ {commit.sha[:12]} {commit.header}")
            for error in errors:
                print(f"  - {error}")
        else:
            print(f"✓ {commit.sha[:12]} {commit.header}")

    if failures:
        print(f"Commit lint failed: {failures}/{len(commits)} commit(s) invalid")
        return 1

    print(f"Commit lint passed: {len(commits)} commit(s) valid")
    return 0


if __name__ == "__main__":
    sys.exit(main())
