package commands

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/mad01/ralph/internal/config"
)

// withConfigPath pins config path resolution to path for the duration of fn,
// so the command never reads the real user config.
func withConfigPath(t *testing.T, path string, fn func()) {
	t.Helper()
	prev := config.GetDefaultConfigPath
	config.GetDefaultConfigPath = func() (string, error) { return path, nil }
	defer func() { config.GetDefaultConfigPath = prev }()
	fn()
}

func TestConfigCmd_LoadedWithOverlay(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config.toml")
	mainTOML := `dotfiles_repo_path = "~/dotfiles"

[dotfiles.gitconfig]
source = "git/.gitconfig"
target = "~/.gitconfig"
`
	if err := os.WriteFile(mainPath, []byte(mainTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	localTOML := `profiles = ["personal"]`
	if err := os.WriteFile(filepath.Join(dir, "config.local.toml"), []byte(localTOML), 0o644); err != nil {
		t.Fatal(err)
	}

	var out string
	withConfigPath(t, mainPath, func() {
		withOutputFormat("text", func() {
			out = captureStdout(t, func() {
				if err := configCmd.RunE(configCmd, nil); err != nil {
					t.Errorf("config RunE: %v", err)
				}
			})
		})
	})

	for _, want := range []string{
		"config file:   " + mainPath + " (loaded)",
		"local overlay: " + filepath.Join(dir, "config.local.toml") + " (present)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	// The body after the blank line is the merged config and must be valid TOML.
	_, body, found := strings.Cut(out, "\n\n")
	if !found {
		t.Fatalf("no blank line separating header from body:\n%s", out)
	}
	var got map[string]any
	if _, err := toml.Decode(body, &got); err != nil {
		t.Fatalf("body is not valid TOML: %v\n%s", err, body)
	}
	if got["dotfiles_repo_path"] != "~/dotfiles" {
		t.Errorf("dotfiles_repo_path = %v, want ~/dotfiles", got["dotfiles_repo_path"])
	}
	// The overlay's profiles must be merged in.
	if !strings.Contains(body, `"personal"`) {
		t.Errorf("overlay profiles not merged into body:\n%s", body)
	}
}

// withEffective runs fn with the config command's --effective flag pinned on.
func withEffective(t *testing.T, fn func()) {
	t.Helper()
	configEffective = true
	defer func() { configEffective = false }()
	fn()
}

// TestConfigCmd_MaterializesDefaults pins the rule that the printed config
// carries the values ralph resolves at the point of use: an unset
// packages_dir and recipes dir print as their defaults, not empty strings.
func TestConfigCmd_MaterializesDefaults(t *testing.T) {
	dir := t.TempDir()
	// The sources-dir header line expands ~, so pin HOME rather than depend
	// on the suite leaving it intact.
	t.Setenv("HOME", dir)
	mainPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(mainPath, []byte(`dotfiles_repo_path = "~/dotfiles"`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out string
	withConfigPath(t, mainPath, func() {
		withOutputFormat("text", func() {
			out = captureStdout(t, func() {
				if err := configCmd.RunE(configCmd, nil); err != nil {
					t.Errorf("config RunE: %v", err)
				}
			})
		})
	})

	for _, want := range []string{
		`packages_dir = "` + config.DefaultPackagesDir + `"`,
		`dir = "` + config.DefaultRecipesDir + `"`,
		"sources dir:   ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// TestConfigCmd_Effective pins the --effective mode: the body carries items a
// recipe contributed, and the recipe provenance prints as TOML comment lines
// with the wave order.
func TestConfigCmd_Effective(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "dotfiles")
	recipeDir := filepath.Join(repo, "recipes", "greeter")
	if err := os.MkdirAll(recipeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	recipeTOML := `[shell.aliases.hi]
command = "echo hi"
`
	if err := os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(recipeTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "config.toml")
	mainTOML := `dotfiles_repo_path = "` + repo + `"

[recipes_config]
auto_discover = true
`
	if err := os.WriteFile(mainPath, []byte(mainTOML), 0o644); err != nil {
		t.Fatal(err)
	}

	var out string
	withConfigPath(t, mainPath, func() {
		withOutputFormat("text", func() {
			withEffective(t, func() {
				out = captureStdout(t, func() {
					if err := configCmd.RunE(configCmd, nil); err != nil {
						t.Errorf("config RunE: %v", err)
					}
				})
			})
		})
	})

	for _, want := range []string{
		"mode:          effective (recipes merged, host/profile gates applied)",
		"[shell.aliases.hi]",
		"# loaded recipes (1):",
		"#   wave 1  greeter",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	// The whole text output after the header must still parse as one TOML
	// document — the provenance lines are comments, not a second format.
	_, body, found := strings.Cut(out, "\n\n")
	if !found {
		t.Fatalf("no blank line separating header from body:\n%s", out)
	}
	var got map[string]any
	if _, err := toml.Decode(body, &got); err != nil {
		t.Fatalf("effective body is not valid TOML: %v\n%s", err, body)
	}
}

// TestConfigCmd_EffectiveJSON pins the JSON shape of --effective: the
// effective marker and the loaded-recipe summaries.
func TestConfigCmd_EffectiveJSON(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(mainPath, []byte(`dotfiles_repo_path = "`+dir+`"`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out string
	withConfigPath(t, mainPath, func() {
		withOutputFormat("json", func() {
			withEffective(t, func() {
				out = captureStdout(t, func() {
					if err := configCmd.RunE(configCmd, nil); err != nil {
						t.Errorf("config RunE: %v", err)
					}
				})
			})
		})
	})

	var doc struct {
		Effective bool           `json:"effective"`
		Status    string         `json:"status"`
		Config    map[string]any `json:"config"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("config --effective -o json did not emit valid JSON: %v\n%s", err, out)
	}
	if !doc.Effective {
		t.Error("effective = false, want true")
	}
	if doc.Status != "loaded" {
		t.Errorf("status = %q, want loaded", doc.Status)
	}
	if doc.Config["packages_dir"] != config.DefaultPackagesDir {
		t.Errorf("config.packages_dir = %v, want the materialized default", doc.Config["packages_dir"])
	}
}

func TestConfigCmd_MissingConfig(t *testing.T) {
	mainPath := filepath.Join(t.TempDir(), "config.toml")

	var out string
	var runErr error
	withConfigPath(t, mainPath, func() {
		withOutputFormat("text", func() {
			out = captureStdout(t, func() {
				runErr = configCmd.RunE(configCmd, nil)
			})
		})
	})

	var exitErr *ExitError
	if !errors.As(runErr, &exitErr) || exitErr.Code != 1 {
		t.Errorf("missing config should exit 1, got %v", runErr)
	}
	if !strings.Contains(out, "missing, run 'ralph init'") {
		t.Errorf("output missing the missing-config status:\n%s", out)
	}
}

func TestConfigCmd_ParseError(t *testing.T) {
	mainPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(mainPath, []byte("not = valid = toml"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out string
	var runErr error
	withConfigPath(t, mainPath, func() {
		withOutputFormat("text", func() {
			out = captureStdout(t, func() {
				runErr = configCmd.RunE(configCmd, nil)
			})
		})
	})

	var exitErr *ExitError
	if !errors.As(runErr, &exitErr) || exitErr.Code != 1 {
		t.Errorf("broken config should exit 1, got %v", runErr)
	}
	if !strings.Contains(out, "parse error:") {
		t.Errorf("output missing the parse-error status:\n%s", out)
	}
}

func TestConfigCmd_JSON(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(mainPath, []byte(`dotfiles_repo_path = "~/dotfiles"`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out string
	withConfigPath(t, mainPath, func() {
		withOutputFormat("json", func() {
			out = captureStdout(t, func() {
				if err := configCmd.RunE(configCmd, nil); err != nil {
					t.Errorf("config RunE: %v", err)
				}
			})
		})
	})

	var doc struct {
		ConfigFile   string         `json:"config_file"`
		Status       string         `json:"status"`
		LocalOverlay string         `json:"local_overlay"`
		LocalPresent bool           `json:"local_present"`
		Config       map[string]any `json:"config"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("config -o json did not emit valid JSON: %v\n%s", err, out)
	}
	if doc.ConfigFile != mainPath {
		t.Errorf("config_file = %q, want %q", doc.ConfigFile, mainPath)
	}
	if doc.Status != "loaded" {
		t.Errorf("status = %q, want loaded", doc.Status)
	}
	if doc.LocalPresent {
		t.Error("local_present = true, want false (no overlay written)")
	}
	if doc.Config["dotfiles_repo_path"] != "~/dotfiles" {
		t.Errorf(
			"config.dotfiles_repo_path = %v, want ~/dotfiles",
			doc.Config["dotfiles_repo_path"],
		)
	}
}
