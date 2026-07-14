package state

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"
)

type testPayload struct {
	Title string `json:"title"`
	ID    int64  `json:"id"`
}

func TestStoreCreatesAndLoadsAPlan(t *testing.T) {
	stateRoot := t.TempDir()
	if err := os.Chmod(stateRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 10, 0, 0, 123, time.UTC)
	random := bytes.Repeat([]byte{0x7f}, tokenBytes)

	store, err := New(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	store.random = bytes.NewReader(random)

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	otherWD := t.TempDir()
	if err := os.Chdir(otherWD); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	created, err := store.Create(testPayload{Title: "정확한 계획", ID: 9_007_199_254_740_993}, "sha256:fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`).MatchString(created.Token) {
		t.Fatalf("token = %q, want 43 canonical base64url characters", created.Token)
	}
	if created.CreatedAt != now || created.ExpiresAt != now.Add(30*time.Minute) {
		t.Fatalf("created metadata = %#v", created)
	}
	if created.State != Pending {
		t.Fatalf("state = %q, want %q", created.State, Pending)
	}

	var loadedPayload testPayload
	loaded, err := store.Load(created.Token, &loadedPayload)
	if err != nil {
		t.Fatal(err)
	}
	if loadedPayload != (testPayload{Title: "정확한 계획", ID: 9_007_199_254_740_993}) {
		t.Fatalf("payload = %#v", loadedPayload)
	}
	if loaded.Fingerprint != "sha256:fingerprint" || loaded.State != Pending {
		t.Fatalf("loaded metadata = %#v", loaded)
	}

	assertMode(t, stateRoot, 0o755)
	assertMode(t, filepath.Join(stateRoot, "oma", "plans"), 0o700)
	assertMode(t, filepath.Join(stateRoot, "oma", "plans", created.Token+pendingSuffix), 0o600)
}

func TestStoreExpiresAtExactBoundary(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	store := newTestStore(t, now, bytes.Repeat([]byte{1}, tokenBytes))
	created, err := store.Create(testPayload{Title: "expires"}, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}

	store.now = func() time.Time { return created.ExpiresAt }
	var payload testPayload
	_, err = store.Load(created.Token, &payload)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("Load() error = %v, want ErrExpired", err)
	}
}

func TestStoreRejectsInvalidTokens(t *testing.T) {
	store := newTestStore(t, time.Now(), bytes.Repeat([]byte{2}, tokenBytes))
	invalid := []string{
		"", "../plan", "plan/other", `plan\\other`,
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA+",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA%",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	for _, token := range invalid {
		t.Run(token, func(t *testing.T) {
			var payload testPayload
			if _, err := store.Load(token, &payload); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("Load(%q) error = %v, want ErrInvalidToken", token, err)
			}
		})
	}
}

func TestStoreDistinguishesMissingCorruptAndUnsafeRecords(t *testing.T) {
	store := newTestStore(t, time.Now(), bytes.Repeat([]byte{3}, tokenBytes))
	missing := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{4}, tokenBytes))
	var payload testPayload
	if _, err := store.Load(missing, &payload); !errors.Is(err, ErrMissing) {
		t.Fatalf("missing error = %v, want ErrMissing", err)
	}

	corrupt := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{5}, tokenBytes))
	if err := os.WriteFile(filepath.Join(store.dir, corrupt+pendingSuffix), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(corrupt, &payload); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("corrupt error = %v, want ErrCorrupt", err)
	}

	target := filepath.Join(store.dir, "target")
	if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	unsafe := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{6}, tokenBytes))
	if err := os.Symlink(target, filepath.Join(store.dir, unsafe+pendingSuffix)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(unsafe, &payload); !errors.Is(err, ErrUnsafeRecord) {
		t.Fatalf("symlink error = %v, want ErrUnsafeRecord", err)
	}
}

