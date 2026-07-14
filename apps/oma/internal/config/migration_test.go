package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
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

func TestPlanMigrationDoesNotRecoverTransactionCreatedAfterReadOnlyCheck(t *testing.T) {
	paths := testPaths(t)
	writeFile(t, paths.Legacy, validTOML, 0o640)
	migration, err := PlanMigration(paths)
	if err != nil {
		t.Fatal(err)
	}
	if migration == nil {
		t.Fatal("PlanMigration() = nil, want migration")
	}

	markerEstablished := make(chan struct{})
	releaseApply := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseApply) }) }
	defer release()
	originalOps := migrationOS
	var startOnce sync.Once
	applyResult := make(chan error, 1)
	migrationOS.afterMarkerEstablished = func() {
		close(markerEstablished)
		<-releaseApply
	}
	migrationOS.afterRecoveryCheck = func() {
		startOnce.Do(func() {
			go func() { applyResult <- migration.Apply(func(Config) error { return nil }) }()
			<-markerEstablished
		})
	}
	t.Cleanup(func() { migrationOS = originalOps })

	if _, err := PlanMigration(paths); err != nil {
		t.Fatalf("read-only PlanMigration() error = %v", err)
	}
	marker, _ := transactionPaths(paths)
	assertRegularExists(t, marker, 0o600)
	release()
	if err := <-applyResult; err != nil {
		t.Fatalf("concurrent Apply() error = %v, want success", err)
	}
	assertRegularFile(t, paths.Canonical, validTOML, 0o600)
	assertSymlink(t, paths.Legacy, paths.Canonical)
}

func TestRenameNoReplaceRouteUsesOperatingSystemDirFD(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		goarch string
		trap   uintptr
		dirFD  uintptr
		flags  uintptr
	}{
		{name: "darwin amd64", goos: "darwin", goarch: "amd64", trap: 488, dirFD: ^uintptr(1), flags: 0x4},
		{name: "darwin arm64", goos: "darwin", goarch: "arm64", trap: 488, dirFD: ^uintptr(1), flags: 0x4},
		{name: "linux amd64", goos: "linux", goarch: "amd64", trap: 316, dirFD: ^uintptr(99), flags: 0x1},
		{name: "linux arm64", goos: "linux", goarch: "arm64", trap: 276, dirFD: ^uintptr(99), flags: 0x1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trap, dirFD, flags, err := renameNoReplaceRoute(tt.goos, tt.goarch)
			if err != nil {
				t.Fatal(err)
			}
			if trap != tt.trap || dirFD != tt.dirFD || flags != tt.flags {
				t.Fatalf("route = (%d, %d, %#x), want (%d, %d, %#x)", trap, dirFD, flags, tt.trap, tt.dirFD, tt.flags)
			}
		})
	}

	t.Run("linux relative paths reach syscall with linux dirfd", func(t *testing.T) {
		called := false
		err := renameNoReplaceForPlatform(
			"relative-source",
			"relative-destination",
			"linux",
			"amd64",
			func(trap, sourceDirFD, sourcePointer, destinationDirFD, destinationPointer, flags, _ uintptr) (uintptr, uintptr, syscall.Errno) {
				called = true
				if trap != 316 || sourceDirFD != ^uintptr(99) || destinationDirFD != ^uintptr(99) || flags != 0x1 {
					t.Fatalf("syscall route = (%d, %d, %d, %#x), want linux renameat2 route", trap, sourceDirFD, destinationDirFD, flags)
				}
				if sourcePointer == 0 || destinationPointer == 0 {
					t.Fatal("relative path pointers must be passed to syscall")
				}
				return 0, 0, 0
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Fatal("raw syscall seam was not called")
		}
	})
}

