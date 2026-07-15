package jira

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func WriteSnapshot(path string, raw []byte) (resultErr error) {
	if path == "" {
		return errors.New("snapshot path is required")
	}
	path = filepath.Clean(path)
	parent := filepath.Dir(path)
	if err := ensurePrivateDirectory(parent); err != nil {
		return err
	}
	if err := validateSnapshotTarget(path); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(parent, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create snapshot temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		if resultErr != nil {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("set snapshot temporary file mode: %w", err)
	}
	if _, err := temporary.Write(raw); err != nil {
		return fmt.Errorf("write snapshot temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync snapshot temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close snapshot temporary file: %w", err)
	}
	if err := validateSnapshotTarget(path); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace snapshot: %w", err)
	}
	if err := syncDirectory(parent); err != nil {
		return err
	}
	return nil
}

func ensurePrivateDirectory(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("snapshot parent must be a real directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect snapshot parent: %w", err)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create snapshot parent: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("set snapshot parent mode: %w", err)
	}
	return nil
}

func validateSnapshotTarget(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect snapshot target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("snapshot target must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return errors.New("snapshot target must be a regular file")
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open snapshot parent for sync: %w", err)
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return fmt.Errorf("sync snapshot parent: %w", err)
	}
	if err := directory.Close(); err != nil {
		return fmt.Errorf("close snapshot parent: %w", err)
	}
	return nil
}
