package state

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	tokenBytes        = 32
	tokenLength       = 43
	planTTL           = 30 * time.Minute
	pendingSuffix     = ".json"
	claimedSuffix     = ".in-use"
	consumedSuffix    = ".consumed"
	reservationSuffix = ".reserved"
	maxCollisions     = 16
)

type State string

const (
	Pending  State = "pending"
	Claimed  State = "claimed"
	Consumed State = "consumed"
)

var (
	ErrInvalidToken       = errors.New("invalid plan token")
	ErrMissing            = errors.New("plan is missing")
	ErrExpired            = errors.New("plan has expired")
	ErrCorrupt            = errors.New("plan record is corrupt")
	ErrUnsafeRoot         = errors.New("unsafe state root")
	ErrUnsafeRecord       = errors.New("unsafe plan record")
	ErrClaimed            = errors.New("plan is already in use")
	ErrConsumed           = errors.New("plan is already consumed")
	ErrNotClaimed         = errors.New("plan has not been claimed")
	ErrStateConflict      = errors.New("plan state conflicts with an existing record")
	ErrInvalidFingerprint = errors.New("plan fingerprint is empty")
)

// CommittedError reports that the named state is already visible. Callers must
// treat the token as committed; Ambiguous means directory durability could not
// be confirmed even though the state is visible in the current filesystem view.
type CommittedError struct {
	Token     string
	State     State
	Ambiguous bool
	Err       error
}

func (e *CommittedError) Error() string {
	return fmt.Sprintf("plan %s state committed with a post-publication error: %v", e.State, e.Err)
}

func (e *CommittedError) Unwrap() error { return e.Err }

type Record struct {
	Token       string
	Fingerprint string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	State       State
}

