#!/usr/bin/env bash
set -euo pipefail

check_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: '$1' is required but not found." >&2
    exit 1
  fi
}

check_command gh
check_command git

if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working directory is not clean. Commit or stash changes first." >&2
  git status --short
  exit 1
fi

branch=$(git branch --show-current)
if [[ "${branch}" != "main" ]]; then
  echo "error: release workflow dispatch must run from main (current: ${branch})." >&2
  exit 1
fi

git fetch origin --prune --tags
git pull --ff-only origin main

latest_tag=$(git tag -l 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -n 1)
latest_tag=${latest_tag:-v0.0.0}
echo "Latest tag: ${latest_tag}"

echo "Select bump type: patch, minor, major, or custom"
read -r -p "Bump [patch]: " bump
bump=${bump:-patch}
version=""
if [[ "${bump}" == "custom" ]]; then
  read -r -p "Version (e.g. 0.2.0 or v0.2.0): " version
  bump="patch"
fi

if [[ ! "${bump}" =~ ^(patch|minor|major)$ ]]; then
  echo "error: invalid bump '${bump}'" >&2
  exit 1
fi

args=(workflow run release.yml --ref main -f "bump=${bump}")
if [[ -n "${version}" ]]; then
  args+=(-f "version=${version}")
fi

echo "Dispatching GitHub Actions release workflow..."
gh "${args[@]}"

echo "Release workflow dispatched. Track progress:"
gh run list --workflow release.yml --branch main --limit 5
