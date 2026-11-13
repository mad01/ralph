package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/mad01/ralph/internal/paths"
)

// DefaultConfigFileName is the expected name of the configuration file.
const DefaultConfigFileName = paths.ConfigFileName

// GetDefaultConfigPath defines the function to get the default config path.
// This is a variable to allow for easier testing.
var GetDefaultConfigPath = getDefaultConfigPathInternal

// LoadConfig attempts to load the ralph configuration from the default location.
// Default location: $XDG_CONFIG_HOME/ralph/config.toml or ~/.config/ralph/config.toml.
func LoadConfig() (*Config, error) {
	return LoadConfigWithHost("")
}

// LoadConfigWithHost loads the ralph configuration with a specific host for filtering.
// If host is empty, it uses the current host.
func LoadConfigWithHost(host string) (*Config, error) {
	configPath, err := GetDefaultConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to determine config path: %w", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf(
			"configuration file not found at %s. Run 'ralph init' to create one",
			configPath,
		)
	}

	cfg, _, err := LoadUserConfig(configPath)
	if err != nil {
		return nil, err
	}

	// Validate the base config first
	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Process recipes if configured
	currentHost := host
	if currentHost == "" {
		currentHost = GetCurrentHost()
	}

	if err := ProcessRecipes(cfg, currentHost); err != nil {
		return nil, fmt.Errorf("recipe processing failed: %w", err)
	}

	// Validate the merged config (recipes may have added items)
	if err := ValidateMergedConfig(cfg); err != nil {
		return nil, fmt.Errorf("merged configuration validation failed: %w", err)
	}

	return cfg, nil
}

// LoadUserConfig decodes the main config file at mainPath and merges the
// optional, git-ignored config.local.toml overlay sitting next to it. Unlike
// LoadConfig it stops there — no validation, no recipe processing — so the
// result mirrors what the user's own files say. It reports whether an overlay
// file was found and merged.
func LoadUserConfig(mainPath string) (*Config, bool, error) {
	var cfg Config
	if _, err := toml.DecodeFile(mainPath, &cfg); err != nil {
		return nil, false, fmt.Errorf("failed to decode config file %s: %w", mainPath, err)
	}

	// Overlay the optional, git-ignored config.local.toml (machine-local
	// overrides) before any validation or recipe processing, so the merged
	// result is validated as one config and recipe overrides set locally apply.
	localPresent, err := loadLocalOverlay(&cfg, mainPath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to load local config overlay: %w", err)
	}
	return &cfg, localPresent, nil
}

// getDefaultConfigPathInternal is the actual implementation for GetDefaultConfigPath.
// paths.ConfigFile owns the lookup order (RALPH_CONFIG, then the XDG path).
func getDefaultConfigPathInternal() (string, error) {
	configPath, err := paths.ConfigFile()
	if err != nil {
		return "", err
	}
	// A RALPH_CONFIG file must exist: fail loudly rather than let a typo look
	// like a fresh machine. Only `ralph init`, which creates the file, skips
	// this check by calling paths.ConfigFile directly.
	if envPath := os.Getenv(paths.ConfigEnv); envPath != "" {
		if _, err := os.Stat(configPath); err != nil {
			return "", fmt.Errorf("%s=%s: %w", paths.ConfigEnv, envPath, err)
		}
	}
	return configPath, nil
}

// DefaultPackagesDir returns the directory remote packages clone into when
// packages_dir is unset: pkg/ in the state directory (paths.StateDir).
func DefaultPackagesDir() (string, error) {
	dir, err := paths.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pkg"), nil
}
