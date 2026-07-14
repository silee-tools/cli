package state

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/silee-tools/oma/internal/config"
	"golang.org/x/sys/unix"
)

type testPayload struct {
	Title string `json:"title"`
	ID    int64  `json:"id"`
}

var _ interface{ Close() error } = (*Store)(nil)

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
	t.Cleanup(func() { _ = store.Close() })
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

	assertMode(t, stateRoot, 0o700)
	assertMode(t, filepath.Join(stateRoot, "plans"), 0o700)
	assertMode(t, filepath.Join(stateRoot, "plans", created.Token+pendingSuffix), 0o600)
}

func TestStoreUsesResolvedApplicationStateRoot(t *testing.T) {
	xdgState := t.TempDir()
	paths := config.ResolvePaths(func(key string) string {
		if key == "XDG_STATE_HOME" {
			return xdgState
		}
		return ""
	}, t.TempDir())
	store, err := New(paths.StateRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	canonicalRoot, err := filepath.EvalSymlinks(paths.StateRoot)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonicalRoot, "plans")
	if store.dir != want {
		t.Fatalf("store dir = %q, want %q", store.dir, want)
	}
	if _, err := os.Stat(filepath.Join(paths.StateRoot, "oma")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected double oma directory: %v", err)
	}

	emptyPaths := config.ResolvePaths(func(string) string { return "" }, "")
	if _, err := New(emptyPaths.StateRoot); !errors.Is(err, ErrUnsafeRoot) {
		t.Fatalf("New(empty HOME/XDG StateRoot %q) error = %v, want ErrUnsafeRoot", emptyPaths.StateRoot, err)
	}
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

func TestStoreRejectsEmptyFingerprint(t *testing.T) {
	store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x12}, tokenBytes))
	if _, err := store.Create(testPayload{Title: "fingerprint"}, " \t\n"); !errors.Is(err, ErrInvalidFingerprint) {
		t.Fatalf("Create() error = %v, want ErrInvalidFingerprint", err)
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
	if _, err := New("relative/state"); !errors.Is(err, ErrUnsafeRoot) {
		t.Fatalf("New(relative) error = %v, want ErrUnsafeRoot", err)
	}
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
	if err := os.Symlink(realRoot, filepath.Join(stateRoot, "plans")); err != nil {
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

func TestStoreReservationOwnsTokenNamespaceForItsLifetime(t *testing.T) {
	firstBytes := bytes.Repeat([]byte{0x31}, tokenBytes)
	secondBytes := bytes.Repeat([]byte{0x32}, tokenBytes)
	firstToken := base64.RawURLEncoding.EncodeToString(firstBytes)
	for _, suffix := range []string{pendingSuffix, claimedSuffix, consumedSuffix, reservationSuffix} {
		t.Run(suffix, func(t *testing.T) {
			store := newTestStore(t, time.Now(), append(append([]byte{}, firstBytes...), secondBytes...))
			if err := os.WriteFile(store.path(firstToken, suffix), []byte("occupied"), 0o600); err != nil {
				t.Fatal(err)
			}
			created, err := store.Create(testPayload{Title: "collision"}, "fingerprint")
			if err != nil {
				t.Fatal(err)
			}
			if created.Token == firstToken {
				t.Fatalf("Create() reused token occupied by %s", suffix)
			}
			assertMode(t, store.path(created.Token, reservationSuffix), 0o600)
		})
	}

	store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x33}, tokenBytes))
	created, err := store.Create(testPayload{Title: "lifetime"}, "fingerprint")
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
	if _, err := os.Stat(store.path(created.Token, reservationSuffix)); err != nil {
		t.Fatalf("reservation was not retained after Consume: %v", err)
	}
}

