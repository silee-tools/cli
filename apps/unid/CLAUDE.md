# unid

## Development

Run `mise tasks` to list available tasks.

## Guide 예제 관리

- `printGuide()` 예제 DSL 변경 시 OUTPUT 영역을 실제 `go run ./cmd/unid` 결과로 교체 (필수)
- 예제의 다양성 유지: 모든 테두리 스타일, 다양한 arrow 형태, CJK 텍스트, overflow 데모 포함

## DSL 옵션 규칙

- 새 속성 추가 시 shorthand(약어) 필수 (예: `align` / `a`, `content` / `c`, `legend` / `lg`)

## 코드 변경 시 문서 업데이트

- DSL 문법, 옵션, 커맨드 변경 시 `printGuide()` 업데이트 검토 (필수)
- CLI 인터페이스, 서브커맨드 변경 시 help 메시지 업데이트 검토 (필수)
