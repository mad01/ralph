package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Helper function to create a temporary config file for testing
func createTempConfigFile(t *testing.T, content string) (path string, cleanup func()) {
	t.Helper()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "dotter_test_config.toml")

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp config file: %v", err)
	}

	cleanup = func() {
		// os.RemoveAll(tempDir) // t.TempDir() handles cleanup
	}
	return filePath, cleanup
}

// Helper function to set XDG_CONFIG_HOME for testing GetDefaultConfigPath
func setXdgConfigHome(t *testing.T, value string) (originalValue string, wasSet bool, cleanup func()) {
	t.Helper()
	originalValue, wasSet = os.LookupEnv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", value)

	cleanup = func() {
		if wasSet {
			os.Setenv("XDG_CONFIG_HOME", originalValue)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}
	return
}

func TestLoadConfig_ValidConfig(t *testing.T) {
	validTomlContent := `
	dotfiles_repo_path = "~/.dotfiles"

	[dotfiles.bashrc]
	source = ".bashrc"
	target = "~/.bashrc"

	[shell.aliases.ll]
	command = "ls -alh"
	`
	tempCfgPath, cleanup := createTempConfigFile(t, validTomlContent)
	defer cleanup()

	// Temporarily override GetDefaultConfigPath to point to our temp file
	originalGetDefaultConfigPath := GetDefaultConfigPath // Save original
	GetDefaultConfigPath = func() (string, error) {      // Override
		return tempCfgPath, nil
	}
	defer func() { GetDefaultConfigPath = originalGetDefaultConfigPath }() // Restore

	expectedConfig := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Dotfiles: map[string]Dotfile{
			"bashrc": {Source: ".bashrc", Target: "~/.bashrc"},
		},
		Shell: ShellConfig{
			Aliases: map[string]ShellAlias{"ll": {Command: "ls -alh"}},
		},
		// Tools and TemplateVariables would be nil/empty if not in TOML
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() with valid config returned error: %v", err)
	}

	if !reflect.DeepEqual(cfg, expectedConfig) {
		t.Errorf("LoadConfig() got = %v, want %v", cfg, expectedConfig)
	}
}

func TestLoadConfig_WithHostsField(t *testing.T) {
	validTomlContent := `
	dotfiles_repo_path = "~/.dotfiles"

	[dotfiles.zshrc]
	source = ".zshrc"
	target = "~/.zshrc"
	hosts = ["work-laptop", "home-desktop"]

	[directories.workdir]
	target = "~/work"
	hosts = ["work-laptop"]

	[repos.tools]
	url = "https://github.com/example/tools.git"
	target = "~/tools"
	hosts = ["work-laptop"]

	[[tools]]
	name = "docker"
	check_command = "command -v docker"
	install_hint = "Install Docker"
	hosts = ["work-laptop"]

	[shell.aliases.vim]
	command = "nvim"
	hosts = ["home-desktop"]

	[shell.functions.work-setup]
	body = "echo setup"
	hosts = ["work-laptop"]

	[hooks.builds.work_build]
	commands = ["echo build"]
	run = "once"
	hosts = ["work-laptop"]
	`
	tempCfgPath, cleanup := createTempConfigFile(t, validTomlContent)
	defer cleanup()

	originalGetDefaultConfigPath := GetDefaultConfigPath
	GetDefaultConfigPath = func() (string, error) {
		return tempCfgPath, nil
	}
	defer func() { GetDefaultConfigPath = originalGetDefaultConfigPath }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() with hosts field returned error: %v", err)
	}

	// Verify hosts fields were parsed correctly
	if len(cfg.Dotfiles["zshrc"].Hosts) != 2 {
		t.Errorf("Expected 2 hosts for dotfile, got %d", len(cfg.Dotfiles["zshrc"].Hosts))
	}
	if cfg.Dotfiles["zshrc"].Hosts[0] != "work-laptop" && cfg.Dotfiles["zshrc"].Hosts[1] != "work-laptop" {
		t.Errorf("Expected 'work-laptop' in dotfile hosts, got %v", cfg.Dotfiles["zshrc"].Hosts)
	}

	if len(cfg.Directories["workdir"].Hosts) != 1 {
		t.Errorf("Expected 1 host for directory, got %d", len(cfg.Directories["workdir"].Hosts))
	}

	if len(cfg.Repos["tools"].Hosts) != 1 {
		t.Errorf("Expected 1 host for repo, got %d", len(cfg.Repos["tools"].Hosts))
	}

	if len(cfg.Tools[0].Hosts) != 1 {
		t.Errorf("Expected 1 host for tool, got %d", len(cfg.Tools[0].Hosts))
	}

	if len(cfg.Shell.Aliases["vim"].Hosts) != 1 {
		t.Errorf("Expected 1 host for alias, got %d", len(cfg.Shell.Aliases["vim"].Hosts))
	}

	if len(cfg.Shell.Functions["work-setup"].Hosts) != 1 {
		t.Errorf("Expected 1 host for function, got %d", len(cfg.Shell.Functions["work-setup"].Hosts))
	}

	if len(cfg.Hooks.Builds["work_build"].Hosts) != 1 {
		t.Errorf("Expected 1 host for build, got %d", len(cfg.Hooks.Builds["work_build"].Hosts))
	}
}

