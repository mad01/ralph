package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRecipeVersionCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), RecipeFileName)
	content := `[recipe]
name = "versioned-tools"

[hooks.builds.generated]
commands = ["true"]
run = "once"
install_paths = ["~/code/bin/generated"]
version_check = true

[packages.tool]
source = "local"
working_dir = "~/src/tool"
build = ["true"]
install_paths = ["~/code/bin/tool"]
version_check = true
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	recipe, err := LoadRecipe(path)
	if err != nil {
		t.Fatal(err)
	}
	if !recipe.Packages["tool"].VersionCheck {
		t.Error("package version_check was not decoded")
	}
	if !recipe.Hooks.Builds["generated"].VersionCheck {
		t.Error("build version_check was not decoded")
	}
}

func TestValidateConfigVersionCheckRequiresInstallPath(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Packages: map[string]Package{
			"tool": {
				Source:       "local",
				WorkingDir:   "~/src/tool",
				Build:        []string{"true"},
				VersionCheck: true,
			},
		},
	}

	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "install_paths") {
		t.Fatalf("ValidateConfig error = %v, want install_paths requirement", err)
	}
}

func TestValidateConfigBuildVersionCheckRequiresInstallPath(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Hooks: HooksConfig{Builds: map[string]Build{
			"generated": {
				Commands:     []string{"true"},
				Run:          "once",
				VersionCheck: true,
			},
		}},
	}

	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "install_paths") {
		t.Fatalf("ValidateConfig error = %v, want build install_paths requirement", err)
	}
}

func TestValidateConfigVersionCheckRejectsGoInstall(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Packages: map[string]Package{
			"tool": {
				Source:       "go-install",
				Module:       "example.invalid/tool",
				Version:      "v1.0.0",
				InstallPaths: []string{"~/code/bin/tool"},
				VersionCheck: true,
			},
		},
	}

	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("ValidateConfig error = %v, want go-install rejection", err)
	}
}