func TestStoreRejectsUnsafeRecordDescriptors(t *testing.T) {
	store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x41}, tokenBytes))
	created, err := store.Create(testPayload{Title: "descriptor"}, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	pendingPath := store.path(created.Token, pendingSuffix)
	var payload testPayload

	if err := os.Chmod(pendingPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(created.Token, &payload); !errors.Is(err, ErrUnsafeRecord) {
		t.Fatalf("Load(0644) error = %v, want ErrUnsafeRecord", err)
	}
	if err := os.Chmod(pendingPath, 0o600); err != nil {
		t.Fatal(err)
	}

	externalLink := filepath.Join(t.TempDir(), "external-link")
	if err := os.Link(pendingPath, externalLink); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(created.Token, &payload); !errors.Is(err, ErrUnsafeRecord) {
		t.Fatalf("Load(external hard link) error = %v, want ErrUnsafeRecord", err)
	}
	if err := os.Remove(externalLink); err != nil {
		t.Fatal(err)
	}

	originalOpen := store.openRead
	store.openRead = func(path string) (*os.File, error) {
		backup := path + ".backup"
		if err := os.Rename(path, backup); err != nil {
			return nil, err
		}
		if err := os.Symlink(backup, path); err != nil {
			return nil, err
		}
		return originalOpen(path)
	}
	if _, err := store.Load(created.Token, &payload); !errors.Is(err, ErrUnsafeRecord) {
		t.Fatalf("Load(symlink swap) error = %v, want ErrUnsafeRecord", err)
	}

	for _, kind := range []string{"directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			store := newTestStore(t, time.Now(), bytes.Repeat([]byte{byte(len(kind) + 80)}, tokenBytes))
			created, err := store.Create(testPayload{Title: kind}, "fingerprint")
			if err != nil {
				t.Fatal(err)
			}
			path := store.path(created.Token, pendingSuffix)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if kind == "directory" {
				if err := os.Mkdir(path, 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := syscall.Mkfifo(path, 0o600); err != nil {
				t.Fatal(err)
			}
			var payload testPayload
			if _, err := store.Load(created.Token, &payload); !errors.Is(err, ErrUnsafeRecord) {
				t.Fatalf("Load(%s) error = %v, want ErrUnsafeRecord", kind, err)
			}
		})
	}
}

func TestStoreReportsCommittedPostPublicationErrors(t *testing.T) {
	injected := errors.New("injected filesystem failure")
	t.Run("prepublication cleanup", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x50}, tokenBytes))
		store.link = func(string, string) error { return os.ErrExist }
		originalRemove := store.remove
		store.remove = func(path string) error {
			if strings.Contains(filepath.Base(path), ".tmp-") {
				return injected
			}
			return originalRemove(path)
		}
		created, err := store.Create(testPayload{Title: "cleanup"}, "fingerprint")
		if created.Token != "" || !errors.Is(err, injected) {
			t.Fatalf("Create() = (%#v, %v), want zero record with cleanup error", created, err)
		}
	})
	t.Run("create temp cleanup", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x51}, tokenBytes))
		originalRemove := store.remove
		store.remove = func(path string) error {
			if strings.Contains(filepath.Base(path), ".tmp-") {
				return injected
			}
			return originalRemove(path)
		}
		created, err := store.Create(testPayload{Title: "committed"}, "fingerprint")
		assertCommittedError(t, err, Pending, false)
		if created.Token == "" {
			t.Fatal("Create() hid the committed token")
		}
		if _, statErr := os.Stat(store.path(created.Token, pendingSuffix)); statErr != nil {
			t.Fatalf("committed pending record missing: %v", statErr)
		}
		var payload testPayload
		if _, loadErr := store.Load(created.Token, &payload); loadErr != nil {
			t.Fatalf("Load() with known temp sibling = %v", loadErr)
		}
	})

	t.Run("claim source cleanup", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x52}, tokenBytes))
		created, err := store.Create(testPayload{Title: "claim"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		originalRemove := store.remove
		store.remove = func(path string) error {
			if strings.HasSuffix(path, pendingSuffix) {
				return injected
			}
			return originalRemove(path)
		}
		var payload testPayload
		claimed, err := store.Claim(created.Token, &payload)
		assertCommittedError(t, err, Claimed, false)
		if claimed.Token != created.Token {
			t.Fatalf("Claim() metadata = %#v", claimed)
		}
		store.remove = originalRemove
		if err := store.Consume(created.Token); err != nil {
			t.Fatalf("deferred Consume() = %v", err)
		}
	})

	t.Run("directory sync ambiguity", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x53}, tokenBytes))
		originalSync := store.syncDir
		syncCalls := 0
		store.syncDir = func(path string) error {
			syncCalls++
			if syncCalls == 2 {
				return injected
			}
			return originalSync(path)
		}
		created, err := store.Create(testPayload{Title: "sync"}, "fingerprint")
		assertCommittedError(t, err, Pending, true)
		if created.Token == "" {
			t.Fatal("Create() hid token after directory sync failure")
		}
	})

	t.Run("claim final sync ambiguity", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x54}, tokenBytes))
		created, err := store.Create(testPayload{Title: "second sync"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		originalSync := store.syncDir
		syncCalls := 0
		store.syncDir = func(path string) error {
			syncCalls++
			if syncCalls == 2 {
				return injected
			}
			return originalSync(path)
		}
		var payload testPayload
		claimed, err := store.Claim(created.Token, &payload)
		assertCommittedError(t, err, Claimed, true)
		if claimed.Token != created.Token {
			t.Fatalf("Claim() metadata = %#v", claimed)
		}
	})

	t.Run("consume source cleanup", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x55}, tokenBytes))
		created, err := store.Create(testPayload{Title: "consume cleanup"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		var payload testPayload
		if _, err := store.Claim(created.Token, &payload); err != nil {
			t.Fatal(err)
		}
		originalRemove := store.remove
		store.remove = func(path string) error {
			if strings.HasSuffix(path, claimedSuffix) {
				return injected
			}
			return originalRemove(path)
		}
		err = store.Consume(created.Token)
		assertCommittedError(t, err, Consumed, false)
		store.remove = originalRemove
		if err := store.Consume(created.Token); err != nil {
			t.Fatalf("Consume() reconciliation = %v", err)
		}
	})
}

