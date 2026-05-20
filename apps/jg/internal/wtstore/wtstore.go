package wtstore

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/silee-tools/jg/internal/xdgpath"
)

// DataFile is the path to the worktree frecency data file. Override in tests.
var DataFile string

func init() {
	stateDir := xdgpath.StateDir("jg")
	if stateDir != "" {
		_ = os.MkdirAll(stateDir, 0755)
		DataFile = filepath.Join(stateDir, "worktrees")
	} else {
		DataFile = filepath.Join(os.TempDir(), "jg-worktrees")
	}
}

// Entry holds frecency data for a single worktree path.
type Entry struct {
	Path      string
	Rank      float64
	Timestamp int64
}

func parseLine(line string) (Entry, bool) {
	last := strings.LastIndex(line, "|")
	if last < 0 {
		return Entry{}, false
	}
	prev := strings.LastIndex(line[:last], "|")
	if prev < 0 {
		return Entry{}, false
	}
	rank, err := strconv.ParseFloat(line[prev+1:last], 64)
	if err != nil {
		return Entry{}, false
	}
	ts, err := strconv.ParseInt(line[last+1:], 10, 64)
	if err != nil {
		return Entry{}, false
	}
	return Entry{Path: line[:prev], Rank: rank, Timestamp: ts}, true
}

func formatLine(e Entry) string {
	return fmt.Sprintf("%s|%g|%d", e.Path, e.Rank, e.Timestamp)
}

// Load reads all worktree entries from DataFile.
// Returns (nil, nil) when the file does not exist yet.
func Load() ([]Entry, error) {
	f, err := os.Open(DataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_SH)
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	var entries []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if e, ok := parseLine(line); ok {
			entries = append(entries, e)
		}
	}
	return entries, scanner.Err()
}

// Save writes entries to DataFile atomically via a tmp file + rename.
func Save(entries []Entry) error {
	tmp := DataFile + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	w := bufio.NewWriter(f)
	for _, e := range entries {
		fmt.Fprintln(w, formatLine(e))
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	f.Close()
	return os.Rename(tmp, DataFile)
}

// AddOrUpdate increments the rank of path if it already exists, or adds it
// with rank 1 if it is new.
func AddOrUpdate(path string) error {
	entries, err := Load()
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	for i, e := range entries {
		if e.Path == path {
			entries[i].Rank++
			entries[i].Timestamp = now
			return Save(entries)
		}
	}
	entries = append(entries, Entry{Path: path, Rank: 1, Timestamp: now})
	return Save(entries)
}
