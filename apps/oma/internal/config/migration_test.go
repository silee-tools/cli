package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestMigration(t *testing.T) {
	t.Run("success creates protected canonical and compatibility link", func(t *testing.T) {
		paths := testPaths(t)
		writeFile(t, paths.Legacy, validTOML, 0o640)

		migration, err := PlanMigration(paths)
		if err != nil {
			t.Fatal(err)
		}
		if migration == nil {
			t.Fatal("PlanMigration() = nil, want migration")
		}
		if err := migration.Apply(func(Config) error { return nil }); err != nil {
			t.Fatal(err)
		}

		assertRegularFile(t, paths.Canonical, validTOML, 0o600)
		assertMode(t, filepath.Dir(paths.Canonical), 0o700)
		assertSymlink(t, paths.Legacy, paths.Canonical)
	})

	t.Run("invalid TOML leaves legacy untouched", func(t *testing.T) {
		paths := testPaths(t)
		original := "jira_base_url = ["
		writeFile(t, paths.Legacy, original, 0o640)

		migration, err := PlanMigration(paths)
		if err != nil {
			t.Fatal(err)
		}
		if migration == nil {
			t.Fatal("PlanMigration() = nil, want migration")
		}
		if err := migration.Apply(func(Config) error { return nil }); err == nil {
			t.Fatal("Apply() error = nil, want TOML decoding error")
		}
		assertRegularFile(t, paths.Legacy, original, 0o640)
		assertAbsent(t, paths.Canonical)
	})

	t.Run("validation failure leaves legacy untouched", func(t *testing.T) {
		paths := testPaths(t)
		writeFile(t, paths.Legacy, validTOML, 0o640)
		validationErr := errors.New("authentication rejected")

		migration, err := PlanMigration(paths)
		if err != nil {
			t.Fatal(err)
		}
		if migration == nil {
			t.Fatal("PlanMigration() = nil, want migration")
		}
		err = migration.Apply(func(Config) error { return validationErr })
		if !errors.Is(err, validationErr) {
			t.Fatalf("Apply() error = %v, want %v", err, validationErr)
		}
		assertRegularFile(t, paths.Legacy, validTOML, 0o640)
		assertAbsent(t, paths.Canonical)
	})

	t.Run("symlink failure restores original legacy file", func(t *testing.T) {
		paths := testPaths(t)
		writeFile(t, paths.Legacy, validTOML, 0o640)
		symlinkErr := errors.New("injected symlink failure")
		originalOps := migrationOS
		migrationOS.symlink = func(string, string) error { return symlinkErr }
		t.Cleanup(func() { migrationOS = originalOps })

		migration, err := PlanMigration(paths)
		if err != nil {
			t.Fatal(err)
		}
		if migration == nil {
			t.Fatal("PlanMigration() = nil, want migration")
		}
		err = migration.Apply(func(Config) error { return nil })
		if !errors.Is(err, symlinkErr) {
			t.Fatalf("Apply() error = %v, want %v", err, symlinkErr)
		}
		assertRegularFile(t, paths.Legacy, validTOML, 0o640)
		assertAbsent(t, paths.Canonical)
	})

	t.Run("rollback failure is joined while original is restored", func(t *testing.T) {
		paths := testPaths(t)
		writeFile(t, paths.Legacy, validTOML, 0o640)
		symlinkErr := errors.New("injected symlink failure")
		rollbackErr := errors.New("injected rollback cleanup failure")
		originalOps := migrationOS
		migrationOS.symlink = func(string, string) error { return symlinkErr }
		migrationOS.remove = func(path string) error {
			err := os.Remove(path)
			if strings.HasPrefix(path, paths.Canonical+".oma-quarantine-") && err == nil {
				return rollbackErr
			}
			return err
		}
		t.Cleanup(func() { migrationOS = originalOps })

		migration, err := PlanMigration(paths)
		if err != nil {
			t.Fatal(err)
		}
		if migration == nil {
			t.Fatal("PlanMigration() = nil, want migration")
		}
		err = migration.Apply(func(Config) error { return nil })
		if !errors.Is(err, symlinkErr) || !errors.Is(err, rollbackErr) {
			t.Fatalf("Apply() error = %v, want joined symlink and rollback errors", err)
		}
		assertRegularFile(t, paths.Legacy, validTOML, 0o640)
		assertAbsent(t, paths.Canonical)
	})
}

