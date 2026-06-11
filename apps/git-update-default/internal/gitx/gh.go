package gitx

import (
	"bytes"
	"os/exec"
	"strings"
)

// GitHubDefault 는 gh CLI 로 현재 저장소의 GitHub default branch 를 조회한다.
// gh 가 없거나, 인증되지 않았거나, GitHub 저장소가 아니면 ok=false 를 돌려준다.
func GitHubDefault() (string, bool) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", false
	}
	cmd := exec.Command("gh", "repo", "view", "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", false
	}
	name := strings.TrimSpace(stdout.String())
	return name, name != ""
}