func TestRenameNoReplaceSurvivesGCBeforeDarwinSyscall(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("actual syscall stress runs on Darwin")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	for iteration := 0; iteration < 32; iteration++ {
		content := "marker content"
		writeFile(t, source, content, 0o600)
		err := renameNoReplaceForPlatform(
			source,
			destination,
			runtime.GOOS,
			runtime.GOARCH,
			func(trap, sourceDirFD, sourcePointer, destinationDirFD, destinationPointer, flags, zero uintptr) (uintptr, uintptr, syscall.Errno) {
				runtime.GC()
				debug.FreeOSMemory()
				return syscall.Syscall6(trap, sourceDirFD, sourcePointer, destinationDirFD, destinationPointer, flags, zero)
			},
		)
		if err != nil {
			t.Fatalf("iteration %d: renameNoReplaceForPlatform() error = %v", iteration, err)
		}
		assertRegularFile(t, destination, content, 0o600)
		assertAbsent(t, source)
		if err := os.Remove(destination); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlanMigrationPreservesStagedMarkerReplacementAfterRead(t *testing.T) {
	tests := []struct {
		name           string
		createSentinel func(*testing.T, string)
		assertSentinel func(*testing.T, string)
	}{
		{
			name:           "regular file",
			createSentinel: func(t *testing.T, path string) { writeFile(t, path, "external marker sentinel", 0o640) },
			assertSentinel: func(t *testing.T, path string) { assertRegularFile(t, path, "external marker sentinel", 0o640) },
		},
		{
			name: "symlink",
			createSentinel: func(t *testing.T, path string) {
				if err := os.Symlink("external-marker-target", path); err != nil {
					t.Fatal(err)
				}
			},
			assertSentinel: func(t *testing.T, path string) { assertSymlink(t, path, "external-marker-target") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			marker, target := createCompletedAnchoredMarker(t, paths)
			originalOps := migrationOS
			migrationOS.afterStagedMarkerRead = func(path string) {
				if path != target {
					return
				}
				if err := os.Remove(path); err != nil {
					panic(err)
				}
				tt.createSentinel(t, path)
			}
			t.Cleanup(func() { migrationOS = originalOps })

			if _, err := PlanMigration(paths); err == nil || !strings.Contains(err.Error(), "identity") {
				t.Fatalf("PlanMigration() error = %v, want staged marker identity conflict", err)
			}
			tt.assertSentinel(t, target)
			assertAbsent(t, marker)
			assertAbsent(t, marker+".staged-anchor")
			assertNoMatches(t, marker+".staged-quarantine-*")

			migrationOS = originalOps
			if _, err := PlanMigration(paths); err == nil {
				t.Fatal("reentered PlanMigration() error = nil, want preserved sentinel conflict")
			}
			tt.assertSentinel(t, target)
			assertRegularFile(t, paths.Legacy, validTOML, 0o640)
		})
	}
}

func TestPlanMigrationRecoversStagedMarkerPromotionInterruptions(t *testing.T) {
	tests := []struct {
		name     string
		setFault func(*migrationFileOps, string, error)
		wantErr  bool
	}{
		{
			name: "after promotion quarantine move",
			setFault: func(ops *migrationFileOps, target string, _ error) {
				ops.afterQuarantineMove = func(path, _ string) error {
					if path == target {
						panic(simulatedCrash{})
					}
					return nil
				}
			},
		},
		{
			name:    "promotion quarantine directory sync failure",
			wantErr: true,
			setFault: func(ops *migrationFileOps, _ string, injected error) {
				ops.markerDirectorySync = func(string) error { return injected }
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			marker, target := createCompletedAnchoredMarker(t, paths)
			promotion := stagedPromotionPathForTest(marker, target)
			injected := errors.New("injected promotion directory sync failure")
			originalOps := migrationOS
			tt.setFault(&migrationOS, target, injected)
			t.Cleanup(func() { migrationOS = originalOps })

			if tt.wantErr {
				if _, err := PlanMigration(paths); !errors.Is(err, injected) {
					t.Fatalf("PlanMigration() error = %v, want %v", err, injected)
				}
			} else {
				assertSimulatedCrash(t, func() { _, _ = PlanMigration(paths) })
			}
			assertAbsent(t, target)
			assertRegularExists(t, promotion, 0o600)

			migrationOS = originalOps
			next, err := PlanMigration(paths)
			if err != nil {
				t.Fatal(err)
			}
			if next == nil {
				t.Fatal("PlanMigration() = nil, want migration after promotion recovery")
			}
			assertRegularFile(t, paths.Legacy, validTOML, 0o640)
			assertAbsent(t, marker)
			assertAbsent(t, target)
			assertAbsent(t, promotion)
			assertAbsent(t, marker+".staged-anchor")
		})
	}
}

func TestPlanMigrationPreservesBothObjectsWhenStagedMarkerRestoreIsOccupied(t *testing.T) {
	paths := testPaths(t)
	marker, target := createCompletedAnchoredMarker(t, paths)
	promotion := stagedPromotionPathForTest(marker, target)
	originalOps := migrationOS
	migrationOS.afterStagedMarkerRead = func(path string) {
		if path != target {
			return
		}
		if err := os.Remove(path); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte("moved sentinel"), 0o640); err != nil {
			panic(err)
		}
	}
	migrationOS.afterQuarantineMove = func(path, _ string) error {
		if path == target {
			return os.WriteFile(target, []byte("occupying sentinel"), 0o644)
		}
		return nil
	}
	t.Cleanup(func() { migrationOS = originalOps })

	if _, err := PlanMigration(paths); err == nil || !strings.Contains(err.Error(), "identity") || !errors.Is(err, fs.ErrExist) {
		t.Fatalf("PlanMigration() error = %v, want identity and occupied-restore conflict", err)
	}
	assertRegularFile(t, target, "occupying sentinel", 0o644)
	assertRegularFile(t, promotion, "moved sentinel", 0o640)
	assertAbsent(t, marker)

	migrationOS = originalOps
	if _, err := PlanMigration(paths); err == nil {
		t.Fatal("reentered PlanMigration() error = nil, want preserved evidence conflict")
	}
	assertRegularFile(t, target, "occupying sentinel", 0o644)
	assertRegularFile(t, promotion, "moved sentinel", 0o640)
	assertRegularFile(t, paths.Legacy, validTOML, 0o640)
}

func TestPlanMigrationPreservesReplacementAcrossConflictAnchorInterruptions(t *testing.T) {
	tests := []struct {
		name           string
		createSentinel func(*testing.T, string)
		assertSentinel func(*testing.T, string)
		setCrash       func(*migrationFileOps)
	}{
		{
			name:           "before regular replacement restore",
			createSentinel: createRegularStagedSentinel,
			assertSentinel: assertRegularStagedSentinel,
			setCrash:       func(ops *migrationFileOps) { ops.afterConflictAnchorMove = func() { panic(simulatedCrash{}) } },
		},
		{
			name:           "after regular replacement restore",
			createSentinel: createRegularStagedSentinel,
			assertSentinel: assertRegularStagedSentinel,
			setCrash:       func(ops *migrationFileOps) { ops.afterConflictRestore = func() { panic(simulatedCrash{}) } },
		},
		{
			name:           "after symlink replacement restore",
			createSentinel: createSymlinkStagedSentinel,
			assertSentinel: assertSymlinkStagedSentinel,
			setCrash:       func(ops *migrationFileOps) { ops.afterConflictRestore = func() { panic(simulatedCrash{}) } },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			marker, target := createCompletedAnchoredMarker(t, paths)
			promotion := stagedPromotionPathForTest(marker, target)
			originalOps := migrationOS
			migrationOS.afterStagedMarkerRead = func(path string) {
				if path != target {
					return
				}
				if err := os.Remove(path); err != nil {
					panic(err)
				}
				tt.createSentinel(t, path)
			}
			tt.setCrash(&migrationOS)
			t.Cleanup(func() { migrationOS = originalOps })

			assertSimulatedCrash(t, func() { _, _ = PlanMigration(paths) })
			migrationOS = originalOps
			if _, err := PlanMigration(paths); err == nil {
				t.Fatal("reentered PlanMigration() error = nil, want preserved replacement conflict")
			}
			tt.assertSentinel(t, target)
			assertRegularFile(t, paths.Legacy, validTOML, 0o640)
			assertAbsent(t, marker)
			assertAbsent(t, promotion)
			assertAbsent(t, marker+".staged-anchor")
			assertNoMatches(t, marker+".staged-conflict-*")
		})
	}
}

