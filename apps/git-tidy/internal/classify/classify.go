// Package classify 는 하이브리드 모델로 로컬 브랜치를 삭제 대상과 그 외로 나눈다.
package classify

import "github.com/silee-tools/git-tidy/internal/gitx"

// Signal 은 브랜치가 삭제 후보가 된 이유다.
type Signal string

const (
	SignalGone   Signal = "gone"
	SignalMerged Signal = "merged"
	SignalStale  Signal = "stale"
)

// Input 은 분류에 필요한 모든 데이터다. git 호출과 분리해 순수 함수로 둔다.
type Input struct {
	Now           int64
	StaleDays     int
	Base          string
	Current       string
	Branches      []gitx.BranchRef
	Merged        map[string]bool   // base 에 머지된 브랜치
	Worktrees     map[string]string // 브랜치 → worktree 경로
	MergeBaseUnix func(branch string) (int64, bool)
}

// Result 는 삭제 대상 브랜치 하나다.
type Result struct {
	Name         string
	Signal       Signal
	WorktreePath string // worktree 에 물려 있으면 그 경로, 아니면 빈 문자열
}

// Classified 는 분류 결과다.
type Classified struct {
	ToDelete   []Result
	OtherCount int // 후보도 아니고 보호도 아닌 평범한 브랜치 수
}

// Classify 는 Input 을 받아 삭제 대상을 가려낸다.
func Classify(in Input) Classified {
	var out Classified
	cutoff := in.Now - int64(in.StaleDays)*86400
	for _, b := range in.Branches {
		// 보호 규칙: 현재 브랜치, base 브랜치는 절대 삭제하지 않는다.
		if b.Name == in.Current || b.Name == in.Base {
			continue
		}
		sig, isCandidate := candidateSignal(b, in, cutoff)
		if !isCandidate {
			out.OtherCount++
			continue
		}
		out.ToDelete = append(out.ToDelete, Result{
			Name:         b.Name,
			Signal:       sig,
			WorktreePath: in.Worktrees[b.Name],
		})
	}
	return out
}

// candidateSignal 은 브랜치의 첫 삭제 후보 신호를 돌려준다.
func candidateSignal(b gitx.BranchRef, in Input, cutoff int64) (Signal, bool) {
	if b.UpstreamGone {
		return SignalGone, true
	}
	if in.Merged[b.Name] {
		return SignalMerged, true
	}
	if isStale(b, in, cutoff) {
		return SignalStale, true
	}
	return "", false
}

// isStale 은 마지막 커밋 또는 분기점이 stale 창보다 오래됐는지 본다(OR).
func isStale(b gitx.BranchRef, in Input, cutoff int64) bool {
	if b.CommitUnix > 0 && b.CommitUnix < cutoff {
		return true
	}
	if mb, ok := in.MergeBaseUnix(b.Name); ok && mb < cutoff {
		return true
	}
	return false
}
