package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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

var errMigrationBusy = errors.New("configuration migration is busy")

type Migration struct {
	paths Paths
}

type migrationFileOps struct {
	symlink                func(string, string) error
	remove                 func(string) error
	beforeCanonicalCommit  func(string) error
	afterCanonicalCommit   func()
	afterLegacyBackup      func()
	afterSymlink           func()
	afterOwnershipCheck    func(string)
	beforeMarker           func()
	afterMarkerEstablished func()
	afterCanonicalLink     func()
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
}

type migrationTransaction struct {
	paths              Paths
	record             transactionRecord
	markerID           fileIdentity
	canonicalCommitted bool
	backupCreated      bool
}

func PlanMigration(paths Paths) (*Migration, error) {
	lock, err := acquireMigrationLock(paths)
	if err != nil {
		return nil, err
	}
	defer lock.release()
	return planMigrationLocked(paths)
}

func planMigrationLocked(paths Paths) (*Migration, error) {
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
	lock, err := acquireMigrationLock(m.paths)
	if err != nil {
		return err
	}
	defer lock.release()
	return m.applyLocked(validate)
}

func (m Migration) applyLocked(validate func(Config) error) error {
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
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("create migration lock directory: %w", err)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return nil, fmt.Errorf("protect migration lock directory: %w", err)
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
	file, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return fileIdentity{}, fmt.Errorf("create migration marker without replacement: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fileIdentity{}, fmt.Errorf("protect migration marker: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fileIdentity{}, fmt.Errorf("write migration marker: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fileIdentity{}, fmt.Errorf("sync migration marker: %w", err)
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
	if err := syncDirectory(filepath.Dir(marker)); err != nil {
		return fileIdentity{}, fmt.Errorf("sync legacy directory after marker creation: %w", err)
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
	if record.Version != 2 || record.Token == "" || record.Canonical != paths.Canonical || record.Legacy != paths.Legacy {
		return fmt.Errorf("migration marker does not belong to requested paths: %s", marker)
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
	if err := os.Rename(path, quarantine); err != nil {
		return fmt.Errorf("move regular file to quarantine: %w", err)
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
	if err := os.Rename(path, quarantine); err != nil {
		return fmt.Errorf("move symlink to quarantine: %w", err)
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

func restoreQuarantinedRegular(quarantine, destination string) error {
	if err := os.Link(quarantine, destination); err != nil {
		return fmt.Errorf("restore quarantined regular file without replacement: %w", err)
	}
	return syncDirectory(filepath.Dir(destination))
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
