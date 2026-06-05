package reason

func Description(signal string) string {
	switch signal {
	case "gone":
		return "upstream 추적 브랜치가 사라진 로컬 브랜치"
	case "merged":
		return "base 브랜치에 브랜치 커밋이 그대로 들어간 로컬 브랜치"
	case "absorbed":
		return "같은 Jira 티켓의 더 최신 base 커밋이 있고, 지금 worktree 에서 작업 중이지 않은 로컬 브랜치"
	case "stale":
		return "마지막 커밋 또는 분기점이 stale 기준일보다 오래된 로컬 브랜치"
	default:
		return ""
	}
}
