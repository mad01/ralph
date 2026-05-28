package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const baseConfig = `dotfiles_repo_path = "~/dotfiles"

[recipes_config]
auto_discover = true
`

func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing config fixture: %v", err)
	}
	return path
}

func readConfig(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	return string(data)
}

func TestSetRecipeOverride_AppendNew(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, baseConfig)

	if err := SetRecipeOverride(path, "neovim", false); err != nil {
		t.Fatalf("SetRecipeOverride: %v", err)
	}

	got := readConfig(t, path)

	if !strings.Contains(got, "[recipes_config.overrides.neovim]") {
		t.Error("missing override section header")
	}
	if !strings.Contains(got, "enable = false") {
		t.Error("missing enable = false")
	}

	// Backup should be cleaned up on success.
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error("backup file was not cleaned up")
	}
}

func TestSetRecipeOverride_UpdateExisting(t *testing.T) {
	cfg := baseConfig + `
[recipes_config.overrides.git]
enable = false
`
	dir := t.TempDir()
	path := writeConfig(t, dir, cfg)

	if err := SetRecipeOverride(path, "git", true); err != nil {
		t.Fatalf("SetRecipeOverride: %v", err)
	}

	got := readConfig(t, path)

	if !strings.Contains(got, "enable = true") {
		t.Error("enable was not updated to true")
	}
	if strings.Contains(got, "enable = false") {
		t.Error("old enable = false still present")
	}
	// Must not duplicate the section.
	if strings.Count(got, "[recipes_config.overrides.git]") != 1 {
		t.Error("section was duplicated")
	}
}

func TestSetRecipeOverride_QuotedKey(t *testing.T) {
	cfg := baseConfig + `
[recipes_config.overrides."my-recipe"]
enable = true
`
	dir := t.TempDir()
	path := writeConfig(t, dir, cfg)

	if err := SetRecipeOverride(path, "my-recipe", false); err != nil {
		t.Fatalf("SetRecipeOverride: %v", err)
	}

	got := readConfig(t, path)

	if !strings.Contains(got, "enable = false") {
		t.Error("enable was not updated to false")
	}
	// Must not append a second section.
	count := strings.Count(got, "my-recipe")
	if count != 1 {
		t.Errorf("expected 1 occurrence of 'my-recipe', got %d", count)
	}
}

func TestSetRecipeOverride_RollbackOnInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, baseConfig)

	// Write valid config, then overwrite with intentionally broken content
	// AFTER the backup is made. We simulate this by making the file
	// read-only after SetRecipeOverride writes, but that's complex.
	// Instead, test the rollback path by corrupting the write result.
	// The simplest approach: patch applyOverride indirectly.
	//
	// Better: verify that if we pass a configPath pointing to valid TOML
	// that becomes invalid after modification, the backup is restored.
	// We can do this by having content that applyOverride turns invalid.
	//
	// Actually the cleanest test: manually test the rollback by writing
	// a config that is valid, then making the file unwritable so WriteFile
	// fails. But rollback is for TOML parse failure, not write failure.
	//
	// Let's verify the real path: write a config, modify the .bak after
	// backup is created to confirm rollback restores original.
	// The actual code flow we want to test: file gets written with content
	// that fails TOML parsing.
	// Since applyOverride always produces valid TOML from valid input,
	// we test this by pre-corrupting the file between backup and validate.
	// That requires hooking into the function, which we can't do easily.
	//
	// Pragmatic approach: test that after an error, the backup file IS
	// the original content and the original is restored.

	// Write broken TOML directly and call SetRecipeOverride on it.
	brokenContent := `dotfiles_repo_path = "~/dotfiles"
[recipes_config.overrides.foo
enable = true
`
	if err := os.WriteFile(path, []byte(brokenContent), 0o644); err != nil {
		t.Fatal(err)
	}

	err := SetRecipeOverride(path, "bar", true)
	if err == nil {
		t.Fatal("expected error for invalid TOML, got nil")
	}
	if !strings.Contains(err.Error(), "TOML validation failed") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Original (broken) content should be restored from backup.
	got := readConfig(t, path)
	if got != brokenContent {
		t.Error("rollback did not restore original content")
	}
}

func TestSetRecipeOverride_NoDuplicateSection(t *testing.T) {
	cfg := baseConfig + `
[recipes_config.overrides.tmux]
enable = true
hosts = ["workstation"]
`
	dir := t.TempDir()
	path := writeConfig(t, dir, cfg)

	// Update the same section.
	if err := SetRecipeOverride(path, "tmux", false); err != nil {
		t.Fatalf("SetRecipeOverride: %v", err)
	}

	got := readConfig(t, path)

	if strings.Count(got, "[recipes_config.overrides.tmux]") != 1 {
		t.Error("section was duplicated")
	}
	if !strings.Contains(got, "enable = false") {
		t.Error("enable was not updated")
	}
	// hosts line should be preserved.
	if !strings.Contains(got, `hosts = ["workstation"]`) {
		t.Error("hosts line was lost")
	}
}

func TestSetRecipeOverride_PreservesComments(t *testing.T) {
	cfg := `# Main ralph configuration
dotfiles_repo_path = "~/dotfiles"

# Recipe settings
[recipes_config]
auto_discover = true
# Override specific recipes below
`
	dir := t.TempDir()
	path := writeConfig(t, dir, cfg)

	if err := SetRecipeOverride(path, "zsh", true); err != nil {
		t.Fatalf("SetRecipeOverride: %v", err)
	}

	got := readConfig(t, path)

	if !strings.Contains(got, "# Main ralph configuration") {
		t.Error("top comment was lost")
	}
	if !strings.Contains(got, "# Recipe settings") {
		t.Error("recipe settings comment was lost")
	}
	if !strings.Contains(got, "# Override specific recipes below") {
		t.Error("inline comment was lost")
	}
	if !strings.Contains(got, "[recipes_config.overrides.zsh]") {
		t.Error("override section was not added")
	}
	if !strings.Contains(got, "enable = true") {
		t.Error("enable = true was not added")
	}
}

func TestSetRecipeOverride_InsertEnableWhenMissing(t *testing.T) {
	cfg := baseConfig + `
[recipes_config.overrides.kitty]
hosts = ["laptop"]
`
	dir := t.TempDir()
	path := writeConfig(t, dir, cfg)

	if err := SetRecipeOverride(path, "kitty", false); err != nil {
		t.Fatalf("SetRecipeOverride: %v", err)
	}

	got := readConfig(t, path)

	if !strings.Contains(got, "enable = false") {
		t.Error("enable line was not inserted")
	}
	if !strings.Contains(got, `hosts = ["laptop"]`) {
		t.Error("existing hosts line was lost")
	}
	if strings.Count(got, "[recipes_config.overrides.kitty]") != 1 {
		t.Error("section was duplicated")
	}
}

func TestSetRecipeOverride_MultipleOverrides(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, baseConfig)

	// Add three overrides sequentially.
	for _, tc := range []struct {
		name   string
		enable bool
	}{
		{"alpha", true},
		{"beta", false},
		{"gamma", true},
	} {
		if err := SetRecipeOverride(path, tc.name, tc.enable); err != nil {
			t.Fatalf("SetRecipeOverride(%s): %v", tc.name, err)
		}
	}

	got := readConfig(t, path)

	if !strings.Contains(got, "[recipes_config.overrides.alpha]") {
		t.Error("alpha section missing")
	}
	if !strings.Contains(got, "[recipes_config.overrides.beta]") {
		t.Error("beta section missing")
	}
	if !strings.Contains(got, "[recipes_config.overrides.gamma]") {
		t.Error("gamma section missing")
	}
}
