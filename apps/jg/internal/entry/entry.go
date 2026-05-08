package entry

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DataFile is the path to the data file. Override in tests.
var DataFile string

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		DataFile = filepath.Join(os.TempDir(), ".jg")
		return
	}
	DataFile = filepath.Join(home, ".jg")
}

type Entry struct {
	Path      string
	Rank      float64
	Timestamp int64
}

func parseLine(line string) (Entry, bool) {
	lastSep := strings.LastIndex(line, "|")
	if lastSep < 0 {
		return Entry{}, false
	}

	prefix := line[:lastSep]
	tsText := line[lastSep+1:]
	prevSep := strings.LastIndex(prefix, "|")
	if prevSep < 0 {
		return Entry{}, false
	}

	path := prefix[:prevSep]
	rankText := prefix[prevSep+1:]
	rank, err := strconv.ParseFloat(rankText, 64)
	if err != nil {
		return Entry{}, false
	}
	ts, err := strconv.ParseInt(tsText, 10, 64)
	if err != nil {
		return Entry{}, false
	}
	return Entry{Path: path, Rank: rank, Timestamp: ts}, true
}

func formatLine(e Entry) string {
	return fmt.Sprintf("%s|%g|%d", e.Path, e.Rank, e.Timestamp)
}

func Load() ([]Entry, error) {
	f, err := os.Open(DataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		return nil, err
	}
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

// AddOrUpdate adds a new entry or updates an existing one.
func AddOrUpdate(path string) error {
	entries, err := Load()
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	found := false
	for i, e := range entries {
		if e.Path == path {
			entries[i].Rank++
			entries[i].Timestamp = now
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, Entry{Path: path, Rank: 1, Timestamp: now})
	}

	return Save(entries)
}

// Remove removes an entry by path.
func Remove(path string) (bool, error) {
	entries, err := Load()
	if err != nil {
		return false, err
	}

	filtered := entries[:0]
	removed := false
	for _, e := range entries {
		if e.Path == path {
			removed = true
			continue
		}
		filtered = append(filtered, e)
	}

	if removed {
		return true, Save(filtered)
	}
	return false, nil
}

// Clean removes entries whose directories no longer exist.
func Clean() (int, error) {
	entries, err := Load()
	if err != nil {
		return 0, err
	}

	var kept []Entry
	for _, e := range entries {
		info, err := os.Stat(e.Path)
		if err == nil && info.IsDir() {
			kept = append(kept, e)
		}
	}

	removed := len(entries) - len(kept)
	if removed > 0 {
		return removed, Save(kept)
	}
	return 0, nil
}
