#!/usr/bin/env python3
"""모노레포 릴리스 후 homebrew-tap formula 의 sha256 placeholder 를 실제 값으로 갱신.

호출 형태:
    update-homebrew-formula.py <formula-path> <checksums-path>

동작:
1. checksums.txt 를 파싱하여 {filename: sha256} 매핑 구성
2. formula 의 각 url 라인 바로 다음에 오는 sha256 라인을 갱신
   - url 의 마지막 path 컴포넌트(파일명) 가 checksums 매핑의 키와 일치하면 교체
3. 갱신된 라인 수를 stdout 으로 보고
4. 매칭되는 url 이 없으면 에러 + exit 1

저장소 원칙(안정성) 적용:
- 정규식이 아니라 라인 기반 파싱 (formula 형식 변동에 둔감)
- 멱등성: 이미 sha256 이 갱신되어 있어도(=placeholder 가 아니어도) 동일 결과
- 표준 라이브러리만 사용 (의존성 0)
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

PLACEHOLDER_PREFIX = '"0' * 1  # placeholder sha256 시작은 "0000..."


def parse_checksums(path: Path) -> dict[str, str]:
    """checksums.txt 형식: '<sha256>  <filename>' 라인 모음."""
    mapping: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split()
        if len(parts) < 2:
            continue
        sha = parts[0].lower()
        filename = parts[-1].lstrip("*")  # GNU coreutils 의 binary 마커 '*' 제거
        mapping[filename] = sha
    return mapping


def url_filename(url: str) -> str:
    """formula url 라인에서 마지막 path 컴포넌트 추출."""
    # 예: url "https://.../jg-v0.1.28-darwin-arm64.tar.gz" → jg-v0.1.28-darwin-arm64.tar.gz
    m = re.search(r'"([^"]+)"', url)
    if not m:
        return ""
    full = m.group(1)
    return full.rsplit("/", 1)[-1]


def update_formula(formula_path: Path, checksums: dict[str, str]) -> int:
    """formula 의 sha256 라인을 갱신하고 변경된 라인 수 반환."""
    lines = formula_path.read_text(encoding="utf-8").splitlines(keepends=True)
    updated = 0
    pending_filename: str | None = None

    for i, line in enumerate(lines):
        stripped = line.strip()
        if stripped.startswith("url "):
            pending_filename = url_filename(stripped)
            continue
        if pending_filename and stripped.startswith("sha256 "):
            sha = checksums.get(pending_filename)
            if sha is None:
                print(
                    f"WARN: '{pending_filename}' 에 대응하는 sha256 가 checksums 에 없음 — skip",
                    file=sys.stderr,
                )
                pending_filename = None
                continue
            # 들여쓰기 보존하며 교체
            indent = line[: len(line) - len(line.lstrip())]
            new_line = f'{indent}sha256 "{sha}"\n'
            if line != new_line:
                lines[i] = new_line
                updated += 1
            pending_filename = None

    if updated:
        formula_path.write_text("".join(lines), encoding="utf-8")
    return updated


def main() -> int:
    if len(sys.argv) != 3:
        print(f"usage: {sys.argv[0]} <formula-path> <checksums-path>", file=sys.stderr)
        return 2

    formula_path = Path(sys.argv[1])
    checksums_path = Path(sys.argv[2])

    if not formula_path.is_file():
        print(f"formula 파일 없음: {formula_path}", file=sys.stderr)
        return 1
    if not checksums_path.is_file():
        print(f"checksums 파일 없음: {checksums_path}", file=sys.stderr)
        return 1

    checksums = parse_checksums(checksums_path)
    if not checksums:
        print(f"checksums 가 비어 있음: {checksums_path}", file=sys.stderr)
        return 1

    updated = update_formula(formula_path, checksums)
    print(f"{formula_path.name}: {updated} sha256 라인 갱신")
    if updated == 0:
        print(
            "WARN: 갱신된 라인이 0건 — url 패턴이 checksums 와 매칭되지 않거나 이미 최신",
            file=sys.stderr,
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
