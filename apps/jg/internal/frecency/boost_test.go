package frecency

import (
	"testing"
	"time"

	"github.com/silee-tools/jg/internal/entry"
)

func TestMatchTier(t *testing.T) {
	tests := []struct {
		path  string
		query string
		want  int
	}{
		// tier 0: exact match
		{"/repos/eks", "eks", 0},
		{"/repos/EKS", "eks", 0}, // case-insensitive
		{"/repos/eks", "EKS", 0},

		// tier 1: starts with query word
		{"/repos/eks-cluster", "eks", 1},
		{"/repos/eks_tools", "eks", 1},
		{"/repos/eks.config", "eks", 1},

		// tier 2: contains query word (not first, not last)
		{"/repos/my-eks-tools", "eks", 2},
		{"/repos/aws_eks_config", "eks", 2},

		// tier 3: ends with query word
		{"/repos/tools-eks", "eks", 3},
		{"/repos/aws_eks", "eks", 3},
		{"/repos/my.eks", "eks", 3},

		// tier 4: no word boundary match
		{"/repos/dekstools", "eks", 4},
		{"/repos/infrastructure", "eks", 4},
		{"/repos/rekster", "eks", 4},

		// single-word basename: exact or nothing
		{"/repos/eks", "eks", 0},
		{"/repos/noteks", "eks", 4},

		// two-word basename: middle tier unreachable
		{"/repos/foo-bar", "bar", 3}, // last word, not middle
		{"/repos/bar-foo", "bar", 1}, // first word

		// query with separator: compared as-is, only exact match possible
		{"/repos/eks-tools", "eks-tools", 0},
		{"/repos/my-eks-tools", "eks-tools", 4},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.query, func(t *testing.T) {
			got := matchTier(tt.path, tt.query)
			if got != tt.want {
				t.Errorf("matchTier(%q, %q) = %d, want %d", tt.path, tt.query, got, tt.want)
			}
		})
	}
}