func TestStoreReportsConsumedFinalizeErrorsAsCommitted(t *testing.T) {
	injected := errors.New("injected finalize failure")
	for _, suffix := range []string{claimedSuffix, pendingSuffix} {
		t.Run("cleanup "+suffix, func(t *testing.T) {
			store := newTestStore(t, time.Now(), bytes.Repeat([]byte{byte(0x56 + len(suffix))}, tokenBytes))
			created, err := store.Create(testPayload{Title: "observed consumed"}, "fingerprint")
			if err != nil {
				t.Fatal(err)
			}
			pending, err := os.ReadFile(store.path(created.Token, pendingSuffix))
			if err != nil {
				t.Fatal(err)
			}
			var payload testPayload
			if _, err := store.Claim(created.Token, &payload); err != nil {
				t.Fatal(err)
			}
			claimed, err := os.ReadFile(store.path(created.Token, claimedSuffix))
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Consume(created.Token); err != nil {
				t.Fatal(err)
			}
			stale := pending
			if suffix == claimedSuffix {
				stale = claimed
			}
			if err := os.WriteFile(store.path(created.Token, suffix), stale, 0o600); err != nil {
				t.Fatal(err)
			}
			originalRemove := store.remove
			store.remove = func(path string) error {
				if strings.HasSuffix(path, suffix) {
					return injected
				}
				return originalRemove(path)
			}
			err = store.Consume(created.Token)
			assertCommittedError(t, err, Consumed, false)
			if !errors.Is(err, injected) {
				t.Fatalf("Consume() error = %v, want injected cleanup error", err)
			}
		})
	}

	t.Run("durability sync", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x59}, tokenBytes))
		created, err := store.Create(testPayload{Title: "consumed sync"}, "fingerprint")
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
		store.syncDir = func(string) error { return injected }
		err = store.Consume(created.Token)
		assertCommittedError(t, err, Consumed, true)
		if !errors.Is(err, injected) {
			t.Fatalf("Consume() error = %v, want injected sync error", err)
		}
	})
}

func TestStorePreservesAcceptedPublicationLinks(t *testing.T) {
	t.Run("same inode", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x5a}, tokenBytes))
		created, err := store.Create(testPayload{Title: "legacy temp"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		legacy := filepath.Join(store.dir, ".plan-crash")
		if err := os.Link(store.path(created.Token, pendingSuffix), legacy); err != nil {
			t.Fatal(err)
		}
		var payload testPayload
		if _, err := store.Load(created.Token, &payload); err != nil {
			t.Fatalf("Load() = %v", err)
		}
		if _, err := os.Stat(legacy); err != nil {
			t.Fatalf("legacy link was not preserved: %v", err)
		}
	})

	t.Run("second legacy name", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x5b}, tokenBytes))
		created, err := store.Create(testPayload{Title: "legacy error"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		legacy := filepath.Join(store.dir, ".plan-cleanup")
		if err := os.Link(store.path(created.Token, pendingSuffix), legacy); err != nil {
			t.Fatal(err)
		}
		var payload testPayload
		loaded, err := store.Load(created.Token, &payload)
		if err != nil || loaded.Token != created.Token {
			t.Fatalf("Load() = (%#v, %v)", loaded, err)
		}
		if _, err := os.Stat(legacy); err != nil {
			t.Fatalf("legacy link was not preserved: %v", err)
		}
	})

	t.Run("unrelated inode is ignored", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x5c}, tokenBytes))
		created, err := store.Create(testPayload{Title: "unrelated"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(store.dir, ".plan-unrelated"), []byte("other"), 0o600); err != nil {
			t.Fatal(err)
		}
		var payload testPayload
		unrelated := filepath.Join(store.dir, ".plan-unrelated")
		if _, err := store.Load(created.Token, &payload); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if _, err := os.Stat(unrelated); err != nil {
			t.Fatalf("unrelated legacy file was mutated: %v", err)
		}
	})

	t.Run("two tokens own independent legacy links", func(t *testing.T) {
		random := append(bytes.Repeat([]byte{0x5d}, tokenBytes), bytes.Repeat([]byte{0x5e}, tokenBytes)...)
		store := newTestStore(t, time.Now(), random)
		first, err := store.Create(testPayload{Title: "first"}, "first")
		if err != nil {
			t.Fatal(err)
		}
		second, err := store.Create(testPayload{Title: "second"}, "second")
		if err != nil {
			t.Fatal(err)
		}
		firstLegacy := filepath.Join(store.dir, ".plan-first")
		secondLegacy := filepath.Join(store.dir, ".plan-second")
		if err := os.Link(store.path(first.Token, pendingSuffix), firstLegacy); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(store.path(second.Token, pendingSuffix), secondLegacy); err != nil {
			t.Fatal(err)
		}
		var payload testPayload
		if _, err := store.Load(first.Token, &payload); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(second.Token, &payload); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{firstLegacy, secondLegacy} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("legacy link was not preserved at %s: %v", path, err)
			}
		}
	})

	t.Run("token temp", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x5f}, tokenBytes))
		created, err := store.Create(testPayload{Title: "temp artifact"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		temp := filepath.Join(store.dir, "."+created.Token+".tmp-crash")
		if err := os.Link(store.path(created.Token, pendingSuffix), temp); err != nil {
			t.Fatal(err)
		}
		var payload testPayload
		if _, err := store.Load(created.Token, &payload); err != nil {
			t.Fatalf("Load() = %v", err)
		}
		if _, err := os.Stat(temp); err != nil {
			t.Fatalf("token temp link was not preserved: %v", err)
		}
	})
}

