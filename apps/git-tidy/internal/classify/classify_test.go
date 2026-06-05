package classify

import (
	"reflect"
	"testing"

	"github.com/silee-tools/git-tidy/internal/gitx"
)

func TestClassify(t *testing.T) {
	now := int64(1_000_000_000)
	staleDays := 20
	day := int64(86400)
	old := now - 30*day   // stale 창 밖
	recent := now - 5*day // stale 창 안

	in := Input{
		Now:       now,
		StaleDays: staleDays,
		Base:      "main",
		Current:   "feature-current",
		Branches: []gitx.BranchRef{
			{Name: "feature-current", CommitUnix: old},                                        // 보호: 현재 브랜치
			{Name: "main", CommitUnix: old},                                                   // 보호: base
			{Name: "feature-gone", UpstreamGone: true, HasUpstream: true, CommitUnix: recent}, // 후보: gone
			{Name: "feature-merged", HasUpstream: true, CommitUnix: recent},                   // 후보: merged
			{Name: "feature-stale", HasUpstream: true, CommitUnix: old},                       // 후보: stale(마지막 커밋)
			{Name: "feature-active", HasUpstream: true, CommitUnix: recent},                   // 후보 아님
		},
		Merged:        map[string]bool{"feature-merged": true, "main": true},
		Worktrees:     map[string]string{},
		MergeBaseUnix: func(branch string) (int64, bool) { return recent, true }, // 분기점은 전부 최근
	}

	got := Classify(in)

	wantDelete := []Result{
		{Name: "feature-gone", Signal: SignalGone},
		{Name: "feature-merged", Signal: SignalMerged},
		{Name: "feature-stale", Signal: SignalStale, AgeDays: 30},
	}
	if !reflect.DeepEqual(got.ToDelete, wantDelete) {
		t.Errorf("ToDelete mismatch\n got=%+v\nwant=%+v", got.ToDelete, wantDelete)
	}
	if got.OtherCount != 1 { // feature-active
		t.Errorf("OtherCount = %d, want 1", got.OtherCount)
	}
	wantExcluded := []Excluded{
		{Name: "feature-current", Signal: SignalStale, Reason: "현재 브랜치"},
		{Name: "main", Signal: SignalMerged, Reason: "base 브랜치"},
	}
	if !reflect.DeepEqual(got.Excluded, wantExcluded) {
		t.Errorf("Excluded mismatch\n got=%+v\nwant=%+v", got.Excluded, wantExcluded)
	}
}

func TestClassifySortsBySignalThenName(t *testing.T) {
	now := int64(1_000_000_000)
	day := int64(86400)
	old := now - 40*day
	in := Input{
		Now:       now,
		StaleDays: 20,
		Base:      "main",
		Current:   "",
		Branches: []gitx.BranchRef{
			{Name: "zzz-stale", HasUpstream: true, CommitUnix: old},
			{Name: "aaa-stale", HasUpstream: true, CommitUnix: old},
			{Name: "mmm-gone", UpstreamGone: true, HasUpstream: true, CommitUnix: now},
			{Name: "ccc-merged", HasUpstream: true, CommitUnix: now},
			{Name: "bbb-absorbed", HasUpstream: true, CommitUnix: now - 10*day, Subject: "[ABC-1375] docs: absorbed branch"},
		},
		Merged:    map[string]bool{"ccc-merged": true},
		Worktrees: map[string]string{},
		BaseCommits: []gitx.CommitRef{
			{ShortHash: "9a640b52f", Subject: "[ABC-1375] feat: absorbed base", CommitUnix: now - day},
		},
		MergeBaseUnix: func(string) (int64, bool) { return now, true },
	}
	got := Classify(in)
	want := []Result{
		{Name: "mmm-gone", Signal: SignalGone},
		{Name: "ccc-merged", Signal: SignalMerged},
		{
			Name:                 "bbb-absorbed",
			Signal:               SignalAbsorbed,
			AbsorbedByShortHash:  "9a640b52f",
			AbsorbedBySubject:    "[ABC-1375] feat: absorbed base",
			AbsorbedByCommitUnix: now - day,
		},
		{Name: "aaa-stale", Signal: SignalStale, AgeDays: 40},
		{Name: "zzz-stale", Signal: SignalStale, AgeDays: 40},
	}
	if !reflect.DeepEqual(got.ToDelete, want) {
		t.Errorf("정렬 결과 mismatch\n got=%+v\nwant=%+v", got.ToDelete, want)
	}
}

func TestClassifyAgeDaysNeverNegative(t *testing.T) {
	now := int64(1_000_000_000)
	day := int64(86400)
	old := now - 40*day
	future := now + 10*day
	in := Input{
		Now:       now,
		StaleDays: 20,
		Base:      "main",
		Current:   "",
		Branches: []gitx.BranchRef{
			// 분기점은 오래돼 stale 로 판정되지만, 마지막 커밋 시각이 미래라
			// AgeDays 계산이 음수가 될 수 있는 경우다.
			{Name: "future-stale", HasUpstream: true, CommitUnix: future},
		},
		Merged:        map[string]bool{},
		Worktrees:     map[string]string{},
		MergeBaseUnix: func(string) (int64, bool) { return old, true },
	}
	got := Classify(in)
	want := []Result{
		{Name: "future-stale", Signal: SignalStale, AgeDays: 0},
	}
	if !reflect.DeepEqual(got.ToDelete, want) {
		t.Errorf("미래 시각 AgeDays mismatch\n got=%+v\nwant=%+v", got.ToDelete, want)
	}
}

