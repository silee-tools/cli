# unid (Unicode Diagram)

Unicode box-drawing 문자를 활용한 ASCII 다이어그램 렌더링 CLI 도구.

Monodraw, ASCIIFlow 같은 도구의 텍스트 기반 대안입니다.
간단한 DSL을 stdin으로 입력하면 정확한 Unicode box-drawing 다이어그램을 렌더링합니다.

[English](../README.md)

## 설치

### Homebrew (macOS)

```sh
brew install silee-tools/tap/unid
```

### 소스 빌드

```sh
git clone https://github.com/silee-tools/unid.git
cd unid
go build -o unid ./cmd/unid
# $GOPATH/bin에 설치:
go install ./cmd/unid
```

## 사용법

```sh
echo "..." | unid          # stdin으로 다이어그램 렌더링
echo "..." | unid list     # 다이어그램 내 객체 목록 출력
echo "..." | unid lint     # DSL 입력 검증
unid guide                 # 사용 가이드 출력
```

## 예시

```sh
echo 'canvas auto
collision off
box 0 0 10 3 id=a c=Client
box 16 0 10 3 id=b c=Server
box 32 0 8 3 id=c c=DB
arrow a.r b.l
arrow b.r c.l' | unid
```

```
┌──────────┐    ┌──────────┐    ┌────────┐
│          │    │          │    │        │
│Client    │───▶│Server    │───▶│DB      │
│          │    │          │    │        │
└──────────┘    └──────────┘    └────────┘
```

## 특징

- **자동 너비 계산** — CJK 문자(한글, 漢字, かな) 포함 코딩 폰트 기준 display-column 계산
- **다양한 테두리 스타일** — single, double, bold, round, dashed 등
- **DSL 기반** — 텍스트로 다이어그램을 선언적으로 정의
- **Lint 지원** — DSL 입력의 오류와 경고를 검출

자세한 사용법은 `unid guide`를 참조하세요.

## AI와 함께 사용

```
unid 명령어를 사용해서 시스템 아키텍처를 그려줘. unid guide를 참고하면 돼.
```

## 라이선스

[MIT](../LICENSE)
