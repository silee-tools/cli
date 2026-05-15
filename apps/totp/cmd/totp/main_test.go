package main

import (
	"bytes"
	"testing"
)

// TestRunVersion 은 -v / --version 입력 시 표준 출력에 모노레포 공통 형식
// "totp v<버전> © 2026 silee-tools" 한 줄을 찍고 에러 없이 끝나는지 검증한다.
// 테스트 빌드에서 version 변수는 기본값 "dev" 이다.
func TestRunVersion(t *testing.T) {
	for _, flag := range []string{"-v", "--version"} {
		var stdout, stderr bytes.Buffer
		if err := run([]string{flag}, nil, &stdout, &stderr); err != nil {
			t.Fatalf("%s: run returned error: %v", flag, err)
		}
		want := "totp vdev © 2026 silee-tools\n"
		if got := stdout.String(); got != want {
			t.Errorf("%s: stdout = %q, want %q", flag, got, want)
		}
		if stderr.Len() != 0 {
			t.Errorf("%s: stderr = %q, want empty", flag, stderr.String())
		}
	}
}
