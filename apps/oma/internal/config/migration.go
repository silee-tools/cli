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
	"sort"
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

var (
	errMigrationBusy         = errors.New("configuration migration is busy")
	ErrMigrationStateChanged = errors.New("configuration migration state changed after inspection")
)

type Migration struct {
	paths      Paths
	inspection migrationInspection
}

type migrationInspection struct {
	fingerprint string
	sourcePath  string
	sourceID    fileIdentity
	sourceHash  [32]byte
	legacyID    fileIdentity
	recovery    bool
}

type migrationArtifact struct {
	path       string
	mode       os.FileMode
	identity   fileIdentity
	digest     [32]byte
	data       []byte
	linkTarget string
}

type migrationFileOps struct {
	symlink                            func(string, string) error
	remove                             func(string) error
	afterInspectionReadDir             func(string)
	beforeInspectionLstat              func(string)
	beforeInspectionRead               func(string)
	afterInspectionRead                func(string)
	beforeCanonicalCommit              func(string) error
	afterCanonicalCommit               func()
	afterLegacyBackup                  func()
	afterSymlink                       func()
	afterOwnershipCheck                func(string)
	beforeMarker                       func()
	afterMarkerEstablished             func()
	afterCanonicalLink                 func()
	beforeQuarantineMove               func(string, string)
	afterQuarantineMove                func(string, string) error
	afterMarkerOpen                    func()
	afterMarkerPartialWrite            func()
	afterMarkerFileSync                func()
	markerDirectorySync                func(string) error
	afterRecoveryCheck                 func()
	afterMarkerMagicWrite              func(int)
	afterMarkerAnchorSync              func()
	afterMarkerCommit                  func()
	afterStagedMarkerRead              func(string)
	afterConflictAnchorMove            func()
	afterConflictRestore               func()
	conflictDirectorySync              func(string) error
	afterConflictIntentSync            func()
	afterConflictActiveMove            func(string, string)
	afterConflictIntentOpen            func()
	afterConflictIntentPart            func()
	afterConflictIntentFileSync        func()
	afterConflictIntentDraftAnchorSync func()
	afterConflictIntentDraftAnchorMove func()
	conflictIntentDraftAnchorSync      func(string) error
}

var migrationOS = migrationFileOps{
	symlink: os.Symlink,
	remove:  os.Remove,
}

type fileIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

type stagedMarkerConflictIntent struct {
	Version  int          `json:"version"`
	Token    string       `json:"token"`
	Target   string       `json:"target"`
	AnchorID fileIdentity `json:"anchor_identity"`
	Nonce    string       `json:"nonce"`
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
	if needed {
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
	} else if migrationOS.afterRecoveryCheck != nil {
		migrationOS.afterRecoveryCheck()
	}
	return InspectMigration(paths)
}

// InspectMigration reports the logical migration plan without recovering or
// mutating any configuration artifact.
func InspectMigration(paths Paths) (*Migration, error) {
	inspection, needed, err := inspectMigration(paths)
	if err != nil {
		return nil, err
	}
	if !needed {
		return nil, nil
	}
	return &Migration{paths: paths, inspection: inspection}, nil
}

// Fingerprint binds an approval to the complete migration artifact topology.
func (m Migration) Fingerprint() string { return m.inspection.fingerprint }

// Load reads the logical configuration selected during read-only inspection.
// Identity and content are rechecked so callers never consume a substituted file.
func (m Migration) Load() (Config, error) {
	data, err := readInspectedConfig(m.inspection)
	if err != nil {
		return Config{}, err
	}
	config, err := decodeConfig(data)
	if err != nil {
		return Config{}, fmt.Errorf("decode inspected migration configuration: %w", err)
	}
	return config, nil
}

func inspectMigration(paths Paths) (migrationInspection, bool, error) {
	first, firstFingerprint, err := snapshotMigrationArtifacts(paths)
	if err != nil {
		return migrationInspection{}, false, err
	}
	second, secondFingerprint, err := snapshotMigrationArtifacts(paths)
	if err != nil {
		return migrationInspection{}, false, err
	}
	if firstFingerprint != secondFingerprint {
		return migrationInspection{}, false, fmt.Errorf("%w: artifacts changed during inspection", ErrMigrationStateChanged)
	}
	_ = first

	recovery, err := recoveryNeeded(paths)
	if err != nil {
		return migrationInspection{}, false, err
	}
	if !recovery {
		planned, err := planMigrationReadOnly(paths)
		if err != nil {
			return migrationInspection{}, false, err
		}
		if planned == nil {
			return migrationInspection{}, false, nil
		}
		source, ok := second[paths.Legacy]
		if !ok || !source.mode.IsRegular() {
			return migrationInspection{}, false, fmt.Errorf("legacy configuration is unavailable during inspection: %s", paths.Legacy)
		}
		return migrationInspection{fingerprint: secondFingerprint, sourcePath: source.path, sourceID: source.identity, sourceHash: source.digest, legacyID: source.identity}, true, nil
	}

	inspection, err := inspectInterruptedMigration(paths, second, secondFingerprint)
	if err != nil {
		return migrationInspection{}, false, err
	}
	return inspection, true, nil
}

func snapshotMigrationArtifacts(paths Paths) (map[string]migrationArtifact, string, error) {
	roots := []string{filepath.Dir(paths.Canonical), filepath.Dir(paths.Legacy)}
	seenRoots := make(map[string]bool)
	artifacts := make(map[string]migrationArtifact)
	candidates := make(map[string]fileIdentity)
	for _, root := range roots {
		if seenRoots[root] {
			continue
		}
		seenRoots[root] = true
		if _, err := os.Lstat(root); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, "", fmt.Errorf("inspect migration directory: %w", err)
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, "", fmt.Errorf("enumerate migration directory: %w", err)
		}
		if migrationOS.afterInspectionReadDir != nil {
			migrationOS.afterInspectionReadDir(root)
		}
		for _, entry := range entries {
			path := filepath.Join(root, entry.Name())
			if !isMigrationArtifactPath(paths, path) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil, "", fmt.Errorf("%w: artifact disappeared after directory enumeration: %s", ErrMigrationStateChanged, path)
				}
				return nil, "", err
			}
			identity, err := identityOf(info)
			if err != nil {
				return nil, "", err
			}
			candidates[path] = identity
		}
	}
	candidatePaths := make([]string, 0, len(candidates))
	for path := range candidates {
		candidatePaths = append(candidatePaths, path)
	}
	sort.Strings(candidatePaths)
	for _, path := range candidatePaths {
		if migrationOS.beforeInspectionLstat != nil {
			migrationOS.beforeInspectionLstat(path)
		}
		artifact, err := readMigrationArtifact(path, candidates[path])
		if err != nil {
			return nil, "", err
		}
		artifacts[path] = artifact
	}
	pathsSorted := sortedMigrationArtifactPaths(artifacts)
	hash := sha256.New()
	for _, path := range pathsSorted {
		artifact := artifacts[path]
		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%d\x00%d\x00%d\x00%x\x00%s\x00", path, artifact.mode, artifact.identity.Device, artifact.identity.Inode, len(artifact.data), artifact.digest, artifact.linkTarget)
	}
	return artifacts, hex.EncodeToString(hash.Sum(nil)), nil
}

