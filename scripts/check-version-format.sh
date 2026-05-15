#!/usr/bin/env bash
#
# 모노레포 전 도구 --version 형식 conformance 게이트.
#
# apps/<tool>/ 을 자동 순회하여, 각 도구가 `-v` 와 `--version` 입력 시
# 표준 출력에 정확히 다음 한 줄을 찍고 종료 코드 0 으로 끝나는지 검증한다.
#
#   <tool> v<version> © 2026 silee-tools
#
# 도구 이름을 하드코딩하지 않으므로 apps/<new-tool>/ 추가만으로 자동 검사
# 대상이 된다. OS 라우팅의 단일 진실 소스는 각 도구의 .goreleaser.yaml 이다:
#   - builds 가 skip:true  → 순수 zsh 플러그인. 어느 OS 에서나 source 후 실행.
#   - goos 섹션에 linux    → ubuntu leg 에서 검증.
#   - goos 가 darwin 전용  → macos leg 에서 검증.
#
# leg 분기:
#   - 환경변수 CONFORMANCE_LEG=ubuntu|macos 가 설정되면(CI 매트릭스) 해당
#     leg 대상 도구만 검사하고, 다른 leg 대상은 "covered on <leg> leg" 로
#     명시 출력하여 커버리지가 눈에 보이게 한다(조용한 skip 금지).
#   - 미설정(로컬)이면 현재 호스트 OS 가 실행 가능한 도구를 모두 검사한다.
#     macOS 호스트는 네 도구 모두, Linux 호스트는 ubuntu leg + zsh 도구.

set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

expected_suffix=" © 2026 silee-tools"

host_os="unknown"
case "$(uname -s)" in
  Darwin) host_os="macos" ;;
  Linux)  host_os="ubuntu" ;;
esac

leg="${CONFORMANCE_LEG:-}"

fail=0
checked=0

# 도구가 어느 leg 에서 검증되어야 하는지 .goreleaser.yaml 로 결정한다.
required_leg_of() {
  local gr="$1"
  if grep -qE 'skip:[[:space:]]*true' "$gr"; then
    echo "ubuntu"   # 순수 zsh 플러그인
    return
  fi
  # goos: 부터 goarch: 직전까지를 goos 섹션으로 보고 그 안에서만 linux 를 찾는다
  # (파일 주석의 "Linux" 대문자 표기에 오탐하지 않도록 grep -w 는 대소문자 구분).
  local goos_section
  goos_section="$(sed -n '/^[[:space:]]*goos:/,/^[[:space:]]*goarch:/p' "$gr")"
  if printf '%s\n' "$goos_section" | grep -qw 'linux'; then
    echo "ubuntu"
  else
    echo "macos"
  fi
}

# 한 출력 문자열이 표준 형식인지 검사하고 결과를 집계한다.
check_output() {
  local tool="$1" flag="$2" out="$3" rc="$4"
  local pattern="^${tool} v[^[:space:]]+${expected_suffix}\$"
  if [[ $rc -ne 0 ]]; then
    echo "  FAIL ${tool} ${flag}: 종료 코드 ${rc} (기대 0), 출력=[${out}]"
    fail=1
    return
  fi
  if [[ ! "$out" =~ $pattern ]]; then
    echo "  FAIL ${tool} ${flag}: 형식 불일치"
    echo "       기대 정규식: ${pattern}"
    echo "       실제 출력  : [${out}]"
    fail=1
    return
  fi
  echo "  OK   ${tool} ${flag}: ${out}"
}

# Go 도구: mise 로 go 런타임을 해석해 1회 빌드 후 두 플래그를 검증한다.
assert_go() {
  local tool="$1" bin out rc f
  bin="$(mktemp -d)/$tool"
  if ! (cd "apps/$tool" && mise exec -- go build -o "$bin" "./cmd/$tool") >/dev/null 2>&1; then
    echo "  FAIL ${tool}: go build 실패 (apps/${tool}/cmd/${tool})"
    fail=1
    checked=$((checked + 1))
    return
  fi
  for f in -v --version; do
    out="$("$bin" "$f" 2>&1)"; rc=$?
    check_output "$tool" "$f" "$out" "$rc"
  done
  checked=$((checked + 1))
}

# zsh 플러그인 도구: 비대화형 zsh 에서 compdef 를 무력화하고 source 후 실행.
# (compdef 는 compinit 가 로드된 대화형 zsh 에서만 존재하므로 하니스에서 스텁)
assert_zsh() {
  local tool="$1" out rc f
  for f in -v --version; do
    out="$(zsh -c "compdef() { :; }; source 'apps/$tool/$tool.plugin.zsh'; $tool $f" 2>&1)"; rc=$?
    check_output "$tool" "$f" "$out" "$rc"
  done
  checked=$((checked + 1))
}

echo "== version-format conformance =="
echo "host_os=${host_os} leg=${leg:-<local-all>}"
echo

for dir in apps/*/; do
  tool="$(basename "$dir")"
  gr="$dir/.goreleaser.yaml"

  if [[ ! -f "$gr" ]]; then
    echo "  FAIL ${tool}: .goreleaser.yaml 없음 (OS 라우팅 SSOT 누락)"
    fail=1
    continue
  fi

  # 도구 종류 판별: cmd/<tool>/main.go 가 있으면 Go, 없고 <tool>.plugin.zsh
  # 가 패키지 루트에 있으면 zsh 플러그인.
  kind=""
  if [[ -f "${dir}cmd/${tool}/main.go" ]]; then
    kind="go"
  elif [[ -f "${dir}${tool}.plugin.zsh" ]]; then
    kind="zsh"
  else
    echo "  FAIL ${tool}: Go(cmd/${tool}/main.go) 도 zsh(${tool}.plugin.zsh) 도 아님"
    fail=1
    continue
  fi

  rleg="$(required_leg_of "$gr")"

  # 이번 실행에서 이 도구를 검사할지 결정.
  if [[ -n "$leg" ]]; then
    if [[ "$rleg" != "$leg" ]]; then
      echo "  ---- ${tool}: covered on ${rleg} leg (skip on ${leg})"
      continue
    fi
  else
    # 로컬: 호스트가 실행 가능한지 확인. macos 호스트는 전부 가능,
    # ubuntu 호스트는 macos 전용 도구를 검사할 수 없다.
    if [[ "$rleg" == "macos" && "$host_os" != "macos" ]]; then
      echo "  ---- ${tool}: darwin 전용, 현재 호스트(${host_os})에서 검증 불가"
      continue
    fi
  fi

  if [[ "$kind" == "go" ]]; then
    assert_go "$tool"
  else
    assert_zsh "$tool"
  fi
done

echo
if [[ $fail -ne 0 ]]; then
  echo "RESULT: FAIL (검사 도구 ${checked}개 중 위반 존재)"
  exit 1
fi
echo "RESULT: PASS (검사 도구 ${checked}개 전원 통과)"
