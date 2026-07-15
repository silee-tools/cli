#!/bin/sh

set -eu

CDPATH=
export CDPATH
repo_root=$(cd -- "$(dirname "$0")/.." && pwd)
umask 077
work=$(mktemp -d "${TMPDIR:-/tmp}/silee-tools-runtime-channel.XXXXXX")
real_mv=$(command -v mv)
tap_dir=${HOMEBREW_TAP_DIR:-}
if [ -z "$tap_dir" ] && [ -d "$(dirname "$repo_root")/homebrew-tap" ]; then
  tap_dir=$(dirname "$repo_root")/homebrew-tap
fi
cleanup() {
  chmod -R u+w "$work" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$work/fake-bin" "$work/homebrew"
fake_brew=$work/fake-bin/brew
fake_mv=$work/fake-bin/mv
# 생성되는 스크립트가 실행될 때 환경변수를 읽는다.
# shellcheck disable=SC2016
printf '%s\n' '#!/bin/sh' 'test "$1" = "--prefix"' 'printf "%s\\n" "$FAKE_HOMEBREW_PREFIX"' >"$fake_brew"
chmod +x "$fake_brew"
# shellcheck disable=SC2016
printf '%s\n' '#!/bin/sh' \
  'if [ "$#" -eq 2 ] && [ -n "${FAIL_MV_DEST:-}" ] && [ "$2" = "$FAIL_MV_DEST" ]; then exit 73; fi' \
  'exec "$REAL_MV" "$@"' >"$fake_mv"
chmod +x "$fake_mv"

assert_output() {
  expected=$1
  shift
  if ! actual=$("$@" 2>&1); then
    printf 'command failed: %s\n%s\n' "$*" "$actual" >&2
    return 1
  fi
  if [ "$actual" != "$expected" ]; then
    printf 'unexpected output from %s\nexpected: %s\nactual: %s\n' \
      "$*" "$expected" "$actual" >&2
    return 1
  fi
}

checksum_of() {
  cksum <"$1"
}

check_tool() {
  tool=$1
  install_config=$repo_root/apps/$tool/.mise.toml
  home=$work/home-$tool
  prefix=$work/homebrew-$tool
  state=$prefix/var/silee-tools/$tool/active-channel
  release=$prefix/opt/$tool/bin/$tool
  gomodcache=$(cd "$repo_root/apps/$tool" && mise exec -- go env GOMODCACHE)
  gocache=$(cd "$repo_root/apps/$tool" && mise exec -- go env GOCACHE)
  aliases=""
  case $tool in
  git-tidy) aliases="gtidy gtidy!" ;;
  jg) aliases="jgw" ;;
  esac

  # 설치 설정에 남은 리터럴 패턴을 검사한다.
  # shellcheck disable=SC2016
  if grep -F '${TMPDIR:-/tmp}/' "$install_config" >/dev/null; then
    printf '%s\n' "$tool: install temporary binary must be created beside its destination" >&2
    return 1
  fi

  mkdir -p "$home/.local/bin" "$prefix"
  (
    cd "$repo_root/apps/$tool"
    HOME=$home \
      MISE_TRUSTED_CONFIG_PATHS=$repo_root \
      FAKE_HOMEBREW_PREFIX=$prefix \
      GOCACHE=$gocache \
      GOMODCACHE=$gomodcache \
      REAL_MV=$real_mv \
      PATH=$work/fake-bin:$PATH \
      mise run install >/dev/null
  )

  test -x "$home/.local/bin/$tool"
  test "$(sed -n 's/^channel=//p' "$state")" = dev
  assert_output "$tool vdev © 2026 silee-tools" \
    "$home/.local/bin/$tool" --version
  for command in $aliases; do
    test -L "$home/.local/bin/$command"
    assert_output "$command vdev © 2026 silee-tools" \
      "$home/.local/bin/$command" --version
  done

  printf '%s\n' '#!/bin/sh' "printf '%s\\n' '$tool preserved-dev-binary'" \
    >"$home/.local/bin/$tool"
  chmod +x "$home/.local/bin/$tool"
  for command in $aliases; do
    rm -f "$home/.local/bin/$command"
  done
  printf 'channel=release\n' >"$state"
  installed_checksum=$(checksum_of "$home/.local/bin/$tool")
  if (
    cd "$repo_root/apps/$tool"
    HOME=$home \
      MISE_TRUSTED_CONFIG_PATHS=$repo_root \
      FAKE_HOMEBREW_PREFIX=$prefix \
      FAIL_MV_DEST=$state \
      GOCACHE=$gocache \
      GOMODCACHE=$gomodcache \
      REAL_MV=$real_mv \
      PATH=$work/fake-bin:$PATH \
      mise run install >/dev/null 2>&1
  ); then
    printf '%s\n' "$tool: install unexpectedly succeeded when state replacement failed" >&2
    return 1
  fi
  if [ "$(checksum_of "$home/.local/bin/$tool")" != "$installed_checksum" ]; then
    printf '%s\n' "$tool: failed state replacement changed the installed binary" >&2
    return 1
  fi
  if [ "$(sed -n 's/^channel=//p' "$state")" != release ]; then
    printf '%s\n' "$tool: failed state replacement changed the active channel" >&2
    return 1
  fi
  for command in $aliases; do
    if [ -e "$home/.local/bin/$command" ] || [ -L "$home/.local/bin/$command" ]; then
      printf '%s\n' "$tool: failed state replacement created alias $command" >&2
      return 1
    fi
  done

  (
    cd "$repo_root/apps/$tool"
    HOME=$home \
      MISE_TRUSTED_CONFIG_PATHS=$repo_root \
      FAKE_HOMEBREW_PREFIX=$prefix \
      GOCACHE=$gocache \
      GOMODCACHE=$gomodcache \
      REAL_MV=$real_mv \
      PATH=$work/fake-bin:$PATH \
      mise run install >/dev/null
  )
  assert_output "$tool vdev © 2026 silee-tools" \
    "$home/.local/bin/$tool" --version

  mkdir -p "$(dirname "$release")" "$(dirname "$state")"
  printf '%s\n' '#!/bin/sh' "printf '%s\\n' '$tool release-channel'" >"$release"
  chmod +x "$release"
  state_tmp=$state.tmp
  printf 'channel=release\nexecutable=/tmp/override\n' >"$state_tmp"
  mv "$state_tmp" "$state"
  if "$home/.local/bin/$tool" --version >"$work/override-output" 2>&1; then
    printf '%s\n' "$tool: executable override was accepted" >&2
    return 1
  fi
  grep -q 'invalid state' "$work/override-output"

  if [ -n "$tap_dir" ] && [ -f "$tap_dir/Formula/$tool.rb" ]; then
    sh "$tap_dir/scripts/check-runtime-channel-contract.sh" \
      --exercise "$tool" "$prefix/var"
  else
    printf 'channel=release\n' >"$state_tmp"
    mv "$state_tmp" "$state"
    printf 'SKIP %s: Homebrew Formula unavailable for cross-repository contract\n' \
      "$tool"
  fi
  assert_output "$tool release-channel" "$home/.local/bin/$tool" --version
  for command in $aliases; do
    assert_output "$tool release-channel" "$home/.local/bin/$command" --version
  done

  (
    cd "$repo_root/apps/$tool"
    HOME=$home \
      MISE_TRUSTED_CONFIG_PATHS=$repo_root \
      FAKE_HOMEBREW_PREFIX=$prefix \
      GOCACHE=$gocache \
      GOMODCACHE=$gomodcache \
      REAL_MV=$real_mv \
      PATH=$work/fake-bin:$PATH \
      mise run install >/dev/null
  )
  test "$(sed -n 's/^channel=//p' "$state")" = dev
  assert_output "$tool vdev © 2026 silee-tools" \
    "$home/.local/bin/$tool" --version
  for command in $aliases; do
    assert_output "$command vdev © 2026 silee-tools" \
      "$home/.local/bin/$command" --version
  done

  printf 'OK %s: dev -> release -> dev\n' "$tool"
}

case $(uname -s) in
Darwin) host_goos=darwin ;;
Linux) host_goos=linux ;;
*)
  printf '%s\n' "unsupported host: $(uname -s)" >&2
  exit 1
  ;;
esac

for app_dir in "$repo_root"/apps/*; do
  [ -d "$app_dir" ] || continue
  [ -f "$app_dir/.mise.toml" ] || continue
  [ -f "$app_dir/.goreleaser.yaml" ] || continue
  tool=${app_dir##*/}
  if grep -Eq "^[[:space:]]*-[[:space:]]*${host_goos}[[:space:]]*$" \
    "$app_dir/.goreleaser.yaml"; then
    check_tool "$tool"
  else
    printf 'SKIP %s: %s is not a release target\n' "$tool" "$host_goos"
  fi
done