func TestClassifyAgeFromMergeBaseFallback(t *testing.T) {
	now := int64(1_000_000_000)
	day := int64(86400)
	old := now - 50*day // stale 창(20일) 밖, merge-base 시각
	in := Input{
		Now:       now,
		StaleDays: 20,
		Base:      "main",
		Current:   "",
		Branches: []gitx.BranchRef{
			{Name: "no-commitdate", HasUpstream: true, CommitUnix: 0}, // 커밋 시각 없음
		},
		Merged:        map[string]bool{},
		Worktrees:     map[string]string{},
		MergeBaseUnix: func(string) (int64, bool) { return old, true }, // merge-base 는 50일 전
	}
	got := Classify(in)
	if len(got.ToDelete) != 1 {
		t.Fatalf("stale 후보 1개 기대, got %+v", got.ToDelete)
	}
	if got.ToDelete[0].Signal != SignalStale {
		t.Errorf("stale 신호 기대, got %s", got.ToDelete[0].Signal)
	}
	if got.ToDelete[0].AgeDays != 50 {
		t.Errorf("merge-base 기준 경과 50일 기대, got %d", got.ToDelete[0].AgeDays)
	}
}

func TestClassifyAbsorbedWhenNewerBaseCommitHasSameTicket(t *testing.T) {
	now := int64(1_000_000_000)
	branchTime := now - 10*86400
	baseTime := now - 2*86400
	in := Input{
		Now:       now,
		StaleDays: 20,
		Base:      "main",
		Current:   "",
		Branches: []gitx.BranchRef{
			{Name: "claude/example-absorbed-branch", CommitUnix: branchTime, Subject: "[ABC-1375] docs: 새 git worktree 셋업 안내 추가"},
		},
		Merged:    map[string]bool{},
		Worktrees: map[string]string{},
		BaseCommits: []gitx.CommitRef{
			{ShortHash: "9a640b52f", Subject: "[ABC-1375] feat: 새 worktree 셋업 자동화 스크립트 + 안내 문서", CommitUnix: baseTime},
		},
		MergeBaseUnix: func(string) (int64, bool) { return now, true },
	}

	got := Classify(in)
	want := []Result{
		{
			Name:                 "claude/example-absorbed-branch",
			Signal:               SignalAbsorbed,
			AbsorbedByShortHash:  "9a640b52f",
			AbsorbedBySubject:    "[ABC-1375] feat: 새 worktree 셋업 자동화 스크립트 + 안내 문서",
			AbsorbedByCommitUnix: baseTime,
		},
	}
	if !reflect.DeepEqual(got.ToDelete, want) {
		t.Errorf("absorbed 후보 mismatch\n got=%+v\nwant=%+v", got.ToDelete, want)
	}
}

func TestClassifyDoesNotAbsorbCheckedOutWorktreeBranch(t *testing.T) {
	now := int64(1_000_000_000)
	branchTime := now - 10*86400
	in := Input{
		Now:       now,
		StaleDays: 20,
		Base:      "main",
		Current:   "",
		Branches: []gitx.BranchRef{
			{Name: "claude/in-progress", CommitUnix: branchTime, Subject: "[ABC-1375] docs: worktree 안내"},
		},
		Merged:    map[string]bool{},
		Worktrees: map[string]string{"claude/in-progress": "/tmp/worktree"},
		BaseCommits: []gitx.CommitRef{
			{ShortHash: "9a640b52f", Subject: "[ABC-1375] feat: 새 worktree 셋업", CommitUnix: now - 2*86400},
		},
		MergeBaseUnix: func(string) (int64, bool) { return now, true },
	}

	got := Classify(in)
	if len(got.ToDelete) != 0 {
		t.Fatalf("worktree 에 체크아웃된 브랜치는 absorbed 후보가 아니어야 함: %+v", got.ToDelete)
	}
	if got.OtherCount != 1 {
		t.Fatalf("worktree 브랜치는 일반 브랜치로 남아야 함, OtherCount=%d", got.OtherCount)
	}
}

func TestClassifyDoesNotAbsorbWhenBaseCommitIsOlder(t *testing.T) {
	now := int64(1_000_000_000)
	branchTime := now - 2*86400
	in := Input{
		Now:       now,
		StaleDays: 20,
		Base:      "main",
		Current:   "",
		Branches: []gitx.BranchRef{
			{Name: "claude/newer-branch", CommitUnix: branchTime, Subject: "[ABC-1375] docs: 최신 브랜치 작업"},
		},
		Merged:    map[string]bool{},
		Worktrees: map[string]string{},
		BaseCommits: []gitx.CommitRef{
			{ShortHash: "111111111", Subject: "[ABC-1375] docs: 오래된 base 작업", CommitUnix: now - 10*86400},
		},
		MergeBaseUnix: func(string) (int64, bool) { return now, true },
	}

	got := Classify(in)
	if len(got.ToDelete) != 0 {
		t.Fatalf("base 관련 커밋이 더 오래되면 absorbed 후보가 아니어야 함: %+v", got.ToDelete)
	}
	if got.OtherCount != 1 {
		t.Fatalf("브랜치는 일반 브랜치로 남아야 함, OtherCount=%d", got.OtherCount)
	}
}