func TestPlanMigrationRecoversConflictAnchorCleanupFailures(t *testing.T) {
	tests := []struct {
		name           string
		createSentinel func(*testing.T, string)
		assertSentinel func(*testing.T, string)
		setFailure     func(*migrationFileOps, string, error)
	}{
		{
			name:           "directory sync failure preserves regular replacement",
			createSentinel: createRegularStagedSentinel,
			assertSentinel: assertRegularStagedSentinel,
			setFailure: func(ops *migrationFileOps, _ string, injected error) {
				ops.conflictDirectorySync = func(string) error { return injected }
			},
		},
		{
			name:           "remove failure preserves symlink replacement",
			createSentinel: createSymlinkStagedSentinel,
			assertSentinel: assertSymlinkStagedSentinel,
			setFailure: func(ops *migrationFileOps, conflict string, injected error) {
				ops.remove = func(path string) error {
					if strings.HasPrefix(path, conflict+".oma-quarantine-") {
						return injected
					}
					return os.Remove(path)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			marker, target := createCompletedAnchoredMarker(t, paths)
			conflict := stagedConflictPathForTest(marker, target)
			injected := errors.New("injected conflict anchor cleanup failure")
			originalOps := migrationOS
			migrationOS.afterStagedMarkerRead = func(path string) {
				if path != target {
					return
				}
				if err := os.Remove(path); err != nil {
					panic(err)
				}
				tt.createSentinel(t, path)
			}
			tt.setFailure(&migrationOS, conflict, injected)
			t.Cleanup(func() { migrationOS = originalOps })

			if _, err := PlanMigration(paths); !errors.Is(err, injected) {
				t.Fatalf("PlanMigration() error = %v, want %v", err, injected)
			}
			tt.assertSentinel(t, target)
			assertMatches(t, marker+".staged-conflict-*")

			migrationOS = originalOps
			if _, err := PlanMigration(paths); err == nil {
				t.Fatal("reentered PlanMigration() error = nil, want preserved replacement conflict")
			}
			tt.assertSentinel(t, target)
			assertRegularFile(t, paths.Legacy, validTOML, 0o640)
			assertAbsent(t, marker)
			assertAbsent(t, marker+".staged-anchor")
			assertNoMatches(t, marker+".staged-conflict-*")
			assertNoMatches(t, marker+".staged-quarantine-*")
		})
	}
}

func TestPlanMigrationPreservesEvidenceWhenConflictAnchorTransitionFails(t *testing.T) {
	paths := testPaths(t)
	marker, target := createCompletedAnchoredMarker(t, paths)
	promotion := stagedPromotionPathForTest(marker, target)
	conflict := stagedConflictPathForTest(marker, target)
	originalOps := migrationOS
	migrationOS.afterStagedMarkerRead = func(path string) {
		if path != target {
			return
		}
		if err := os.Remove(path); err != nil {
			panic(err)
		}
		createRegularStagedSentinel(t, path)
		writeFile(t, conflict, "external conflict-anchor sentinel", 0o644)
	}
	t.Cleanup(func() { migrationOS = originalOps })

	if _, err := PlanMigration(paths); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("PlanMigration() error = %v, want conflict-anchor destination exists", err)
	}
	assertAbsent(t, target)
	assertRegularStagedSentinel(t, promotion)
	assertSymlink(t, marker+".staged-anchor", target)
	assertRegularFile(t, conflict, "external conflict-anchor sentinel", 0o644)

	migrationOS = originalOps
	if _, err := PlanMigration(paths); err == nil {
		t.Fatal("reentered PlanMigration() error = nil, want preserved transition evidence conflict")
	}
	assertAbsent(t, target)
	assertRegularStagedSentinel(t, promotion)
	assertSymlink(t, marker+".staged-anchor", target)
	assertRegularFile(t, conflict, "external conflict-anchor sentinel", 0o644)
	assertRegularFile(t, paths.Legacy, validTOML, 0o640)
}