func TestPlanMigrationRejectsConflictingFiles(t *testing.T) {
	paths := testPaths(t)
	writeFile(t, paths.Canonical, validTOML, 0o600)
	writeFile(t, paths.Legacy, validTOML, 0o600)
	if _, err := PlanMigration(paths); err == nil || !strings.Contains(err.Error(), "configuration conflict") {
		t.Fatalf("PlanMigration() error = %v, want conflict", err)
	}
}

func TestMigrationRollbackPreservesCompetingLegacyFile(t *testing.T) {
	paths := testPaths(t)
	writeFile(t, paths.Legacy, validTOML, 0o640)
	sentinel := "created by a competing process"
	originalOps := migrationOS
	migrationOS.symlink = func(_, destination string) error {
		if err := os.WriteFile(destination, []byte(sentinel), 0o644); err != nil {
			return err
		}
		return fs.ErrExist
	}
	t.Cleanup(func() { migrationOS = originalOps })

	migration, err := PlanMigration(paths)
	if err != nil {
		t.Fatal(err)
	}
	if migration == nil {
		t.Fatal("PlanMigration() = nil, want migration")
	}
	err = migration.Apply(func(Config) error { return nil })
	if !errors.Is(err, fs.ErrExist) || !strings.Contains(err.Error(), "replace legacy configuration") || !strings.Contains(err.Error(), "restore legacy configuration") {
		t.Fatalf("Apply() error = %v, want joined symlink and restore-conflict errors", err)
	}

	assertRegularFile(t, paths.Legacy, sentinel, 0o644)
	marker, backup := transactionPaths(paths)
	assertRegularFile(t, backup, validTOML, 0o640)
	assertRegularExists(t, marker, 0o600)
}

func TestMigrationRollbackPreservesCompetingCanonicalFile(t *testing.T) {
	paths := testPaths(t)
	writeFile(t, paths.Legacy, validTOML, 0o640)
	sentinel := "replacement canonical from a competing process"
	symlinkErr := errors.New("injected symlink failure")
	originalOps := migrationOS
	migrationOS.symlink = func(_, _ string) error {
		if err := os.Remove(paths.Canonical); err != nil {
			return err
		}
		if err := os.WriteFile(paths.Canonical, []byte(sentinel), 0o644); err != nil {
			return err
		}
		return symlinkErr
	}
	t.Cleanup(func() { migrationOS = originalOps })

	migration, err := PlanMigration(paths)
	if err != nil {
		t.Fatal(err)
	}
	if migration == nil {
		t.Fatal("PlanMigration() = nil, want migration")
	}
	err = migration.Apply(func(Config) error { return nil })
	if !errors.Is(err, symlinkErr) || !strings.Contains(err.Error(), "unexpected identity") {
		t.Fatalf("Apply() error = %v, want symlink and ownership errors", err)
	}

	assertRegularFile(t, paths.Canonical, sentinel, 0o644)
	assertRegularFile(t, paths.Legacy, validTOML, 0o640)
}

func TestMigrationCanonicalCommitDoesNotReplaceCompetingFile(t *testing.T) {
	paths := testPaths(t)
	writeFile(t, paths.Legacy, validTOML, 0o640)
	sentinel := "canonical from a competing process"
	originalOps := migrationOS
	migrationOS.beforeCanonicalCommit = func(path string) error {
		return os.WriteFile(path, []byte(sentinel), 0o644)
	}
	t.Cleanup(func() { migrationOS = originalOps })

	migration, err := PlanMigration(paths)
	if err != nil {
		t.Fatal(err)
	}
	if migration == nil {
		t.Fatal("PlanMigration() = nil, want migration")
	}
	err = migration.Apply(func(Config) error { return nil })
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("Apply() error = %v, want existing-file error", err)
	}

	assertRegularFile(t, paths.Canonical, sentinel, 0o644)
	assertRegularFile(t, paths.Legacy, validTOML, 0o640)
	marker, backup := transactionPaths(paths)
	assertAbsent(t, marker)
	assertAbsent(t, backup)
}