func TestSplitWords(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{"eks-cluster", []string{"eks", "cluster"}},
		{"my_eks_tools", []string{"my", "eks", "tools"}},
		{"app.config", []string{"app", "config"}},
		{"eks-my_tools.v2", []string{"eks", "my", "tools", "v2"}},
		{"singleword", []string{"singleword"}},
		{"", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitWords(tt.name)
			if len(got) != len(tt.want) {
				t.Fatalf("splitWords(%q) = %v, want %v", tt.name, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitWords(%q)[%d] = %q, want %q", tt.name, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSortWithBoostEmptyQuery(t *testing.T) {
	now := time.Now().Unix()
	entries := []entry.Entry{
		{Path: "/repos/low-freq", Rank: 1, Timestamp: now - 3600},
		{Path: "/repos/high-freq", Rank: 100, Timestamp: now - 60},
	}

	result := SortWithBoost(entries, "")
	if result[0].Path != "/repos/high-freq" {
		t.Errorf("empty query: expected high-freq first, got %s", result[0].Path)
	}
}

func TestSortWithBoostTrimmedQuery(t *testing.T) {
	now := time.Now().Unix()
	entries := []entry.Entry{
		{Path: "/repos/other", Rank: 100, Timestamp: now},
		{Path: "/repos/eks", Rank: 50, Timestamp: now},
	}

	result := SortWithBoost(entries, " eks ")
	if result[0].Path != "/repos/eks" {
		t.Errorf("trimmed query: expected eks first, got %s", result[0].Path)
	}
}

func TestSortWithBoostWhitespaceQuery(t *testing.T) {
	now := time.Now().Unix()
	entries := []entry.Entry{
		{Path: "/repos/low-freq", Rank: 1, Timestamp: now - 3600},
		{Path: "/repos/high-freq", Rank: 100, Timestamp: now - 60},
	}

	result := SortWithBoost(entries, "   ")
	if result[0].Path != "/repos/high-freq" {
		t.Errorf("whitespace query: expected high-freq first, got %s", result[0].Path)
	}
}

func TestSortWithBoostTierOrdering(t *testing.T) {
	now := time.Now().Unix()

	// All entries have same high frecency to isolate tier effect
	entries := []entry.Entry{
		{Path: "/repos/infrastructure", Rank: 100, Timestamp: now}, // tier 4
		{Path: "/repos/my-eks-tools", Rank: 100, Timestamp: now},   // tier 2
		{Path: "/repos/eks-cluster", Rank: 100, Timestamp: now},    // tier 1
		{Path: "/repos/tools-eks", Rank: 100, Timestamp: now},      // tier 3
		{Path: "/repos/eks", Rank: 100, Timestamp: now},            // tier 0
	}

	result := SortWithBoost(entries, "eks")

	expected := []string{
		"/repos/eks",            // tier 0
		"/repos/eks-cluster",    // tier 1
		"/repos/my-eks-tools",   // tier 2
		"/repos/tools-eks",      // tier 3
		"/repos/infrastructure", // tier 4
	}

	for i, want := range expected {
		if result[i].Path != want {
			t.Errorf("position %d: got %s, want %s", i, result[i].Path, want)
		}
	}
}

func TestSortWithBoostFrecencyPreservedWithinTier(t *testing.T) {
	now := time.Now().Unix()

	// Two tier-1 entries with clearly different frecency
	entries := []entry.Entry{
		{Path: "/repos/eks-low", Rank: 1, Timestamp: now - 86400}, // tier 1, low frecency
		{Path: "/repos/eks-high", Rank: 100, Timestamp: now - 60}, // tier 1, high frecency
		{Path: "/repos/other", Rank: 200, Timestamp: now},         // tier 4, highest frecency
	}

	result := SortWithBoost(entries, "eks")

	// tier 1 should come before tier 4, and within tier 1, high-freq first
	if result[0].Path != "/repos/eks-high" {
		t.Errorf("expected eks-high first, got %s", result[0].Path)
	}
	if result[1].Path != "/repos/eks-low" {
		t.Errorf("expected eks-low second, got %s", result[1].Path)
	}
	if result[2].Path != "/repos/other" {
		t.Errorf("expected other third, got %s", result[2].Path)
	}
}

func TestSortWithBoostCaseInsensitive(t *testing.T) {
	now := time.Now().Unix()

	entries := []entry.Entry{
		{Path: "/repos/other", Rank: 100, Timestamp: now},
		{Path: "/repos/EKS-Cluster", Rank: 50, Timestamp: now},
	}

	result := SortWithBoost(entries, "eks")

	if result[0].Path != "/repos/EKS-Cluster" {
		t.Errorf("case insensitive: expected EKS-Cluster first, got %s", result[0].Path)
	}
}

func TestSortWithBoostNoWordBoundaryNoBoost(t *testing.T) {
	now := time.Now().Unix()

	entries := []entry.Entry{
		{Path: "/repos/dekstools", Rank: 50, Timestamp: now}, // "eks" embedded, no word boundary
		{Path: "/repos/other", Rank: 100, Timestamp: now},    // higher frecency
	}

	result := SortWithBoost(entries, "eks")

	// Both are tier 4, so frecency order: other first
	if result[0].Path != "/repos/other" {
		t.Errorf("no word boundary: expected other first (higher frecency), got %s", result[0].Path)
	}
}

func TestSortWithBoostDoesNotMutateOriginal(t *testing.T) {
	now := time.Now().Unix()
	entries := []entry.Entry{
		{Path: "/repos/other", Rank: 100, Timestamp: now},
		{Path: "/repos/eks", Rank: 1, Timestamp: now - 86400},
	}
	original := make([]entry.Entry, len(entries))
	copy(original, entries)

	SortWithBoost(entries, "eks")

	for i := range entries {
		if entries[i].Path != original[i].Path {
			t.Errorf("SortWithBoost mutated the original slice at index %d: got %s, want %s",
				i, entries[i].Path, original[i].Path)
		}
	}
}
