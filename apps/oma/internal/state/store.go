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
	"strings"
	"syscall"
	"time"
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
	dir      string
	now      func() time.Time
	random   io.Reader
	lstat    func(string) (os.FileInfo, error)
	link     func(string, string) error
	remove   func(string) error
	syncDir  func(string) error
	openRead func(string) (*os.File, error)
}

func New(stateRoot string) (*Store, error) {
	if stateRoot == "" {
		return nil, fmt.Errorf("%w: path is empty", ErrUnsafeRoot)
	}
	if !filepath.IsAbs(stateRoot) {
		return nil, fmt.Errorf("%w: state root must be absolute", ErrUnsafeRoot)
	}
	appRoot := filepath.Clean(stateRoot)
	if err := ensureSecurePath(appRoot); err != nil {
		return nil, err
	}
	plansDir := filepath.Join(appRoot, "plans")
	if err := ensureSecurePath(plansDir); err != nil {
		return nil, err
	}
	return &Store{
		dir: plansDir, now: time.Now, random: rand.Reader,
		lstat: os.Lstat, link: os.Link, remove: os.Remove,
		syncDir: syncDirectory, openRead: openNoFollow,
	}, nil
}

// Create stores a plan under an opaque, non-empty fingerprint. Expiration uses
// the local clock captured here and the local clock at Load or Claim; a clock
// moved backward therefore extends validity until it reaches ExpiresAt again.
func (s *Store) Create(payload any, fingerprint string) (Record, error) {
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
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
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
	data, cleaned, err := s.readSafeFile(s.path(token, reservationSuffix), token)
	if err != nil {
		return err
	}
	if err := cleaned.err(); err != nil {
		return &CommittedError{Token: token, State: Pending, Ambiguous: cleaned.syncErr != nil, Err: err}
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
	data, cleaned, err := s.readSafeFile(path, token)
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
	if cleanupErr := cleaned.err(); cleanupErr != nil {
		return disk, &CommittedError{Token: token, State: wantState, Ambiguous: cleaned.syncErr != nil, Err: cleanupErr}
	}
	return disk, nil
}

func (s *Store) readSafeFile(path, token string) ([]byte, finalizeOutcome, error) {
	file, err := s.openRead(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, finalizeOutcome{}, ErrMissing
		}
		if errors.Is(err, syscall.ELOOP) {
			return nil, finalizeOutcome{}, ErrUnsafeRecord
		}
		return nil, finalizeOutcome{}, fmt.Errorf("open plan record: %w", err)
	}
	info, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return nil, finalizeOutcome{}, fmt.Errorf("inspect open plan record: %w", statErr)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	fileSecurityMode := info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if !ok || !info.Mode().IsRegular() || fileSecurityMode != 0o600 ||
		uint32(stat.Uid) != uint32(os.Geteuid()) {
		_ = file.Close()
		return nil, finalizeOutcome{}, ErrUnsafeRecord
	}
	cleaned, err := s.validateLinkTopology(file, path, token)
	if err != nil {
		_ = file.Close()
		return nil, finalizeOutcome{}, err
	}
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, finalizeOutcome{}, fmt.Errorf("read plan record: %w", readErr)
	}
	if closeErr != nil {
		return nil, finalizeOutcome{}, fmt.Errorf("close plan record: %w", closeErr)
	}
	return data, cleaned, nil
}

func (s *Store) validateLinkTopology(file *os.File, path, token string) (finalizeOutcome, error) {
	for range 4 {
		before, err := file.Stat()
		if err != nil {
			return finalizeOutcome{}, fmt.Errorf("inspect plan link count: %w", err)
		}
		beforeStat, ok := before.Sys().(*syscall.Stat_t)
		if !ok {
			return finalizeOutcome{}, ErrUnsafeRecord
		}
		if beforeStat.Nlink == 0 {
			return finalizeOutcome{}, ErrMissing
		}
		entries, err := os.ReadDir(s.dir)
		if err != nil {
			return finalizeOutcome{}, fmt.Errorf("inspect plan link topology: %w", err)
		}
		wantBase := filepath.Base(path)
		tempPrefix := "." + token + ".tmp-"
		knownLinks := uint64(0)
		sawTarget := false
		legacyLinks := make([]string, 0)
		for _, entry := range entries {
			entryPath := filepath.Join(s.dir, entry.Name())
			info, err := s.lstat(entryPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return finalizeOutcome{}, fmt.Errorf("inspect plan link: %w", err)
			}
			sameFile := os.SameFile(before, info)
			if strings.HasPrefix(entry.Name(), ".plan-") && !sameFile && !strings.HasSuffix(wantBase, reservationSuffix) {
				return finalizeOutcome{}, ErrUnsafeRecord
			}
			if !sameFile {
				continue
			}
			legacy := strings.HasPrefix(entry.Name(), ".plan-")
			if entry.Name() != wantBase && !strings.HasPrefix(entry.Name(), tempPrefix) && !legacy {
				return finalizeOutcome{}, ErrUnsafeRecord
			}
			if legacy {
				legacyLinks = append(legacyLinks, entryPath)
			}
			sawTarget = sawTarget || entry.Name() == wantBase
			knownLinks++
		}
		after, err := file.Stat()
		if err != nil {
			return finalizeOutcome{}, fmt.Errorf("recheck plan link count: %w", err)
		}
		afterStat, ok := after.Sys().(*syscall.Stat_t)
		if !ok {
			return finalizeOutcome{}, ErrUnsafeRecord
		}
		if beforeStat.Nlink != afterStat.Nlink {
			continue
		}
		if !sawTarget || knownLinks != uint64(afterStat.Nlink) {
			return finalizeOutcome{}, ErrUnsafeRecord
		}
		var cleaned finalizeOutcome
		for _, legacyPath := range legacyLinks {
			if err := s.remove(legacyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleaned.cleanupErr = errors.Join(cleaned.cleanupErr, fmt.Errorf("remove legacy publication link: %w", err))
			}
		}
		if len(legacyLinks) > 0 {
			if err := s.syncDir(s.dir); err != nil {
				cleaned.syncErr = fmt.Errorf("sync legacy publication cleanup: %w", err)
			}
		}
		return cleaned, nil
	}
	return finalizeOutcome{}, ErrUnsafeRecord
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
	temp, err := os.CreateTemp(s.dir, "."+token+".tmp-*")
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

func ensureSecurePath(path string) error {
	volume := filepath.VolumeName(path)
	current := volume + string(os.PathSeparator)
	relative := strings.TrimPrefix(path, current)
	parts := strings.Split(relative, string(os.PathSeparator))
	for index, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil {
				return fmt.Errorf("create state directory: %w", err)
			}
			if err := os.Chmod(current, 0o700); err != nil {
				return fmt.Errorf("secure new state directory: %w", err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUnsafeRoot, err)
		}
		if info.Mode()&os.ModeSymlink != 0 && index != len(parts)-1 {
			resolved, statErr := os.Stat(current)
			if statErr != nil || !resolved.IsDir() {
				return fmt.Errorf("%w: %s is not a directory", ErrUnsafeRoot, current)
			}
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: %s is not a real directory", ErrUnsafeRoot, current)
		}
		if index == len(parts)-1 {
			if err := os.Chmod(current, 0o700); err != nil {
				return fmt.Errorf("secure application state directory: %w", err)
			}
		}
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
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
	_, err := s.lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func openNoFollow(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
