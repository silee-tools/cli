package config

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const (
	migrationMarkerSuffix = ".oma-migration"
	migrationBackupSuffix = ".oma-migration-backup"
	migrationMarkerMagic  = "OMA-MIGRATION-V2\n"
	migrationAnchorSuffix = ".staged-anchor"
)

var errMigrationBusy = errors.New("configuration migration is busy")

type Migration struct {
	paths Paths
}

type migrationFileOps struct {
	symlink                 func(string, string) error
	remove                  func(string) error
	beforeCanonicalCommit   func(string) error
	afterCanonicalCommit    func()
	afterLegacyBackup       func()
	afterSymlink            func()
	afterOwnershipCheck     func(string)
	beforeMarker            func()
	afterMarkerEstablished  func()
	afterCanonicalLink      func()
	beforeQuarantineMove    func(string, string)
	afterQuarantineMove     func(string, string) error
	afterMarkerOpen         func()
	afterMarkerPartialWrite func()
	afterMarkerFileSync     func()
	markerDirectorySync     func(string) error
	afterRecoveryCheck      func()
	afterMarkerMagicWrite   func(int)
	afterMarkerAnchorSync   func()
	afterMarkerCommit       func()
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
	Token       string       `json:"token"`
	Canonical   string       `json:"canonical"`
	Legacy      string       `json:"legacy"`
	CanonicalID fileIdentity `json:"canonical_identity"`
	LegacyID    fileIdentity `json:"legacy_identity"`
	Digest      string       `json:"config_sha256"`
	Quarantines []string     `json:"quarantine_paths"`
}

type migrationTransaction struct {
	paths              Paths
	record             transactionRecord
	markerID           fileIdentity
	canonicalCommitted bool
	backupCreated      bool
}

func PlanMigration(paths Paths) (*Migration, error) {
	needed, err := recoveryNeeded(paths)
	if err != nil {
		return nil, err
	}
	if !needed {
		if migrationOS.afterRecoveryCheck != nil {
			migrationOS.afterRecoveryCheck()
		}
		return planMigrationReadOnly(paths)
	}
	lock, err := acquireMigrationLock(paths)
	if err != nil {
		return nil, err
	}
	defer lock.release()
	needed, err = recoveryNeeded(paths)
	if err != nil {
		return nil, err
	}
	if needed {
		if err := recoverInterruptedMigration(paths); err != nil {
			return nil, err
		}
	}
	return planMigrationReadOnly(paths)
}

func planMigrationReadOnly(paths Paths) (*Migration, error) {
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
	data, legacyID, err := validateLegacyBeforeMutation(m.paths, validate)
	if err != nil {
		return err
	}
	lock, err := acquireMigrationLock(m.paths)
	if err != nil {
		return err
	}
	defer lock.release()
	if err := recoverInterruptedMigration(m.paths); err != nil {
		return err
	}
	currentData, currentID, err := readLegacyIdentity(m.paths.Legacy)
	if err != nil {
		return err
	}
	if currentID != legacyID || sha256.Sum256(currentData) != sha256.Sum256(data) {
		return errors.New("legacy configuration changed after validation")
	}
	return m.applyLocked(data, legacyID)
}