func TestStoreNeverMutatesLegacyLinksDuringRead(t *testing.T) {
	t.Run("corrupt JSON remains untouched", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x5f}, tokenBytes))
		created, err := store.Create(testPayload{Title: "corrupt"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		legacy := filepath.Join(store.dir, ".plan-corrupt")
		if err := os.Link(store.path(created.Token, pendingSuffix), legacy); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(store.path(created.Token, pendingSuffix), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		var payload testPayload
		if _, err := store.Load(created.Token, &payload); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("Load() error = %v, want ErrCorrupt", err)
		}
		if _, err := os.Stat(legacy); err != nil {
			t.Fatalf("legacy link was removed before decode validation: %v", err)
		}
	})

	t.Run("close failure remains untouched", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x60}, tokenBytes))
		created, err := store.Create(testPayload{Title: "close"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		legacy := filepath.Join(store.dir, ".plan-close")
		if err := os.Link(store.path(created.Token, pendingSuffix), legacy); err != nil {
			t.Fatal(err)
		}
		injected := errors.New("close failed")
		originalClose := store.closeRead
		closeCalls := 0
		store.closeRead = func(file *os.File) error {
			closeCalls++
			err := originalClose(file)
			if closeCalls == 2 {
				return errors.Join(err, injected)
			}
			return err
		}
		var payload testPayload
		if _, err := store.Load(created.Token, &payload); !errors.Is(err, injected) {
			t.Fatalf("Load() error = %v, want close failure", err)
		}
		if _, err := os.Stat(legacy); err != nil {
			t.Fatalf("legacy link was removed before close succeeded: %v", err)
		}
	})

	t.Run("valid legacy link is preserved without sync", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x61}, tokenBytes))
		created, err := store.Create(testPayload{Title: "sync"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		legacy := filepath.Join(store.dir, ".plan-sync")
		if err := os.Link(store.path(created.Token, pendingSuffix), legacy); err != nil {
			t.Fatal(err)
		}
		store.syncDir = func(string) error { return errors.New("unexpected sync") }
		var payload testPayload
		loaded, err := store.Load(created.Token, &payload)
		if err != nil || loaded.Token != created.Token {
			t.Fatalf("Load() = (%#v, %v)", loaded, err)
		}
		if _, err := os.Stat(legacy); err != nil {
			t.Fatalf("legacy link was not preserved: %v", err)
		}
	})
}

func TestStoreCanonicalizesIntermediateSymlinkOnce(t *testing.T) {
	parent := t.TempDir()
	firstTarget := filepath.Join(parent, "first")
	secondTarget := filepath.Join(parent, "second")
	for _, path := range []string{firstTarget, secondTarget} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(parent, "state-link")
	if err := os.Symlink(firstTarget, link); err != nil {
		t.Fatal(err)
	}
	logicalRoot := filepath.Join(link, "nested", "oma")
	store, err := New(logicalRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	canonicalFirst, err := filepath.EvalSymlinks(firstTarget)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(canonicalFirst, "nested", "oma", "plans")
	if store.dir != wantDir {
		t.Fatalf("store dir = %q, want canonical %q", store.dir, wantDir)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secondTarget, link); err != nil {
		t.Fatal(err)
	}
	store.random = bytes.NewReader(bytes.Repeat([]byte{0x62}, tokenBytes))
	created, err := store.Create(testPayload{Title: "canonical"}, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wantDir, created.Token+pendingSuffix)); err != nil {
		t.Fatalf("record missing from original canonical target: %v", err)
	}
	if _, err := os.Stat(filepath.Join(secondTarget, "nested")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retargeted symlink moved Store: %v", err)
	}
}

