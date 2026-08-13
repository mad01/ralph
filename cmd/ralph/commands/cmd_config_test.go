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