func TestStoreRejectsUnsafeStateRoots(t *testing.T) {
	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real")
	if err := os.Mkdir(realRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	symlinkRoot := filepath.Join(parent, "linked")
	if err := os.Symlink(realRoot, symlinkRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := New(symlinkRoot); !errors.Is(err, ErrUnsafeRoot) {
		t.Fatalf("New(symlink) error = %v, want ErrUnsafeRoot", err)
	}

	fileRoot := filepath.Join(parent, "file")
	if err := os.WriteFile(fileRoot, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(fileRoot); !errors.Is(err, ErrUnsafeRoot) {
		t.Fatalf("New(file) error = %v, want ErrUnsafeRoot", err)
	}

	stateRoot := filepath.Join(parent, "state")
	if err := os.Mkdir(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRoot, filepath.Join(stateRoot, "oma")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(stateRoot); !errors.Is(err, ErrUnsafeRoot) {
		t.Fatalf("New(root with linked child) error = %v, want ErrUnsafeRoot", err)
	}
}

func TestStoreRetriesTokenCollisionWithoutReplacingExistingRecord(t *testing.T) {
	firstBytes := bytes.Repeat([]byte{0x11}, tokenBytes)
	secondBytes := bytes.Repeat([]byte{0x22}, tokenBytes)
	store := newTestStore(t, time.Now(), firstBytes)
	first, err := store.Create(testPayload{Title: "keep me"}, "first")
	if err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(store.dir, first.Token+pendingSuffix)
	before, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}

	store.random = bytes.NewReader(append(append([]byte{}, firstBytes...), secondBytes...))
	second, err := store.Create(testPayload{Title: "new plan"}, "second")
	if err != nil {
		t.Fatal(err)
	}
	if second.Token == first.Token {
		t.Fatal("Create() reused a colliding token")
	}
	after, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("Create() replaced the existing collision target")
	}
}

func TestClaimAllowsExactlyOneConcurrentCaller(t *testing.T) {
	store := newTestStore(t, time.Now(), bytes.Repeat([]byte{7}, tokenBytes))
	created, err := store.Create(testPayload{Title: "claim"}, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			var payload testPayload
			_, claimErr := store.Claim(created.Token, &payload)
			errs <- claimErr
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	claimed := 0
	for claimErr := range errs {
		switch {
		case claimErr == nil:
			successes++
		case errors.Is(claimErr, ErrClaimed):
			claimed++
		default:
			t.Fatalf("Claim() error = %v", claimErr)
		}
	}
	if successes != 1 || claimed != 1 {
		t.Fatalf("successes = %d, claimed errors = %d", successes, claimed)
	}
	if _, err := os.Lstat(filepath.Join(store.dir, created.Token+pendingSuffix)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending record still exists: %v", err)
	}
	assertMode(t, filepath.Join(store.dir, created.Token+claimedSuffix), 0o600)

	var payload testPayload
	if _, err := store.Claim(created.Token, &payload); !errors.Is(err, ErrClaimed) {
		t.Fatalf("second Claim() error = %v, want ErrClaimed", err)
	}
}

func TestClaimDoesNotOverwritePreexistingInUseRecord(t *testing.T) {
	store := newTestStore(t, time.Now(), bytes.Repeat([]byte{8}, tokenBytes))
	created, err := store.Create(testPayload{Title: "pending"}, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	inUsePath := filepath.Join(store.dir, created.Token+claimedSuffix)
	foreign := []byte("foreign claimed record")
	if err := os.WriteFile(inUsePath, foreign, 0o600); err != nil {
		t.Fatal(err)
	}

	var payload testPayload
	if _, err := store.Claim(created.Token, &payload); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("Claim() error = %v, want ErrStateConflict", err)
	}
	got, err := os.ReadFile(inUsePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, foreign) {
		t.Fatal("Claim() overwrote a preexisting in-use record")
	}
	if _, err := os.Stat(filepath.Join(store.dir, created.Token+pendingSuffix)); err != nil {
		t.Fatalf("pending record was removed: %v", err)
	}
}

func TestStoreConsumePreservesTombstoneAndPreventsReplay(t *testing.T) {
	store := newTestStore(t, time.Now(), bytes.Repeat([]byte{9}, tokenBytes))
	created, err := store.Create(testPayload{Title: "consume"}, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	var payload testPayload
	if _, err := store.Claim(created.Token, &payload); err != nil {
		t.Fatal(err)
	}
	claimedPath := filepath.Join(store.dir, created.Token+claimedSuffix)
	before, err := os.ReadFile(claimedPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Consume(created.Token); err != nil {
		t.Fatal(err)
	}
	consumedPath := filepath.Join(store.dir, created.Token+consumedSuffix)
	after, err := os.ReadFile(consumedPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, after) {
		t.Fatal("Consume() did not mark the tombstone as consumed")
	}
	tombstone, err := store.readWithoutExpiry(created.Token, consumedSuffix, Consumed)
	if err != nil {
		t.Fatal(err)
	}
	if tombstone.Fingerprint != "fingerprint" {
		t.Fatalf("tombstone fingerprint = %q", tombstone.Fingerprint)
	}
	var tombstonePayload testPayload
	if err := json.Unmarshal(tombstone.Payload, &tombstonePayload); err != nil {
		t.Fatal(err)
	}
	if tombstonePayload.Title != "consume" {
		t.Fatalf("tombstone payload = %#v", tombstonePayload)
	}
	if err := store.Consume(created.Token); err != nil {
		t.Fatalf("idempotent Consume() error = %v", err)
	}
	if _, err := store.Claim(created.Token, &payload); !errors.Is(err, ErrConsumed) {
		t.Fatalf("Claim() after Consume error = %v, want ErrConsumed", err)
	}
	if _, err := store.Load(created.Token, &payload); !errors.Is(err, ErrConsumed) {
		t.Fatalf("Load() after Consume error = %v, want ErrConsumed", err)
	}
}

func TestStoreConsumesAClaimAfterItsTTL(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	store := newTestStore(t, now, bytes.Repeat([]byte{11}, tokenBytes))
	created, err := store.Create(testPayload{Title: "long apply"}, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	var payload testPayload
	if _, err := store.Claim(created.Token, &payload); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return created.ExpiresAt.Add(time.Second) }
	if err := store.Consume(created.Token); err != nil {
		t.Fatalf("Consume() after claim expiry = %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.dir, created.Token+consumedSuffix)); err != nil {
		t.Fatalf("consumed tombstone missing: %v", err)
	}
}

func TestClaimPrioritizesDurableConsumedState(t *testing.T) {
	store := newTestStore(t, time.Now(), bytes.Repeat([]byte{10}, tokenBytes))
	created, err := store.Create(testPayload{Title: "no replay"}, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	pendingPath := filepath.Join(store.dir, created.Token+pendingSuffix)
	pending, err := os.ReadFile(pendingPath)
	if err != nil {
		t.Fatal(err)
	}
	var payload testPayload
	if _, err := store.Claim(created.Token, &payload); err != nil {
		t.Fatal(err)
	}
	if err := store.Consume(created.Token); err != nil {
		t.Fatal(err)
	}
	// Simulate a stale pending artifact restored after consumption. The durable
	// consumed state remains authoritative and must prevent replay.
	if err := os.WriteFile(pendingPath, pending, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(created.Token, &payload); !errors.Is(err, ErrConsumed) {
		t.Fatalf("Claim() error = %v, want ErrConsumed", err)
	}
}

func newTestStore(t *testing.T, now time.Time, random []byte) *Store {
	t.Helper()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	store.random = bytes.NewReader(random)
	return store
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode %s = %#o, want %#o", path, got, want)
	}
}