func TestPlanMigrationRecoversConflictIntentPersistenceStates(t *testing.T) {
	tests := []struct {
		name           string
		createSentinel func(*testing.T, string)
		assertSentinel func(*testing.T, string)
		setCrash       func(*migrationFileOps)
	}{
		{
			name:           "after durable intent sync with active anchor",
			createSentinel: createRegularStagedSentinel,
			assertSentinel: assertRegularStagedSentinel,
			setCrash: func(ops *migrationFileOps) {
				ops.afterConflictIntentSync = func() { panic(simulatedCrash{}) }
			},
		},
		{
			name:           "active anchor move persisted",
			createSentinel: createRegularStagedSentinel,
			assertSentinel: assertRegularStagedSentinel,
			setCrash: func(ops *migrationFileOps) {
				ops.afterConflictActiveMove = func(string, string) { panic(simulatedCrash{}) }
			},
		},
		{
			name:           "active anchor move rolled back to pre-state",
			createSentinel: createSymlinkStagedSentinel,
			assertSentinel: assertSymlinkStagedSentinel,
			setCrash: func(ops *migrationFileOps) {
				ops.afterConflictActiveMove = func(active, quarantine string) {
					if err := renameNoReplace(quarantine, active); err != nil {
						panic(err)
					}
					panic(simulatedCrash{})
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			marker, target := createCompletedAnchoredMarker(t, paths)
			promotion := stagedPromotionPathForTest(marker, target)
			originalOps := migrationOS
			migrationOS.afterStagedMarkerRead = func(path string) {
				if path != target {
					return
				}
				if err := os.Remove(path); err != nil {
					panic(err)
				}
				tt.createSentinel(t, path)
			}
			tt.setCrash(&migrationOS)
			t.Cleanup(func() { migrationOS = originalOps })

			assertSimulatedCrash(t, func() { _, _ = PlanMigration(paths) })
			assertMatches(t, marker+".staged-conflict-intent-*")
			migrationOS = originalOps
			if _, err := PlanMigration(paths); err == nil {
				t.Fatal("reentered PlanMigration() error = nil, want preserved intent conflict")
			}
			tt.assertSentinel(t, target)
			assertRegularFile(t, paths.Legacy, validTOML, 0o640)
			assertAbsent(t, marker)
			assertAbsent(t, promotion)
			assertAbsent(t, marker+".staged-anchor")
			assertNoMatches(t, marker+".staged-conflict-*")
		})
	}
}

func TestPlanMigrationFindsFixedConflictIntentUnderReservedParentNames(t *testing.T) {
	tests := []struct {
		parent         string
		createSentinel func(*testing.T, string)
		assertSentinel func(*testing.T, string)
	}{
		{"parent.oma-draft-anchor", createRegularStagedSentinel, assertRegularStagedSentinel},
		{"parent.oma-staged-token", createSymlinkStagedSentinel, assertSymlinkStagedSentinel},
		{"parent.oma-draft-anchor.oma-staged-token", createRegularStagedSentinel, assertRegularStagedSentinel},
	}
	for _, tt := range tests {
		t.Run(tt.parent, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), tt.parent)
			paths := Paths{
				Canonical: filepath.Join(root, "config", "oma", "config.toml"),
				Legacy:    filepath.Join(root, "config", "prep-task", "config.toml"),
				CacheRoot: filepath.Join(root, "cache", "oma"),
				StateRoot: filepath.Join(root, "state", "oma"),
				Netrc:     filepath.Join(root, "home", ".netrc"),
			}
			marker, target := createCompletedAnchoredMarker(t, paths)
			originalOps := migrationOS
			migrationOS.afterStagedMarkerRead = func(path string) {
				if path == target {
					if err := os.Remove(path); err != nil {
						panic(err)
					}
					tt.createSentinel(t, path)
				}
			}
			migrationOS.afterConflictIntentSync = func() { panic(simulatedCrash{}) }
			t.Cleanup(func() { migrationOS = originalOps })
			assertSimulatedCrash(t, func() { _, _ = PlanMigration(paths) })

			migrationOS = originalOps
			if _, err := PlanMigration(paths); err == nil {
				t.Fatal("reentered PlanMigration() error = nil, want preserved replacement conflict")
			}
			tt.assertSentinel(t, target)
			assertAbsent(t, marker+".staged-anchor")
		})
	}
}

