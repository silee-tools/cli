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
		},
		Merged:        map[string]bool{},
		Worktrees:     map[string]string{},
		MergeBaseUnix: func(string) (int64, bool) { return now, true },
	}
	got := Classify(in)
	want := []Result{
		{Name: "mmm-gone", Signal: SignalGone},
		{Name: "aaa-stale", Signal: SignalStale, AgeDays: 40},
		{Name: "zzz-stale", Signal: SignalStale, AgeDays: 40},
	}
	if !reflect.DeepEqual(got.ToDelete, want) {
		t.Errorf("정렬 결과 mismatch\n got=%+v\nwant=%+v", got.ToDelete, want)
	}
}
