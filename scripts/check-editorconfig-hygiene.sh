#!/bin/sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)

check_one() {
  path=$1
  [ -f "$path" ] || return 0
  if [ -s "$path" ] && ! LC_ALL=C grep -Iq . "$path"; then
    return 0
  fi

  rel=${path#"$ROOT"/}

  case $rel in
    .obsidian/* | *.bat) return 0 ;;
  esac

  file_failed=0

  if [ -s "$path" ]; then
    last_byte=$(tail -c 1 "$path" | od -An -tx1 | tr -d '[:space:]')
    if [ "$last_byte" != "0a" ]; then
      printf '%s: missing final newline
' "$rel" >&2
      file_failed=1
    fi
  fi

  if LC_ALL=C grep -q "$(printf '\r')" "$path"; then
    printf '%s: uses CRLF line endings
' "$rel" >&2
    file_failed=1
  fi

  case $rel in
    *.md | *.patch) return "$file_failed" ;;
  esac

  trailing_lines=$(awk '/[ 	]$/ { print FNR }' "$path")
  if [ -n "$trailing_lines" ]; then
    printf '%s
' "$trailing_lines" | while IFS= read -r line_number; do
      printf '%s:%s: trailing whitespace
' "$rel" "$line_number" >&2
    done
    file_failed=1
  fi

  return "$file_failed"
}

failed=0
(
  cd "$ROOT"
  git ls-files --cached --others --exclude-standard
) | while IFS= read -r rel; do
  check_one "$ROOT/$rel" || failed=1
  printf '%s
' "$failed" > "${TMPDIR:-/tmp}/editorconfig-hygiene-status-$$"
done

status_file="${TMPDIR:-/tmp}/editorconfig-hygiene-status-$$"
if [ -f "$status_file" ]; then
  failed=$(tail -n 1 "$status_file")
  rm -f "$status_file"
fi

exit "$failed"