func TestPlanMigrationRecoversConflictIntentDraftPersistenceStates(t *testing.T) {
	tests := []struct {
		name           string
		setCrash       func(*migrationFileOps, string)
		createSentinel func(*testing.T, string)
		assertSentinel func(*testing.T, string)
	}{
		{
			name: "after durable draft anchor before open",
			setCrash: func(ops *migrationFileOps, _ string) {
				ops.afterConflictIntentDraftAnchorSync = func() { panic(simulatedCrash{}) }
			},
		},
		{
			name: "draft entry loss leaves anchor only",
			setCrash: func(ops *migrationFileOps, marker string) {
				ops.afterConflictIntentOpen = func() {
					matches, err := filepath.Glob(marker + ".staged-conflict-intent-*.oma-staged-*")
					if err != nil || len(matches) != 1 {
						panic("intent draft not found")
					}
					if err := os.Remove(matches[0]); err != nil {
						panic(err)
					}
					panic(simulatedCrash{})
				}
			},
		},
		{
			name: "after intent draft open",
			setCrash: func(ops *migrationFileOps, _ string) {
				ops.afterConflictIntentOpen = func() { panic(simulatedCrash{}) }
			},
		},
		{
			name: "after intent draft partial write",
			setCrash: func(ops *migrationFileOps, _ string) {
				ops.afterConflictIntentPart = func() { panic(simulatedCrash{}) }
			},
		},
		{
			name: "after intent draft file sync",
			setCrash: func(ops *migrationFileOps, _ string) {
				ops.afterConflictIntentFileSync = func() { panic(simulatedCrash{}) }
			},
		},
		{
			name: "after fixed intent commit with dangling anchor",
			setCrash: func(ops *migrationFileOps, _ string) {
				ops.afterConflictIntentSync = func() { panic(simulatedCrash{}) }
			},
			createSentinel: createSymlinkStagedSentinel,
			assertSentinel: assertSymlinkStagedSentinel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			marker, target := createCompletedAnchoredMarker(t, paths)
			promotion := stagedPromotionPathForTest(marker, target)
			createSentinel := tt.createSentinel
			assertSentinel := tt.assertSentinel
			if createSentinel == nil {
				createSentinel = createRegularStagedSentinel
				assertSentinel = assertRegularStagedSentinel
			}
			originalOps := migrationOS
			migrationOS.afterStagedMarkerRead = func(path string) {
				if path != target {
					return
				}
				if err := os.Remove(path); err != nil {
					panic(err)
				}
				createSentinel(t, path)
			}
			tt.setCrash(&migrationOS, marker)
			t.Cleanup(func() { migrationOS = originalOps })

			assertSimulatedCrash(t, func() { _, _ = PlanMigration(paths) })
			migrationOS = originalOps
			if _, err := PlanMigration(paths); err == nil {
				t.Fatal("reentered PlanMigration() error = nil, want preserved replacement conflict")
			}
			assertRegularFile(t, paths.Legacy, validTOML, 0o640)
			assertSentinel(t, target)
			assertAbsent(t, promotion)
			assertAbsent(t, marker+".staged-anchor")
			assertNoMatches(t, marker+".staged-conflict-*")
		})
	}
}

func TestPlanMigrationPreservesUnknownConflictIntentDraftAnchors(t *testing.T) {
	tests := []struct {
		name         string
		createAnchor func(*testing.T, string)
		assertAnchor func(*testing.T, string)
	}{
		{
			name: "regular file",
			createAnchor: func(t *testing.T, path string) {
				writeFile(t, path, "external draft anchor", 0o640)
			},
			assertAnchor: func(t *testing.T, path string) {
				assertRegularFile(t, path, "external draft anchor", 0o640)
			},
		},
		{
			name: "mismatched symlink",
			createAnchor: func(t *testing.T, path string) {
				if err := os.Symlink("external-draft-target", path); err != nil {
					t.Fatal(err)
				}
			},
			assertAnchor: func(t *testing.T, path string) {
				assertSymlink(t, path, "external-draft-target")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			marker, target := createCompletedAnchoredMarker(t, paths)
			promotion := stagedPromotionPathForTest(marker, target)
			intent := stagedConflictIntentPathForTest(marker, target)
			draftAnchor := intent + ".oma-draft-anchor"
			originalOps := migrationOS
			migrationOS.afterStagedMarkerRead = func(path string) {
				if path != target {
					return
				}
				if err := os.Remove(path); err != nil {
					panic(err)
				}
				createRegularStagedSentinel(t, path)
				tt.createAnchor(t, draftAnchor)
			}
			t.Cleanup(func() { migrationOS = originalOps })

			if _, err := PlanMigration(paths); !errors.Is(err, fs.ErrExist) {
				t.Fatalf("PlanMigration() error = %v, want draft anchor destination exists", err)
			}
			assertAbsent(t, target)
			assertRegularStagedSentinel(t, promotion)
			assertSymlink(t, marker+".staged-anchor", target)
			tt.assertAnchor(t, draftAnchor)

			migrationOS = originalOps
			if _, err := PlanMigration(paths); err == nil {
				t.Fatal("reentered PlanMigration() error = nil, want preserved draft anchor conflict")
			}
			assertAbsent(t, target)
			assertRegularStagedSentinel(t, promotion)
			assertSymlink(t, marker+".staged-anchor", target)
			tt.assertAnchor(t, draftAnchor)
		})
	}
}

