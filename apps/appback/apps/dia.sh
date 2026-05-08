# shellcheck shell=bash
# This file is sourced by appback — variables are used by the caller
# shellcheck disable=SC2034

APP_NAME="Dia"
BUNDLE_ID="company.thebrowser.dia"
PROCESS_NAME="Dia"

# 백업할 경로 (~ 기준 상대경로)
BACKUP_PATHS=(
  "Library/Application Support/Dia"
  "Library/Preferences/company.thebrowser.dia.plist"
)

# 제외할 경로
EXCLUDE_PATHS=(
  "Library/Caches/company.thebrowser.dia"
)

# 키체인 항목 (service:account 형식)
KEYCHAIN_ITEMS=(
  "Dia Safe Storage:Dia"
)
