package entry

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/silee-tools/jg/internal/xdgpath"
)

// DataFile is the path to the data file. Override in tests.
var DataFile string

// LegacyDataFile 은 마이그레이션 이전의 옛 데이터 파일 경로 (~/.jg) 다.
// Load() 가 DataFile 이 비어 있을 때만 읽기 전용으로 fallback 한다.
// Save() 는 항상 DataFile 에만 쓴다 — 이 경로로는 절대 쓰지 않는다.
var LegacyDataFile string

func init() {
	stateDir := xdgpath.StateDir("jg")
	if stateDir != "" {
		_ = os.MkdirAll(stateDir, 0755)
		DataFile = filepath.Join(stateDir, "repos")
	} else {
		DataFile = filepath.Join(os.TempDir(), "jg-repos")
	}

	if home, err := os.UserHomeDir(); err == nil {
		LegacyDataFile = filepath.Join(home, ".jg")
	}
}

type Entry struct {
	Path      string
	Rank      float64
	Timestamp int64
}

type InvalidReason string

const (
	ReasonMissing      InvalidReason = "missing"
	ReasonNotDirectory InvalidReason = "not-dir"
	ReasonNotGit       InvalidReason = "not-git"
	ReasonSubmodule    InvalidReason = "submodule"
)

type CleanReport struct {
	Removed int
	Reasons map[InvalidReason]int
}

type Validator func(path string) (InvalidReason, bool)

func ValidatePath(path string) (InvalidReason, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return ReasonMissing, false
	}
	if !info.IsDir() {
		return ReasonNotDirectory, false
	}

	if _, err := exec.LookPath("git"); err != nil {
		return "", true
	}

	cmd := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return ReasonNotGit, false
	}

	cmd = exec.Command("git", "-C", path, "rev-parse", "--show-superproject-working-tree")
	out, err = cmd.Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		return ReasonSubmodule, false
	}

	return "", true
}

func FilterValid(entries []Entry, validate Validator) ([]Entry, CleanReport) {
	report := CleanReport{Reasons: make(map[InvalidReason]int)}
	kept := make([]Entry, 0, len(entries))
	for _, e := range entries {
		reason, ok := validate(e.Path)
		if ok {
			kept = append(kept, e)
			continue
		}
		report.Removed++
		report.Reasons[reason]++
	}
	return kept, report
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

// loadFromPath 는 단일 데이터 파일에서 entries 를 읽어 반환한다.
// 파일이 없으면 (nil, nil) 을 반환한다. 빈 파일(파싱 가능한 줄 0개) 역시
// nil 을 반환하므로, Load() 는 빈 신규 파일과 "없음" 을 동일하게 처리해 legacy 로 fallback 한다.
func loadFromPath(path string) ([]Entry, error) {
	f, err := os.Open(path)
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

// Load reads entries from DataFile. When DataFile does not exist, it falls back
// to LegacyDataFile (~/.jg) so existing users keep their history transparently.
func Load() ([]Entry, error) {
	entries, err := loadFromPath(DataFile)
	if err != nil {
		return nil, err
	}
	if entries != nil {
		return entries, nil
	}
	if LegacyDataFile == "" {
		return nil, nil
	}
	return loadFromPath(LegacyDataFile)
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

// Clean removes entries that are no longer valid jump targets.
func Clean() (int, error) {
	report, err := CleanWithReport()
	return report.Removed, err
}

func CleanWithReport() (CleanReport, error) {
	entries, err := Load()
	if err != nil {
		return CleanReport{}, err
	}

	kept, report := FilterValid(entries, ValidatePath)
	if report.Removed > 0 {
		return report, Save(kept)
	}
	return report, nil
}
