package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalConfigPath(t *testing.T) {
	tests := []struct {
		name string
		main string
		want string
	}{
		{
			"default config",
			"/home/u/.config/ralph/config.toml",
			"/home/u/.config/ralph/config.local.toml",
		},
		{"custom name", "/tmp/myralph.toml", "/tmp/myralph.local.toml"},
		{"no extension", "/tmp/ralphcfg", "/tmp/ralphcfg.local.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LocalConfigPath(tt.main); got != tt.want {
				t.Errorf("LocalConfigPath(%q) = %q, want %q", tt.main, got, tt.want)
			}
		})
	}
}

// writeConfigPair writes main + optional local config into a temp dir and
// returns the main config path. Pass local == "" to skip the overlay file.
func writeConfigPair(t *testing.T, main, local string) string {
	t.Helper()
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatalf("write main config: %v", err)
	}
	if local != "" {
		localPath := filepath.Join(dir, "config.local.toml")
		if err := os.WriteFile(localPath, []byte(local), 0o644); err != nil {
			t.Fatalf("write local config: %v", err)
		}
	}
	return mainPath
}

func TestLoadLocalOverlay_NoFile(t *testing.T) {
	mainPath := writeConfigPair(t, `dotfiles_repo_path = "~/dots"`, "")

	var cfg Config
	cfg.DotfilesRepoPath = "~/dots"

	merged, err := loadLocalOverlay(&cfg, mainPath)
	if err != nil {
		t.Fatalf("loadLocalOverlay() error = %v", err)
	}
	if merged {
		t.Error("loadLocalOverlay() reported a merge when no local file exists")
	}
	if cfg.DotfilesRepoPath != "~/dots" {
		t.Errorf("DotfilesRepoPath changed unexpectedly: %q", cfg.DotfilesRepoPath)
	}
}

func TestLoadLocalOverlay_ScalarOverride(t *testing.T) {
	main := `
dotfiles_repo_path = "~/dots"
packages_dir = "~/.config/ralph/pkg"
`
	local := `
packages_dir = "/opt/ralph/pkg"
`
	mainPath := writeConfigPair(t, main, local)

	cfg := Config{DotfilesRepoPath: "~/dots", PackagesDir: "~/.config/ralph/pkg"}
	merged, err := loadLocalOverlay(&cfg, mainPath)
	if err != nil {
		t.Fatalf("loadLocalOverlay() error = %v", err)
	}
	if !merged {
		t.Fatal("loadLocalOverlay() reported no merge")
	}
	if cfg.PackagesDir != "/opt/ralph/pkg" {
		t.Errorf("PackagesDir = %q, want %q", cfg.PackagesDir, "/opt/ralph/pkg")
	}
	// Scalar not set in local must be preserved.
	if cfg.DotfilesRepoPath != "~/dots" {
		t.Errorf("DotfilesRepoPath = %q, want %q", cfg.DotfilesRepoPath, "~/dots")
	}
}

func TestLoadLocalOverlay_MapMergeLocalWins(t *testing.T) {
	main := `
[recipes_config.overrides.apple-dev]
enable = false

[recipes_config.overrides.pi]
enable = true
`
	local := `
[recipes_config.overrides.apple-dev]
enable = true
`
	mainPath := writeConfigPair(t, main, local)

	var cfg Config
	cfg.RecipesConfig.Overrides = map[string]RecipeOverride{
		"apple-dev": {Enable: boolPtr(false)},
		"pi":        {Enable: boolPtr(true)},
	}

	if _, err := loadLocalOverlay(&cfg, mainPath); err != nil {
		t.Fatalf("loadLocalOverlay() error = %v", err)
	}

	// Local wins for the overridden key.
	if got := cfg.RecipesConfig.Overrides["apple-dev"].Enable; got == nil || !*got {
		t.Errorf("apple-dev override = %v, want enable=true", got)
	}
	// Untouched base key is preserved.
	if got := cfg.RecipesConfig.Overrides["pi"].Enable; got == nil || !*got {
		t.Errorf("pi override = %v, want enable=true", got)
	}
}

func TestLoadLocalOverlay_MapMergeIntoNilBase(t *testing.T) {
	main := `dotfiles_repo_path = "~/dots"`
	local := `
[shell.aliases.ll]
command = "ls -alh"
`
	mainPath := writeConfigPair(t, main, local)

	var cfg Config // Shell.Aliases is nil
	if _, err := loadLocalOverlay(&cfg, mainPath); err != nil {
		t.Fatalf("loadLocalOverlay() error = %v", err)
	}
	if cfg.Shell.Aliases["ll"].Command != "ls -alh" {
		t.Errorf("alias ll = %q, want %q", cfg.Shell.Aliases["ll"].Command, "ls -alh")
	}
}

func TestLoadLocalOverlay_SliceReplace(t *testing.T) {
	main := `
[[tools]]
name = "fzf"
check_command = "command -v fzf"
install_hint = "brew install fzf"
`
	local := `
[[tools]]
name = "ripgrep"
check_command = "command -v rg"
install_hint = "brew install ripgrep"
`
	mainPath := writeConfigPair(t, main, local)

	cfg := Config{Tools: []Tool{{Name: "fzf"}}}
	if _, err := loadLocalOverlay(&cfg, mainPath); err != nil {
		t.Fatalf("loadLocalOverlay() error = %v", err)
	}

	if len(cfg.Tools) != 1 || cfg.Tools[0].Name != "ripgrep" {
		t.Errorf("Tools = %+v, want a single ripgrep entry (slice replace)", cfg.Tools)
	}
}

func TestLoadLocalOverlay_SetsProfiles(t *testing.T) {
	main := `dotfiles_repo_path = "~/dots"`
	local := `profiles = ["personal", "homelab"]`
	mainPath := writeConfigPair(t, main, local)

	cfg := Config{DotfilesRepoPath: "~/dots"}
	if _, err := loadLocalOverlay(&cfg, mainPath); err != nil {
		t.Fatalf("loadLocalOverlay() error = %v", err)
	}

	want := []string{"personal", "homelab"}
	if len(cfg.Profiles) != len(want) {
		t.Fatalf("Profiles = %v, want %v", cfg.Profiles, want)
	}
	for i, p := range want {
		if cfg.Profiles[i] != p {
			t.Errorf("Profiles[%d] = %q, want %q", i, cfg.Profiles[i], p)
		}
	}
}

func boolPtr(b bool) *bool { return &b }
