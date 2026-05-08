package frecency

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/silee-tools/jg/internal/entry"
)

// SortWithBoost sorts entries by frecency, then stably re-sorts by
// how well the path's basename matches the query at word boundaries.
// Within the same match tier, frecency order is preserved.
//
// Tiers (lower = higher priority):
//
//	0: basename == query
//	1: first word of basename == query
//	2: a middle word of basename == query
//	3: last word of basename == query
//	4: no word-boundary match
func SortWithBoost(entries []entry.Entry, query string) []entry.Entry {
	q := strings.TrimSpace(query)
	if q == "" {
		return Sort(entries)
	}

	sorted := Sort(entries)

	tiers := make(map[string]int, len(sorted))
	for _, e := range sorted {
		tiers[e.Path] = matchTier(e.Path, q)
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return tiers[sorted[i].Path] < tiers[sorted[j].Path]
	})

	return sorted
}

// matchTier returns 0-4 based on how well the path's basename matches
// the query at word boundaries.
func matchTier(path, query string) int {
	base := strings.ToLower(filepath.Base(path))
	q := strings.ToLower(query)

	if base == q {
		return 0
	}

	words := splitWords(base)
	if len(words) < 2 {
		return 4
	}

	if words[0] == q {
		return 1
	}
	if words[len(words)-1] == q {
		return 3
	}
	for _, w := range words[1 : len(words)-1] {
		if w == q {
			return 2
		}
	}

	return 4
}

// splitWords splits a name by common separators: '-', '_', '.'.
func splitWords(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
}