func sortedMigrationArtifactPaths(artifacts map[string]migrationArtifact) []string {
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func isMigrationArtifactPath(paths Paths, path string) bool {
	marker := migrationMarkerPath(paths)
	backup := migrationBackupPath(paths)
	anchor := stagedMarkerAnchorPath(marker)
	for _, fixed := range []string{paths.Canonical, paths.Legacy, marker, backup, anchor} {
		if path == fixed {
			return true
		}
		if token, ok := strings.CutPrefix(path, fixed+".oma-quarantine-"); ok && validTransactionToken(token) {
			return true
		}
	}
	if suffix, ok := strings.CutPrefix(path, paths.Canonical+".oma-staged-"); ok {
		return validTransactionToken(suffix) || validTokenPairSuffix(suffix, ".oma-quarantine-")
	}
	suffix, ok := strings.CutPrefix(path, marker+".staged-")
	if !ok {
		return false
	}
	if validTransactionToken(suffix) || validTokenPairSuffix(suffix, ".oma-quarantine-") {
		return true
	}
	if token, ok := strings.CutPrefix(suffix, "quarantine-"); ok {
		return validTransactionToken(token)
	}
	if token, ok := strings.CutPrefix(suffix, "conflict-"); ok {
		if validTransactionToken(token) || validTokenPairSuffix(token, ".oma-quarantine-") {
			return true
		}
	}
	intent, ok := strings.CutPrefix(suffix, "conflict-intent-")
	if !ok {
		return false
	}
	if validTokenPairSuffix(intent, ".oma-quarantine-") {
		return true
	}
	if len(intent) < 32 || !validTransactionToken(intent[:32]) {
		return false
	}
	remainder := intent[32:]
	if remainder == "" || remainder == ".oma-draft-anchor" {
		return true
	}
	if token, ok := strings.CutPrefix(remainder, ".oma-draft-anchor.oma-quarantine-"); ok {
		return validTransactionToken(token)
	}
	if nonce, ok := strings.CutPrefix(remainder, ".oma-staged-"); ok {
		return validTransactionToken(nonce)
	}
	return false
}

func validTokenPairSuffix(value, separator string) bool {
	if len(value) != 32+len(separator)+32 || value[32:32+len(separator)] != separator {
		return false
	}
	return validTransactionToken(value[:32]) && value[:32] == value[32+len(separator):]
}

func validTransactionToken(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16
}

func readMigrationArtifact(path string, enumeratedIdentity fileIdentity) (migrationArtifact, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return migrationArtifact{}, fmt.Errorf("%w: artifact disappeared before inspection: %s", ErrMigrationStateChanged, path)
		}
		return migrationArtifact{}, err
	}
	identity, err := identityOf(info)
	if err != nil {
		return migrationArtifact{}, err
	}
	if identity != enumeratedIdentity {
		return migrationArtifact{}, fmt.Errorf("%w: artifact identity changed after directory enumeration: %s", ErrMigrationStateChanged, path)
	}
	artifact := migrationArtifact{path: path, mode: info.Mode(), identity: identity}
	if migrationOS.beforeInspectionRead != nil {
		migrationOS.beforeInspectionRead(path)
	}
	switch {
	case info.Mode().IsRegular():
		data, err := os.ReadFile(path)
		if err != nil {
			if stateErr := verifyMigrationArtifactState(path, identity, info.Mode(), "while regular content was read"); stateErr != nil {
				return migrationArtifact{}, stateErr
			}
			return migrationArtifact{}, err
		}
		if migrationOS.afterInspectionRead != nil {
			migrationOS.afterInspectionRead(path)
		}
		if err := verifyMigrationArtifactState(path, identity, info.Mode(), "after regular content was read"); err != nil {
			return migrationArtifact{}, err
		}
		artifact.data = data
		artifact.digest = sha256.Sum256(data)
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			if stateErr := verifyMigrationArtifactState(path, identity, info.Mode(), "while link target was read"); stateErr != nil {
				return migrationArtifact{}, stateErr
			}
			return migrationArtifact{}, err
		}
		if migrationOS.afterInspectionRead != nil {
			migrationOS.afterInspectionRead(path)
		}
		if err := verifyMigrationArtifactState(path, identity, info.Mode(), "after link target was read"); err != nil {
			return migrationArtifact{}, err
		}
		artifact.linkTarget = target
	}
	return artifact, nil
}

func verifyMigrationArtifactState(path string, identity fileIdentity, mode os.FileMode, phase string) error {
	current, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: artifact disappeared %s: %s", ErrMigrationStateChanged, phase, path)
		}
		return err
	}
	owned, err := hasIdentity(current, identity)
	if err != nil {
		return err
	}
	if !owned || current.Mode().Type() != mode.Type() {
		return fmt.Errorf("%w: artifact identity or type changed %s: %s", ErrMigrationStateChanged, phase, path)
	}
	return nil
}

func inspectInterruptedMigration(paths Paths, artifacts map[string]migrationArtifact, fingerprint string) (migrationInspection, error) {
	marker := migrationMarkerPath(paths)
	if fixed, ok := artifacts[marker]; ok && fixed.mode.IsRegular() && !bytes.HasPrefix(fixed.data, []byte(migrationMarkerMagic)) {
		return migrationInspection{}, errors.New("corrupt migration marker: ownership prefix is missing")
	}

	var records []transactionRecord
	for _, path := range sortedMigrationArtifactPaths(artifacts) {
		artifact := artifacts[path]
		if !artifact.mode.IsRegular() || !bytes.HasPrefix(artifact.data, []byte(migrationMarkerMagic)) {
			continue
		}
		var record transactionRecord
		if err := json.Unmarshal(artifact.data[len(migrationMarkerMagic):], &record); err != nil {
			if path == marker || !strings.Contains(path, ".staged-") {
				return migrationInspection{}, fmt.Errorf("corrupt migration marker %s: %w", path, err)
			}
			continue
		}
		if err := validateInspectedRecord(paths, record); err != nil {
			return migrationInspection{}, err
		}
		records = append(records, record)
	}

	for path, artifact := range artifacts {
		if strings.Contains(path, ".oma-quarantine-") {
			token := path[strings.LastIndex(path, ".oma-quarantine-")+len(".oma-quarantine-"):]
			decoded, err := hex.DecodeString(token)
			if err != nil || len(decoded) != 16 {
				return migrationInspection{}, fmt.Errorf("migration quarantine has invalid token: %s", path)
			}
		}
		if path == stagedMarkerAnchorPath(marker) && artifact.mode&os.ModeSymlink != 0 {
			if _, err := stagedMarkerToken(marker, artifact.linkTarget); err != nil {
				return migrationInspection{}, err
			}
		}
	}

	if len(records) == 0 {
		legacy, ok := artifacts[paths.Legacy]
		if !ok || !legacy.mode.IsRegular() {
			return migrationInspection{}, errors.New("migration recovery has no verified logical configuration")
		}
		return migrationInspection{fingerprint: fingerprint, sourcePath: legacy.path, sourceID: legacy.identity, sourceHash: legacy.digest, legacyID: legacy.identity, recovery: true}, nil
	}
	for _, record := range records[1:] {
		if !sameTransactionRecord(records[0], record) {
			return migrationInspection{}, errors.New("conflicting migration markers describe different transactions")
		}
	}
	record := records[0]
	digestBytes, _ := hex.DecodeString(record.Digest)
	var expected [32]byte
	copy(expected[:], digestBytes)

	preferred := []string{paths.Legacy, migrationBackupPath(paths), paths.Canonical, canonicalStagedPath(paths, record.Token)}
	for _, path := range preferred {
		artifact, ok := artifacts[path]
		if !ok || !artifact.mode.IsRegular() || artifact.digest != expected {
			continue
		}
		if path == paths.Legacy || path == migrationBackupPath(paths) {
			if artifact.identity != record.LegacyID {
				continue
			}
		}
		return migrationInspection{fingerprint: fingerprint, sourcePath: path, sourceID: artifact.identity, sourceHash: artifact.digest, legacyID: record.LegacyID, recovery: true}, nil
	}
	for _, path := range sortedMigrationArtifactPaths(artifacts) {
		artifact := artifacts[path]
		if artifact.mode.IsRegular() && artifact.digest == expected && strings.Contains(path, ".oma-quarantine-") {
			return migrationInspection{fingerprint: fingerprint, sourcePath: path, sourceID: artifact.identity, sourceHash: artifact.digest, legacyID: record.LegacyID, recovery: true}, nil
		}
	}
	return migrationInspection{}, errors.New("migration recovery has no artifact matching the recorded configuration digest")
}

