#!/usr/bin/env bats

load helpers/setup

# === app definition loading ===

@test "list shows available apps" {
  run "$APPBACK_BIN" list
  [ "$status" -eq 0 ]
  [[ "$output" == *"dia"* ]] || [[ "$output" == *"Dia"* ]]
}

@test "export nonexistent app returns error" {
  run "$APPBACK_BIN" export nonexistent --non-interactive
  [ "$status" -ne 0 ]
  [[ "$output" == *"not found"* ]] || [[ "$output" == *"찾을 수 없습니다"* ]]
}

# === help / version ===

@test "help shows usage info" {
  run "$APPBACK_BIN" --help
  [ "$status" -eq 0 ]
  [[ "$output" == *"export"* ]]
  [[ "$output" == *"import"* ]]
  [[ "$output" == *"list"* ]]
}

@test "no args shows help" {
  run "$APPBACK_BIN"
  [ "$status" -eq 0 ]
  [[ "$output" == *"export"* ]]
}

@test "version shows semver" {
  run "$APPBACK_BIN" --version
  [ "$status" -eq 0 ]
  [[ "$output" =~ [0-9]+\.[0-9]+\.[0-9]+ ]]
}

@test "version is 0.2.3" {
  run "$APPBACK_BIN" --version
  [ "$status" -eq 0 ]
  [[ "$output" == *"0.2.3"* ]]
}

# === completions ===

@test "--completions outputs completion script" {
  run "$APPBACK_BIN" --completions
  [ "$status" -eq 0 ]
  [[ "$output" == *"appback"* ]]
}

@test "zsh completion uses compdef instead of direct call" {
  local comp_file="$BATS_TEST_DIRNAME/../completions/_appback"
  # _appback "$@" 직접 호출이 없어야 함 (eval 시 에러 발생 원인)
  run grep -c '^_appback "\$@"' "$comp_file"
  [ "$output" = "0" ]
  # compdef로 등록해야 함
  run grep -c 'compdef _appback appback' "$comp_file"
  [ "$output" != "0" ]
}

@test "zsh completion uses state-based subcommand handling" {
  local comp_file="$BATS_TEST_DIRNAME/../completions/_appback"
  # _arguments -C 사용 (subcommand 컨텍스트 전환)
  run grep -c '_arguments -C' "$comp_file"
  [ "$output" != "0" ]
  # state 기반 분기
  run grep -c 'case \$state' "$comp_file"
  [ "$output" != "0" ]
}

@test "bash completion uses subcmd for subcommand detection" {
  local comp_file="$BATS_TEST_DIRNAME/../completions/appback.bash"
  # COMP_WORDS[1]로 subcommand 판단
  run grep -c 'COMP_WORDS\[1\]' "$comp_file"
  [ "$output" != "0" ]
}

@test "install-completions is listed in help" {
  run "$APPBACK_BIN" --help
  [ "$status" -eq 0 ]
  [[ "$output" == *"install-completions"* ]]
}

# === export ===

@test "export creates backup archive" {
  run "$APPBACK_BIN" export dia --output "$TEST_TMPDIR" --no-keychain --non-interactive
  [ "$status" -eq 0 ]
  ls "$TEST_TMPDIR"/dia-backup-*.tar.gz 2>/dev/null
  [ "$?" -eq 0 ]
}

@test "export fails for nonexistent app" {
  run "$APPBACK_BIN" export foobar --non-interactive
  [ "$status" -ne 0 ]
}

@test "export fails for invalid output dir" {
  run "$APPBACK_BIN" export dia --output /nonexistent/path --non-interactive
  [ "$status" -ne 0 ]
}

# === import ===

@test "import fails for missing backup file" {
  run "$APPBACK_BIN" import dia /nonexistent/backup.tar.gz --non-interactive
  [ "$status" -ne 0 ]
}

# === integration: export -> import roundtrip ===

@test "export then import roundtrip preserves files" {
  # 테스트용 가짜 앱 정의 생성
  local fake_app_dir="$TEST_TMPDIR/fake_home/Library/Application Support/TestApp"
  local fake_plist_dir="$TEST_TMPDIR/fake_home/Library/Preferences"
  mkdir -p "$fake_app_dir" "$fake_plist_dir"
  echo "test-bookmark-data" > "$fake_app_dir/Bookmarks"
  echo "test-pref-data" > "$fake_plist_dir/com.test.app.plist"

  # 테스트용 앱 정의
  local test_app_file="$BATS_TEST_DIRNAME/../apps/_test.sh"
  cat > "$test_app_file" <<'APPDEF'
# shellcheck shell=bash
# shellcheck disable=SC2034
APP_NAME="TestApp"
BUNDLE_ID="com.test.app"
PROCESS_NAME="TestAppProcess"
BACKUP_PATHS=(
  "Library/Application Support/TestApp"
  "Library/Preferences/com.test.app.plist"
)
EXCLUDE_PATHS=()
KEYCHAIN_ITEMS=()
APPDEF

  # HOME을 가짜 경로로 변경하여 export
  local backup_dir="$TEST_TMPDIR/backups"
  mkdir -p "$backup_dir"
  MISE_TRUSTED_CONFIG_PATHS="$BATS_TEST_DIRNAME/.." HOME="$TEST_TMPDIR/fake_home" run "$APPBACK_BIN" export _test --output "$backup_dir" --no-keychain --non-interactive
  [ "$status" -eq 0 ]

  local backup_file
  backup_file=$(ls "$backup_dir"/_test-backup-*.tar.gz 2>/dev/null | head -1)
  [ -n "$backup_file" ]

  # 다른 HOME으로 import
  local restore_home="$TEST_TMPDIR/restore_home"
  mkdir -p "$restore_home"
  MISE_TRUSTED_CONFIG_PATHS="$BATS_TEST_DIRNAME/.." HOME="$restore_home" run "$APPBACK_BIN" import _test "$backup_file" --non-interactive
  [ "$status" -eq 0 ]

  # 파일이 복원되었는지 확인
  [ -f "$restore_home/Library/Application Support/TestApp/Bookmarks" ]
  [ -f "$restore_home/Library/Preferences/com.test.app.plist" ]

  # 내용이 일치하는지 확인
  [ "$(cat "$restore_home/Library/Application Support/TestApp/Bookmarks")" = "test-bookmark-data" ]
  [ "$(cat "$restore_home/Library/Preferences/com.test.app.plist")" = "test-pref-data" ]

  # 테스트용 앱 정의 정리
  rm -f "$test_app_file"
}