func (m Migration) applyLocked(data []byte, legacyID fileIdentity) error {
	if _, err := os.Lstat(m.paths.Canonical); err == nil {
		return fmt.Errorf("canonical configuration already exists: %s", m.paths.Canonical)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect canonical configuration: %w", err)
	}
	token, err := newTransactionToken()
	if err != nil {
		return err
	}
	if migrationOS.beforeMarker != nil {
		migrationOS.beforeMarker()
	}
	record := transactionRecord{
		Version:   2,
		Token:     token,
		Canonical: m.paths.Canonical,
		Legacy:    m.paths.Legacy,
		LegacyID:  legacyID,
		Digest:    fmt.Sprintf("%x", sha256.Sum256(data)),
	}
	record.Quarantines = []string{
		canonicalStagedPath(m.paths, token),
		m.paths.Legacy,
		migrationBackupPath(m.paths),
		migrationMarkerPath(m.paths),
		m.paths.Canonical,
	}
	markerID, err := createTransactionMarker(m.paths, record)
	if err != nil {
		return err
	}
	if migrationOS.afterMarkerEstablished != nil {
		migrationOS.afterMarkerEstablished()
	}
	tmpPath, canonicalID, err := createCanonicalTemporary(m.paths.Canonical, data, token)
	if err != nil {
		return errors.Join(err, removeOwnedRegular(migrationMarkerPath(m.paths), markerID, token))
	}
	record.CanonicalID = canonicalID
	tmpPresent := true
	defer func() {
		if tmpPresent {
			_ = removeOwnedRegular(tmpPath, canonicalID, token)
		}
	}()
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
	if migrationOS.afterCanonicalLink != nil {
		migrationOS.afterCanonicalLink()
	}
	if err := removeOwnedRegular(tmpPath, canonicalID, token); err != nil {
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
	if err := removeOwnedRegular(m.paths.Legacy, legacyID, token); err != nil {
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

	if err := verifyFinalState(m.paths, canonicalID); err != nil {
		return fmt.Errorf("verify final configuration before backup cleanup: %w", err)
	}
	if err := removeOwnedRegular(backup, legacyID, token); err != nil {
		return fmt.Errorf("remove legacy backup: %w", err)
	}
	if err := removeOwnedRegular(migrationMarkerPath(m.paths), markerID, token); err != nil {
		return fmt.Errorf("remove migration marker: %w", err)
	}
	return nil
}

type migrationLock struct {
	file *os.File
}

func acquireMigrationLock(paths Paths) (*migrationLock, error) {
	parent := filepath.Dir(paths.Legacy)
	info, err := os.Stat(parent)
	if err != nil {
		return nil, fmt.Errorf("migration lock parent is unavailable: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("migration lock parent is not a directory: %s", parent)
	}
	path := paths.Legacy + ".oma-migration.lock"
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open migration lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("protect migration lock: %w", err)
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, errMigrationBusy
		}
		return nil, fmt.Errorf("lock configuration migration: %w", err)
	}
	return &migrationLock{file: file}, nil
}

func recoveryNeeded(paths Paths) (bool, error) {
	marker := migrationMarkerPath(paths)
	for _, path := range []string{marker, migrationBackupPath(paths), stagedMarkerAnchorPath(marker)} {
		if _, err := os.Lstat(path); err == nil {
			return true, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}
	matches, err := filepath.Glob(migrationMarkerPath(paths) + ".oma-quarantine-*")
	if err != nil {
		return false, err
	}
	if len(matches) > 0 {
		return true, nil
	}
	matches, err = filepath.Glob(stagedMarkerAnchorPath(marker) + ".oma-quarantine-*")
	if err != nil {
		return false, err
	}
	if len(matches) > 0 {
		return true, nil
	}
	matches, err = filepath.Glob(migrationMarkerPath(paths) + ".staged-*")
	if err != nil {
		return false, err
	}
	if len(matches) > 0 {
		return true, nil
	}
	return false, nil
}

func validateLegacyBeforeMutation(paths Paths, validate func(Config) error) ([]byte, fileIdentity, error) {
	data, id, err := readLegacyIdentity(paths.Legacy)
	if err != nil {
		return nil, fileIdentity{}, err
	}
	config, err := decodeConfig(data)
	if err != nil {
		return nil, fileIdentity{}, fmt.Errorf("decode legacy configuration: %w", err)
	}
	if validate != nil {
		if err := validate(config); err != nil {
			return nil, fileIdentity{}, fmt.Errorf("validate legacy configuration: %w", err)
		}
	}
	return data, id, nil
}

func readLegacyIdentity(path string) ([]byte, fileIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fileIdentity{}, fmt.Errorf("inspect legacy configuration: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fileIdentity{}, fmt.Errorf("legacy configuration is not a regular file: %s", path)
	}
	id, err := identityOf(info)
	if err != nil {
		return nil, fileIdentity{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fileIdentity{}, err
	}
	return data, id, nil
}

func (l *migrationLock) release() {
	if l == nil || l.file == nil {
		return
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
}

func createCanonicalTemporary(path string, data []byte, token string) (string, fileIdentity, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fileIdentity{}, fmt.Errorf("create canonical configuration directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fileIdentity{}, fmt.Errorf("protect canonical configuration directory: %w", err)
	}
	tmpPath := path + ".oma-staged-" + token
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return "", fileIdentity{}, fmt.Errorf("create canonical configuration temporary file: %w", err)
	}
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
	staged := marker + ".staged-" + record.Token
	anchor := stagedMarkerAnchorPath(marker)
	if _, err := os.Lstat(staged); err == nil {
		return fileIdentity{}, fmt.Errorf("staged migration marker already exists: %s", staged)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fileIdentity{}, fmt.Errorf("inspect staged migration marker: %w", err)
	}
	if err := os.Symlink(staged, anchor); err != nil {
		return fileIdentity{}, fmt.Errorf("establish staged migration marker anchor without replacement: %w", err)
	}
	if err := syncMarkerDirectory(filepath.Dir(marker)); err != nil {
		return fileIdentity{}, fmt.Errorf("sync legacy directory after marker anchor creation: %w", err)
	}
	if migrationOS.afterMarkerAnchorSync != nil {
		migrationOS.afterMarkerAnchorSync()
	}
	file, err := os.OpenFile(staged, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return fileIdentity{}, fmt.Errorf("create staged migration marker without replacement: %w", err)
	}
	if migrationOS.afterMarkerOpen != nil {
		migrationOS.afterMarkerOpen()
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fileIdentity{}, fmt.Errorf("protect migration marker: %w", err)
	}
	magicHalf := len(migrationMarkerMagic) / 2
	if _, err := file.WriteString(migrationMarkerMagic[:magicHalf]); err != nil {
		_ = file.Close()
		return fileIdentity{}, fmt.Errorf("write first migration marker ownership prefix part: %w", err)
	}
	if migrationOS.afterMarkerMagicWrite != nil {
		migrationOS.afterMarkerMagicWrite(1)
	}
	if _, err := file.WriteString(migrationMarkerMagic[magicHalf:]); err != nil {
		_ = file.Close()
		return fileIdentity{}, fmt.Errorf("write second migration marker ownership prefix part: %w", err)
	}
	if migrationOS.afterMarkerMagicWrite != nil {
		migrationOS.afterMarkerMagicWrite(2)
	}
	half := len(data) / 2
	if _, err := file.Write(data[:half]); err != nil {
		_ = file.Close()
		return fileIdentity{}, fmt.Errorf("write migration marker: %w", err)
	}
	if migrationOS.afterMarkerPartialWrite != nil {
		migrationOS.afterMarkerPartialWrite()
	}
	if _, err := file.Write(data[half:]); err != nil {
		_ = file.Close()
		return fileIdentity{}, fmt.Errorf("finish migration marker: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fileIdentity{}, fmt.Errorf("sync migration marker: %w", err)
	}
	if migrationOS.afterMarkerFileSync != nil {
		migrationOS.afterMarkerFileSync()
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fileIdentity{}, fmt.Errorf("stat migration marker: %w", err)
	}
	id, err := identityOf(info)
	if err != nil {
		_ = file.Close()
		return fileIdentity{}, fmt.Errorf("identify migration marker: %w", err)
	}
	if err := file.Close(); err != nil {
		return fileIdentity{}, fmt.Errorf("close migration marker: %w", err)
	}
	if err := renameNoReplace(staged, marker); err != nil {
		return fileIdentity{}, fmt.Errorf("commit migration marker without replacement: %w", err)
	}
	if err := syncMarkerDirectory(filepath.Dir(marker)); err != nil {
		return fileIdentity{}, fmt.Errorf("sync legacy directory after marker creation: %w", err)
	}
	if migrationOS.afterMarkerCommit != nil {
		migrationOS.afterMarkerCommit()
	}
	if err := removeOwnedSymlink(anchor, staged, record.Token); err != nil {
		return fileIdentity{}, fmt.Errorf("remove staged migration marker anchor: %w", err)
	}
	return id, nil
}

func syncMarkerDirectory(path string) error {
	if migrationOS.markerDirectorySync != nil {
		return migrationOS.markerDirectorySync(path)
	}
	return syncDirectory(path)
}

func recoverInterruptedMigration(paths Paths) error {
	marker := migrationMarkerPath(paths)
	if err := recoverStagedMarker(marker); err != nil {
		return err
	}
	if err := restoreQuarantinedMarker(marker); err != nil {
		return err
	}
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
	if record.Version != 2 || record.Token == "" || len(record.Quarantines) == 0 || record.Canonical != paths.Canonical || record.Legacy != paths.Legacy {
		return fmt.Errorf("migration marker does not belong to requested paths: %s", marker)
	}
	if err := restoreRecordedQuarantines(record); err != nil {
		return err
	}
	record, err = hydrateCanonicalIdentity(paths, record)
	if err != nil {
		return err
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
		if err := verifyFinalState(paths, record.CanonicalID); err != nil {
			return fmt.Errorf("recovery final-state verification: %w", err)
		}
		if err := removeOwnedRegularIfPresent(migrationBackupPath(paths), record.LegacyID, record.Token); err != nil {
			return err
		}
		return removeOwnedRegular(marker, markerID, record.Token)
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
	if err := restoreRecordedQuarantines(tx.record); err != nil {
		return err
	}
	legacyInfo, legacyErr := os.Lstat(tx.paths.Legacy)
	legacySecured := false
	var rollbackErrors []error
	if legacyErr == nil && legacyInfo.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(tx.paths.Legacy)
		if err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("read legacy symlink during rollback: %w", err))
		} else if target != tx.paths.Canonical {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("rollback conflict: legacy symlink belongs to another process"))
		} else if err := removeOwnedSymlink(tx.paths.Legacy, tx.paths.Canonical, tx.record.Token); err != nil {
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
		if err := removeOwnedRegularIfPresent(migrationBackupPath(tx.paths), tx.record.LegacyID, tx.record.Token); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if tx.canonicalCommitted {
		if err := removeOwnedRegularIfPresent(tx.paths.Canonical, tx.record.CanonicalID, tx.record.Token); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if err := removeOwnedRegularIfPresent(migrationMarkerPath(tx.paths), tx.markerID, tx.record.Token); err != nil {
		rollbackErrors = append(rollbackErrors, err)
	}
	return errors.Join(rollbackErrors...)
}

func finishRollback(paths Paths, record transactionRecord, markerID fileIdentity) error {
	var rollbackErrors []error
	if record.CanonicalID != (fileIdentity{}) {
		if err := removeOwnedRegularIfPresent(canonicalStagedPath(paths, record.Token), record.CanonicalID, record.Token); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if err := removeOwnedRegularIfPresent(migrationBackupPath(paths), record.LegacyID, record.Token); err != nil {
		rollbackErrors = append(rollbackErrors, err)
	}
	if record.CanonicalID != (fileIdentity{}) {
		if err := removeOwnedRegularIfPresent(paths.Canonical, record.CanonicalID, record.Token); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if err := removeOwnedRegularIfPresent(migrationMarkerPath(paths), markerID, record.Token); err != nil {
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

func removeOwnedRegularIfPresent(path string, expected fileIdentity, token string) error {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return removeOwnedRegular(path, expected, token)
}

func removeOwnedRegular(path string, expected fileIdentity, token string) error {
	if err := requireOwnedRegular(path, expected); err != nil {
		return err
	}
	if migrationOS.afterOwnershipCheck != nil {
		migrationOS.afterOwnershipCheck(path)
	}
	quarantine := path + ".oma-quarantine-" + token
	if _, err := os.Lstat(quarantine); err == nil {
		return fmt.Errorf("quarantine conflict: %s already exists", quarantine)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if migrationOS.beforeQuarantineMove != nil {
		migrationOS.beforeQuarantineMove(path, quarantine)
	}
	if err := renameNoReplace(path, quarantine); err != nil {
		return fmt.Errorf("move regular file to quarantine: %w", err)
	}
	if migrationOS.afterQuarantineMove != nil {
		if err := migrationOS.afterQuarantineMove(path, quarantine); err != nil {
			return fmt.Errorf("after regular quarantine move: %w", err)
		}
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if err := requireOwnedRegular(quarantine, expected); err != nil {
		restoreErr := restoreQuarantinedRegular(quarantine, path)
		return errors.Join(fmt.Errorf("quarantine identity conflict: %w", err), restoreErr)
	}
	removeErr := migrationOS.remove(quarantine)
	syncErr := syncDirectory(filepath.Dir(quarantine))
	return errors.Join(removeErr, syncErr)
}

func removeOwnedSymlink(path, expectedTarget, token string) error {
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
	if migrationOS.afterOwnershipCheck != nil {
		migrationOS.afterOwnershipCheck(path)
	}
	quarantine := path + ".oma-quarantine-" + token
	if migrationOS.beforeQuarantineMove != nil {
		migrationOS.beforeQuarantineMove(path, quarantine)
	}
	if err := renameNoReplace(path, quarantine); err != nil {
		return fmt.Errorf("move symlink to quarantine: %w", err)
	}
	if migrationOS.afterQuarantineMove != nil {
		if err := migrationOS.afterQuarantineMove(path, quarantine); err != nil {
			return fmt.Errorf("after symlink quarantine move: %w", err)
		}
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	actualTarget, err := os.Readlink(quarantine)
	if err != nil || actualTarget != expectedTarget {
		restoreErr := restoreQuarantinedObject(quarantine, path)
		return errors.Join(fmt.Errorf("quarantine symlink target conflict: got %q: %w", actualTarget, err), restoreErr)
	}
	removeErr := migrationOS.remove(quarantine)
	syncErr := syncDirectory(filepath.Dir(quarantine))
	return errors.Join(removeErr, syncErr)
}

func renameNoReplace(source, destination string) error {
	return renameNoReplaceForPlatform(source, destination, runtime.GOOS, runtime.GOARCH, syscall.Syscall6)
}

func renameNoReplaceForPlatform(
	source, destination, goos, goarch string,
	call func(uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr) (uintptr, uintptr, syscall.Errno),
) error {
	sourcePtr, err := syscall.BytePtrFromString(source)
	if err != nil {
		return err
	}
	destinationPtr, err := syscall.BytePtrFromString(destination)
	if err != nil {
		return err
	}
	trap, atFDCWD, flags, err := renameNoReplaceRoute(goos, goarch)
	if err != nil {
		return err
	}
	_, _, errno := call(
		trap,
		atFDCWD,
		uintptr(unsafe.Pointer(sourcePtr)),
		atFDCWD,
		uintptr(unsafe.Pointer(destinationPtr)),
		flags,
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}

func renameNoReplaceRoute(goos, goarch string) (trap, dirFD, flags uintptr, err error) {
	switch goos {
	case "darwin":
		switch goarch {
		case "amd64", "arm64":
			return 488, ^uintptr(1), 0x4, nil // renameatx_np with RENAME_EXCL and AT_FDCWD=-2.
		default:
			return 0, 0, 0, fmt.Errorf("no no-replace rename syscall for darwin/%s", goarch)
		}
	case "linux":
		switch goarch {
		case "amd64":
			return 316, ^uintptr(99), 0x1, nil // renameat2 with RENAME_NOREPLACE and AT_FDCWD=-100.
		case "arm64":
			return 276, ^uintptr(99), 0x1, nil
		default:
			return 0, 0, 0, fmt.Errorf("no no-replace rename syscall for linux/%s", goarch)
		}
	default:
		return 0, 0, 0, fmt.Errorf("no no-replace rename syscall for %s", goos)
	}
}

func restoreQuarantinedRegular(quarantine, destination string) error {
	if err := os.Link(quarantine, destination); err != nil {
		return fmt.Errorf("restore quarantined regular file without replacement: %w", err)
	}
	return syncDirectory(filepath.Dir(destination))
}

func restoreQuarantinedMarker(marker string) error {
	if _, err := os.Lstat(marker); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	matches, err := filepath.Glob(marker + ".oma-quarantine-*")
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) != 1 {
		return fmt.Errorf("multiple quarantined migration markers conflict: %v", matches)
	}
	if err := renameNoReplace(matches[0], marker); err != nil {
		return fmt.Errorf("restore quarantined migration marker: %w", err)
	}
	return syncDirectory(filepath.Dir(marker))
}

func restoreRecordedQuarantines(record transactionRecord) error {
	for _, original := range record.Quarantines {
		quarantine := original + ".oma-quarantine-" + record.Token
		if _, err := os.Lstat(quarantine); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if _, err := os.Lstat(original); err == nil {
			return fmt.Errorf("quarantine recovery conflict: both %s and %s exist", original, quarantine)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := renameNoReplace(quarantine, original); err != nil {
			return fmt.Errorf("restore recorded quarantine %s: %w", quarantine, err)
		}
		if err := syncDirectory(filepath.Dir(original)); err != nil {
			return err
		}
	}
	return nil
}

func restoreQuarantinedObject(quarantine, destination string) error {
	info, err := os.Lstat(quarantine)
	if err != nil {
		return err
	}
	if info.Mode().IsRegular() {
		return restoreQuarantinedRegular(quarantine, destination)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("quarantined replacement has unsupported type: %s", info.Mode())
	}
	target, err := os.Readlink(quarantine)
	if err != nil {
		return err
	}
	if err := os.Symlink(target, destination); err != nil {
		return fmt.Errorf("restore quarantined symlink without replacement: %w", err)
	}
	return syncDirectory(filepath.Dir(destination))
}

func verifyFinalState(paths Paths, canonicalID fileIdentity) error {
	if err := requireOwnedRegular(paths.Canonical, canonicalID); err != nil {
		return err
	}
	info, err := os.Lstat(paths.Legacy)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("legacy path is not a symlink")
	}
	target, err := os.Readlink(paths.Legacy)
	if err != nil {
		return err
	}
	if target != paths.Canonical {
		return fmt.Errorf("legacy symlink target is %s", target)
	}
	return nil
}

func hydrateCanonicalIdentity(paths Paths, record transactionRecord) (transactionRecord, error) {
	if record.CanonicalID != (fileIdentity{}) {
		return record, nil
	}
	for _, path := range []string{canonicalStagedPath(paths, record.Token), paths.Canonical} {
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return record, err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return record, fmt.Errorf("recovery conflict: staged canonical has unexpected type or mode")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return record, err
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != record.Digest {
			return record, fmt.Errorf("recovery conflict: staged canonical digest does not match marker")
		}
		record.CanonicalID, err = identityOf(info)
		return record, err
	}
	return record, nil
}

func canonicalStagedPath(paths Paths, token string) string {
	return paths.Canonical + ".oma-staged-" + token
}

func newTransactionToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate migration transaction token: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
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
	if !bytes.HasPrefix(data, []byte(migrationMarkerMagic)) {
		return transactionRecord{}, fileIdentity{}, errors.New("migration marker ownership prefix is missing")
	}
	data = data[len(migrationMarkerMagic):]
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

func recoverStagedMarker(marker string) error {
	anchor := stagedMarkerAnchorPath(marker)
	if err := restoreQuarantinedStagedAnchor(marker, anchor); err != nil {
		return err
	}
	anchorInfo, err := os.Lstat(anchor)
	if errors.Is(err, fs.ErrNotExist) {
		return recoverLegacyStagedMarker(marker)
	}
	if err != nil {
		return err
	}
	if anchorInfo.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("staged migration marker anchor is not a symlink: %s", anchor)
	}
	target, err := os.Readlink(anchor)
	if err != nil {
		return fmt.Errorf("read staged migration marker anchor: %w", err)
	}
	token, err := stagedMarkerToken(marker, target)
	if err != nil {
		return err
	}
	if err := restoreAnchoredTargetQuarantine(target, token); err != nil {
		return err
	}

	if _, err := os.Lstat(marker); err == nil {
		record, _, err := readTransactionMarker(marker)
		if err != nil {
			return err
		}
		if record.Token != token {
			return fmt.Errorf("staged migration marker anchor token does not match fixed marker")
		}
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("staged marker target conflicts with committed marker: %s", target)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return removeOwnedSymlink(anchor, target, token)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	targetInfo, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return removeOwnedSymlink(anchor, target, token)
	}
	if err != nil {
		return err
	}
	if !targetInfo.Mode().IsRegular() {
		return fmt.Errorf("staged migration marker target is not a regular file: %s", target)
	}
	targetID, err := identityOf(targetInfo)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	var record transactionRecord
	complete := bytes.HasPrefix(data, []byte(migrationMarkerMagic)) && json.Unmarshal(data[len(migrationMarkerMagic):], &record) == nil
	if complete {
		if record.Token != token {
			return fmt.Errorf("staged migration marker token mismatch: %s", target)
		}
		if err := renameNoReplace(target, marker); err != nil {
			return fmt.Errorf("commit recovered staged migration marker: %w", err)
		}
		if err := syncDirectory(filepath.Dir(marker)); err != nil {
			return err
		}
	} else if err := removeOwnedRegular(target, targetID, token); err != nil {
		return fmt.Errorf("discard incomplete anchored migration marker: %w", err)
	}
	return removeOwnedSymlink(anchor, target, token)
}

func recoverLegacyStagedMarker(marker string) error {
	if _, err := os.Lstat(marker); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	matches, err := filepath.Glob(marker + ".staged-*")
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) != 1 {
		return fmt.Errorf("multiple staged migration markers conflict: %v", matches)
	}
	staged := matches[0]
	data, err := os.ReadFile(staged)
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(data, []byte(migrationMarkerMagic)) {
		return fmt.Errorf("staged migration marker ownership cannot be proven: %s", staged)
	}
	var record transactionRecord
	if err := json.Unmarshal(data[len(migrationMarkerMagic):], &record); err != nil {
		discard := staged + ".discard"
		if err := renameNoReplace(staged, discard); err != nil {
			return fmt.Errorf("isolate incomplete staged marker: %w", err)
		}
		removeErr := os.Remove(discard)
		syncErr := syncDirectory(filepath.Dir(discard))
		return errors.Join(removeErr, syncErr)
	}
	if record.Token == "" || !strings.HasSuffix(staged, ".staged-"+record.Token) {
		return fmt.Errorf("staged migration marker token mismatch: %s", staged)
	}
	if err := renameNoReplace(staged, marker); err != nil {
		return fmt.Errorf("commit recovered staged migration marker: %w", err)
	}
	return syncDirectory(filepath.Dir(marker))
}

func restoreQuarantinedStagedAnchor(marker, anchor string) error {
	matches, err := filepath.Glob(anchor + ".oma-quarantine-*")
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) != 1 {
		return fmt.Errorf("multiple quarantined staged marker anchors conflict: %v", matches)
	}
	if _, err := os.Lstat(anchor); err == nil {
		return fmt.Errorf("staged marker anchor and quarantine both exist")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	quarantine := matches[0]
	target, err := os.Readlink(quarantine)
	if err != nil {
		return fmt.Errorf("quarantined staged marker anchor is not a symlink: %w", err)
	}
	token, err := stagedMarkerToken(marker, target)
	if err != nil {
		return err
	}
	if quarantine != anchor+".oma-quarantine-"+token {
		return fmt.Errorf("quarantined staged marker anchor token mismatch: %s", quarantine)
	}
	if err := renameNoReplace(quarantine, anchor); err != nil {
		return fmt.Errorf("restore staged marker anchor quarantine: %w", err)
	}
	return syncDirectory(filepath.Dir(anchor))
}

func restoreAnchoredTargetQuarantine(target, token string) error {
	quarantine := target + ".oma-quarantine-" + token
	if _, err := os.Lstat(quarantine); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("staged marker target and quarantine both exist")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := renameNoReplace(quarantine, target); err != nil {
		return fmt.Errorf("restore staged marker target quarantine: %w", err)
	}
	return syncDirectory(filepath.Dir(target))
}

func stagedMarkerToken(marker, target string) (string, error) {
	prefix := marker + ".staged-"
	if filepath.Dir(target) != filepath.Dir(marker) || !strings.HasPrefix(target, prefix) {
		return "", fmt.Errorf("staged migration marker anchor has unexpected target: %s", target)
	}
	token := strings.TrimPrefix(target, prefix)
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != 16 {
		return "", fmt.Errorf("staged migration marker anchor has invalid token: %s", target)
	}
	return token, nil
}

func stagedMarkerAnchorPath(marker string) string {
	return marker + migrationAnchorSuffix
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