func TestPlanMigrationRecoversInterruptedTransaction(t *testing.T) {
	tests := []struct {
		name      string
		setCrash  func(*migrationFileOps)
		wantFinal bool
	}{
		{
			name: "after canonical commit",
			setCrash: func(ops *migrationFileOps) {
				ops.afterCanonicalCommit = func() { panic(simulatedCrash{}) }
			},
		},
		{
			name: "after legacy backup",
			setCrash: func(ops *migrationFileOps) {
				ops.afterLegacyBackup = func() { panic(simulatedCrash{}) }
			},
		},
		{
			name: "after symlink creation",
			setCrash: func(ops *migrationFileOps) {
				ops.afterSymlink = func() { panic(simulatedCrash{}) }
			},
			wantFinal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			writeFile(t, paths.Legacy, validTOML, 0o640)
			migration, err := PlanMigration(paths)
			if err != nil {
				t.Fatal(err)
			}
			if migration == nil {
				t.Fatal("PlanMigration() = nil, want migration")
			}

			originalOps := migrationOS
			tt.setCrash(&migrationOS)
			assertSimulatedCrash(t, func() {
				_ = migration.Apply(func(Config) error { return nil })
			})
			migrationOS = originalOps
			t.Cleanup(func() { migrationOS = originalOps })

			next, err := PlanMigration(paths)
			if err != nil {
				t.Fatal(err)
			}
			marker, backup := transactionPaths(paths)
			assertAbsent(t, marker)
			assertAbsent(t, backup)
			if tt.wantFinal {
				if next != nil {
					t.Fatal("PlanMigration() returned migration after completed recovery")
				}
				assertRegularFile(t, paths.Canonical, validTOML, 0o600)
				assertSymlink(t, paths.Legacy, paths.Canonical)
				return
			}
			if next == nil {
				t.Fatal("PlanMigration() = nil, want migration after rollback recovery")
			}
			assertRegularFile(t, paths.Legacy, validTOML, 0o640)
			assertAbsent(t, paths.Canonical)
		})
	}
}

func TestDirectorySyncSupported(t *testing.T) {
	dir := t.TempDir()
	if err := syncDirectory(dir); err != nil {
		t.Fatalf("syncDirectory(%s): %v", dir, err)
	}
}

func TestMigrationSerializesActiveApplyAndRecovery(t *testing.T) {
	paths := testPaths(t)
	writeFile(t, paths.Legacy, validTOML, 0o640)
	migration, err := PlanMigration(paths)
	if err != nil {
		t.Fatal(err)
	}
	if migration == nil {
		t.Fatal("PlanMigration() = nil, want migration")
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseFirst := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseFirst()
	originalOps := migrationOS
	var once sync.Once
	migrationOS.afterCanonicalCommit = func() {
		once.Do(func() { close(entered) })
		<-release
	}
	t.Cleanup(func() { migrationOS = originalOps })

	applyResult := make(chan error, 1)
	go func() { applyResult <- migration.Apply(func(Config) error { return nil }) }()
	<-entered
	_, planBusyErr := PlanMigration(paths)
	applyBusyErr := migration.Apply(func(Config) error { return nil })
	releaseFirst()
	if err := <-applyResult; err != nil {
		t.Fatal(err)
	}
	if !errors.Is(planBusyErr, errMigrationBusy) {
		t.Errorf("concurrent PlanMigration() error = %v, want busy", planBusyErr)
	}
	if !errors.Is(applyBusyErr, errMigrationBusy) {
		t.Errorf("concurrent Apply() error = %v, want busy", applyBusyErr)
	}

	assertRegularFile(t, paths.Canonical, validTOML, 0o600)
	assertSymlink(t, paths.Legacy, paths.Canonical)
	marker, backup := transactionPaths(paths)
	assertAbsent(t, marker)
	assertAbsent(t, backup)
}

func TestQuarantinePreservesReplacementAfterOwnershipCheck(t *testing.T) {
	paths := testPaths(t)
	writeFile(t, paths.Legacy, validTOML, 0o640)
	sentinel := "replacement after ownership check"
	symlinkErr := errors.New("injected symlink failure")
	originalOps := migrationOS
	replaced := false
	migrationOS.afterOwnershipCheck = func(path string) {
		if path != paths.Canonical || replaced {
			return
		}
		replaced = true
		if err := os.Remove(path); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte(sentinel), 0o644); err != nil {
			panic(err)
		}
	}
	migrationOS.symlink = func(string, string) error { return symlinkErr }
	t.Cleanup(func() { migrationOS = originalOps })

	migration, err := PlanMigration(paths)
	if err != nil {
		t.Fatal(err)
	}
	err = migration.Apply(func(Config) error { return nil })
	if !errors.Is(err, symlinkErr) || !strings.Contains(err.Error(), "quarantine") {
		t.Fatalf("Apply() error = %v, want symlink and quarantine ownership errors", err)
	}
	assertRegularFile(t, paths.Canonical, sentinel, 0o644)
	matches, err := filepath.Glob(paths.Canonical + ".oma-quarantine-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("quarantine matches = %v, want one preserved copy", matches)
	}
	assertRegularFile(t, matches[0], sentinel, 0o644)
}

