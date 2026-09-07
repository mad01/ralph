package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

// withOverrideFixture writes a config.toml pointing at a temp dotfiles repo
// that holds one recipe ("app"), points GetDefaultConfigPath at it, and
// returns the config path so tests can inspect what enable/disable wrote.
func withOverrideFixture(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	recipeDir := filepath.Join(repo, "recipes", "app")
	if err := os.MkdirAll(recipeDir, 0o755); err != nil {
		t.Fatalf("mkdir recipe: %v", err)
	}
	recipe := "[recipe]\nname = \"app\"\n"
	if err := os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(recipe), 0o644); err != nil {
		t.Fatalf("write recipe: %v", err)
	}

	configPath := filepath.Join(t.TempDir(), "config.toml")
	body := "dotfiles_repo_path = \"" + repo + "\"\n"
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	orig := config.GetDefaultConfigPath
	config.GetDefaultConfigPath = func() (string, error) { return configPath, nil }
	t.Cleanup(func() { config.GetDefaultConfigPath = orig })

	return configPath
}

// setDryRun flips the global --dry-run flag for one test and restores it after.
func setDryRun(t *testing.T, on bool) {
	t.Helper()
	orig := dryRun
	dryRun = on
	t.Cleanup(func() { dryRun = orig })
}

func readConfig(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	return string(data)
}

// `ralph disable --dry-run` must not touch config.toml.
func TestDisable_DryRunLeavesConfigUntouched(t *testing.T) {
	configPath := withOverrideFixture(t)
	before := readConfig(t, configPath)
	setDryRun(t, true)

	if err := disableCmd.RunE(disableCmd, []string{"app"}); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if after := readConfig(t, configPath); after != before {
		t.Fatalf("dry-run disable modified config.toml:\n%s", after)
	}
}

// Without --dry-run, disable writes the enable = false override.
func TestDisable_WritesOverride(t *testing.T) {
	configPath := withOverrideFixture(t)
	setDryRun(t, false)

	if err := disableCmd.RunE(disableCmd, []string{"app"}); err != nil {
		t.Fatalf("disable: %v", err)
	}

	after := readConfig(t, configPath)
	if !strings.Contains(after, "[recipes_config.overrides.app]") ||
		!strings.Contains(after, "enable = false") {
		t.Fatalf("disable did not write the override:\n%s", after)
	}
}

// `ralph enable --dry-run` must leave an existing override in place.
func TestEnable_DryRunLeavesOverrideInPlace(t *testing.T) {
	configPath := withOverrideFixture(t)
	if err := config.SetRecipeOverride(configPath, "app", false); err != nil {
		t.Fatalf("seed override: %v", err)
	}
	before := readConfig(t, configPath)
	setDryRun(t, true)

	if err := enableCmd.RunE(enableCmd, []string{"app"}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if after := readConfig(t, configPath); after != before {
		t.Fatalf("dry-run enable modified config.toml:\n%s", after)
	}
}

// Without --dry-run, enable removes the override.
func TestEnable_RemovesOverride(t *testing.T) {
	configPath := withOverrideFixture(t)
	if err := config.SetRecipeOverride(configPath, "app", false); err != nil {
		t.Fatalf("seed override: %v", err)
	}
	setDryRun(t, false)

	if err := enableCmd.RunE(enableCmd, []string{"app"}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if after := readConfig(t, configPath); strings.Contains(after, "overrides.app") {
		t.Fatalf("enable left the override in place:\n%s", after)
	}
}
