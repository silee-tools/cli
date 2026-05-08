#!/usr/bin/env bash

# appback 경로
APPBACK_BIN="$BATS_TEST_DIRNAME/../appback"

# 테스트용 임시 디렉토리
setup() {
  TEST_TMPDIR="$(mktemp -d)"
  export TEST_TMPDIR
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}
