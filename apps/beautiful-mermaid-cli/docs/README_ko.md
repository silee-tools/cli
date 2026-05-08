# beautiful-mermaid-cli

[English (영어)](../README.md)

Mermaid 다이어그램을 ASCII 아트로 렌더링하는 beautiful-mermaid 기반 경량 CLI 도구.

## 기술 스택

- 런타임: [Bun](https://bun.sh/)
- 언어: TypeScript
- 코어: [beautiful-mermaid](https://github.com/nicholasgasior/beautiful-mermaid)

## 설치

```bash
brew install silee-tools/tap/bmm
```

## 시작하기

```bash
# stdin으로 렌더링
printf 'graph LR\n  A --> B\n  B --> C\n' | bmm

# 파일 렌더링
bmm diagram.mmd

# SVG 출력
bmm --svg diagram.mmd

# 순수 ASCII (Unicode 박스 드로잉 미사용)
bmm --ascii diagram.mmd

# Mermaid 문법 팁 포함 도움말
bmm --help
```

## 개발

```bash
bun install
printf 'graph LR\n  A --> B\n' | bun src/cli.ts
bun link  # 소스에서 글로벌 설치
```
