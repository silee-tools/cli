package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type Migration struct {
	paths Paths
}

type migrationFileOps struct {
	symlink func(string, string) error
	remove  func(string) error
}

var migrationOS = migrationFileOps{
	symlink: os.Symlink,
	remove:  os.Remove,
}

func PlanMigration(paths Paths) (*Migration, error) {
	canonicalInfo, canonicalErr := os.Lstat(paths.Canonical)
	legacyInfo, legacyErr := os.Lstat(paths.Legacy)
	if err := unexpectedStatError(paths.Canonical, canonicalErr); err != nil {
		return nil, err
	}
	if err := unexpectedStatError(paths.Legacy, legacyErr); err != nil {
		return nil, err
	}
	if canonicalErr == nil && legacyErr == nil && canonicalInfo.Mode().IsRegular() && legacyInfo.Mode().IsRegular() {
		canonicalStat, err := os.Stat(paths.Canonical)
		if err != nil {
			return nil, err
		}
		legacyStat, err := os.Stat(paths.Legacy)
		if err != nil {
			return nil, err
		}
		if !os.SameFile(canonicalStat, legacyStat) {
			return nil, configurationConflict(paths)
		}
	}
	if canonicalErr == nil || legacyErr != nil || !legacyInfo.Mode().IsRegular() {
		return nil, nil
	}
	return &Migration{paths: paths}, nil
}

func (m Migration) Apply(validate func(Config) error) error {
	if _, err := os.Lstat(m.paths.Canonical); err == nil {
		return fmt.Errorf("canonical configuration already exists: %s", m.paths.Canonical)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect canonical configuration: %w", err)
	}
	legacyInfo, err := os.Lstat(m.paths.Legacy)
	if err != nil {
		return fmt.Errorf("inspect legacy configuration: %w", err)
	}
	if !legacyInfo.Mode().IsRegular() {
		return fmt.Errorf("legacy configuration is not a regular file: %s", m.paths.Legacy)
	}
	data, err := os.ReadFile(m.paths.Legacy)
	if err != nil {
		return fmt.Errorf("read legacy configuration: %w", err)
	}
	config, err := decodeConfig(data)
	if err != nil {
		return fmt.Errorf("decode legacy configuration: %w", err)
	}
	if validate != nil {
		if err := validate(config); err != nil {
			return fmt.Errorf("validate legacy configuration: %w", err)
		}
	}

	if err := writeCanonical(m.paths.Canonical, data); err != nil {
		return err
	}
	backup, err := reserveBackupPath(m.paths.Legacy)
	if err != nil {
		return errors.Join(err, removeForRollback(m.paths.Canonical))
	}
	if err := os.Rename(m.paths.Legacy, backup); err != nil {
		return errors.Join(fmt.Errorf("backup legacy configuration: %w", err), removeForRollback(m.paths.Canonical))
	}
	if err := migrationOS.symlink(m.paths.Canonical, m.paths.Legacy); err != nil {
		rollbackErr := rollbackMigration(m.paths, backup, false)
		return errors.Join(fmt.Errorf("replace legacy configuration with symlink: %w", err), rollbackErr)
	}
	if err := migrationOS.remove(backup); err != nil {
		rollbackErr := rollbackMigration(m.paths, backup, true)
		return errors.Join(fmt.Errorf("remove legacy backup: %w", err), rollbackErr)
	}
	return nil
}

func writeCanonical(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create canonical configuration directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("protect canonical configuration directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".config.toml.*")
	if err != nil {
		return fmt.Errorf("create canonical configuration temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("protect canonical configuration temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write canonical configuration temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync canonical configuration temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close canonical configuration temporary file: %w", err)
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("canonical configuration already exists: %s", path)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect canonical configuration before rename: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("commit canonical configuration: %w", err)
	}
	committed = true
	return nil
}

func reserveBackupPath(legacy string) (string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(legacy), ".config.toml.backup.*")
	if err != nil {
		return "", fmt.Errorf("reserve legacy backup path: %w", err)
	}
	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close legacy backup placeholder: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("remove legacy backup placeholder: %w", err)
	}
	return path, nil
}

func rollbackMigration(paths Paths, backup string, linkExists bool) error {
	var rollbackErrors []error
	if linkExists {
		if err := migrationOS.remove(paths.Legacy); err != nil && !errors.Is(err, fs.ErrNotExist) {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("rollback legacy symlink: %w", err))
		}
	}
	if err := os.Rename(backup, paths.Legacy); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("restore legacy configuration: %w", err))
	}
	if err := removeForRollback(paths.Canonical); err != nil {
		rollbackErrors = append(rollbackErrors, err)
	}
	return errors.Join(rollbackErrors...)
}

func removeForRollback(path string) error {
	if err := migrationOS.remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove canonical configuration during rollback: %w", err)
	}
	return nil
}