func sameTransactionRecord(left, right transactionRecord) bool {
	if left.Version != right.Version || left.Token != right.Token || left.Canonical != right.Canonical || left.Legacy != right.Legacy || left.CanonicalID != right.CanonicalID || left.LegacyID != right.LegacyID || left.Digest != right.Digest {
		return false
	}
	leftQuarantines := append([]string(nil), left.Quarantines...)
	rightQuarantines := append([]string(nil), right.Quarantines...)
	sort.Strings(leftQuarantines)
	sort.Strings(rightQuarantines)
	return strings.Join(leftQuarantines, "\x00") == strings.Join(rightQuarantines, "\x00")
}

func validateInspectedRecord(paths Paths, record transactionRecord) error {
	decoded, err := hex.DecodeString(record.Token)
	if record.Version != 2 || err != nil || len(decoded) != 16 || record.Canonical != paths.Canonical || record.Legacy != paths.Legacy || record.LegacyID == (fileIdentity{}) {
		return errors.New("migration marker schema or path ownership is invalid")
	}
	digest, err := hex.DecodeString(record.Digest)
	if err != nil || len(digest) != sha256.Size {
		return errors.New("migration marker configuration digest is invalid")
	}
	expected := []string{canonicalStagedPath(paths, record.Token), paths.Legacy, migrationBackupPath(paths), migrationMarkerPath(paths), paths.Canonical}
	actual := append([]string(nil), record.Quarantines...)
	sort.Strings(expected)
	sort.Strings(actual)
	if strings.Join(expected, "\x00") != strings.Join(actual, "\x00") {
		return errors.New("migration marker quarantine topology is invalid")
	}
	return nil
}

func readInspectedConfig(inspection migrationInspection) ([]byte, error) {
	artifact, err := readMigrationArtifact(inspection.sourcePath, inspection.sourceID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: inspected source disappeared", ErrMigrationStateChanged)
		}
		return nil, fmt.Errorf("read inspected migration configuration: %w", err)
	}
	if artifact.identity != inspection.sourceID || artifact.digest != inspection.sourceHash || !artifact.mode.IsRegular() {
		return nil, fmt.Errorf("%w: inspected source identity or digest changed", ErrMigrationStateChanged)
	}
	return append([]byte(nil), artifact.data...), nil
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
	data, err := readInspectedConfig(m.inspection)
	if err != nil {
		return err
	}
	cfg, err := decodeConfig(data)
	if err != nil {
		return fmt.Errorf("decode inspected migration configuration: %w", err)
	}
	if validate != nil {
		if err := validate(cfg); err != nil {
			return fmt.Errorf("validate inspected migration configuration: %w", err)
		}
	}
	lock, err := acquireMigrationLock(m.paths)
	if err != nil {
		return err
	}
	defer lock.release()
	_, lockedFingerprint, err := snapshotMigrationArtifacts(m.paths)
	if err != nil {
		return err
	}
	if lockedFingerprint != m.inspection.fingerprint {
		return ErrMigrationStateChanged
	}
	current, needed, err := inspectMigration(m.paths)
	if err != nil {
		return err
	}
	if !needed || current.fingerprint != m.inspection.fingerprint {
		return ErrMigrationStateChanged
	}
	data, err = readInspectedConfig(current)
	if err != nil {
		return err
	}
	if sha256.Sum256(data) != m.inspection.sourceHash {
		return fmt.Errorf("%w: migration configuration changed after validation", ErrMigrationStateChanged)
	}
	if err := recoverInterruptedMigration(m.paths); err != nil {
		return err
	}
	planned, err := planMigrationReadOnly(m.paths)
	if err != nil {
		return err
	}
	if planned == nil {
		return nil
	}
	currentData, currentID, err := readLegacyIdentity(m.paths.Legacy)
	if err != nil {
		return err
	}
	if currentID != current.legacyID || sha256.Sum256(currentData) != current.sourceHash {
		return fmt.Errorf("%w: legacy configuration identity or digest changed after recovery", ErrMigrationStateChanged)
	}
	return m.applyLocked(currentData, currentID)
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
		if err := removeOwnedRegularIfPresentWithDigest(tx.paths.Canonical, tx.record.CanonicalID, transactionDigest(tx.record), tx.record.Token); err != nil {
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
	return removeOwnedRegularIfPresentWithExpectedDigest(path, expected, nil, token)
}

func removeOwnedRegularIfPresentWithDigest(path string, expected fileIdentity, digest [sha256.Size]byte, token string) error {
	return removeOwnedRegularIfPresentWithExpectedDigest(path, expected, &digest, token)
}

func removeOwnedRegularIfPresentWithExpectedDigest(path string, expected fileIdentity, digest *[sha256.Size]byte, token string) error {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return removeOwnedRegularWithExpectedDigest(path, expected, digest, token)
}

func removeOwnedRegular(path string, expected fileIdentity, token string) error {
	return removeOwnedRegularWithExpectedDigest(path, expected, nil, token)
}