func TestPlanMigrationRecoversConflictIntentDraftAnchorCleanupFailures(t *testing.T) {
	tests := []struct {
		name       string
		crash      bool
		setFailure func(*migrationFileOps, string, error)
	}{
		{
			name:  "after cleanup move",
			crash: true,
			setFailure: func(ops *migrationFileOps, _ string, _ error) {
				ops.afterConflictIntentDraftAnchorMove = func() { panic(simulatedCrash{}) }
			},
		},
		{
			name: "cleanup directory sync failure",
			setFailure: func(ops *migrationFileOps, _ string, injected error) {
				ops.conflictIntentDraftAnchorSync = func(string) error { return injected }
			},
		},
		{
			name: "cleanup remove failure",
			setFailure: func(ops *migrationFileOps, anchor string, injected error) {
				ops.remove = func(path string) error {
					if strings.HasPrefix(path, anchor+".oma-quarantine-") {
						return injected
					}
					return os.Remove(path)
				}
			},
		},
		{
			name: "cleanup final sync failure",
			setFailure: func(ops *migrationFileOps, _ string, injected error) {
				calls := 0
				ops.conflictIntentDraftAnchorSync = func(string) error {
					calls++
					if calls == 2 {
						return injected
					}
					return nil
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			marker, target := createCompletedAnchoredMarker(t, paths)
			intent := stagedConflictIntentPathForTest(marker, target)
			draftAnchor := intent + ".oma-draft-anchor"
			injected := errors.New("injected draft anchor cleanup failure")
			originalOps := migrationOS
			migrationOS.afterStagedMarkerRead = func(path string) {
				if path != target {
					return
				}
				if err := os.Remove(path); err != nil {
					panic(err)
				}
				createRegularStagedSentinel(t, path)
			}
			tt.setFailure(&migrationOS, draftAnchor, injected)
			t.Cleanup(func() { migrationOS = originalOps })

			if tt.crash {
				assertSimulatedCrash(t, func() { _, _ = PlanMigration(paths) })
			} else if _, err := PlanMigration(paths); !errors.Is(err, injected) {
				t.Fatalf("PlanMigration() error = %v, want %v", err, injected)
			}
			assertMatches(t, intent)

			migrationOS = originalOps
			if _, err := PlanMigration(paths); err == nil {
				t.Fatal("reentered PlanMigration() error = nil, want preserved replacement conflict")
			}
			assertRegularStagedSentinel(t, target)
			assertRegularFile(t, paths.Legacy, validTOML, 0o640)
			assertNoMatches(t, marker+".staged-conflict-*")
		})
	}
}

func TestPlanMigrationPreservesConflictingIntentDraftAnchorCleanupQuarantines(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, string)
	}{
		{
			name: "fixed anchor and quarantine coexist",
			mutate: func(t *testing.T, anchor, quarantine string) {
				target, err := os.Readlink(quarantine)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, anchor); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "quarantine target mismatches",
			mutate: func(t *testing.T, _ string, quarantine string) {
				if err := os.Remove(quarantine); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("external-cleanup-target", quarantine); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			marker, target := createCompletedAnchoredMarker(t, paths)
			intent := stagedConflictIntentPathForTest(marker, target)
			anchor := intent + ".oma-draft-anchor"
			originalOps := migrationOS
			migrationOS.afterStagedMarkerRead = func(path string) {
				if path != target {
					return
				}
				if err := os.Remove(path); err != nil {
					panic(err)
				}
				createRegularStagedSentinel(t, path)
			}
			migrationOS.afterConflictIntentDraftAnchorMove = func() { panic(simulatedCrash{}) }
			t.Cleanup(func() { migrationOS = originalOps })

			assertSimulatedCrash(t, func() { _, _ = PlanMigration(paths) })
			matches, err := filepath.Glob(anchor + ".oma-quarantine-*")
			if err != nil || len(matches) != 1 {
				t.Fatalf("cleanup quarantine matches = %v, error = %v", matches, err)
			}
			tt.mutate(t, anchor, matches[0])
			migrationOS = originalOps
			if _, err := PlanMigration(paths); err == nil {
				t.Fatal("reentered PlanMigration() error = nil, want cleanup quarantine conflict")
			}
			assertMatches(t, matches[0])
			assertMatches(t, intent)
		})
	}
}

