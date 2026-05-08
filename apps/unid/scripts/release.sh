#!/usr/bin/env bash
set -euo pipefail

# ─── Prerequisites ──────────────────────────────────────────
check_command() {
  if ! command -v "$1" &>/dev/null; then
    gum log --level error "'$1' is required but not found."
    exit 1
  fi
}

check_command gum
check_command gh
check_command git
check_command go

# ─── Git clean state ───────────────────────────────────────
if [[ -n "$(git status --porcelain)" ]]; then
  gum log --level error "Working directory is not clean. Commit or stash changes first."
  git status --short
  exit 1
fi

# ─── Current version ───────────────────────────────────────
LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
CURRENT_VERSION="${LAST_TAG#v}"
gum log --level info "Current version: v${CURRENT_VERSION}"

# ─── Version bump type ─────────────────────────────────────
IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT_VERSION"

BUMP_TYPE=$(gum choose \
  --header "Version bump type:" \
  "patch (${MAJOR}.${MINOR}.$((PATCH + 1)))" \
  "minor (${MAJOR}.$((MINOR + 1)).0)" \
  "major ($((MAJOR + 1)).0.0)" \
  "custom")

case "$BUMP_TYPE" in
  patch*) NEW_VERSION="${MAJOR}.${MINOR}.$((PATCH + 1))" ;;
  minor*) NEW_VERSION="${MAJOR}.$((MINOR + 1)).0" ;;
  major*) NEW_VERSION="$((MAJOR + 1)).0.0" ;;
  custom)
    NEW_VERSION=$(gum input \
      --prompt "New version: " \
      --placeholder "e.g. 0.2.0")
    ;;
esac

# Validate semver format
if [[ ! "$NEW_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  gum log --level error "Invalid version format '${NEW_VERSION}'. Expected: MAJOR.MINOR.PATCH"
  exit 1
fi

# ─── Verify build ──────────────────────────────────────────
gum spin --title "Building..." -- go build -o /dev/null ./cmd/unid
gum spin --title "Running tests..." -- go test ./...

# ─── Show changelog since last tag ─────────────────────────
echo ""
gum style --bold "Changes since last release:"
if [[ "$LAST_TAG" != "v0.0.0" ]]; then
  git log "${LAST_TAG}..HEAD" --oneline --no-decorate
else
  git log --oneline --no-decorate
fi

# ─── Confirm ───────────────────────────────────────────────
echo ""
gum confirm "Release v${NEW_VERSION}?" || { echo "Aborted."; exit 0; }

# ─── Tag and push ──────────────────────────────────────────
git tag -a "v${NEW_VERSION}" -m "v${NEW_VERSION}"

gum spin --title "Pushing to origin..." -- bash -c \
  "git push origin main && git push origin v${NEW_VERSION}"

echo ""
gum style --bold --foreground 2 "✓ Released v${NEW_VERSION}!"
echo "GitHub Actions will now build and publish the release."
echo "Track progress: https://github.com/silee-tools/unid/actions"