func TestSymlinkQuarantineRestoresRegularReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy-link")
	target := filepath.Join(root, "canonical")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	sentinel := "regular replacement for owned symlink"
	originalOps := migrationOS
	migrationOS.afterOwnershipCheck = func(checked string) {
		if checked != path {
			return
		}
		if err := os.Remove(path); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte(sentinel), 0o644); err != nil {
			panic(err)
		}
	}
	t.Cleanup(func() { migrationOS = originalOps })

	err := removeOwnedSymlink(path, target, "test-token")
	if err == nil || !strings.Contains(err.Error(), "quarantine") {
		t.Fatalf("removeOwnedSymlink() error = %v, want quarantine conflict", err)
	}
	assertRegularFile(t, path, sentinel, 0o644)
	quarantine := path + ".oma-quarantine-test-token"
	assertRegularFile(t, quarantine, sentinel, 0o644)
}

func TestPlanMigrationRecoversEarlyTransactionInterruptions(t *testing.T) {
	tests := []struct {
		name     string
		setCrash func(*migrationFileOps)
	}{
		{
			name: "before marker",
			setCrash: func(ops *migrationFileOps) {
				ops.beforeMarker = func() { panic(simulatedCrash{}) }
			},
		},
		{
			name: "after marker established",
			setCrash: func(ops *migrationFileOps) {
				ops.afterMarkerEstablished = func() { panic(simulatedCrash{}) }
			},
		},
		{
			name: "after canonical link before staged cleanup",
			setCrash: func(ops *migrationFileOps) {
				ops.afterCanonicalLink = func() { panic(simulatedCrash{}) }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			writeFile(t, paths.Legacy, validTOML, 0o640)
			migration, err := PlanMigration(paths)
			if err != nil {
				t.Fatal(err)
			}
			originalOps := migrationOS
			tt.setCrash(&migrationOS)
			assertSimulatedCrash(t, func() {
				_ = migration.Apply(func(Config) error { return nil })
			})
			migrationOS = originalOps
			t.Cleanup(func() { migrationOS = originalOps })

			if _, err := PlanMigration(paths); err != nil {
				t.Fatal(err)
			}
			assertNoMatches(t, filepath.Join(filepath.Dir(paths.Canonical), ".config.toml.*"))
			assertNoMatches(t, paths.Canonical+".oma-staged-*")
			assertNoMatches(t, filepath.Join(filepath.Dir(paths.Legacy), ".oma-migration.*"))
			marker, backup := transactionPaths(paths)
			assertAbsent(t, marker)
			assertAbsent(t, backup)
			if _, err := os.Lstat(paths.Legacy); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPlanAndRejectedApplyLeaveLegacyDirectoryUnchanged(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		validate func(Config) error
	}{
		{name: "invalid TOML", content: "jira_base_url = [", validate: func(Config) error { return nil }},
		{name: "validation failure", content: validTOML, validate: func(Config) error { return errors.New("validation rejected") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			writeFile(t, paths.Legacy, tt.content, 0o640)
			if err := os.Chmod(filepath.Dir(paths.Legacy), 0o750); err != nil {
				t.Fatal(err)
			}
			before := snapshotDirectory(t, filepath.Dir(paths.Legacy))
			migration, err := PlanMigration(paths)
			if err != nil {
				t.Fatal(err)
			}
			if migration == nil {
				t.Fatal("PlanMigration() = nil, want migration")
			}
			assertDirectorySnapshot(t, filepath.Dir(paths.Legacy), before)
			if err := migration.Apply(tt.validate); err == nil {
				t.Fatal("Apply() error = nil, want rejection")
			}
			assertDirectorySnapshot(t, filepath.Dir(paths.Legacy), before)
		})
	}
}

func TestQuarantineMoveDoesNotReplacePreemptedDestination(t *testing.T) {
	tests := []struct {
		name           string
		createSentinel func(t *testing.T, path string)
		assertSentinel func(t *testing.T, path string)
	}{
		{
			name:           "regular destination",
			createSentinel: func(t *testing.T, path string) { writeFile(t, path, "quarantine sentinel", 0o640) },
			assertSentinel: func(t *testing.T, path string) { assertRegularFile(t, path, "quarantine sentinel", 0o640) },
		},
		{
			name: "symlink destination",
			createSentinel: func(t *testing.T, path string) {
				if err := os.Symlink("sentinel-target", path); err != nil {
					t.Fatal(err)
				}
			},
			assertSentinel: func(t *testing.T, path string) { assertSymlink(t, path, "sentinel-target") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "owned")
			writeFile(t, path, "owned content", 0o600)
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			id, err := identityOf(info)
			if err != nil {
				t.Fatal(err)
			}
			quarantine := path + ".oma-quarantine-known-token"
			originalOps := migrationOS
			migrationOS.beforeQuarantineMove = func(_, destination string) { tt.createSentinel(t, destination) }
			t.Cleanup(func() { migrationOS = originalOps })

			err = removeOwnedRegular(path, id, "known-token")
			if !errors.Is(err, fs.ErrExist) {
				t.Fatalf("removeOwnedRegular() error = %v, want destination-exists error", err)
			}
			assertRegularFile(t, path, "owned content", 0o600)
			tt.assertSentinel(t, quarantine)
		})
	}
}

func TestPlanMigrationRecoversQuarantineInterruptions(t *testing.T) {
	tests := []struct {
		name  string
		match func(Paths, string) bool
		err   error
	}{
		{name: "canonical staged move crash", match: func(p Paths, path string) bool { return strings.HasPrefix(path, p.Canonical+".oma-staged-") }},
		{name: "legacy move crash", match: func(p Paths, path string) bool { return path == p.Legacy }},
		{name: "backup move crash", match: func(p Paths, path string) bool { return path == p.Legacy+".oma-migration-backup" }},
		{name: "marker move crash", match: func(p Paths, path string) bool { return path == p.Legacy+".oma-migration" }},
		{name: "legacy move sync failure", match: func(p Paths, path string) bool { return path == p.Legacy }, err: errors.New("injected directory sync failure")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			writeFile(t, paths.Legacy, validTOML, 0o640)
			migration, err := PlanMigration(paths)
			if err != nil {
				t.Fatal(err)
			}
			originalOps := migrationOS
			triggered := false
			migrationOS.afterQuarantineMove = func(path, _ string) error {
				if triggered || !tt.match(paths, path) {
					return nil
				}
				triggered = true
				if tt.err != nil {
					return tt.err
				}
				panic(simulatedCrash{})
			}
			if tt.err == nil {
				assertSimulatedCrash(t, func() { _ = migration.Apply(func(Config) error { return nil }) })
			} else if err := migration.Apply(func(Config) error { return nil }); !errors.Is(err, tt.err) {
				t.Fatalf("Apply() error = %v, want %v", err, tt.err)
			}
			migrationOS = originalOps
			t.Cleanup(func() { migrationOS = originalOps })
			if !triggered {
				t.Fatal("quarantine interruption hook was not reached")
			}

			if _, err := PlanMigration(paths); err != nil {
				t.Fatal(err)
			}
			assertNoMatches(t, paths.Canonical+".oma-staged-*")
			assertNoMatches(t, paths.Canonical+"*.oma-quarantine-*")
			assertNoMatches(t, paths.Legacy+"*.oma-quarantine-*")
			marker, backup := transactionPaths(paths)
			assertAbsent(t, marker)
			assertAbsent(t, backup)
			if _, err := os.Lstat(paths.Canonical); err == nil {
				assertRegularFile(t, paths.Canonical, validTOML, 0o600)
				assertSymlink(t, paths.Legacy, paths.Canonical)
			} else {
				assertRegularFile(t, paths.Legacy, validTOML, 0o640)
			}
		})
	}
}

func TestPlanMigrationRecoversMarkerEstablishmentFailures(t *testing.T) {
	tests := []struct {
		name     string
		setFault func(*migrationFileOps, error)
		wantErr  bool
	}{
		{name: "after staged marker open", setFault: func(ops *migrationFileOps, _ error) { ops.afterMarkerOpen = func() { panic(simulatedCrash{}) } }},
		{name: "after partial marker write", setFault: func(ops *migrationFileOps, _ error) { ops.afterMarkerPartialWrite = func() { panic(simulatedCrash{}) } }},
		{name: "after marker file sync", setFault: func(ops *migrationFileOps, _ error) { ops.afterMarkerFileSync = func() { panic(simulatedCrash{}) } }},
		{name: "marker parent sync failure", wantErr: true, setFault: func(ops *migrationFileOps, injected error) {
			ops.markerDirectorySync = func(string) error { return injected }
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			writeFile(t, paths.Legacy, validTOML, 0o640)
			migration, err := PlanMigration(paths)
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected marker sync failure")
			originalOps := migrationOS
			tt.setFault(&migrationOS, injected)
			if tt.wantErr {
				if err := migration.Apply(func(Config) error { return nil }); !errors.Is(err, injected) {
					t.Fatalf("Apply() error = %v, want %v", err, injected)
				}
			} else {
				assertSimulatedCrash(t, func() { _ = migration.Apply(func(Config) error { return nil }) })
			}
			migrationOS = originalOps
			t.Cleanup(func() { migrationOS = originalOps })

			if _, err := PlanMigration(paths); err != nil {
				t.Fatal(err)
			}
			assertRegularFile(t, paths.Legacy, validTOML, 0o640)
			assertAbsent(t, paths.Canonical)
			marker, backup := transactionPaths(paths)
			assertAbsent(t, marker)
			assertAbsent(t, backup)
			assertNoMatches(t, marker+".staged-*")
		})
	}
}

type directorySnapshot struct {
	mode    os.FileMode
	entries []string
}

func snapshotDirectory(t *testing.T, path string) directorySnapshot {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return directorySnapshot{mode: info.Mode().Perm(), entries: names}
}

func assertDirectorySnapshot(t *testing.T, path string, want directorySnapshot) {
	t.Helper()
	got := snapshotDirectory(t, path)
	if got.mode != want.mode || strings.Join(got.entries, "\x00") != strings.Join(want.entries, "\x00") {
		t.Fatalf("directory snapshot = %#v, want %#v", got, want)
	}
}

func assertNoMatches(t *testing.T, pattern string) {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches for %s = %v, want none", pattern, matches)
	}
}