func TestPlanMigrationPreservesEvidenceWhenConflictIntentIsPreempted(t *testing.T) {
	paths := testPaths(t)
	marker, target := createCompletedAnchoredMarker(t, paths)
	promotion := stagedPromotionPathForTest(marker, target)
	intent := stagedConflictIntentPathForTest(marker, target)
	originalOps := migrationOS
	migrationOS.afterStagedMarkerRead = func(path string) {
		if path != target {
			return
		}
		if err := os.Remove(path); err != nil {
			panic(err)
		}
		createRegularStagedSentinel(t, path)
		writeFile(t, intent, "external conflict-intent sentinel", 0o644)
	}
	t.Cleanup(func() { migrationOS = originalOps })

	if _, err := PlanMigration(paths); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("PlanMigration() error = %v, want conflict-intent destination exists", err)
	}
	assertAbsent(t, target)
	assertRegularStagedSentinel(t, promotion)
	assertSymlink(t, marker+".staged-anchor", target)
	assertRegularFile(t, intent, "external conflict-intent sentinel", 0o644)

	migrationOS = originalOps
	if _, err := PlanMigration(paths); err == nil {
		t.Fatal("reentered PlanMigration() error = nil, want preserved intent evidence conflict")
	}
	assertAbsent(t, target)
	assertRegularStagedSentinel(t, promotion)
	assertSymlink(t, marker+".staged-anchor", target)
	assertRegularFile(t, intent, "external conflict-intent sentinel", 0o644)
	assertRegularFile(t, paths.Legacy, validTOML, 0o640)
}

func TestPlanMigrationPreservesMatchingSymlinkWhenConflictIntentIsPreempted(t *testing.T) {
	paths := testPaths(t)
	marker, target := createCompletedAnchoredMarker(t, paths)
	promotion := stagedPromotionPathForTest(marker, target)
	intent := stagedConflictIntentPathForTest(marker, target)
	originalOps := migrationOS
	migrationOS.afterStagedMarkerRead = func(path string) {
		if path != target {
			return
		}
		if err := os.Remove(path); err != nil {
			panic(err)
		}
		createRegularStagedSentinel(t, path)
		if err := os.Symlink(target, intent); err != nil {
			panic(err)
		}
	}
	t.Cleanup(func() { migrationOS = originalOps })

	if _, err := PlanMigration(paths); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("PlanMigration() error = %v, want conflict-intent destination exists", err)
	}
	assertAbsent(t, target)
	assertRegularStagedSentinel(t, promotion)
	assertSymlink(t, marker+".staged-anchor", target)
	assertSymlink(t, intent, target)

	migrationOS = originalOps
	if _, err := PlanMigration(paths); err == nil {
		t.Fatal("reentered PlanMigration() error = nil, want preserved intent evidence conflict")
	}
	assertAbsent(t, target)
	assertRegularStagedSentinel(t, promotion)
	assertSymlink(t, marker+".staged-anchor", target)
	assertSymlink(t, intent, target)
	assertRegularFile(t, paths.Legacy, validTOML, 0o640)
}

func createRegularStagedSentinel(t *testing.T, path string) {
	t.Helper()
	writeFile(t, path, "external staged marker sentinel", 0o640)
}

func assertRegularStagedSentinel(t *testing.T, path string) {
	t.Helper()
	assertRegularFile(t, path, "external staged marker sentinel", 0o640)
}

func createSymlinkStagedSentinel(t *testing.T, path string) {
	t.Helper()
	if err := os.Symlink("external-staged-marker-target", path); err != nil {
		t.Fatal(err)
	}
}

func assertSymlinkStagedSentinel(t *testing.T, path string) {
	t.Helper()
	assertSymlink(t, path, "external-staged-marker-target")
}

func stagedConflictPathForTest(marker, target string) string {
	token := strings.TrimPrefix(target, marker+".staged-")
	return marker + ".staged-conflict-" + token
}

func stagedConflictIntentPathForTest(marker, target string) string {
	token := strings.TrimPrefix(target, marker+".staged-")
	return marker + ".staged-conflict-intent-" + token
}

func createCompletedAnchoredMarker(t *testing.T, paths Paths) (string, string) {
	t.Helper()
	writeFile(t, paths.Legacy, validTOML, 0o640)
	migration, err := PlanMigration(paths)
	if err != nil {
		t.Fatal(err)
	}
	originalOps := migrationOS
	migrationOS.afterMarkerFileSync = func() { panic(simulatedCrash{}) }
	assertSimulatedCrash(t, func() { _ = migration.Apply(func(Config) error { return nil }) })
	migrationOS = originalOps
	t.Cleanup(func() { migrationOS = originalOps })
	marker, _ := transactionPaths(paths)
	return marker, findStagedMarkerTarget(t, marker)
}

