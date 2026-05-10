#!/usr/bin/env bash
set -euo pipefail

TOOL="jg"
WORKFLOW="release-please.yml"

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
  echo "error: release rebuild dispatch must run from main (current: ${branch})." >&2
  exit 1
fi

git fetch origin --prune --tags
git pull --ff-only origin main

latest_tag=$(git tag -l "${TOOL}/v[0-9]*.[0-9]*.[0-9]*" --sort=-v:refname | head -n 1)
latest_version=""
if [[ -n "${latest_tag}" ]]; then
  latest_version="${latest_tag#${TOOL}/}"
  echo "Latest ${TOOL} tag: ${latest_tag}"
else
  echo "No existing ${TOOL}/v* tag found. Enter the version to rebuild manually."
fi

prompt="Version to rebuild"
if [[ -n "${latest_version}" ]]; then
  prompt="Version to rebuild [${latest_version}]"
fi
read -r -p "${prompt}: " version
version=${version:-${latest_version}}

if [[ -z "${version}" ]]; then
  echo "error: version is required." >&2
  exit 1
fi
case "${version}" in v*) ;; *) version="v${version}" ;; esac

if ! git rev-parse --verify --quiet "refs/tags/${TOOL}/${version}" >/dev/null; then
  echo "error: tag ${TOOL}/${version} does not exist." >&2
  echo "Release versions are created by release-please PR merges; this helper only rebuilds an existing release." >&2
  exit 1
fi

echo "Dispatching ${WORKFLOW} rebuild for ${TOOL} ${version}..."
gh workflow run "${WORKFLOW}" --ref main -f "tool=${TOOL}" -f "version=${version}"

echo "Release rebuild workflow dispatched. Track progress:"
gh run list --workflow "${WORKFLOW}" --branch main --limit 5