func TestStoreRejectsSymlinkSwapForMissingComponents(t *testing.T) {
	for _, name := range []string{"intermediate", "final-app-root"} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			attacker := filepath.Join(parent, "attacker")
			if err := os.Mkdir(attacker, 0o755); err != nil {
				t.Fatal(err)
			}
			firstMissing := filepath.Join(parent, "missing")
			logicalRoot := filepath.Join(firstMissing, "oma")
			if name == "final-app-root" {
				firstMissing = filepath.Join(parent, "oma")
				logicalRoot = firstMissing
			}
			hookCalled := false
			_, err := newWithDirectoryHook(logicalRoot, func(path string) {
				if path != firstMissing || hookCalled {
					return
				}
				hookCalled = true
				if err := os.Symlink(attacker, path); err != nil {
					t.Fatalf("install swap symlink: %v", err)
				}
			})
			if !hookCalled {
				t.Fatal("directory creation hook was not reached")
			}
			if !errors.Is(err, ErrUnsafeRoot) {
				t.Fatalf("New() error = %v, want ErrUnsafeRoot", err)
			}
			assertMode(t, attacker, 0o755)
			if _, err := os.Stat(filepath.Join(attacker, "oma")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("attacker directory was modified: %v", err)
			}
			if _, err := os.Stat(filepath.Join(attacker, "plans")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("attacker directory received plans: %v", err)
			}
		})
	}
}

func TestStoreFallsBackWhenNoFollowFchmodatIsUnsupported(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux fchmodat2 compatibility contract")
	}
	parent := t.TempDir()
	stateRoot := filepath.Join(parent, "missing", "xdg", "state", "oma")
	oldUmask := syscall.Umask(0o777)
	defer syscall.Umask(oldUmask)
	hooks := directoryHooks{
		fchmodat: func(dirfd int, path string, mode uint32, flags int) error {
			if flags != 0 {
				return unix.EOPNOTSUPP
			}
			return unix.Fchmodat(dirfd, path, mode, flags)
		},
	}
	store, err := newWithDirectoryHooks(stateRoot, hooks)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	assertMode(t, store.dir, 0o700)
	for path := stateRoot; path != parent; path = filepath.Dir(path) {
		assertMode(t, path, 0o700)
	}
}

func TestStoreRemovesUnusableDirectoryWhenNoFollowFchmodatIsUnsupported(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux fchmodat2 compatibility contract")
	}
	parent := t.TempDir()
	stateRoot := filepath.Join(parent, "orphan", "oma")
	oldUmask := syscall.Umask(0o777)
	defer syscall.Umask(oldUmask)
	hooks := directoryHooks{
		fchmodat: func(_ int, _ string, _ uint32, flags int) error {
			if flags != 0 {
				return unix.EOPNOTSUPP
			}
			return unix.EIO
		},
	}
	if _, err := newWithDirectoryHooks(stateRoot, hooks); !errors.Is(err, unix.EIO) {
		t.Fatalf("New() error = %v, want EIO", err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "orphan")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed creation left an orphan: %v", err)
	}
}

func TestStoreCloseIsIdempotentAndReleasesDescriptor(t *testing.T) {
	baseline := countOpenFileDescriptors(t)
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	if got := countOpenFileDescriptors(t); got != baseline {
		t.Fatalf("open descriptor count = %d, want baseline %d", got, baseline)
	}

	failedStore, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := failedStore.resource.file.Close(); err != nil {
		t.Fatal(err)
	}
	firstErr := failedStore.Close()
	if firstErr == nil {
		t.Fatal("Close() error = nil after descriptor was already closed")
	}
	if secondErr := failedStore.Close(); secondErr != firstErr {
		t.Fatalf("second Close() error = %v, want identical %v", secondErr, firstErr)
	}
}

