# mydesk

## Development

Run `mise tasks` to list available tasks.

## 핵심 개념

- **네이티브 디렉토리**: home/, config/, ssh/, vscode/, brew/, macos/, omz/ — 자동 감지
- **links.conf**: 네이티브로 커버되지 않는 추가 심볼릭 링크 매핑
- **설정 레포**: 사용자가 별도 레포에서 관리하는 dotfiles (이 레포가 아님)
- 글로벌 플래그는 서브커맨드 앞에 위치: `mydesk --dry-run link`
