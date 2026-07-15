package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/pelletier/go-toml/v2"
)

type Source string

const (
	SourceCanonical Source = "canonical"
	SourceLegacy    Source = "legacy"
)

type Config struct {
	JiraBaseURL        string            `toml:"jira_base_url"`
	DefaultProject     string            `toml:"default_project"`
	ProductTypeField   string            `toml:"product_type_field"`
	StartDateField     string            `toml:"start_date_field"`
	ProductTypeOptions map[string]string `toml:"product_type_options"`
}

func Load(paths Paths) (Config, Source, error) {
	canonicalInfo, canonicalErr := os.Lstat(paths.Canonical)
	legacyInfo, legacyErr := os.Lstat(paths.Legacy)
	if err := unexpectedStatError(paths.Canonical, canonicalErr); err != nil {
		return Config{}, "", err
	}
	if err := unexpectedStatError(paths.Legacy, legacyErr); err != nil {
		return Config{}, "", err
	}

	if canonicalErr == nil && legacyErr == nil && canonicalInfo.Mode().IsRegular() && legacyInfo.Mode().IsRegular() {
		canonicalStat, err := os.Stat(paths.Canonical)
		if err != nil {
			return Config{}, "", fmt.Errorf("stat canonical configuration: %w", err)
		}
		legacyStat, err := os.Stat(paths.Legacy)
		if err != nil {
			return Config{}, "", fmt.Errorf("stat legacy configuration: %w", err)
		}
		if !os.SameFile(canonicalStat, legacyStat) {
			return Config{}, "", configurationConflict(paths)
		}
	}

	path, source := paths.Canonical, SourceCanonical
	if canonicalErr != nil {
		path, source = paths.Legacy, SourceLegacy
		if legacyErr != nil {
			return Config{}, "", fmt.Errorf("configuration not found: %w", fs.ErrNotExist)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, "", fmt.Errorf("read %s configuration: %w", source, err)
	}
	config, err := decodeConfig(data)
	if err != nil {
		return Config{}, "", fmt.Errorf("decode %s configuration: %w", source, err)
	}
	return config, source, nil
}

func decodeConfig(data []byte) (Config, error) {
	var config Config
	if err := toml.Unmarshal(data, &config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func unexpectedStatError(path string, err error) error {
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("inspect configuration %s: %w", path, err)
}

func configurationConflict(paths Paths) error {
	return fmt.Errorf("configuration conflict: both %s and %s are different regular files", paths.Canonical, paths.Legacy)
}