func TestStoreCloseWaitsForEveryActivePublicOperation(t *testing.T) {
	for _, action := range []string{"Create", "Load", "Claim", "Consume"} {
		t.Run(action, func(t *testing.T) {
			store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x63}, tokenBytes))
			var token string
			if action != "Create" {
				created, err := store.Create(testPayload{Title: "close race"}, "fingerprint")
				if err != nil {
					t.Fatal(err)
				}
				token = created.Token
				if action == "Consume" {
					var payload testPayload
					if _, err := store.Claim(token, &payload); err != nil {
						t.Fatal(err)
					}
				}
			}
			originalStatAt := store.statAt
			entered := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			store.statAt = func(path string) (unix.Stat_t, error) {
				once.Do(func() {
					close(entered)
					<-release
				})
				return originalStatAt(path)
			}
			operationDone := make(chan error, 1)
			go func() {
				var payload testPayload
				switch action {
				case "Create":
					_, err := store.Create(testPayload{}, "fingerprint")
					operationDone <- err
				case "Load":
					_, err := store.Load(token, &payload)
					operationDone <- err
				case "Claim":
					_, err := store.Claim(token, &payload)
					operationDone <- err
				case "Consume":
					operationDone <- store.Consume(token)
				}
			}()
			<-entered
			closeStarted := make(chan struct{})
			closeDone := make(chan error, 1)
			go func() {
				close(closeStarted)
				closeDone <- store.Close()
			}()
			<-closeStarted
			select {
			case err := <-closeDone:
				t.Fatalf("Close() returned before active %s(): %v", action, err)
			case <-time.After(50 * time.Millisecond):
			}
			close(release)
			if err := <-operationDone; err != nil {
				t.Fatalf("%s() = %v", action, err)
			}
			if err := <-closeDone; err != nil {
				t.Fatalf("Close() = %v", err)
			}
		})
	}
}

func TestStoreRejectsOperationsAfterDirectoryDescriptorCloses(t *testing.T) {
	store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x63}, tokenBytes))
	created, err := store.Create(testPayload{Title: "closed"}, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	var payload testPayload
	checks := []struct {
		name string
		run  func() error
	}{
		{name: "Create", run: func() error { _, err := store.Create(testPayload{}, "fingerprint"); return err }},
		{name: "Load", run: func() error { _, err := store.Load(created.Token, &payload); return err }},
		{name: "Claim", run: func() error { _, err := store.Claim(created.Token, &payload); return err }},
		{name: "Consume", run: func() error { return store.Consume(created.Token) }},
	}
	for _, check := range checks {
		if err := check.run(); !errors.Is(err, ErrUnsafeRoot) {
			t.Errorf("%s() error = %v, want ErrUnsafeRoot", check.name, err)
		}
	}
}