func TestLoadConfig_NonExistentConfig(t *testing.T) {
	// Ensure no config file exists at the path GetDefaultConfigPath would return
	// For this, we can point GetDefaultConfigPath to a non-existent file in a temp dir
	tempDir := t.TempDir()
	nonExistentPath := filepath.Join(tempDir, "non_existent_config.toml")

	originalGetDefaultConfigPath := GetDefaultConfigPath
	GetDefaultConfigPath = func() (string, error) {
		return nonExistentPath, nil
	}
	defer func() { GetDefaultConfigPath = originalGetDefaultConfigPath }()

	_, err := LoadConfig()
	if err == nil {
		t.Errorf("LoadConfig() with non-existent config did not return an error")
	}
	// We could also check for a specific error message here if desired
}

func TestLoadConfig_MalformedToml(t *testing.T) {
	malformedTomlContent := `dotfiles_repo_path = "~/.dotfiles" this is not valid toml`
	tempCfgPath, cleanup := createTempConfigFile(t, malformedTomlContent)
	defer cleanup()

	originalGetDefaultConfigPath := GetDefaultConfigPath
	GetDefaultConfigPath = func() (string, error) {
		return tempCfgPath, nil
	}
	defer func() { GetDefaultConfigPath = originalGetDefaultConfigPath }()

	_, err := LoadConfig()
	if err == nil {
		t.Errorf("LoadConfig() with malformed TOML did not return an error")
	}
	// Check for TOML decode error specifically if possible, or just that an error occurred
}

func TestLoadConfig_InvalidSemanticConfig(t *testing.T) {
	// Valid TOML, but missing required field (dotfiles_repo_path)
	invalidSemanticContent := `
	[dotfiles.bashrc]
	source = ".bashrc"
	target = "~/.bashrc"
	`
	tempCfgPath, cleanup := createTempConfigFile(t, invalidSemanticContent)
	defer cleanup()

	originalGetDefaultConfigPath := GetDefaultConfigPath
	GetDefaultConfigPath = func() (string, error) {
		return tempCfgPath, nil
	}
	defer func() { GetDefaultConfigPath = originalGetDefaultConfigPath }()

	_, err := LoadConfig()
	if err == nil {
		t.Errorf("LoadConfig() with semantically invalid config did not return an error")
	}
	// Should be a validation error from ValidateConfig
}

