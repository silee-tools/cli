APP_NAME="Raycast"
BUNDLE_ID="com.raycast.macos"
PROCESS_NAME="Raycast"

BACKUP_PATHS=(
  "Library/Application Support/com.raycast.macos"
  "Library/Application Support/com.raycast.shared"
  "Library/Preferences/com.raycast.macos.plist"
)

EXCLUDE_PATHS=(
  "Library/Caches/com.raycast.macos"
)

KEYCHAIN_ITEMS=(
  "Raycast:database_key"
)