type diskRecord struct {
	Token       string          `json:"token"`
	Payload     json.RawMessage `json:"payload"`
	Fingerprint string          `json:"fingerprint"`
	CreatedAt   time.Time       `json:"created_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
	State       State           `json:"state"`
}

type Store struct {
	dir       string
	resource  *directoryResource
	now       func() time.Time
	random    io.Reader
	statAt    func(string) (unix.Stat_t, error)
	readDir   func() ([]os.DirEntry, error)
	link      func(string, string) error
	remove    func(string) error
	syncDir   func(string) error
	openRead  func(string) (*os.File, error)
	closeRead func(*os.File) error
}

type directoryResource struct {
	mu       sync.RWMutex
	file     *os.File
	dev      uint64
	ino      uint64
	closed   bool
	closeErr error
}

type directoryHooks struct {
	beforeCreate func(string)
	fchmodat     func(int, string, uint32, int) error
}

func New(stateRoot string) (*Store, error) {
	return newWithDirectoryHook(stateRoot, nil)
}

func newWithDirectoryHook(stateRoot string, hook func(string)) (*Store, error) {
	return newWithDirectoryHooks(stateRoot, directoryHooks{beforeCreate: hook})
}

func newWithDirectoryHooks(stateRoot string, hooks directoryHooks) (*Store, error) {
	if hooks.fchmodat == nil {
		hooks.fchmodat = unix.Fchmodat
	}
	if stateRoot == "" {
		return nil, fmt.Errorf("%w: path is empty", ErrUnsafeRoot)
	}
	if !filepath.IsAbs(stateRoot) {
		return nil, fmt.Errorf("%w: state root must be absolute", ErrUnsafeRoot)
	}
	appRoot, err := canonicalApplicationRoot(filepath.Clean(stateRoot))
	if err != nil {
		return nil, err
	}
	appFD, err := openApplicationRoot(stateRoot, hooks)
	if err != nil {
		return nil, err
	}
	plansFD, err := openOrCreateDirectoryAt(appFD, "plans", true, hooks, filepath.Join(stateRoot, "plans"))
	if err == nil {
		if chmodErr := unix.Fchmod(plansFD, 0o700); chmodErr != nil {
			err = fmt.Errorf("%w: secure plans directory: %v", ErrUnsafeRoot, chmodErr)
		}
	}
	closeAppErr := unix.Close(appFD)
	if err != nil {
		if plansFD >= 0 {
			return nil, errors.Join(err, closeAppErr, unix.Close(plansFD))
		}
		return nil, errors.Join(err, closeAppErr)
	}
	if closeAppErr != nil {
		_ = unix.Close(plansFD)
		return nil, closeAppErr
	}
	plansFile := os.NewFile(uintptr(plansFD), filepath.Join(appRoot, "plans"))
	if plansFile == nil {
		_ = unix.Close(plansFD)
		return nil, fmt.Errorf("%w: retain plans directory", ErrUnsafeRoot)
	}
	var plansStat unix.Stat_t
	if err := unix.Fstat(plansFD, &plansStat); err != nil {
		_ = plansFile.Close()
		return nil, fmt.Errorf("%w: inspect plans directory: %v", ErrUnsafeRoot, err)
	}
	resource := &directoryResource{
		file: plansFile, dev: uint64(plansStat.Dev), ino: plansStat.Ino,
	}
	store := &Store{
		dir: filepath.Join(appRoot, "plans"), resource: resource,
		now: time.Now, random: rand.Reader,
		closeRead: func(file *os.File) error { return file.Close() },
	}
	store.statAt = resource.statAt
	store.readDir = resource.readDir
	store.link = resource.link
	store.remove = resource.remove
	store.syncDir = resource.sync
	store.openRead = resource.openRead
	runtime.SetFinalizer(resource, func(value *directoryResource) { _ = value.close() })
	return store, nil
}

func canonicalApplicationRoot(logicalRoot string) (string, error) {
	if info, err := os.Lstat(logicalRoot); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("%w: %s is not a real directory", ErrUnsafeRoot, logicalRoot)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: %v", ErrUnsafeRoot, err)
	}
	existing := logicalRoot
	for {
		_, err := os.Lstat(existing)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %v", ErrUnsafeRoot, err)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", fmt.Errorf("%w: no existing ancestor", ErrUnsafeRoot)
		}
		existing = parent
	}
	canonicalAncestor, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", fmt.Errorf("%w: resolve existing ancestor: %v", ErrUnsafeRoot, err)
	}
	info, err := os.Stat(canonicalAncestor)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: existing ancestor is not a directory", ErrUnsafeRoot)
	}
	remainder, err := filepath.Rel(existing, logicalRoot)
	if err != nil {
		return "", fmt.Errorf("%w: resolve state root: %v", ErrUnsafeRoot, err)
	}
	if remainder == "." {
		return canonicalAncestor, nil
	}
	return filepath.Join(canonicalAncestor, remainder), nil
}

func openApplicationRoot(stateRoot string, hooks directoryHooks) (int, error) {
	cleaned := filepath.Clean(stateRoot)
	currentFD, err := unix.Open(string(os.PathSeparator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("%w: open filesystem root: %v", ErrUnsafeRoot, err)
	}
	logicalPath := string(os.PathSeparator)
	parts := strings.Split(strings.TrimPrefix(cleaned, string(os.PathSeparator)), string(os.PathSeparator))
	for index, part := range parts {
		if part == "" {
			continue
		}
		logicalPath = filepath.Join(logicalPath, part)
		nextFD, openErr := openOrCreateDirectoryAt(currentFD, part, index == len(parts)-1, hooks, logicalPath)
		closeErr := unix.Close(currentFD)
		if openErr != nil {
			return -1, errors.Join(openErr, closeErr)
		}
		if closeErr != nil {
			_ = unix.Close(nextFD)
			return -1, fmt.Errorf("%w: close state root ancestor: %v", ErrUnsafeRoot, closeErr)
		}
		currentFD = nextFD
	}
	if err := unix.Fchmod(currentFD, 0o700); err != nil {
		closeErr := unix.Close(currentFD)
		return -1, errors.Join(fmt.Errorf("%w: secure application state directory: %v", ErrUnsafeRoot, err), closeErr)
	}
	return currentFD, nil
}

func openOrCreateDirectoryAt(parentFD int, name string, noFollow bool, hooks directoryHooks, logicalPath string) (int, error) {
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC
	if noFollow {
		flags |= unix.O_NOFOLLOW
	}
	fd, err := unix.Openat(parentFD, name, flags, 0)
	if err == nil {
		return fd, nil
	}
	if !errors.Is(err, unix.ENOENT) {
		return -1, fmt.Errorf("%w: open state directory %s: %v", ErrUnsafeRoot, logicalPath, err)
	}
	if hooks.beforeCreate != nil {
		hooks.beforeCreate(logicalPath)
	}
	if err := unix.Mkdirat(parentFD, name, 0o700); err != nil {
		return -1, fmt.Errorf("%w: create state directory %s: %v", ErrUnsafeRoot, logicalPath, err)
	}
	fd, created, err := secureCreatedDirectoryAt(parentFD, name, hooks)
	if err != nil {
		cleanupErr := removeCreatedDirectoryAt(parentFD, name, created)
		return -1, errors.Join(fmt.Errorf("%w: secure new state directory %s: %w", ErrUnsafeRoot, logicalPath, err), cleanupErr)
	}
	if fd < 0 {
		fd, err = unix.Openat(parentFD, name, flags|unix.O_NOFOLLOW, 0)
		if err != nil {
			cleanupErr := removeCreatedDirectoryAt(parentFD, name, created)
			return -1, errors.Join(fmt.Errorf("%w: open new state directory %s: %v", ErrUnsafeRoot, logicalPath, err), cleanupErr)
		}
	}
	if err := unix.Fchmod(fd, 0o700); err != nil {
		closeErr := unix.Close(fd)
		return -1, errors.Join(fmt.Errorf("%w: secure new state directory %s: %v", ErrUnsafeRoot, logicalPath, err), closeErr)
	}
	return fd, nil
}

const (
	linuxOPath       = 0x200000
	linuxATEmptyPath = 0x1000
)

func secureCreatedDirectoryAt(parentFD int, name string, hooks directoryHooks) (int, unix.Stat_t, error) {
	if runtime.GOOS != "linux" {
		var created unix.Stat_t
		if err := unix.Fstatat(parentFD, name, &created, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return -1, created, err
		}
		return -1, created, hooks.fchmodat(parentFD, name, 0o700, unix.AT_SYMLINK_NOFOLLOW)
	}
	pathFD, err := unix.Openat(parentFD, name, linuxOPath|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, unix.Stat_t{}, err
	}
	var anchored unix.Stat_t
	if err := unix.Fstat(pathFD, &anchored); err != nil {
		_ = unix.Close(pathFD)
		return -1, anchored, err
	}
	chmodErr := hooks.fchmodat(pathFD, "", 0o700, linuxATEmptyPath)
	fallback := errors.Is(chmodErr, unix.EOPNOTSUPP) || errors.Is(chmodErr, unix.EINVAL)
	if chmodErr != nil && !fallback {
		_ = unix.Close(pathFD)
		return -1, anchored, chmodErr
	}
	if fallback {
		procPath := fmt.Sprintf("/proc/self/fd/%d", pathFD)
		if err := hooks.fchmodat(unix.AT_FDCWD, procPath, 0o700, 0); err != nil {
			_ = unix.Close(pathFD)
			return -1, anchored, err
		}
		fd, err := unix.Open(procPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		closeErr := unix.Close(pathFD)
		if err != nil {
			return -1, anchored, errors.Join(err, closeErr)
		}
		if closeErr != nil {
			_ = unix.Close(fd)
			return -1, anchored, closeErr
		}
		if err := verifyDirectoryIdentity(fd, anchored); err != nil {
			_ = unix.Close(fd)
			return -1, anchored, err
		}
		if err := verifyDirectoryNameAt(parentFD, name, anchored); err != nil {
			_ = unix.Close(fd)
			return -1, anchored, err
		}
		return fd, anchored, nil
	}
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	closeErr := unix.Close(pathFD)
	if err != nil {
		return -1, anchored, errors.Join(err, closeErr)
	}
	if closeErr != nil {
		_ = unix.Close(fd)
		return -1, anchored, closeErr
	}
	if err := verifyDirectoryIdentity(fd, anchored); err != nil {
		_ = unix.Close(fd)
		return -1, anchored, err
	}
	return fd, anchored, nil
}

func verifyDirectoryIdentity(fd int, want unix.Stat_t) error {
	var got unix.Stat_t
	if err := unix.Fstat(fd, &got); err != nil {
		return err
	}
	if got.Mode&unix.S_IFMT != unix.S_IFDIR || uint64(got.Dev) != uint64(want.Dev) || got.Ino != want.Ino {
		return ErrUnsafeRoot
	}
	return nil
}

func verifyDirectoryNameAt(parentFD int, name string, want unix.Stat_t) error {
	var got unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &got, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if got.Mode&unix.S_IFMT != unix.S_IFDIR || uint64(got.Dev) != uint64(want.Dev) || got.Ino != want.Ino {
		return ErrUnsafeRoot
	}
	return nil
}

func removeCreatedDirectoryAt(parentFD int, name string, want unix.Stat_t) error {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return ErrUnsafeRoot
	}
	if uint64(stat.Dev) != uint64(want.Dev) || stat.Ino != want.Ino {
		return ErrUnsafeRoot
	}
	return unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR)
}

// Create stores a plan under an opaque, non-empty fingerprint. Expiration uses
// the local clock captured here and the local clock at Load or Claim; a clock
// moved backward therefore extends validity until it reaches ExpiresAt again.
func (s *Store) Create(payload any, fingerprint string) (Record, error) {
	if err := s.resource.begin(); err != nil {
		return Record{}, err
	}
	defer s.resource.end()
	if strings.TrimSpace(fingerprint) == "" {
		return Record{}, ErrInvalidFingerprint
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return Record{}, fmt.Errorf("encode plan payload: %w", err)
	}
	createdAt := s.now().UTC()
	for range maxCollisions {
		token, err := s.newToken()
		if err != nil {
			return Record{}, err
		}
		occupied, err := s.tokenOccupied(token)
		if err != nil {
			return Record{}, err
		}
		if occupied {
			continue
		}
		if err := s.createReservation(token); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return Record{}, fmt.Errorf("reserve plan token: %w", err)
		}
		disk := diskRecord{
			Token:       token,
			Payload:     payloadJSON,
			Fingerprint: fingerprint,
			CreatedAt:   createdAt,
			ExpiresAt:   createdAt.Add(planTTL),
			State:       Pending,
		}
		data, err := json.Marshal(disk)
		if err != nil {
			return Record{}, fmt.Errorf("encode plan record: %w", err)
		}
		outcome := s.publish(token, s.path(token, pendingSuffix), data)
		if outcome.err != nil {
			if !outcome.committed && outcome.collision {
				continue
			}
			if outcome.committed {
				return metadata(disk), committedError(token, Pending, outcome)
			}
			return Record{}, fmt.Errorf("persist plan: %w", outcome.err)
		}
		return metadata(disk), nil
	}
	return Record{}, fmt.Errorf("generate unique plan token after %d collisions", maxCollisions)
}

func (s *Store) Load(token string, payload any) (Record, error) {
	if err := s.resource.begin(); err != nil {
		return Record{}, err
	}
	defer s.resource.end()
	if err := validateToken(token); err != nil {
		return Record{}, err
	}
	if err := s.ensureReservation(token); err != nil {
		return Record{}, err
	}
	if err := s.nonPendingState(token); err != nil {
		return Record{}, err
	}
	disk, err := s.read(token, pendingSuffix, Pending, payload)
	if err == nil {
		return metadata(disk), nil
	}
	var committed *CommittedError
	if errors.As(err, &committed) {
		return metadata(disk), err
	}
	if !errors.Is(err, ErrMissing) {
		return Record{}, err
	}
	return Record{}, s.absentState(token)
}

func (s *Store) Claim(token string, payload any) (Record, error) {
	if err := s.resource.begin(); err != nil {
		return Record{}, err
	}
	defer s.resource.end()
	if err := validateToken(token); err != nil {
		return Record{}, err
	}
	if err := s.ensureReservation(token); err != nil {
		return Record{}, err
	}
	if err := s.nonPendingState(token); err != nil {
		return Record{}, err
	}
	disk, err := s.read(token, pendingSuffix, Pending, payload)
	if err != nil {
		var committed *CommittedError
		if errors.As(err, &committed) {
			return metadata(disk), err
		}
		if errors.Is(err, ErrMissing) {
			return Record{}, s.absentState(token)
		}
		return Record{}, err
	}
	disk.State = Claimed
	data, err := json.Marshal(disk)
	if err != nil {
		return Record{}, fmt.Errorf("encode claimed plan: %w", err)
	}
	claimedPath := s.path(token, claimedSuffix)
	outcome := s.publish(token, claimedPath, data)
	if outcome.err != nil && !outcome.committed {
		if outcome.collision {
			if ok, matchErr := s.recordMatches(token, claimedSuffix, Claimed); matchErr != nil {
				return Record{}, stateConflict(matchErr)
			} else if ok {
				return Record{}, ErrClaimed
			}
			return Record{}, ErrStateConflict
		}
		return Record{}, fmt.Errorf("claim plan: %w", outcome.err)
	}
	transitionErr := outcome.err
	if err := s.remove(s.path(token, pendingSuffix)); err != nil && !errors.Is(err, os.ErrNotExist) {
		transitionErr = errors.Join(transitionErr, fmt.Errorf("remove pending plan after claim: %w", err))
	}
	if err := s.syncDir(s.dir); err != nil {
		outcome.ambiguous = true
		transitionErr = errors.Join(transitionErr, fmt.Errorf("sync claimed plan: %w", err))
	}
	if transitionErr != nil {
		outcome.err = transitionErr
		return metadata(disk), committedError(token, Claimed, outcome)
	}
	return metadata(disk), nil
}

// Consume is idempotent for an already-consumed token so callers can safely
// defer it after Claim. The durable tombstone remains and Claim never replays it.
func (s *Store) Consume(token string) error {
	if err := s.resource.begin(); err != nil {
		return err
	}
	defer s.resource.end()
	if err := validateToken(token); err != nil {
		return err
	}
	if err := s.ensureReservation(token); err != nil {
		return err
	}
	consumedExists, err := s.exists(s.path(token, consumedSuffix))
	if err != nil {
		return err
	}
	if consumedExists {
		ok, matchErr := s.recordMatches(token, consumedSuffix, Consumed)
		if matchErr != nil {
			return stateConflict(matchErr)
		}
		if !ok {
			return ErrStateConflict
		}
		return s.finishConsumed(token)
	}
	disk, err := s.readWithoutExpiry(token, claimedSuffix, Claimed)
	if err != nil {
		if errors.Is(err, ErrMissing) {
			consumedExists, existsErr := s.exists(s.path(token, consumedSuffix))
			if existsErr != nil {
				return existsErr
			}
			if consumedExists {
				ok, matchErr := s.recordMatches(token, consumedSuffix, Consumed)
				if matchErr != nil || !ok {
					return stateConflict(matchErr)
				}
				return s.finishConsumed(token)
			}
			pendingExists, existsErr := s.exists(s.path(token, pendingSuffix))
			if existsErr != nil {
				return existsErr
			}
			if pendingExists {
				return ErrNotClaimed
			}
			return s.absentState(token)
		}
		return err
	}
	disk.State = Consumed
	data, err := json.Marshal(disk)
	if err != nil {
		return fmt.Errorf("encode consumed plan: %w", err)
	}
	outcome := s.publish(token, s.path(token, consumedSuffix), data)
	if outcome.err != nil && !outcome.committed {
		if outcome.collision {
			ok, matchErr := s.recordMatches(token, consumedSuffix, Consumed)
			if matchErr != nil {
				return stateConflict(matchErr)
			}
			if ok {
				return s.finishConsumed(token)
			}
			return ErrStateConflict
		}
		return fmt.Errorf("consume plan: %w", outcome.err)
	}
	finalized := s.finalizeConsumed(token)
	finalizeErr := finalized.err()
	if outcome.err != nil || finalizeErr != nil {
		outcome.err = errors.Join(outcome.err, finalizeErr)
		outcome.ambiguous = outcome.ambiguous || finalized.syncErr != nil
		return committedError(token, Consumed, outcome)
	}
	return nil
}

type finalizeOutcome struct {
	cleanupErr error
	syncErr    error
}

func (o finalizeOutcome) err() error { return errors.Join(o.cleanupErr, o.syncErr) }

func (s *Store) finishConsumed(token string) error {
	finalized := s.finalizeConsumed(token)
	if err := finalized.err(); err != nil {
		return &CommittedError{Token: token, State: Consumed, Ambiguous: finalized.syncErr != nil, Err: err}
	}
	return nil
}

func (s *Store) finalizeConsumed(token string) finalizeOutcome {
	var outcome finalizeOutcome
	for _, suffix := range []string{claimedSuffix, pendingSuffix} {
		if err := s.remove(s.path(token, suffix)); err != nil && !errors.Is(err, os.ErrNotExist) {
			outcome.cleanupErr = errors.Join(outcome.cleanupErr, fmt.Errorf("remove stale %s plan state: %w", suffix, err))
		}
	}
	if err := s.syncDir(s.dir); err != nil {
		outcome.syncErr = fmt.Errorf("sync consumed plan: %w", err)
	}
	return outcome
}

func (s *Store) newToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := io.ReadFull(s.random, raw); err != nil {
		return "", fmt.Errorf("generate plan token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (s *Store) tokenOccupied(token string) (bool, error) {
	for _, suffix := range []string{reservationSuffix, pendingSuffix, claimedSuffix, consumedSuffix} {
		exists, err := s.exists(s.path(token, suffix))
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) createReservation(token string) error {
	path := s.path(token, reservationSuffix)
	fd, err := unix.Openat(s.resource.fd(), filepath.Base(path), unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return fmt.Errorf("open plan reservation")
	}
	var result error
	if err := file.Chmod(0o600); err != nil {
		result = errors.Join(result, err)
	} else if _, err := io.WriteString(file, token+"\n"); err != nil {
		result = errors.Join(result, err)
	} else if err := file.Sync(); err != nil {
		result = errors.Join(result, err)
	}
	result = errors.Join(result, file.Close())
	if result != nil {
		result = errors.Join(result, s.remove(path))
		return result
	}
	return s.syncDir(s.dir)
}

func (s *Store) ensureReservation(token string) error {
	exists, err := s.exists(s.path(token, reservationSuffix))
	if err != nil {
		return err
	}
	if exists {
		return s.validateReservation(token)
	}
	stateExists := false
	for _, suffix := range []string{pendingSuffix, claimedSuffix, consumedSuffix} {
		exists, err := s.exists(s.path(token, suffix))
		if err != nil {
			return err
		}
		stateExists = stateExists || exists
	}
	if !stateExists {
		return ErrMissing
	}
	if err := s.createReservation(token); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("reconcile plan reservation: %w", err)
		}
	}
	return s.validateReservation(token)
}

func (s *Store) validateReservation(token string) error {
	data, err := s.readSafeFile(s.path(token, reservationSuffix), token)
	if err != nil {
		return err
	}
	if string(data) != token+"\n" {
		return ErrStateConflict
	}
	return nil
}

func validateToken(token string) error {
	if len(token) != tokenLength {
		return ErrInvalidToken
	}
	for i := range len(token) {
		if !isTokenCharacter(token[i]) {
			return ErrInvalidToken
		}
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != tokenBytes || base64.RawURLEncoding.EncodeToString(raw) != token {
		return ErrInvalidToken
	}
	return nil
}

func isTokenCharacter(char byte) bool {
	return (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') ||
		(char >= '0' && char <= '9') || char == '_' || char == '-'
}

func (s *Store) read(token, suffix string, wantState State, payload any) (diskRecord, error) {
	disk, err := s.decodeRecord(token, suffix, wantState)
	if err != nil {
		var committed *CommittedError
		if errors.As(err, &committed) {
			return disk, err
		}
		return diskRecord{}, err
	}
	if !s.now().Before(disk.ExpiresAt) {
		return diskRecord{}, ErrExpired
	}
	if payload != nil {
		if err := json.Unmarshal(disk.Payload, payload); err != nil {
			return diskRecord{}, fmt.Errorf("%w: decode payload: %v", ErrCorrupt, err)
		}
	}
	return disk, nil
}

func (s *Store) decodeRecord(token, suffix string, wantState State) (diskRecord, error) {
	path := s.path(token, suffix)
	data, err := s.readSafeFile(path, token)
	if err != nil {
		return diskRecord{}, err
	}
	var disk diskRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&disk); err != nil {
		return diskRecord{}, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF || disk.Token != token || disk.State != wantState ||
		disk.CreatedAt.IsZero() || disk.ExpiresAt.IsZero() ||
		!disk.ExpiresAt.Equal(disk.CreatedAt.Add(planTTL)) || !json.Valid(disk.Payload) {
		return diskRecord{}, ErrCorrupt
	}
	return disk, nil
}

func (s *Store) readSafeFile(path, token string) ([]byte, error) {
	file, err := s.openRead(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrMissing
		}
		if errors.Is(err, syscall.ELOOP) {
			return nil, ErrUnsafeRecord
		}
		return nil, fmt.Errorf("open plan record: %w", err)
	}
	info, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect open plan record: %w", statErr)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	fileSecurityMode := info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if !ok || !info.Mode().IsRegular() || fileSecurityMode != 0o600 ||
		uint32(stat.Uid) != uint32(os.Geteuid()) {
		_ = file.Close()
		return nil, ErrUnsafeRecord
	}
	err = s.validateLinkTopology(file, path, token)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	data, readErr := io.ReadAll(file)
	closeErr := s.closeRead(file)
	if readErr != nil {
		return nil, fmt.Errorf("read plan record: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close plan record: %w", closeErr)
	}
	return data, nil
}

func (s *Store) validateLinkTopology(file *os.File, path, token string) error {
	for range 4 {
		before, err := file.Stat()
		if err != nil {
			return fmt.Errorf("inspect plan link count: %w", err)
		}
		beforeStat, ok := before.Sys().(*syscall.Stat_t)
		if !ok {
			return ErrUnsafeRecord
		}
		if beforeStat.Nlink == 0 {
			return ErrMissing
		}
		entries, err := s.readDir()
		if err != nil {
			return fmt.Errorf("inspect plan link topology: %w", err)
		}
		wantBase := filepath.Base(path)
		tempPrefix := "." + token + ".tmp-"
		knownLinks := uint64(0)
		sawTarget := false
		for _, entry := range entries {
			entryPath := filepath.Join(s.dir, entry.Name())
			entryStat, err := s.statAt(entryPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return fmt.Errorf("inspect plan link: %w", err)
			}
			sameFile := uint64(entryStat.Dev) == uint64(beforeStat.Dev) && entryStat.Ino == beforeStat.Ino
			if !sameFile {
				continue
			}
			legacy := strings.HasPrefix(entry.Name(), ".plan-")
			if entry.Name() != wantBase && !strings.HasPrefix(entry.Name(), tempPrefix) && !legacy {
				return ErrUnsafeRecord
			}
			sawTarget = sawTarget || entry.Name() == wantBase
			knownLinks++
		}
		after, err := file.Stat()
		if err != nil {
			return fmt.Errorf("recheck plan link count: %w", err)
		}
		afterStat, ok := after.Sys().(*syscall.Stat_t)
		if !ok {
			return ErrUnsafeRecord
		}
		if beforeStat.Nlink != afterStat.Nlink {
			continue
		}
		if !sawTarget || knownLinks != uint64(afterStat.Nlink) {
			return ErrUnsafeRecord
		}
		return nil
	}
	return ErrUnsafeRecord
}

func (s *Store) absentState(token string) error {
	consumedExists, err := s.exists(s.path(token, consumedSuffix))
	if err != nil {
		return err
	}
	if consumedExists {
		if ok, matchErr := s.recordMatches(token, consumedSuffix, Consumed); matchErr != nil {
			return stateConflict(matchErr)
		} else if ok {
			return ErrConsumed
		}
		return ErrStateConflict
	}
	claimedExists, err := s.exists(s.path(token, claimedSuffix))
	if err != nil {
		return err
	}
	if claimedExists {
		if ok, matchErr := s.recordMatches(token, claimedSuffix, Claimed); matchErr != nil {
			return stateConflict(matchErr)
		} else if ok {
			return ErrClaimed
		}
		return ErrStateConflict
	}
	return ErrMissing
}

func (s *Store) nonPendingState(token string) error {
	consumedExists, err := s.exists(s.path(token, consumedSuffix))
	if err != nil {
		return err
	}
	if consumedExists {
		if ok, matchErr := s.recordMatches(token, consumedSuffix, Consumed); matchErr != nil {
			return stateConflict(matchErr)
		} else if ok {
			return ErrConsumed
		}
		return ErrStateConflict
	}
	claimedExists, err := s.exists(s.path(token, claimedSuffix))
	if err != nil {
		return err
	}
	if claimedExists {
		if ok, matchErr := s.recordMatches(token, claimedSuffix, Claimed); matchErr != nil {
			return stateConflict(matchErr)
		} else if ok {
			return ErrClaimed
		}
		return ErrStateConflict
	}
	return nil
}

func (s *Store) recordMatches(token, suffix string, state State) (bool, error) {
	_, err := s.readWithoutExpiry(token, suffix, state)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrMissing) {
		return false, nil
	}
	var committed *CommittedError
	if errors.As(err, &committed) {
		return true, err
	}
	return false, err
}

func stateConflict(err error) error {
	var committed *CommittedError
	if errors.As(err, &committed) {
		return err
	}
	return errors.Join(ErrStateConflict, err)
}

func (s *Store) readWithoutExpiry(token, suffix string, wantState State) (diskRecord, error) {
	return s.decodeRecord(token, suffix, wantState)
}

type publishOutcome struct {
	committed bool
	ambiguous bool
	collision bool
	err       error
}

func committedError(token string, state State, outcome publishOutcome) error {
	return &CommittedError{Token: token, State: state, Ambiguous: outcome.ambiguous, Err: outcome.err}
}

func (s *Store) publish(token, target string, data []byte) publishOutcome {
	temp, err := s.createTemp(token)
	if err != nil {
		return publishOutcome{err: err}
	}
	tempPath := temp.Name()
	if err := temp.Chmod(0o600); err != nil {
		closeErr := temp.Close()
		removeErr := s.remove(tempPath)
		return publishOutcome{err: errors.Join(err, closeErr, removeErr)}
	}
	if _, err := temp.Write(data); err != nil {
		closeErr := temp.Close()
		removeErr := s.remove(tempPath)
		return publishOutcome{err: errors.Join(err, closeErr, removeErr)}
	}
	if err := temp.Sync(); err != nil {
		closeErr := temp.Close()
		removeErr := s.remove(tempPath)
		return publishOutcome{err: errors.Join(err, closeErr, removeErr)}
	}
	if err := temp.Close(); err != nil {
		removeErr := s.remove(tempPath)
		return publishOutcome{err: errors.Join(err, removeErr)}
	}
	// A same-directory hard-link publication has rename-like atomic visibility
	// while retaining O_EXCL semantics: an existing state file is never replaced.
	if err := s.link(tempPath, target); err != nil {
		removeErr := s.remove(tempPath)
		return publishOutcome{collision: errors.Is(err, os.ErrExist) && removeErr == nil, err: errors.Join(err, removeErr)}
	}
	outcome := publishOutcome{committed: true}
	if err := s.remove(tempPath); err != nil {
		outcome.err = errors.Join(outcome.err, fmt.Errorf("remove published temp: %w", err))
	}
	if err := s.syncDir(s.dir); err != nil {
		outcome.ambiguous = true
		outcome.err = errors.Join(outcome.err, fmt.Errorf("sync published state: %w", err))
	}
	return outcome
}

func (s *Store) createTemp(token string) (*os.File, error) {
	for range maxCollisions {
		raw := make([]byte, 8)
		if _, err := io.ReadFull(rand.Reader, raw); err != nil {
			return nil, err
		}
		name := "." + token + ".tmp-" + base64.RawURLEncoding.EncodeToString(raw)
		fd, err := unix.Openat(s.resource.fd(), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return nil, err
		}
		file := os.NewFile(uintptr(fd), filepath.Join(s.dir, name))
		if file == nil {
			_ = unix.Close(fd)
			return nil, fmt.Errorf("retain plan temp file")
		}
		return file, nil
	}
	return nil, fmt.Errorf("create plan temp after %d collisions", maxCollisions)
}

func metadata(disk diskRecord) Record {
	return Record{
		Token:       disk.Token,
		Fingerprint: disk.Fingerprint,
		CreatedAt:   disk.CreatedAt,
		ExpiresAt:   disk.ExpiresAt,
		State:       disk.State,
	}
}

func (s *Store) path(token, suffix string) string {
	return filepath.Join(s.dir, token+suffix)
}

func (s *Store) exists(path string) (bool, error) {
	_, err := s.statAt(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// Close releases the stable plans directory descriptor. It is safe to call
// concurrently with Store operations and returns the same result on every call.
func (s *Store) Close() error {
	if s == nil || s.resource == nil {
		return nil
	}
	return s.resource.close()
}

func (r *directoryResource) begin() error {
	r.mu.RLock()
	if err := r.verifyLocked(); err != nil {
		r.mu.RUnlock()
		return err
	}
	return nil
}

func (r *directoryResource) end() { r.mu.RUnlock() }

func (r *directoryResource) fd() int { return int(r.file.Fd()) }

func (r *directoryResource) verifyLocked() error {
	if r.closed || r.file == nil {
		return ErrUnsafeRoot
	}
	var stat unix.Stat_t
	if err := unix.Fstat(r.fd(), &stat); err != nil {
		return fmt.Errorf("%w: verify plans directory: %v", ErrUnsafeRoot, err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Nlink == 0 || uint64(stat.Dev) != r.dev || stat.Ino != r.ino {
		return fmt.Errorf("%w: plans directory identity changed", ErrUnsafeRoot)
	}
	return nil
}

func (r *directoryResource) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		runtime.SetFinalizer(r, nil)
		r.closed = true
		if r.file != nil {
			r.closeErr = r.file.Close()
		}
	}
	return r.closeErr
}

func (r *directoryResource) statAt(path string) (unix.Stat_t, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(r.fd(), filepath.Base(path), &stat, unix.AT_SYMLINK_NOFOLLOW)
	return stat, err
}

func (r *directoryResource) readDir() ([]os.DirEntry, error) {
	fd, err := unix.Openat(r.fd(), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "plans")
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("retain plans directory scan")
	}
	entries, readErr := file.ReadDir(-1)
	closeErr := file.Close()
	return entries, errors.Join(readErr, closeErr)
}

func (r *directoryResource) link(oldPath, newPath string) error {
	fd := r.fd()
	return unix.Linkat(fd, filepath.Base(oldPath), fd, filepath.Base(newPath), 0)
}

func (r *directoryResource) remove(path string) error {
	return unix.Unlinkat(r.fd(), filepath.Base(path), 0)
}

func (r *directoryResource) sync(string) error { return r.file.Sync() }

func (r *directoryResource) openRead(path string) (*os.File, error) {
	fd, err := unix.Openat(r.fd(), filepath.Base(path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("retain open plan record")
	}
	return file, nil
}