// TestGetDefaultConfigPath needs to handle XDG_CONFIG_HOME and fallback to ~/.config
func TestLoadConfig_WithEnableField(t *testing.T) {
	validTomlContent := `
	dotfiles_repo_path = "~/.dotfiles"

	[dotfiles.enabled_file]
	source = ".enabled"
	target = "~/.enabled"

	[dotfiles.disabled_file]
	source = ".disabled"
	target = "~/.disabled"
	enable = false

	[dotfiles.explicitly_enabled]
	source = ".explicit"
	target = "~/.explicit"
	enable = true

	[directories.enabled_dir]
	target = "~/enabled"

	[directories.disabled_dir]
	target = "~/disabled"
	enable = false

	[repos.enabled_repo]
	url = "https://github.com/example/enabled.git"
	target = "~/enabled"

	[repos.disabled_repo]
	url = "https://github.com/example/disabled.git"
	target = "~/disabled"
	enable = false

	[[tools]]
	name = "enabled-tool"
	check_command = "command -v enabled-tool"
	install_hint = "Install enabled-tool"

	[[tools]]
	name = "disabled-tool"
	check_command = "command -v disabled-tool"
	install_hint = "Install disabled-tool"
	enable = false

	[shell.aliases.enabled_alias]
	command = "echo enabled"

	[shell.aliases.disabled_alias]
	command = "echo disabled"
	enable = false

	[shell.functions.enabled_func]
	body = "echo enabled"

	[shell.functions.disabled_func]
	body = "echo disabled"
	enable = false

	[hooks.builds.enabled_build]
	commands = ["echo enabled"]
	run = "once"

	[hooks.builds.disabled_build]
	commands = ["echo disabled"]
	run = "once"
	enable = false
	`
	tempCfgPath, cleanup := createTempConfigFile(t, validTomlContent)
	defer cleanup()

	originalGetDefaultConfigPath := GetDefaultConfigPath
	GetDefaultConfigPath = func() (string, error) {
		return tempCfgPath, nil
	}
	defer func() { GetDefaultConfigPath = originalGetDefaultConfigPath }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() with enable field returned error: %v", err)
	}

	// Verify enable fields for dotfiles
	if cfg.Dotfiles["enabled_file"].Enable != nil {
		t.Error("Expected enabled_file.Enable to be nil (default)")
	}
	if cfg.Dotfiles["disabled_file"].Enable == nil || *cfg.Dotfiles["disabled_file"].Enable != false {
		t.Error("Expected disabled_file.Enable to be false")
	}
	if cfg.Dotfiles["explicitly_enabled"].Enable == nil || *cfg.Dotfiles["explicitly_enabled"].Enable != true {
		t.Error("Expected explicitly_enabled.Enable to be true")
	}

	// Verify enable fields for directories
	if cfg.Directories["enabled_dir"].Enable != nil {
		t.Error("Expected enabled_dir.Enable to be nil (default)")
	}
	if cfg.Directories["disabled_dir"].Enable == nil || *cfg.Directories["disabled_dir"].Enable != false {
		t.Error("Expected disabled_dir.Enable to be false")
	}

	// Verify enable fields for repos
	if cfg.Repos["enabled_repo"].Enable != nil {
		t.Error("Expected enabled_repo.Enable to be nil (default)")
	}
	if cfg.Repos["disabled_repo"].Enable == nil || *cfg.Repos["disabled_repo"].Enable != false {
		t.Error("Expected disabled_repo.Enable to be false")
	}

	// Verify enable fields for tools
	if cfg.Tools[0].Enable != nil {
		t.Error("Expected enabled-tool.Enable to be nil (default)")
	}
	if cfg.Tools[1].Enable == nil || *cfg.Tools[1].Enable != false {
		t.Error("Expected disabled-tool.Enable to be false")
	}

	// Verify enable fields for aliases
	if cfg.Shell.Aliases["enabled_alias"].Enable != nil {
		t.Error("Expected enabled_alias.Enable to be nil (default)")
	}
	if cfg.Shell.Aliases["disabled_alias"].Enable == nil || *cfg.Shell.Aliases["disabled_alias"].Enable != false {
		t.Error("Expected disabled_alias.Enable to be false")
	}

	// Verify enable fields for functions
	if cfg.Shell.Functions["enabled_func"].Enable != nil {
		t.Error("Expected enabled_func.Enable to be nil (default)")
	}
	if cfg.Shell.Functions["disabled_func"].Enable == nil || *cfg.Shell.Functions["disabled_func"].Enable != false {
		t.Error("Expected disabled_func.Enable to be false")
	}

	// Verify enable fields for builds
	if cfg.Hooks.Builds["enabled_build"].Enable != nil {
		t.Error("Expected enabled_build.Enable to be nil (default)")
	}
	if cfg.Hooks.Builds["disabled_build"].Enable == nil || *cfg.Hooks.Builds["disabled_build"].Enable != false {
		t.Error("Expected disabled_build.Enable to be false")
	}
}

func TestGetDefaultConfigPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get user home dir: %v", err)
	}

	tests := []struct {
		name          string
		xdgConfigHome string // Value to set for XDG_CONFIG_HOME
		setXdg        bool   // Whether to set XDG_CONFIG_HOME at all
		want          string
	}{
		{
			name:          "XDG_CONFIG_HOME is set",
			xdgConfigHome: "/tmp/custom_xdg_config",
			setXdg:        true,
			want:          "/tmp/custom_xdg_config/dotter/config.toml",
		},
		{
			name:   "XDG_CONFIG_HOME is not set",
			setXdg: false,
			want:   filepath.Join(homeDir, ".config", "dotter", "config.toml"),
		},
		{
			name:          "XDG_CONFIG_HOME is set but empty",
			xdgConfigHome: "",
			setXdg:        true,
			want:          filepath.Join(homeDir, ".config", "dotter", "config.toml"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cleanupXdg func()
			if tt.setXdg {
				_, _, cleanupXdg = setXdgConfigHome(t, tt.xdgConfigHome)
				defer cleanupXdg()
			} else {
				// Ensure XDG_CONFIG_HOME is unset if the test case requires it
				originalXdg, xdgWasSet, cleanupUnset := setXdgConfigHome(t, "") // Set to empty to effectively unset or override
				if xdgWasSet {                                                  // If it was originally set, restore it, otherwise ensure it remains unset.
					defer func() { os.Setenv("XDG_CONFIG_HOME", originalXdg) }()
				} else {
					defer os.Unsetenv("XDG_CONFIG_HOME")
				}
				_ = cleanupUnset // To satisfy go vet, though we defer more specific logic above
			}

			got, err := GetDefaultConfigPath()
			if err != nil {
				t.Fatalf("GetDefaultConfigPath() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("GetDefaultConfigPath() = %v, want %v", got, tt.want)
			}
		})
	}
}
