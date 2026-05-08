package frecency

import (
	"testing"
	"time"

	"github.com/silee-tools/jg/internal/entry"
)

func TestScoreAtZeroDelta(t *testing.T) {
	now := int64(1700000000)
	score := Score(10, now, now)
	// dx=0: 10 * (3.75 / (1 + 0.25)) = 10 * 3.0 = 30
	expected := 30.0
	if diff := score - expected; diff > 0.001 || diff < -0.001 {
		t.Errorf("Score at dx=0: got %f, want %f", score, expected)
	}
}

func TestScoreDecaysOverTime(t *testing.T) {
	now := int64(1700000000)
	recent := Score(10, now-60, now)      // 1 minute ago
	hourAgo := Score(10, now-3600, now)   // 1 hour ago
	dayAgo := Score(10, now-86400, now)   // 1 day ago
	weekAgo := Score(10, now-604800, now) // 1 week ago

	if recent <= hourAgo || hourAgo <= dayAgo || dayAgo <= weekAgo {
		t.Errorf("scores should decay: recent=%f, hour=%f, day=%f, week=%f",
			recent, hourAgo, dayAgo, weekAgo)
	}
}

func TestScoreRankMatters(t *testing.T) {
	now := int64(1700000000)
	ts := now - 3600 // same timestamp, 1 hour ago

	highRank := Score(100, ts, now)
	lowRank := Score(1, ts, now)

	if highRank <= lowRank {
		t.Errorf("higher rank should score higher: high=%f, low=%f", highRank, lowRank)
	}
}

func TestSort(t *testing.T) {
	now := time.Now().Unix()

	entries := []entry.Entry{
		{Path: "/old-frequent", Rank: 50, Timestamp: now - 86400*7}, // old but frequent
		{Path: "/recent", Rank: 2, Timestamp: now - 60},             // very recent
		{Path: "/medium", Rank: 10, Timestamp: now - 3600},          // medium
	}

	sorted := Sort(entries)

	// Verify descending order
	for i := 1; i < len(sorted); i++ {
		si := Score(sorted[i-1].Rank, sorted[i-1].Timestamp, now)
		sj := Score(sorted[i].Rank, sorted[i].Timestamp, now)
		if si < sj {
			t.Errorf("not sorted: entry[%d] score %f < entry[%d] score %f",
				i-1, si, i, sj)
		}
	}
}

func TestSortDoesNotMutateOriginal(t *testing.T) {
	entries := []entry.Entry{
		{Path: "/a", Rank: 1, Timestamp: 1700000000},
		{Path: "/b", Rank: 10, Timestamp: 1700000000},
	}
	originalFirst := entries[0].Path

	Sort(entries)

	if entries[0].Path != originalFirst {
		t.Error("Sort mutated the original slice")
	}
}
