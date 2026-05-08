package drift

import (
	"os"

	"github.com/silee-tools/mydesk/internal/linker"
)

type DriftType int

const (
	DriftNone        DriftType = iota
	DriftBroken                // symlink target doesn't exist
	DriftMissing               // no symlink at target path
	DriftWrongTarget           // symlink points to wrong source
	DriftNotSymlink            // target exists but is not a symlink
)

func (d DriftType) String() string {
	switch d {
	case DriftNone:
		return "ok"
	case DriftBroken:
		return "broken symlink"
	case DriftMissing:
		return "missing"
	case DriftWrongTarget:
		return "wrong target"
	case DriftNotSymlink:
		return "not a symlink"
	default:
		return "unknown"
	}
}

type DriftResult struct {
	Entry  linker.LinkEntry
	Type   DriftType
	Detail string
}

func Detect(entries []linker.LinkEntry) []DriftResult {
	var results []DriftResult

	for _, e := range entries {
		r := check(e)
		if r.Type != DriftNone {
			results = append(results, r)
		}
	}

	return results
}

func check(e linker.LinkEntry) DriftResult {
	info, err := os.Lstat(e.DstAbs)
	if err != nil {
		return DriftResult{Entry: e, Type: DriftMissing, Detail: "target does not exist"}
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return DriftResult{Entry: e, Type: DriftNotSymlink, Detail: "target is a regular file/directory"}
	}

	target, err := os.Readlink(e.DstAbs)
	if err != nil {
		return DriftResult{Entry: e, Type: DriftBroken, Detail: "cannot read symlink"}
	}

	if target != e.SrcAbs {
		return DriftResult{Entry: e, Type: DriftWrongTarget, Detail: "points to " + target}
	}

	// check if source actually exists
	if _, err := os.Stat(e.SrcAbs); err != nil {
		return DriftResult{Entry: e, Type: DriftBroken, Detail: "source does not exist: " + e.SrcAbs}
	}

	return DriftResult{Entry: e, Type: DriftNone}
}