type simulatedCrash struct{}

func assertSimulatedCrash(t *testing.T, run func()) {
	t.Helper()
	defer func() {
		if _, ok := recover().(simulatedCrash); !ok {
			t.Fatal("operation did not stop at injected crash point")
		}
	}()
	run()
}

func transactionPaths(paths Paths) (string, string) {
	return paths.Legacy + ".oma-migration", paths.Legacy + ".oma-migration-backup"
}

func assertRegularFile(t *testing.T, path, wantContent string, wantMode os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("%s mode = %s, want regular file", path, info.Mode())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != wantContent {
		t.Fatalf("%s content = %q, want %q", path, content, wantContent)
	}
	if info.Mode().Perm() != wantMode {
		t.Fatalf("%s mode = %#o, want %#o", path, info.Mode().Perm(), wantMode)
	}
}

func assertRegularExists(t *testing.T, path string, wantMode os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("%s mode = %s, want regular file", path, info.Mode())
	}
	if info.Mode().Perm() != wantMode {
		t.Fatalf("%s mode = %#o, want %#o", path, info.Mode().Perm(), wantMode)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s mode = %#o, want %#o", path, info.Mode().Perm(), want)
	}
}

func assertSymlink(t *testing.T, path, wantTarget string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s mode = %s, want symlink", path, info.Mode())
	}
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatal(err)
	}
	if target != wantTarget {
		t.Fatalf("symlink target = %q, want %q", target, wantTarget)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Lstat(%s) error = %v, want not exist", path, err)
	}
}
