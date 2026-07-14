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
	"time"
)

const (
	tokenBytes     = 32
	tokenLength    = 43
	planTTL        = 30 * time.Minute
	pendingSuffix  = ".json"
	claimedSuffix  = ".in-use"
	consumedSuffix = ".consumed"
	maxCollisions  = 16
)

type State string

const (
	Pending  State = "pending"
	Claimed  State = "claimed"
	Consumed State = "consumed"
)

var (
	ErrInvalidToken  = errors.New("invalid plan token")
	ErrMissing       = errors.New("plan is missing")
	ErrExpired       = errors.New("plan has expired")
	ErrCorrupt       = errors.New("plan record is corrupt")
	ErrUnsafeRoot    = errors.New("unsafe state root")
	ErrUnsafeRecord  = errors.New("unsafe plan record")
	ErrClaimed       = errors.New("plan is already in use")
	ErrConsumed      = errors.New("plan is already consumed")
	ErrNotClaimed    = errors.New("plan has not been claimed")
	ErrStateConflict = errors.New("plan state conflicts with an existing record")
)

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
	dir    string
	now    func() time.Time
	random io.Reader
}

func New(stateRoot string) (*Store, error) {
	if stateRoot == "" {
		return nil, fmt.Errorf("%w: path is empty", ErrUnsafeRoot)
	}
	absRoot, err := filepath.Abs(stateRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsafeRoot, err)
	}
	if err := ensureDirectory(absRoot, 0o700, false); err != nil {
		return nil, err
	}
	omaDir := filepath.Join(absRoot, "oma")
	if err := ensureDirectory(omaDir, 0o700, false); err != nil {
		return nil, err
	}
	plansDir := filepath.Join(omaDir, "plans")
	if err := ensureDirectory(plansDir, 0o700, true); err != nil {
		return nil, err
	}
	return &Store{dir: plansDir, now: time.Now, random: rand.Reader}, nil
}

func (s *Store) Create(payload any, fingerprint string) (Record, error) {
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
		path := s.path(token, pendingSuffix)
		if err := s.publish(path, data); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return Record{}, fmt.Errorf("persist plan: %w", err)
		}
		return metadata(disk), nil
	}
	return Record{}, fmt.Errorf("generate unique plan token after %d collisions", maxCollisions)
}

func (s *Store) Load(token string, payload any) (Record, error) {
	if err := validateToken(token); err != nil {
		return Record{}, err
	}
	if err := s.nonPendingState(token); err != nil {
		return Record{}, err
	}
	disk, err := s.read(token, pendingSuffix, Pending, payload)
	if err == nil {
		return metadata(disk), nil
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
	if err := s.nonPendingState(token); err != nil {
		return Record{}, err
	}
	disk, err := s.read(token, pendingSuffix, Pending, payload)
	if err != nil {
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
	if err := s.publish(claimedPath, data); err != nil {
		if errors.Is(err, os.ErrExist) {
			if s.hasRecord(token, claimedSuffix, Claimed) {
				return Record{}, ErrClaimed
			}
			return Record{}, ErrStateConflict
		}
		return Record{}, fmt.Errorf("claim plan: %w", err)
	}
	if err := os.Remove(s.path(token, pendingSuffix)); err != nil {
		return Record{}, fmt.Errorf("remove pending plan after claim: %w", err)
	}
	if err := syncDirectory(s.dir); err != nil {
		return Record{}, fmt.Errorf("sync claimed plan: %w", err)
	}
	return metadata(disk), nil
}

// Consume is idempotent for an already-consumed token so callers can safely
// defer it after Claim. The durable tombstone remains and Claim never replays it.
func (s *Store) Consume(token string) error {
	if err := validateToken(token); err != nil {
		return err
	}
	if s.exists(s.path(token, consumedSuffix)) {
		if !s.hasRecord(token, consumedSuffix, Consumed) {
			return ErrStateConflict
		}
		if s.exists(s.path(token, claimedSuffix)) {
			if !s.hasRecord(token, claimedSuffix, Claimed) {
				return ErrStateConflict
			}
			if err := os.Remove(s.path(token, claimedSuffix)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("finish consuming plan: %w", err)
			}
			return syncDirectory(s.dir)
		}
		return nil
	}
	disk, err := s.readWithoutExpiry(token, claimedSuffix, Claimed)
	if err != nil {
		if errors.Is(err, ErrMissing) {
			if s.exists(s.path(token, pendingSuffix)) {
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
	if err := s.publish(s.path(token, consumedSuffix), data); err != nil {
		if errors.Is(err, os.ErrExist) && s.hasRecord(token, consumedSuffix, Consumed) {
			return s.Consume(token)
		}
		if errors.Is(err, os.ErrExist) {
			return ErrStateConflict
		}
		return fmt.Errorf("consume plan: %w", err)
	}
	if err := os.Remove(s.path(token, claimedSuffix)); err != nil {
		return fmt.Errorf("remove claimed plan after consume: %w", err)
	}
	if err := syncDirectory(s.dir); err != nil {
		return fmt.Errorf("sync consumed plan: %w", err)
	}
	return nil
}

func (s *Store) newToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := io.ReadFull(s.random, raw); err != nil {
		return "", fmt.Errorf("generate plan token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
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
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return diskRecord{}, ErrMissing
		}
		return diskRecord{}, fmt.Errorf("inspect plan record: %w", err)
	}
	if !info.Mode().IsRegular() {
		return diskRecord{}, ErrUnsafeRecord
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return diskRecord{}, fmt.Errorf("read plan record: %w", err)
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

func (s *Store) absentState(token string) error {
	if s.exists(s.path(token, consumedSuffix)) {
		if s.hasRecord(token, consumedSuffix, Consumed) {
			return ErrConsumed
		}
		return ErrStateConflict
	}
	if s.exists(s.path(token, claimedSuffix)) {
		if s.hasRecord(token, claimedSuffix, Claimed) {
			return ErrClaimed
		}
		return ErrStateConflict
	}
	return ErrMissing
}

func (s *Store) nonPendingState(token string) error {
	if s.exists(s.path(token, consumedSuffix)) {
		if s.hasRecord(token, consumedSuffix, Consumed) {
			return ErrConsumed
		}
		return ErrStateConflict
	}
	if s.exists(s.path(token, claimedSuffix)) {
		if s.hasRecord(token, claimedSuffix, Claimed) {
			return ErrClaimed
		}
		return ErrStateConflict
	}
	return nil
}

func (s *Store) hasRecord(token, suffix string, state State) bool {
	_, err := s.readWithoutExpiry(token, suffix, state)
	return err == nil
}

func (s *Store) readWithoutExpiry(token, suffix string, wantState State) (diskRecord, error) {
	return s.decodeRecord(token, suffix, wantState)
}

func (s *Store) publish(target string, data []byte) error {
	temp, err := os.CreateTemp(s.dir, ".plan-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// A same-directory hard-link publication has rename-like atomic visibility
	// while retaining O_EXCL semantics: an existing state file is never replaced.
	if err := os.Link(tempPath, target); err != nil {
		return err
	}
	if err := os.Remove(tempPath); err != nil {
		return err
	}
	removeTemp = false
	return syncDirectory(s.dir)
}

func ensureDirectory(path string, mode os.FileMode, enforceMode bool) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, mode); err != nil {
			return fmt.Errorf("create state directory: %w", err)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnsafeRoot, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: %s is not a real directory", ErrUnsafeRoot, path)
	}
	if enforceMode {
		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("secure state directory: %w", err)
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

func (s *Store) exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