func TestStorePreservesRecordMatchErrors(t *testing.T) {
	tests := []struct {
		name   string
		state  State
		want   error
		mutate func(*testing.T, *Store, string)
	}{
		{name: "consumed corrupt", state: Consumed, want: ErrCorrupt, mutate: func(t *testing.T, store *Store, token string) {
			t.Helper()
			if err := os.WriteFile(store.path(token, consumedSuffix), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "claimed unsafe mode", state: Claimed, want: ErrUnsafeRecord, mutate: func(t *testing.T, store *Store, token string) {
			t.Helper()
			if err := os.Chmod(store.path(token, claimedSuffix), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "claimed read error", state: Claimed, want: syscall.EIO, mutate: func(t *testing.T, store *Store, token string) {
			t.Helper()
			original := store.openRead
			store.openRead = func(path string) (*os.File, error) {
				if strings.HasSuffix(path, claimedSuffix) {
					return nil, syscall.EIO
				}
				return original(path)
			}
		}},
		{name: "claimed wrong state", state: Claimed, want: ErrCorrupt, mutate: func(t *testing.T, store *Store, token string) {
			t.Helper()
			path := store.path(token, claimedSuffix)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data = bytes.Replace(data, []byte(`"state":"claimed"`), []byte(`"state":"pending"`), 1)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "consumed unsafe mode", state: Consumed, want: ErrUnsafeRecord, mutate: func(t *testing.T, store *Store, token string) {
			t.Helper()
			if err := os.Chmod(store.path(token, consumedSuffix), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "consumed read error", state: Consumed, want: syscall.EIO, mutate: func(t *testing.T, store *Store, token string) {
			t.Helper()
			original := store.openRead
			store.openRead = func(path string) (*os.File, error) {
				if strings.HasSuffix(path, consumedSuffix) {
					return nil, syscall.EIO
				}
				return original(path)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newTestStore(t, time.Now(), bytes.Repeat([]byte{byte(0x60 + len(test.name))}, tokenBytes))
			created, err := store.Create(testPayload{Title: test.name}, "fingerprint")
			if err != nil {
				t.Fatal(err)
			}
			var payload testPayload
			if _, err := store.Claim(created.Token, &payload); err != nil {
				t.Fatal(err)
			}
			if test.state == Consumed {
				if err := store.Consume(created.Token); err != nil {
					t.Fatal(err)
				}
			}
			test.mutate(t, store, created.Token)
			_, err = store.Load(created.Token, &payload)
			if !errors.Is(err, ErrStateConflict) || !errors.Is(err, test.want) {
				t.Fatalf("Load() error = %v, want ErrStateConflict and %v", err, test.want)
			}
		})
	}
}

func TestStorePreservesPendingReadErrors(t *testing.T) {
	store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x6f}, tokenBytes))
	created, err := store.Create(testPayload{Title: "pending read"}, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	original := store.openRead
	store.openRead = func(path string) (*os.File, error) {
		if strings.HasSuffix(path, pendingSuffix) {
			return nil, syscall.EIO
		}
		return original(path)
	}
	var payload testPayload
	if _, err := store.Load(created.Token, &payload); !errors.Is(err, syscall.EIO) {
		t.Fatalf("Load() error = %v, want EIO", err)
	}
}

func TestStoreConcurrentConsumeIsIdempotent(t *testing.T) {
	store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x61}, tokenBytes))
	created, err := store.Create(testPayload{Title: "consume race"}, "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	var payload testPayload
	if _, err := store.Claim(created.Token, &payload); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- store.Consume(created.Token)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Consume() = %v", err)
		}
	}
	for _, suffix := range []string{pendingSuffix, claimedSuffix} {
		if _, err := os.Stat(store.path(created.Token, suffix)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stale %s state remains: %v", suffix, err)
		}
	}
	if _, err := os.Stat(store.path(created.Token, consumedSuffix)); err != nil {
		t.Fatalf("consumed tombstone missing: %v", err)
	}
	if _, err := os.Stat(store.path(created.Token, reservationSuffix)); err != nil {
		t.Fatalf("reservation missing: %v", err)
	}
}

func TestStorePropagatesExistenceErrors(t *testing.T) {
	for _, suffix := range []string{reservationSuffix, consumedSuffix, claimedSuffix} {
		t.Run(suffix, func(t *testing.T) {
			store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x71}, tokenBytes))
			created, err := store.Create(testPayload{Title: "stat errors"}, "fingerprint")
			if err != nil {
				t.Fatal(err)
			}
			injected := &os.PathError{Op: "lstat", Path: store.path(created.Token, suffix), Err: syscall.EACCES}
			originalStatAt := store.statAt
			store.statAt = func(path string) (unix.Stat_t, error) {
				if strings.HasSuffix(path, suffix) {
					return unix.Stat_t{}, injected
				}
				return originalStatAt(path)
			}
			var payload testPayload
			if _, err := store.Load(created.Token, &payload); !errors.Is(err, syscall.EACCES) {
				t.Fatalf("Load() error = %v, want EACCES", err)
			}
		})
	}

	t.Run("pending", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x76}, tokenBytes))
		created, err := store.Create(testPayload{Title: "pending stat"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		injected := &os.PathError{Op: "lstat", Path: store.path(created.Token, pendingSuffix), Err: syscall.EIO}
		originalStatAt := store.statAt
		store.statAt = func(path string) (unix.Stat_t, error) {
			if strings.HasSuffix(path, pendingSuffix) {
				return unix.Stat_t{}, injected
			}
			return originalStatAt(path)
		}
		if err := store.Consume(created.Token); !errors.Is(err, syscall.EIO) {
			t.Fatalf("Consume() error = %v, want EIO", err)
		}
	})
}

