package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
			if path == paths.Canonical && err == nil {
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