func removeOwnedRegularWithExpectedDigest(path string, expected fileIdentity, digest *[sha256.Size]byte, token string) error {
	if err := requireOwnedRegular(path, expected); err != nil {
		return err
	}
	if err := requireOwnedRegularDigest(path, digest); err != nil {
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
	if err := requireOwnedRegularDigest(quarantine, digest); err != nil {
		restoreErr := restoreQuarantinedRegular(quarantine, path)
		return errors.Join(fmt.Errorf("quarantine content conflict: %w", err), restoreErr)
	}
	removeErr := migrationOS.remove(quarantine)
	syncErr := syncDirectory(filepath.Dir(quarantine))
	return errors.Join(removeErr, syncErr)
}

func requireOwnedRegularDigest(path string, expected *[sha256.Size]byte) error {
	if expected == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if actual := sha256.Sum256(data); actual != *expected {
		return fmt.Errorf("refuse to modify file with unexpected content at %s", path)
	}
	return nil
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
	runtime.KeepAlive(sourcePtr)
	runtime.KeepAlive(destinationPtr)
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

func transactionDigest(record transactionRecord) [sha256.Size]byte {
	decoded, _ := hex.DecodeString(record.Digest)
	var digest [sha256.Size]byte
	copy(digest[:], decoded)
	return digest
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
	if handled, err := recoverStagedMarkerConflictIntent(marker); handled {
		return err
	}
	if handled, err := recoverStagedMarkerConflict(marker); handled {
		return err
	}
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
	if err := restoreAnchoredTargetQuarantines(marker, target, token); err != nil {
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
		if migrationOS.afterStagedMarkerRead != nil {
			migrationOS.afterStagedMarkerRead(target)
		}
		if err := promoteCompletedStagedMarker(marker, anchor, target, token, targetID); err != nil {
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

func restoreAnchoredTargetQuarantines(marker, target, token string) error {
	for _, quarantine := range []string{
		target + ".oma-quarantine-" + token,
		stagedMarkerPromotionPath(marker, token),
	} {
		if err := restoreAnchoredTargetQuarantine(quarantine, target); err != nil {
			return err
		}
	}
	return nil
}

func restoreAnchoredTargetQuarantine(quarantine, target string) error {
	if _, err := os.Lstat(quarantine); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("staged marker target and quarantine both exist: %w", fs.ErrExist)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := renameNoReplace(quarantine, target); err != nil {
		return fmt.Errorf("restore staged marker target quarantine: %w", err)
	}
	return syncDirectory(filepath.Dir(target))
}

func promoteCompletedStagedMarker(marker, anchor, target, token string, targetID fileIdentity) error {
	quarantine := stagedMarkerPromotionPath(marker, token)
	if err := renameNoReplace(target, quarantine); err != nil {
		return fmt.Errorf("isolate staged marker for promotion: %w", err)
	}
	if migrationOS.afterQuarantineMove != nil {
		if err := migrationOS.afterQuarantineMove(target, quarantine); err != nil {
			return fmt.Errorf("after staged marker promotion quarantine move: %w", err)
		}
	}
	if err := syncMarkerDirectory(filepath.Dir(quarantine)); err != nil {
		return fmt.Errorf("sync staged marker promotion quarantine: %w", err)
	}

	info, err := os.Lstat(quarantine)
	if err != nil {
		return fmt.Errorf("inspect staged marker promotion quarantine: %w", err)
	}
	owned := false
	if info.Mode().IsRegular() {
		owned, err = hasIdentity(info, targetID)
		if err != nil {
			return err
		}
	}
	if !owned {
		conflict := fmt.Errorf("staged marker changed after read: unexpected identity or type")
		conflictAnchor := stagedMarkerConflictPath(marker, token)
		intent := stagedMarkerConflictIntentPath(marker, token)
		intentID, err := createStagedMarkerConflictIntent(intent, anchor, target, token)
		if err != nil {
			return errors.Join(conflict, fmt.Errorf("establish staged marker conflict intent without replacement: %w", err))
		}
		if err := transitionActiveAnchorToConflict(anchor, conflictAnchor, target, token); err != nil {
			return errors.Join(conflict, err)
		}
		if migrationOS.afterConflictAnchorMove != nil {
			migrationOS.afterConflictAnchorMove()
		}
		restoreErr := restoreStagedMarkerPromotion(quarantine, target)
		if restoreErr != nil {
			return errors.Join(conflict, restoreErr)
		}
		if migrationOS.afterConflictRestore != nil {
			migrationOS.afterConflictRestore()
		}
		if err := removeStagedMarkerConflictIntent(intent, intentID, token); err != nil {
			return errors.Join(conflict, err)
		}
		return errors.Join(conflict, removeStagedMarkerConflictAnchor(conflictAnchor, target, token))
	}
	if err := renameNoReplace(quarantine, marker); err != nil {
		return fmt.Errorf("commit verified staged migration marker: %w", err)
	}
	if err := syncMarkerDirectory(filepath.Dir(marker)); err != nil {
		return fmt.Errorf("sync verified staged migration marker: %w", err)
	}
	return nil
}

func restoreStagedMarkerPromotion(quarantine, target string) error {
	if err := renameNoReplace(quarantine, target); err != nil {
		return fmt.Errorf("restore staged marker replacement without overwrite: %w", err)
	}
	return syncDirectory(filepath.Dir(target))
}

func stagedMarkerPromotionPath(marker, token string) string {
	return marker + ".staged-quarantine-" + token
}

func createStagedMarkerConflictIntent(path, anchor, target, token string) (fileIdentity, error) {
	anchorInfo, err := os.Lstat(anchor)
	if err != nil {
		return fileIdentity{}, err
	}
	anchorID, err := identityOf(anchorInfo)
	if err != nil {
		return fileIdentity{}, err
	}
	nonce, err := newTransactionToken()
	if err != nil {
		return fileIdentity{}, err
	}
	record := stagedMarkerConflictIntent{Version: 1, Token: token, Target: target, AnchorID: anchorID, Nonce: nonce}
	data, err := json.Marshal(record)
	if err != nil {
		return fileIdentity{}, err
	}
	staged := stagedMarkerConflictIntentDraftPath(path, nonce)
	draftAnchor := stagedMarkerConflictIntentDraftAnchorPath(path)
	if err := os.Symlink(staged, draftAnchor); err != nil {
		return fileIdentity{}, err
	}
	if err := syncDirectory(filepath.Dir(draftAnchor)); err != nil {
		return fileIdentity{}, err
	}
	if migrationOS.afterConflictIntentDraftAnchorSync != nil {
		migrationOS.afterConflictIntentDraftAnchorSync()
	}
	file, err := os.OpenFile(staged, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fileIdentity{}, err
	}
	defer func() { _ = file.Close() }()
	if migrationOS.afterConflictIntentOpen != nil {
		migrationOS.afterConflictIntentOpen()
	}
	midpoint := len(data) / 2
	if _, err := file.Write(data[:midpoint]); err != nil {
		return fileIdentity{}, err
	}
	if migrationOS.afterConflictIntentPart != nil {
		migrationOS.afterConflictIntentPart()
	}
	if _, err := file.Write(data[midpoint:]); err != nil {
		return fileIdentity{}, err
	}
	if err := file.Sync(); err != nil {
		return fileIdentity{}, err
	}
	if migrationOS.afterConflictIntentFileSync != nil {
		migrationOS.afterConflictIntentFileSync()
	}
	if err := file.Close(); err != nil {
		return fileIdentity{}, err
	}
	if err := renameNoReplace(staged, path); err != nil {
		return fileIdentity{}, err
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return fileIdentity{}, err
	}
	if migrationOS.afterConflictIntentSync != nil {
		migrationOS.afterConflictIntentSync()
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fileIdentity{}, err
	}
	intentID, err := identityOf(info)
	if err != nil {
		return fileIdentity{}, err
	}
	if err := removeStagedMarkerConflictIntentDraftAnchor(draftAnchor, staged, token); err != nil {
		return fileIdentity{}, err
	}
	return intentID, nil
}

func stagedMarkerConflictIntentDraftPath(intent, nonce string) string {
	return intent + ".oma-staged-" + nonce
}

func stagedMarkerConflictIntentDraftAnchorPath(intent string) string {
	return intent + ".oma-draft-anchor"
}

func removeStagedMarkerConflictIntentDraftAnchor(anchor, expectedTarget, token string) error {
	if err := requireSymlinkTarget(anchor, expectedTarget); err != nil {
		return err
	}
	quarantine := anchor + ".oma-quarantine-" + token
	if err := renameNoReplace(anchor, quarantine); err != nil {
		return fmt.Errorf("quarantine conflict intent draft anchor: %w", err)
	}
	if migrationOS.afterConflictIntentDraftAnchorMove != nil {
		migrationOS.afterConflictIntentDraftAnchorMove()
	}
	if err := syncConflictIntentDraftAnchorDirectory(filepath.Dir(anchor)); err != nil {
		return err
	}
	if err := requireSymlinkTarget(quarantine, expectedTarget); err != nil {
		return err
	}
	removeErr := migrationOS.remove(quarantine)
	syncErr := syncConflictIntentDraftAnchorDirectory(filepath.Dir(anchor))
	return errors.Join(removeErr, syncErr)
}

func syncConflictIntentDraftAnchorDirectory(path string) error {
	if migrationOS.conflictIntentDraftAnchorSync != nil {
		return migrationOS.conflictIntentDraftAnchorSync(path)
	}
	return syncDirectory(path)
}

func recoverStagedMarkerConflictIntentDraftAnchorCleanup(marker string) (bool, error) {
	matches, err := filepath.Glob(marker + ".staged-conflict-intent-*.oma-draft-anchor.oma-quarantine-*")
	if err != nil || len(matches) == 0 {
		return false, err
	}
	if len(matches) != 1 {
		return true, fmt.Errorf("multiple conflict intent draft anchor cleanup quarantines are present")
	}
	quarantine := matches[0]
	separator := strings.LastIndex(quarantine, ".oma-quarantine-")
	anchor := quarantine[:separator]
	cleanupToken := quarantine[separator+len(".oma-quarantine-"):]
	intent := strings.TrimSuffix(anchor, ".oma-draft-anchor")
	prefix := marker + ".staged-conflict-intent-"
	token := strings.TrimPrefix(intent, prefix)
	decoded, decodeErr := hex.DecodeString(token)
	if !strings.HasPrefix(intent, prefix) || cleanupToken != token || decodeErr != nil || len(decoded) != 16 {
		return true, fmt.Errorf("conflict intent draft anchor cleanup token is invalid")
	}
	if _, err := os.Lstat(anchor); err == nil {
		return true, fmt.Errorf("conflict intent draft anchor and cleanup quarantine both exist: %w", fs.ErrExist)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return true, err
	}
	target, err := os.Readlink(quarantine)
	if err != nil {
		return true, fmt.Errorf("conflict intent draft anchor cleanup quarantine is not a symlink: %w", err)
	}
	draftPrefix := intent + ".oma-staged-"
	nonce := strings.TrimPrefix(target, draftPrefix)
	decodedNonce, nonceErr := hex.DecodeString(nonce)
	if !strings.HasPrefix(target, draftPrefix) || nonceErr != nil || len(decodedNonce) != 16 {
		return true, fmt.Errorf("conflict intent draft anchor cleanup target is invalid")
	}
	if err := renameNoReplace(quarantine, anchor); err != nil {
		return true, fmt.Errorf("restore conflict intent draft anchor cleanup quarantine: %w", err)
	}
	if err := syncDirectory(filepath.Dir(anchor)); err != nil {
		return true, err
	}
	return true, nil
}

func recoverStagedMarkerConflictIntentDraftAnchor(marker string) (bool, error) {
	matches, err := filepath.Glob(marker + ".staged-conflict-intent-*.oma-draft-anchor")
	if err != nil || len(matches) == 0 {
		return false, err
	}
	if len(matches) != 1 {
		return true, fmt.Errorf("multiple staged marker conflict intent draft anchors are present")
	}
	draftAnchor := matches[0]
	info, err := os.Lstat(draftAnchor)
	if err != nil {
		return true, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return true, fmt.Errorf("staged marker conflict intent draft anchor is not a symlink")
	}
	draft, err := os.Readlink(draftAnchor)
	if err != nil {
		return true, err
	}
	intent := strings.TrimSuffix(draftAnchor, ".oma-draft-anchor")
	prefix := marker + ".staged-conflict-intent-"
	token := strings.TrimPrefix(intent, prefix)
	draftPrefix := intent + ".oma-staged-"
	nonce := strings.TrimPrefix(draft, draftPrefix)
	decodedToken, tokenErr := hex.DecodeString(token)
	decodedNonce, nonceErr := hex.DecodeString(nonce)
	if !strings.HasPrefix(intent, prefix) || !strings.HasPrefix(draft, draftPrefix) || tokenErr != nil || len(decodedToken) != 16 || nonceErr != nil || len(decodedNonce) != 16 {
		return true, fmt.Errorf("staged marker conflict intent draft anchor target is invalid")
	}
	target := marker + ".staged-" + token
	active := stagedMarkerAnchorPath(marker)
	activeQuarantine := active + ".oma-quarantine-" + token
	conflict := stagedMarkerConflictPath(marker, token)
	conflictQuarantine := conflict + ".oma-quarantine-" + token
	anchorID, err := identityAtAny(active, activeQuarantine, conflict, conflictQuarantine)
	if err != nil {
		return true, err
	}
	if fixedInfo, err := os.Lstat(intent); err == nil {
		if _, draftErr := os.Lstat(draft); draftErr == nil {
			return true, fmt.Errorf("fixed conflict intent and its draft both exist: %w", fs.ErrExist)
		} else if !errors.Is(draftErr, fs.ErrNotExist) {
			return true, draftErr
		}
		if !fixedInfo.Mode().IsRegular() || fixedInfo.Mode().Perm() != 0o600 {
			return true, fmt.Errorf("fixed conflict intent has unexpected type or mode")
		}
		data, err := os.ReadFile(intent)
		if err != nil {
			return true, err
		}
		var record stagedMarkerConflictIntent
		if err := json.Unmarshal(data, &record); err != nil || record.Version != 1 || record.Token != token || record.Nonce != nonce || record.Target != target || record.AnchorID != anchorID {
			return true, fmt.Errorf("fixed conflict intent does not match its draft anchor")
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		if draftInfo, draftErr := os.Lstat(draft); draftErr == nil {
			if !draftInfo.Mode().IsRegular() || draftInfo.Mode().Perm() != 0o600 {
				return true, fmt.Errorf("staged conflict intent draft has unexpected type or mode")
			}
			if err := os.Remove(draft); err != nil {
				return true, err
			}
			if err := syncDirectory(filepath.Dir(draft)); err != nil {
				return true, err
			}
		} else if !errors.Is(draftErr, fs.ErrNotExist) {
			return true, draftErr
		}
		record := stagedMarkerConflictIntent{Version: 1, Token: token, Target: target, AnchorID: anchorID, Nonce: nonce}
		data, err := json.Marshal(record)
		if err != nil {
			return true, err
		}
		file, err := os.OpenFile(draft, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return true, err
		}
		if _, err := file.Write(data); err != nil {
			_ = file.Close()
			return true, err
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return true, err
		}
		if err := file.Close(); err != nil {
			return true, err
		}
		if err := renameNoReplace(draft, intent); err != nil {
			return true, err
		}
		if err := syncDirectory(filepath.Dir(intent)); err != nil {
			return true, err
		}
	} else {
		return true, err
	}
	if err := removeStagedMarkerConflictIntentDraftAnchor(draftAnchor, draft, token); err != nil {
		return true, err
	}
	return true, nil
}

func identityAtAny(paths ...string) (fileIdentity, error) {
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err == nil {
			return identityOf(info)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return fileIdentity{}, err
		}
	}
	return fileIdentity{}, fmt.Errorf("staged marker conflict intent draft anchor has no active anchor")
}

func recoverStagedMarkerConflictIntentDraft(marker string) (bool, error) {
	matches, err := filepath.Glob(marker + ".staged-conflict-intent-*.oma-staged-*")
	if err != nil || len(matches) == 0 {
		return false, err
	}
	if len(matches) != 1 {
		return true, fmt.Errorf("multiple staged marker conflict intent drafts are present")
	}
	draft := matches[0]
	separator := strings.LastIndex(draft, ".oma-staged-")
	if separator < 0 {
		return true, fmt.Errorf("staged marker conflict intent draft path is invalid")
	}
	intent := draft[:separator]
	nonce := draft[separator+len(".oma-staged-"):]
	prefix := marker + ".staged-conflict-intent-"
	token := strings.TrimPrefix(intent, prefix)
	decodedToken, tokenErr := hex.DecodeString(token)
	decodedNonce, nonceErr := hex.DecodeString(nonce)
	if !strings.HasPrefix(intent, prefix) || tokenErr != nil || len(decodedToken) != 16 || nonceErr != nil || len(decodedNonce) != 16 {
		return true, fmt.Errorf("staged marker conflict intent draft path is invalid")
	}
	info, err := os.Lstat(draft)
	if err != nil {
		return true, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return true, fmt.Errorf("staged marker conflict intent draft has unexpected type or mode")
	}
	data, err := os.ReadFile(draft)
	if err != nil {
		return true, err
	}
	var record stagedMarkerConflictIntent
	if err := json.Unmarshal(data, &record); err != nil {
		return true, fmt.Errorf("staged marker conflict intent draft is incomplete: %w", err)
	}
	if record.Version != 1 || record.Token != token || record.Nonce != nonce || record.Target == "" {
		return true, fmt.Errorf("staged marker conflict intent draft record is invalid")
	}
	active := stagedMarkerAnchorPath(marker)
	activeQuarantine := active + ".oma-quarantine-" + token
	conflict := stagedMarkerConflictPath(marker, token)
	conflictQuarantine := conflict + ".oma-quarantine-" + token
	if err := requireIdentityAtAny(record.AnchorID, active, activeQuarantine, conflict, conflictQuarantine); err != nil {
		return true, err
	}
	if err := renameNoReplace(draft, intent); err != nil {
		return true, fmt.Errorf("commit staged marker conflict intent draft: %w", err)
	}
	if err := syncDirectory(filepath.Dir(intent)); err != nil {
		return true, err
	}
	return true, nil
}

func recoverStagedMarkerConflictIntent(marker string) (bool, error) {
	if handled, err := recoverStagedMarkerConflictIntentDraftAnchorCleanup(marker); handled || err != nil {
		if err != nil {
			return true, err
		}
	}
	if handled, err := recoverStagedMarkerConflictIntentDraftAnchor(marker); handled || err != nil {
		if err != nil {
			return true, err
		}
	}
	if handled, err := recoverStagedMarkerConflictIntentDraft(marker); handled || err != nil {
		if err != nil {
			return true, err
		}
	}
	intent, quarantine, token, found, err := findStagedMarkerConflictIntent(marker)
	if err != nil || !found {
		return found, err
	}
	intentPath := intent
	if quarantine != "" {
		if _, err := os.Lstat(intent); err == nil {
			return true, fmt.Errorf("staged marker conflict intent and quarantine both exist: %w", fs.ErrExist)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return true, err
		}
		intentPath = quarantine
	}

	info, err := os.Lstat(intentPath)
	if err != nil {
		return true, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return true, fmt.Errorf("staged marker conflict intent has unexpected type or mode: %s", intentPath)
	}
	intentID, err := identityOf(info)
	if err != nil {
		return true, err
	}
	data, err := os.ReadFile(intentPath)
	if err != nil {
		return true, err
	}
	var record stagedMarkerConflictIntent
	if err := json.Unmarshal(data, &record); err != nil {
		return true, fmt.Errorf("staged marker conflict intent is invalid: %w", err)
	}
	target := record.Target
	anchorToken, err := stagedMarkerToken(marker, target)
	decodedNonce, nonceErr := hex.DecodeString(record.Nonce)
	if err != nil || anchorToken != token || intent != stagedMarkerConflictIntentPath(marker, token) || record.Version != 1 || record.Token != token || nonceErr != nil || len(decodedNonce) != 16 {
		return true, fmt.Errorf("staged marker conflict intent record is invalid")
	}

	active := stagedMarkerAnchorPath(marker)
	activeQuarantine := active + ".oma-quarantine-" + token
	conflict := stagedMarkerConflictPath(marker, token)
	conflictQuarantine := conflict + ".oma-quarantine-" + token
	if err := requireIdentityAtAny(record.AnchorID, active, activeQuarantine, conflict, conflictQuarantine); err != nil {
		return true, err
	}
	if quarantine != "" {
		if err := renameNoReplace(quarantine, intent); err != nil {
			return true, fmt.Errorf("restore staged marker conflict intent: %w", err)
		}
		if err := syncDirectory(filepath.Dir(intent)); err != nil {
			return true, err
		}
	}
	if err := restoreConflictSymlinkQuarantine(conflict, conflictQuarantine, target); err != nil {
		return true, err
	}
	activeExists, err := pathExists(active)
	if err != nil {
		return true, err
	}
	activeQuarantineExists, err := pathExists(activeQuarantine)
	if err != nil {
		return true, err
	}
	conflictExists, err := pathExists(conflict)
	if err != nil {
		return true, err
	}
	if conflictExists {
		if err := requireSymlinkTarget(conflict, target); err != nil {
			return true, err
		}
		if activeExists || activeQuarantineExists {
			return true, fmt.Errorf("active and conflict staged marker anchors coexist: %w", fs.ErrExist)
		}
	} else {
		if activeExists && activeQuarantineExists {
			return true, fmt.Errorf("active staged marker anchor and quarantine coexist: %w", fs.ErrExist)
		}
		if activeExists {
			if err := transitionActiveAnchorToConflict(active, conflict, target, token); err != nil {
				return true, err
			}
			conflictExists = true
		} else if activeQuarantineExists {
			if err := finishActiveAnchorConflictTransition(active, activeQuarantine, conflict, target); err != nil {
				return true, err
			}
			conflictExists = true
		}
	}

	promotion := stagedMarkerPromotionPath(marker, token)
	targetExists, err := pathExists(target)
	if err != nil {
		return true, err
	}
	promotionExists, err := pathExists(promotion)
	if err != nil {
		return true, err
	}
	if targetExists && promotionExists {
		return true, fmt.Errorf("external staged marker replacement and quarantine both exist: %w", fs.ErrExist)
	}
	if !targetExists && promotionExists {
		if err := restoreStagedMarkerPromotion(promotion, target); err != nil {
			return true, err
		}
		targetExists = true
	}
	conflictErr := errors.New("external staged marker replacement is preserved")
	if !targetExists {
		return true, errors.Join(conflictErr, errors.New("replacement evidence is missing"))
	}
	if conflictExists {
		if err := removeStagedMarkerConflictIntent(intent, intentID, token); err != nil {
			return true, errors.Join(conflictErr, err)
		}
	}
	return true, errors.Join(conflictErr, removeStagedMarkerConflictAnchor(conflict, target, token))
}

func findStagedMarkerConflictIntent(marker string) (intent, quarantine, token string, found bool, err error) {
	matches, err := filepath.Glob(marker + ".staged-conflict-intent-*")
	if err != nil || len(matches) == 0 {
		return "", "", "", false, err
	}
	filtered := matches[:0]
	for _, match := range matches {
		basename := filepath.Base(match)
		artifactPrefix := filepath.Base(marker) + ".staged-conflict-intent-"
		artifactSuffix := strings.TrimPrefix(basename, artifactPrefix)
		if !strings.HasPrefix(basename, artifactPrefix) {
			return "", "", "", true, fmt.Errorf("unexpected staged marker conflict intent basename: %s", basename)
		}
		if !strings.Contains(artifactSuffix, ".oma-draft-anchor") && !strings.Contains(artifactSuffix, ".oma-staged-") {
			filtered = append(filtered, match)
		}
	}
	matches = filtered
	if len(matches) == 0 {
		return "", "", "", false, nil
	}
	for _, match := range matches {
		base, matchToken, quarantined, err := parseStagedMarkerConflictIntentPath(marker, match)
		if err != nil {
			return "", "", "", true, err
		}
		if token != "" && token != matchToken {
			return "", "", "", true, fmt.Errorf("multiple staged marker conflict intent tokens are present")
		}
		token = matchToken
		intent = base
		if quarantined {
			if quarantine != "" {
				return "", "", "", true, fmt.Errorf("multiple staged marker conflict intent quarantines are present")
			}
			quarantine = match
		}
	}
	return intent, quarantine, token, true, nil
}

func parseStagedMarkerConflictIntentPath(marker, path string) (base, token string, quarantined bool, err error) {
	prefix := marker + ".staged-conflict-intent-"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false, fmt.Errorf("unexpected staged marker conflict intent path: %s", path)
	}
	suffix := strings.TrimPrefix(path, prefix)
	if index := strings.Index(suffix, ".oma-quarantine-"); index >= 0 {
		token = suffix[:index]
		if suffix[index+len(".oma-quarantine-"):] != token {
			return "", "", false, fmt.Errorf("staged marker conflict intent quarantine token mismatch: %s", path)
		}
		quarantined = true
	} else {
		token = suffix
	}
	decoded, decodeErr := hex.DecodeString(token)
	if decodeErr != nil || len(decoded) != 16 {
		return "", "", false, fmt.Errorf("staged marker conflict intent token is invalid: %s", path)
	}
	return prefix + token, token, quarantined, nil
}

func transitionActiveAnchorToConflict(active, conflict, target, token string) error {
	quarantine := active + ".oma-quarantine-" + token
	if err := requireSymlinkTarget(active, target); err != nil {
		return err
	}
	if err := renameNoReplace(active, quarantine); err != nil {
		return fmt.Errorf("quarantine active staged marker anchor: %w", err)
	}
	if migrationOS.afterConflictActiveMove != nil {
		migrationOS.afterConflictActiveMove(active, quarantine)
	}
	if err := syncDirectory(filepath.Dir(active)); err != nil {
		return fmt.Errorf("sync active staged marker anchor quarantine: %w", err)
	}
	return finishActiveAnchorConflictTransition(active, quarantine, conflict, target)
}

func finishActiveAnchorConflictTransition(active, quarantine, conflict, target string) error {
	if err := requireSymlinkTarget(quarantine, target); err != nil {
		return err
	}
	if err := renameNoReplace(quarantine, conflict); err != nil {
		restoreErr := renameNoReplace(quarantine, active)
		if restoreErr == nil {
			restoreErr = syncDirectory(filepath.Dir(active))
		}
		return errors.Join(fmt.Errorf("commit staged marker conflict anchor: %w", err), restoreErr)
	}
	if err := syncDirectory(filepath.Dir(conflict)); err != nil {
		return fmt.Errorf("sync staged marker conflict anchor: %w", err)
	}
	return nil
}

func restoreConflictSymlinkQuarantine(path, quarantine, target string) error {
	quarantineExists, err := pathExists(quarantine)
	if err != nil || !quarantineExists {
		return err
	}
	pathExists, err := pathExists(path)
	if err != nil {
		return err
	}
	if pathExists {
		return fmt.Errorf("staged marker conflict symlink and quarantine coexist: %w", fs.ErrExist)
	}
	if err := requireSymlinkTarget(quarantine, target); err != nil {
		return err
	}
	if err := renameNoReplace(quarantine, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func requireSymlinkTarget(path, expected string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("expected symlink at %s", path)
	}
	target, err := os.Readlink(path)
	if err != nil {
		return err
	}
	if target != expected {
		return fmt.Errorf("symlink at %s has unexpected target %s", path, target)
	}
	return nil
}

func requireIdentityAtAny(expected fileIdentity, candidates ...string) error {
	for _, candidate := range candidates {
		candidateInfo, err := os.Lstat(candidate)
		if err == nil {
			owned, err := hasIdentity(candidateInfo, expected)
			if err != nil {
				return err
			}
			if owned {
				return nil
			}
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return fmt.Errorf("staged marker conflict intent does not identify an active anchor")
}

func stagedMarkerConflictIntentPath(marker, token string) string {
	return marker + ".staged-conflict-intent-" + token
}

func recoverStagedMarkerConflict(marker string) (bool, error) {
	matches, err := filepath.Glob(marker + ".staged-conflict-*")
	if err != nil {
		return true, err
	}
	filtered := matches[:0]
	for _, match := range matches {
		if !strings.HasPrefix(match, marker+".staged-conflict-intent-") {
			filtered = append(filtered, match)
		}
	}
	matches = filtered
	if len(matches) == 0 {
		return false, nil
	}
	var conflict, quarantine, token string
	for _, match := range matches {
		base, matchToken, quarantined, err := parseStagedMarkerConflictPath(marker, match)
		if err != nil {
			return true, err
		}
		if token != "" && token != matchToken {
			return true, fmt.Errorf("multiple staged marker conflict tokens are present")
		}
		token = matchToken
		if quarantined {
			if quarantine != "" {
				return true, fmt.Errorf("multiple staged marker conflict anchor quarantines are present")
			}
			quarantine = match
			conflict = base
			continue
		}
		if conflict != "" && conflict != match {
			return true, fmt.Errorf("multiple staged marker conflict anchors are present")
		}
		conflict = match
	}
	if quarantine != "" {
		if _, err := os.Lstat(conflict); err == nil {
			return true, fmt.Errorf("staged marker conflict anchor and quarantine both exist: %w", fs.ErrExist)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return true, err
		}
		target, err := os.Readlink(quarantine)
		if err != nil {
			return true, fmt.Errorf("quarantined staged marker conflict anchor is not a symlink: %w", err)
		}
		anchorToken, err := stagedMarkerToken(marker, target)
		if err != nil || anchorToken != token {
			return true, fmt.Errorf("quarantined staged marker conflict anchor target is invalid")
		}
		if err := renameNoReplace(quarantine, conflict); err != nil {
			return true, fmt.Errorf("restore staged marker conflict anchor: %w", err)
		}
		if err := syncDirectory(filepath.Dir(conflict)); err != nil {
			return true, err
		}
	}

	info, err := os.Lstat(conflict)
	if err != nil {
		return true, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return true, fmt.Errorf("staged marker conflict anchor is not a symlink: %s", conflict)
	}
	target, err := os.Readlink(conflict)
	if err != nil {
		return true, err
	}
	anchorToken, err := stagedMarkerToken(marker, target)
	if err != nil || anchorToken != token || conflict != stagedMarkerConflictPath(marker, token) {
		return true, fmt.Errorf("staged marker conflict anchor target or token is invalid")
	}
	promotion := stagedMarkerPromotionPath(marker, token)
	targetExists, err := pathExists(target)
	if err != nil {
		return true, err
	}
	promotionExists, err := pathExists(promotion)
	if err != nil {
		return true, err
	}
	if targetExists && promotionExists {
		return true, fmt.Errorf("external staged marker replacement and quarantine both exist: %w", fs.ErrExist)
	}
	if !targetExists && promotionExists {
		if err := restoreStagedMarkerPromotion(promotion, target); err != nil {
			return true, err
		}
		targetExists = true
	}
	conflictErr := errors.New("external staged marker replacement is preserved")
	if !targetExists {
		return true, errors.Join(conflictErr, errors.New("replacement evidence is missing"))
	}
	return true, errors.Join(conflictErr, removeStagedMarkerConflictAnchor(conflict, target, token))
}

func parseStagedMarkerConflictPath(marker, path string) (base, token string, quarantined bool, err error) {
	prefix := marker + ".staged-conflict-"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false, fmt.Errorf("unexpected staged marker conflict path: %s", path)
	}
	suffix := strings.TrimPrefix(path, prefix)
	if index := strings.Index(suffix, ".oma-quarantine-"); index >= 0 {
		token = suffix[:index]
		if suffix[index+len(".oma-quarantine-"):] != token {
			return "", "", false, fmt.Errorf("staged marker conflict quarantine token mismatch: %s", path)
		}
		quarantined = true
	} else {
		token = suffix
	}
	decoded, decodeErr := hex.DecodeString(token)
	if decodeErr != nil || len(decoded) != 16 {
		return "", "", false, fmt.Errorf("staged marker conflict token is invalid: %s", path)
	}
	return prefix + token, token, quarantined, nil
}

func removeStagedMarkerConflictAnchor(path, expectedTarget, token string) error {
	return removeStagedMarkerConflictAnchorWithIdentity(path, expectedTarget, token, nil)
}

func removeStagedMarkerConflictIntent(path string, expected fileIdentity, token string) error {
	return removeOwnedRegular(path, expected, token)
}

func removeStagedMarkerConflictAnchorWithIdentity(path, expectedTarget, token string, expectedIdentity fs.FileInfo) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if expectedIdentity != nil && !os.SameFile(info, expectedIdentity) {
		return fmt.Errorf("staged marker conflict anchor does not have the expected identity: %s", path)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("staged marker conflict anchor is not a symlink: %s", path)
	}
	target, err := os.Readlink(path)
	if err != nil {
		return err
	}
	if target != expectedTarget {
		return fmt.Errorf("staged marker conflict anchor has unexpected target: %s", target)
	}
	quarantine := path + ".oma-quarantine-" + token
	if err := renameNoReplace(path, quarantine); err != nil {
		return fmt.Errorf("quarantine staged marker conflict anchor: %w", err)
	}
	if err := syncConflictDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	quarantineInfo, err := os.Lstat(quarantine)
	if err != nil {
		return err
	}
	if !os.SameFile(info, quarantineInfo) || expectedIdentity != nil && !os.SameFile(quarantineInfo, expectedIdentity) {
		restoreErr := renameNoReplace(quarantine, path)
		if restoreErr == nil {
			restoreErr = syncConflictDirectory(filepath.Dir(path))
		}
		return errors.Join(fmt.Errorf("staged marker conflict anchor identity changed during quarantine: %s", path), restoreErr)
	}
	actualTarget, err := os.Readlink(quarantine)
	if err != nil || actualTarget != expectedTarget {
		return fmt.Errorf("staged marker conflict anchor quarantine has unexpected target: %q: %w", actualTarget, err)
	}
	removeErr := migrationOS.remove(quarantine)
	syncErr := syncConflictDirectory(filepath.Dir(quarantine))
	return errors.Join(removeErr, syncErr)
}

func syncConflictDirectory(path string) error {
	if migrationOS.conflictDirectorySync != nil {
		return migrationOS.conflictDirectorySync(path)
	}
	return syncDirectory(path)
}

func stagedMarkerConflictPath(marker, token string) string {
	return marker + ".staged-conflict-" + token
}

func pathExists(path string) (bool, error) {
	if _, err := os.Lstat(path); err == nil {
		return true, nil
	} else if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else {
		return false, err
	}
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
