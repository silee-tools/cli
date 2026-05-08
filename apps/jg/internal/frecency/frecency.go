package frecency

import (
	"sort"
	"time"

	"github.com/silee-tools/jg/internal/entry"
)

// Score calculates the frecency score for an entry.
// Uses the same formula as z: score = rank * (3.75 / ((0.0001 * dx + 1) + 0.25))
// where dx is seconds since last access.
func Score(rank float64, timestamp, now int64) float64 {
	dx := float64(now - timestamp)
	return rank * (3.75 / ((0.0001*dx + 1) + 0.25))
}

// Sort sorts entries by frecency score in descending order.
func Sort(entries []entry.Entry) []entry.Entry {
	now := time.Now().Unix()
	sorted := make([]entry.Entry, len(entries))
	copy(sorted, entries)

	sort.Slice(sorted, func(i, j int) bool {
		si := Score(sorted[i].Rank, sorted[i].Timestamp, now)
		sj := Score(sorted[j].Rank, sorted[j].Timestamp, now)
		return si > sj
	})

	return sorted
}

// ScoreEntry returns the frecency score of an entry at the current time.
func ScoreEntry(e entry.Entry) float64 {
	return Score(e.Rank, e.Timestamp, time.Now().Unix())
}