func TestStoreStateTransitionsAcrossProcesses(t *testing.T) {
	t.Run("claim", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x72}, tokenBytes))
		created, err := store.Create(testPayload{Title: "process claim"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		outputs := runStateProcesses(t, store.dir, created.Token, "claim", 2, "")
		if countMarker(outputs, "RESULT=success") != 1 || countMarker(outputs, "RESULT=claimed") != 1 {
			t.Fatalf("claim outputs = %q", outputs)
		}
	})

	t.Run("consume", func(t *testing.T) {
		store := newTestStore(t, time.Now(), bytes.Repeat([]byte{0x73}, tokenBytes))
		created, err := store.Create(testPayload{Title: "process consume"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		var payload testPayload
		if _, err := store.Claim(created.Token, &payload); err != nil {
			t.Fatal(err)
		}
		outputs := runStateProcesses(t, store.dir, created.Token, "consume", 4, "")
		if countMarker(outputs, "RESULT=success") != 4 {
			t.Fatalf("consume outputs = %q", outputs)
		}
	})

	t.Run("create crossing legacy claim", func(t *testing.T) {
		firstBytes := bytes.Repeat([]byte{0x74}, tokenBytes)
		secondBytes := bytes.Repeat([]byte{0x75}, tokenBytes)
		store := newTestStore(t, time.Now(), firstBytes)
		legacy, err := store.Create(testPayload{Title: "legacy"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(store.path(legacy.Token, reservationSuffix)); err != nil {
			t.Fatal(err)
		}
		random := base64.RawStdEncoding.EncodeToString(append(append([]byte{}, firstBytes...), secondBytes...))
		outputs := runStateProcesses(t, store.dir, legacy.Token, "cross", 2, random)
		if countMarker(outputs, "RESULT=claimed") != 1 || countMarker(outputs, "RESULT=created-other") != 1 {
			t.Fatalf("cross outputs = %q", outputs)
		}
	})
}

func TestStoreProcessHelper(t *testing.T) {
	if os.Getenv("OMA_STATE_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	root := os.Getenv("OMA_STATE_ROOT")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	token := os.Getenv("OMA_STATE_TOKEN")
	switch os.Getenv("OMA_STATE_ACTION") {
	case "claim":
		var payload testPayload
		_, err := store.Claim(token, &payload)
		if err == nil {
			fmt.Println("RESULT=success")
		} else if errors.Is(err, ErrClaimed) {
			fmt.Println("RESULT=claimed")
		} else {
			t.Fatal(err)
		}
	case "consume":
		if err := store.Consume(token); err != nil {
			t.Fatal(err)
		}
		fmt.Println("RESULT=success")
	case "cross":
		if os.Getenv("OMA_STATE_CROSS_ROLE") == "0" {
			var payload testPayload
			if _, err := store.Claim(token, &payload); err != nil {
				t.Fatal(err)
			}
			fmt.Println("RESULT=claimed")
			return
		}
		random, err := base64.RawStdEncoding.DecodeString(os.Getenv("OMA_STATE_RANDOM"))
		if err != nil {
			t.Fatal(err)
		}
		store.random = bytes.NewReader(random)
		created, err := store.Create(testPayload{Title: "new"}, "fingerprint")
		if err != nil {
			t.Fatal(err)
		}
		if created.Token == token {
			t.Fatal("Create reused legacy token")
		}
		fmt.Println("RESULT=created-other")
	default:
		t.Fatalf("unknown helper action")
	}
}

func TestStoreCreatesOwnedDirectoriesWithRestrictiveUmask(t *testing.T) {
	parent := t.TempDir()
	stateRoot := filepath.Join(parent, "state")
	if err := os.Mkdir(stateRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(stateRoot, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldUmask := syscall.Umask(0o777)
	defer syscall.Umask(oldUmask)
	store, err := New(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	assertMode(t, stateRoot, 0o700)
	assertMode(t, store.dir, 0o700)

	missingRoot := filepath.Join(parent, "missing", "xdg", "state", "oma")
	missingStore, err := New(missingRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = missingStore.Close() })
	assertMode(t, missingRoot, 0o700)
	assertMode(t, missingStore.dir, 0o700)
	for path := filepath.Dir(missingRoot); path != parent; path = filepath.Dir(path) {
		assertMode(t, path, 0o700)
	}
}

func assertCommittedError(t *testing.T, err error, state State, ambiguous bool) {
	t.Helper()
	var committed *CommittedError
	if !errors.As(err, &committed) {
		t.Fatalf("error = %v, want *CommittedError", err)
	}
	if committed.State != state || committed.Ambiguous != ambiguous {
		t.Fatalf("committed error = %#v, want state %q ambiguous %t", committed, state, ambiguous)
	}
}

func runStateProcesses(t *testing.T, plansDir, token, action string, count int, random string) []string {
	t.Helper()
	stateRoot := filepath.Dir(plansDir)
	commands := make([]*exec.Cmd, count)
	outputs := make([]bytes.Buffer, count)
	for index := range count {
		command := exec.Command(os.Args[0], "-test.run=^TestStoreProcessHelper$", "-test.v=false")
		command.Env = append(os.Environ(),
			"OMA_STATE_HELPER=1",
			"OMA_STATE_ROOT="+stateRoot,
			"OMA_STATE_TOKEN="+token,
			"OMA_STATE_ACTION="+action,
			fmt.Sprintf("OMA_STATE_CROSS_ROLE=%d", index),
			"OMA_STATE_RANDOM="+random,
		)
		command.Stdout = &outputs[index]
		command.Stderr = &outputs[index]
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		commands[index] = command
	}
	result := make([]string, count)
	for index, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("helper %d: %v\n%s", index, err, outputs[index].String())
		}
		result[index] = outputs[index].String()
	}
	return result
}

func countMarker(outputs []string, marker string) int {
	count := 0
	for _, output := range outputs {
		count += strings.Count(output, marker)
	}
	return count
}

func newTestStore(t *testing.T, now time.Time, random []byte) *Store {
	t.Helper()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	store.random = bytes.NewReader(random)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func countOpenFileDescriptors(t *testing.T) int {
	t.Helper()
	count := 0
	for fd := 0; fd < 1024; fd++ {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err == nil {
			count++
		}
	}
	return count
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
