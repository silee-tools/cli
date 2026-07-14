package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

const (
	migrationMarkerSuffix = ".oma-migration"
	migrationBackupSuffix = ".oma-migration-backup"
)

type Migration struct {
	paths Paths
}

type migrationFileOps struct {
	symlink               func(string, string) error
	remove                func(string) error
	beforeCanonicalCommit func(string) error
	afterCanonicalCommit  func()
	afterLegacyBackup     func()
	afterSymlink          func()
}

var migrationOS = migrationFileOps{
	symlink: os.Symlink,
	remove:  os.Remove,
}

type fileIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

type transactionRecord struct {
	Version     int          `json:"version"`
	Canonical   string       `json:"canonical"`
	Legacy      string       `json:"legacy"`
	CanonicalID fileIdentity `json:"canonical_identity"`
	LegacyID    fileIdentity `json:"legacy_identity"`
}

type migrationTransaction struct {
	paths              Paths
	record             transactionRecord
	markerID           fileIdentity
	canonicalCommitted bool
	backupCreated      bool
}

func PlanMigration(paths Paths) (*Migration, error) {
	if err := recoverInterruptedMigration(paths); err != nil {
		return nil, err
	}

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
	if err := recoverInterruptedMigration(m.paths); err != nil {
		return err
	}
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
	legacyID, err := identityOf(legacyInfo)
	if err != nil {
		return fmt.Errorf("identify legacy configuration: %w", err)
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

	tmpPath, canonicalID, err := createCanonicalTemporary(m.paths.Canonical, data)
	if err != nil {
		return err
	}
	tmpPresent := true
	defer func() {
		if tmpPresent {
			_ = removeOwnedRegular(tmpPath, canonicalID)
		}
	}()

	record := transactionRecord{
		Version:     1,
		Canonical:   m.paths.Canonical,
		Legacy:      m.paths.Legacy,
		CanonicalID: canonicalID,
		LegacyID:    legacyID,
	}
	markerID, err := createTransactionMarker(m.paths, record)
	if err != nil {
		return err
	}
	tx := migrationTransaction{paths: m.paths, record: record, markerID: markerID}
	fail := func(primary error) error {
		return errors.Join(primary, rollbackTransaction(tx))
	}

	if migrationOS.beforeCanonicalCommit != nil {
		if err := migrationOS.beforeCanonicalCommit(m.paths.Canonical); err != nil {
			return fail(fmt.Errorf("before canonical commit: %w", err))
		}
	}
	if err := os.Link(tmpPath, m.paths.Canonical); err != nil {
		return fail(fmt.Errorf("commit canonical configuration without replacement: %w", err))
	}
	tx.canonicalCommitted = true
	if err := syncDirectory(filepath.Dir(m.paths.Canonical)); err != nil {
		return fail(fmt.Errorf("sync canonical configuration directory after commit: %w", err))
	}
	if err := removeOwnedRegular(tmpPath, canonicalID); err != nil {
		return fail(fmt.Errorf("remove canonical temporary file: %w", err))
	}
	tmpPresent = false
	if migrationOS.afterCanonicalCommit != nil {
		migrationOS.afterCanonicalCommit()
	}

	backup := migrationBackupPath(m.paths)
	if err := os.Link(m.paths.Legacy, backup); err != nil {
		return fail(fmt.Errorf("create legacy backup without replacement: %w", err))
	}
	tx.backupCreated = true
	if err := syncDirectory(filepath.Dir(backup)); err != nil {
		return fail(fmt.Errorf("sync legacy directory after backup: %w", err))
	}
	if err := removeOwnedRegular(m.paths.Legacy, legacyID); err != nil {
		return fail(fmt.Errorf("remove legacy configuration after backup: %w", err))
	}
	if migrationOS.afterLegacyBackup != nil {
		migrationOS.afterLegacyBackup()
	}

	if err := migrationOS.symlink(m.paths.Canonical, m.paths.Legacy); err != nil {
		return fail(fmt.Errorf("replace legacy configuration with symlink: %w", err))
	}
	if err := syncDirectory(filepath.Dir(m.paths.Legacy)); err != nil {
		return fail(fmt.Errorf("sync legacy directory after symlink: %w", err))
	}
	if migrationOS.afterSymlink != nil {
		migrationOS.afterSymlink()
	}

	if err := removeOwnedRegular(backup, legacyID); err != nil {
		return fmt.Errorf("remove legacy backup: %w", err)
	}
	if err := removeOwnedRegular(migrationMarkerPath(m.paths), markerID); err != nil {
		return fmt.Errorf("remove migration marker: %w", err)
	}
	return nil
}

func createCanonicalTemporary(path string, data []byte) (string, fileIdentity, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fileIdentity{}, fmt.Errorf("create canonical configuration directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fileIdentity{}, fmt.Errorf("protect canonical configuration directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".config.toml.*")
	if err != nil {
		return "", fileIdentity{}, fmt.Errorf("create canonical configuration temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return "", fileIdentity{}, fmt.Errorf("protect canonical configuration temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return "", fileIdentity{}, fmt.Errorf("write canonical configuration temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return "", fileIdentity{}, fmt.Errorf("sync canonical configuration temporary file: %w", err)
	}
	info, err := tmp.Stat()
	if err != nil {
		return "", fileIdentity{}, fmt.Errorf("stat canonical configuration temporary file: %w", err)
	}
	id, err := identityOf(info)
	if err != nil {
		return "", fileIdentity{}, fmt.Errorf("identify canonical configuration temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fileIdentity{}, fmt.Errorf("close canonical configuration temporary file: %w", err)
	}
	ok = true
	return tmpPath, id, nil
}

func createTransactionMarker(paths Paths, record transactionRecord) (fileIdentity, error) {
	marker := migrationMarkerPath(paths)
	data, err := json.Marshal(record)
	if err != nil {
		return fileIdentity{}, fmt.Errorf("encode migration marker: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(marker), ".oma-migration.*")
	if err != nil {
		return fileIdentity{}, fmt.Errorf("create migration marker temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fileIdentity{}, fmt.Errorf("protect migration marker: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fileIdentity{}, fmt.Errorf("write migration marker: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fileIdentity{}, fmt.Errorf("sync migration marker: %w", err)
	}
	info, err := tmp.Stat()
	if err != nil {
		return fileIdentity{}, fmt.Errorf("stat migration marker: %w", err)
	}
	id, err := identityOf(info)
	if err != nil {
		return fileIdentity{}, fmt.Errorf("identify migration marker: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fileIdentity{}, fmt.Errorf("close migration marker: %w", err)
	}
	if err := os.Link(tmpPath, marker); err != nil {
		return fileIdentity{}, fmt.Errorf("create migration marker without replacement: %w", err)
	}
	if err := syncDirectory(filepath.Dir(marker)); err != nil {
		return fileIdentity{}, fmt.Errorf("sync legacy directory after marker creation: %w", err)
	}
	if err := os.Remove(tmpPath); err != nil {
		return fileIdentity{}, fmt.Errorf("remove migration marker temporary file: %w", err)
	}
	if err := syncDirectory(filepath.Dir(marker)); err != nil {
		return fileIdentity{}, fmt.Errorf("sync legacy directory after marker temporary cleanup: %w", err)
	}
	return id, nil
}

func recoverInterruptedMigration(paths Paths) error {
	marker := migrationMarkerPath(paths)
	record, markerID, err := readTransactionMarker(marker)
	if errors.Is(err, fs.ErrNotExist) {
		if _, backupErr := os.Lstat(migrationBackupPath(paths)); backupErr == nil {
			return fmt.Errorf("orphaned migration backup conflicts with new migration: %s", migrationBackupPath(paths))
		} else if !errors.Is(backupErr, fs.ErrNotExist) {
			return fmt.Errorf("inspect migration backup: %w", backupErr)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if record.Version != 1 || record.Canonical != paths.Canonical || record.Legacy != paths.Legacy {
		return fmt.Errorf("migration marker does not belong to requested paths: %s", marker)
	}

	legacyInfo, legacyErr := os.Lstat(paths.Legacy)
	if legacyErr == nil && legacyInfo.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(paths.Legacy)
		if err != nil {
			return fmt.Errorf("read recovered legacy symlink: %w", err)
		}
		if target != paths.Canonical {
			return fmt.Errorf("recovery conflict: legacy symlink targets %s", target)
		}
		if err := requireOwnedRegular(paths.Canonical, record.CanonicalID); err != nil {
			return fmt.Errorf("recovery conflict: %w", err)
		}
		if err := removeOwnedRegularIfPresent(migrationBackupPath(paths), record.LegacyID); err != nil {
			return err
		}
		return removeOwnedRegular(marker, markerID)
	}
	if legacyErr == nil && legacyInfo.Mode().IsRegular() {
		owned, err := hasIdentity(legacyInfo, record.LegacyID)
		if err != nil {
			return err
		}
		if !owned {
			return fmt.Errorf("recovery conflict: legacy path is occupied by another regular file")
		}
		return finishRollback(paths, record, markerID)
	}
	if legacyErr != nil && !errors.Is(legacyErr, fs.ErrNotExist) {
		return fmt.Errorf("inspect legacy path during recovery: %w", legacyErr)
	}
	if legacyErr == nil {
		return fmt.Errorf("recovery conflict: unsupported object occupies legacy path")
	}

	backup := migrationBackupPath(paths)
	if err := requireOwnedRegular(backup, record.LegacyID); err != nil {
		return fmt.Errorf("recovery cannot restore legacy configuration: %w", err)
	}
	if err := linkOwnedNoReplace(backup, paths.Legacy, record.LegacyID); err != nil {
		return fmt.Errorf("restore legacy configuration during recovery: %w", err)
	}
	return finishRollback(paths, record, markerID)
}

func rollbackTransaction(tx migrationTransaction) error {
	legacyInfo, legacyErr := os.Lstat(tx.paths.Legacy)
	legacySecured := false
	var rollbackErrors []error
	if legacyErr == nil && legacyInfo.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(tx.paths.Legacy)
		if err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("read legacy symlink during rollback: %w", err))
		} else if target != tx.paths.Canonical {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("rollback conflict: legacy symlink belongs to another process"))
		} else if err := removeOwnedSymlink(tx.paths.Legacy, tx.paths.Canonical); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		} else {
			legacyErr = fs.ErrNotExist
		}
	}
	if legacyErr == nil && legacyInfo.Mode().IsRegular() {
		owned, err := hasIdentity(legacyInfo, tx.record.LegacyID)
		if err != nil {
			rollbackErrors = append(rollbackErrors, err)
		} else if owned {
			legacySecured = true
		} else {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore legacy configuration: %w: path occupied by another file", fs.ErrExist))
		}
	} else if errors.Is(legacyErr, fs.ErrNotExist) {
		if tx.backupCreated {
			if err := linkOwnedNoReplace(migrationBackupPath(tx.paths), tx.paths.Legacy, tx.record.LegacyID); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore legacy configuration: %w", err))
			} else {
				legacySecured = true
			}
		}
	} else if legacyErr != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("inspect legacy configuration during rollback: %w", legacyErr))
	}

	if !legacySecured && tx.backupCreated {
		return errors.Join(rollbackErrors...)
	}
	if tx.backupCreated {
		if err := removeOwnedRegularIfPresent(migrationBackupPath(tx.paths), tx.record.LegacyID); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if tx.canonicalCommitted {
		if err := removeOwnedRegularIfPresent(tx.paths.Canonical, tx.record.CanonicalID); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if err := removeOwnedRegularIfPresent(migrationMarkerPath(tx.paths), tx.markerID); err != nil {
		rollbackErrors = append(rollbackErrors, err)
	}
	return errors.Join(rollbackErrors...)
}

func finishRollback(paths Paths, record transactionRecord, markerID fileIdentity) error {
	var rollbackErrors []error
	if err := removeOwnedRegularIfPresent(migrationBackupPath(paths), record.LegacyID); err != nil {
		rollbackErrors = append(rollbackErrors, err)
	}
	if err := removeOwnedRegularIfPresent(paths.Canonical, record.CanonicalID); err != nil {
		rollbackErrors = append(rollbackErrors, err)
	}
	if err := removeOwnedRegularIfPresent(migrationMarkerPath(paths), markerID); err != nil {
		rollbackErrors = append(rollbackErrors, err)
	}
	return errors.Join(rollbackErrors...)
}

func linkOwnedNoReplace(source, destination string, expected fileIdentity) error {
	if err := requireOwnedRegular(source, expected); err != nil {
		return err
	}
	if err := os.Link(source, destination); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(destination))
}

func removeOwnedRegularIfPresent(path string, expected fileIdentity) error {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return removeOwnedRegular(path, expected)
}

func removeOwnedRegular(path string, expected fileIdentity) error {
	if err := requireOwnedRegular(path, expected); err != nil {
		return err
	}
	removeErr := migrationOS.remove(path)
	syncErr := syncDirectory(filepath.Dir(path))
	return errors.Join(removeErr, syncErr)
}

func removeOwnedSymlink(path, expectedTarget string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("refuse to remove non-symlink at %s", path)
	}
	target, err := os.Readlink(path)
	if err != nil {
		return err
	}
	if target != expectedTarget {
		return fmt.Errorf("refuse to remove symlink with unexpected target %s", target)
	}
	removeErr := migrationOS.remove(path)
	syncErr := syncDirectory(filepath.Dir(path))
	return errors.Join(removeErr, syncErr)
}

func requireOwnedRegular(path string, expected fileIdentity) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refuse to modify non-regular file at %s", path)
	}
	owned, err := hasIdentity(info, expected)
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("refuse to modify file with unexpected identity at %s", path)
	}
	return nil
}

