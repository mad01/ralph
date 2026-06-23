package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// DefaultConfigFileName is the expected name of the configuration file.
const DefaultConfigFileName = "config.toml"

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

	var cfg Config
	if _, err := toml.DecodeFile(configPath, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config file %s: %w", configPath, err)
	}

	// Overlay the optional, git-ignored config.local.toml (machine-local
	// overrides) before any validation or recipe processing, so the merged
	// result is validated as one config and recipe overrides set locally apply.
	if _, err := loadLocalOverlay(&cfg, configPath); err != nil {
		return nil, fmt.Errorf("failed to load local config overlay: %w", err)
	}

	// Validate the base config first
	if err := ValidateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Process recipes if configured
	currentHost := host
	if currentHost == "" {
		currentHost = GetCurrentHost()
	}

	if err := ProcessRecipes(&cfg, currentHost); err != nil {
		return nil, fmt.Errorf("recipe processing failed: %w", err)
	}

	// Validate the merged config (recipes may have added items)
	if err := ValidateMergedConfig(&cfg); err != nil {
		return nil, fmt.Errorf("merged configuration validation failed: %w", err)
	}

	return &cfg, nil
}

// getDefaultConfigPathInternal is the actual implementation for GetDefaultConfigPath.
func getDefaultConfigPathInternal() (string, error) {
	// RALPH_CONFIG takes precedence over the XDG path. If set, the file must
	// exist; we fail loudly rather than silently falling back to the default.
	if envPath := os.Getenv("RALPH_CONFIG"); envPath != "" {
		expanded, err := ExpandPath(envPath)
		if err != nil {
			return "", fmt.Errorf("RALPH_CONFIG=%s: %w", envPath, err)
		}
		if _, err := os.Stat(expanded); err != nil {
			return "", fmt.Errorf("RALPH_CONFIG=%s: %w", envPath, err)
		}
		return expanded, nil
	}

	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfigHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %w", err)
		}
		xdgConfigHome = filepath.Join(homeDir, ".config")
	}
	return filepath.Join(xdgConfigHome, "ralph", DefaultConfigFileName), nil
}