func stagedPromotionPathForTest(marker, target string) string {
	token := strings.TrimPrefix(target, marker+".staged-")
	return marker + ".staged-quarantine-" + token
}

func TestPlanMigrationRecoversAnchoredMarkerInterruptions(t *testing.T) {
	tests := []struct {
		name     string
		setCrash func(*migrationFileOps, *testing.T, string)
	}{
		{name: "after anchor sync", setCrash: func(ops *migrationFileOps, _ *testing.T, _ string) {
			ops.afterMarkerAnchorSync = func() { panic(simulatedCrash{}) }
		}},
		{name: "immediately after marker open", setCrash: func(ops *migrationFileOps, t *testing.T, marker string) {
			ops.afterMarkerOpen = func() {
				staged := findStagedMarkerTarget(t, marker)
				data, err := os.ReadFile(staged)
				if err != nil {
					t.Fatal(err)
				}
				if len(data) != 0 {
					t.Fatalf("staged marker content = %q, want empty immediately after open", data)
				}
				panic(simulatedCrash{})
			}
		}},
		{name: "after first magic write", setCrash: func(ops *migrationFileOps, t *testing.T, marker string) {
			ops.afterMarkerMagicWrite = func(part int) {
				if part != 1 {
					return
				}
				staged := findStagedMarkerTarget(t, marker)
				data, err := os.ReadFile(staged)
				if err != nil {
					t.Fatal(err)
				}
				want := migrationMarkerMagic[:len(migrationMarkerMagic)/2]
				if string(data) != want {
					t.Fatalf("first magic write = %q, want %q", data, want)
				}
				panic(simulatedCrash{})
			}
		}},
		{name: "after second magic write", setCrash: func(ops *migrationFileOps, t *testing.T, marker string) {
			ops.afterMarkerMagicWrite = func(part int) {
				if part != 2 {
					return
				}
				staged := findStagedMarkerTarget(t, marker)
				data, err := os.ReadFile(staged)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != migrationMarkerMagic {
					t.Fatalf("complete magic write = %q, want %q", data, migrationMarkerMagic)
				}
				panic(simulatedCrash{})
			}
		}},
		{name: "after complete marker sync", setCrash: func(ops *migrationFileOps, _ *testing.T, _ string) {
			ops.afterMarkerFileSync = func() { panic(simulatedCrash{}) }
		}},
		{name: "after fixed marker commit", setCrash: func(ops *migrationFileOps, _ *testing.T, _ string) {
			ops.afterMarkerCommit = func() { panic(simulatedCrash{}) }
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
			marker, backup := transactionPaths(paths)
			originalOps := migrationOS
			tt.setCrash(&migrationOS, t, marker)
			assertSimulatedCrash(t, func() { _ = migration.Apply(func(Config) error { return nil }) })
			migrationOS = originalOps
			t.Cleanup(func() { migrationOS = originalOps })

			next, err := PlanMigration(paths)
			if err != nil {
				t.Fatal(err)
			}
			if next == nil {
				t.Fatal("PlanMigration() = nil, want migration after marker recovery")
			}
			assertRegularFile(t, paths.Legacy, validTOML, 0o640)
			assertAbsent(t, paths.Canonical)
			assertAbsent(t, marker)
			assertAbsent(t, backup)
			assertAbsent(t, marker+".staged-anchor")
			assertNoMatches(t, marker+".staged-*")
		})
	}
}

func TestPlanMigrationPreservesUnknownStagedAnchor(t *testing.T) {
	tests := []struct {
		name   string
		create func(*testing.T, string)
	}{
		{name: "regular file", create: func(t *testing.T, path string) { writeFile(t, path, "unknown anchor", 0o640) }},
		{name: "symlink", create: func(t *testing.T, path string) {
			if err := os.Symlink("unknown-target", path); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			writeFile(t, paths.Legacy, validTOML, 0o640)
			marker, _ := transactionPaths(paths)
			anchor := marker + ".staged-anchor"
			tt.create(t, anchor)
			before, err := os.Lstat(anchor)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := PlanMigration(paths); err == nil {
				t.Fatal("PlanMigration() error = nil, want staged anchor conflict")
			}
			after, err := os.Lstat(anchor)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(before, after) {
				t.Fatal("unknown staged anchor was replaced")
			}
		})
	}
}

func findStagedMarkerTarget(t *testing.T, marker string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(marker))
	if err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Base(marker) + ".staged-"
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) && entry.Name() != filepath.Base(marker)+".staged-anchor" && entry.Type().IsRegular() {
			return filepath.Join(filepath.Dir(marker), entry.Name())
		}
	}
	t.Fatal("staged marker target not found")
	return ""
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

func assertMatches(t *testing.T, pattern string) {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatalf("matches for %s = none, want at least one", pattern)
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