func hasIdentity(info os.FileInfo, expected fileIdentity) (bool, error) {
	actual, err := identityOf(info)
	if err != nil {
		return false, err
	}
	return actual == expected, nil
}

func identityOf(info os.FileInfo) (fileIdentity, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdentity{}, errors.New("file identity is unavailable on this platform")
	}
	return fileIdentity{Device: uint64(stat.Dev), Inode: uint64(stat.Ino)}, nil
}

func readTransactionMarker(path string) (transactionRecord, fileIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return transactionRecord{}, fileIdentity{}, err
	}
	if !info.Mode().IsRegular() {
		return transactionRecord{}, fileIdentity{}, fmt.Errorf("migration marker is not a regular file: %s", path)
	}
	id, err := identityOf(info)
	if err != nil {
		return transactionRecord{}, fileIdentity{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return transactionRecord{}, fileIdentity{}, fmt.Errorf("read migration marker: %w", err)
	}
	current, err := os.Lstat(path)
	if err != nil {
		return transactionRecord{}, fileIdentity{}, fmt.Errorf("recheck migration marker: %w", err)
	}
	owned, err := hasIdentity(current, id)
	if err != nil || !owned {
		return transactionRecord{}, fileIdentity{}, errors.New("migration marker changed while it was read")
	}
	var record transactionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return transactionRecord{}, fileIdentity{}, fmt.Errorf("decode migration marker: %w", err)
	}
	return record, id, nil
}

func migrationMarkerPath(paths Paths) string {
	return paths.Legacy + migrationMarkerSuffix
}

func migrationBackupPath(paths Paths) string {
	return paths.Legacy + migrationBackupSuffix
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}
