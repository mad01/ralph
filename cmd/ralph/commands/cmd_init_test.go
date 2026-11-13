package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlecAivazis/survey/v2"
	"github.com/mad01/ralph/internal/config"
)

func TestEnsureGitignored(t *testing.T) {
	t.Run("creates .gitignore when missing", func(t *testing.T) {
		dir := t.TempDir()
		added, err := ensureGitignored(dir, "config.local.toml")
		if err != nil {
			t.Fatalf("ensureGitignored() error = %v", err)
		}
		if !added {
			t.Fatal("expected added=true")
		}
		got := readFile(t, filepath.Join(dir, ".gitignore"))
		if got != "config.local.toml\n" {
			t.Errorf(".gitignore = %q, want %q", got, "config.local.toml\n")
		}
	})

	t.Run("idempotent when entry already present", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".gitignore"), "node_modules\nconfig.local.toml\n")

		added, err := ensureGitignored(dir, "config.local.toml")
		if err != nil {
			t.Fatalf("ensureGitignored() error = %v", err)
		}
		if added {
			t.Error("expected added=false when entry already present")
		}
		got := readFile(t, filepath.Join(dir, ".gitignore"))
		if got != "node_modules\nconfig.local.toml\n" {
			t.Errorf(".gitignore changed: %q", got)
		}
	})

	t.Run("appends a newline when file lacks trailing newline", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".gitignore"), "node_modules")

		if _, err := ensureGitignored(dir, "config.local.toml"); err != nil {
			t.Fatalf("ensureGitignored() error = %v", err)
		}
		got := readFile(t, filepath.Join(dir, ".gitignore"))
		if got != "node_modules\nconfig.local.toml\n" {
			t.Errorf(".gitignore = %q, want %q", got, "node_modules\nconfig.local.toml\n")
		}
	})

	t.Run("no-op when repo dir does not exist", func(t *testing.T) {
		added, err := ensureGitignored(filepath.Join(t.TempDir(), "nope"), "config.local.toml")
		if err != nil {
			t.Fatalf("ensureGitignored() error = %v", err)
		}
		if added {
			t.Error("expected added=false for missing repo dir")
		}
	})
}

// The starter config comes from the binary, not the working directory, and
// carries the repo path the user entered.
func TestRenderInitConfig_SetsRepoPathFromAnyDir(t *testing.T) {
	t.Chdir(t.TempDir()) // no configs/examples here, as for an installed binary

	for _, repo := range []string{"/home/me/dotfiles", `/home/me/my "dots"`} {
		t.Run(repo, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			writeFile(t, path, string(renderInitConfig(repo)))

			cfg, _, err := config.LoadUserConfig(path)
			if err != nil {
				t.Fatalf("starter config does not decode: %v", err)
			}
			if err := config.ValidateConfig(cfg); err != nil {
				t.Fatalf("starter config does not validate: %v", err)
			}
			if cfg.DotfilesRepoPath != repo {
				t.Errorf("dotfiles_repo_path = %q, want %q", cfg.DotfilesRepoPath, repo)
			}
		})
	}
}

// `ralph init --dry-run` must not create the config, its directory, or the
// dotfiles repo's .gitignore, and must say what it would write.
func TestInstallInitConfig_DryRunWritesNothing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "ralph", "config.toml")
	repo := t.TempDir()
	setDryRun(t, true)

	var runErr error
	out := captureStdout(t, func() {
		runErr = installInitConfig(configPath, repo, renderInitConfig(repo))
	})
	if runErr != nil {
		t.Fatalf("installInitConfig: %v", runErr)
	}

	for _, p := range []string{filepath.Dir(configPath), filepath.Join(repo, ".gitignore")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("dry-run init created %s (stat err = %v)", p, err)
		}
	}
	if !strings.Contains(out, "Would write") || !strings.Contains(out, configPath) {
		t.Errorf("dry-run output does not name the file it would write:\n%s", out)
	}
}

// Without --dry-run, init writes the config and gitignores the local overlay.
func TestInstallInitConfig_WritesConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "ralph", "config.toml")
	repo := t.TempDir()
	setDryRun(t, false)

	content := renderInitConfig(repo)
	if err := installInitConfig(configPath, repo, content); err != nil {
		t.Fatalf("installInitConfig: %v", err)
	}

	if got := readFile(t, configPath); got != string(content) {
		t.Errorf("config.toml does not hold the starter config:\n%s", got)
	}
	if got := readFile(t, filepath.Join(repo, ".gitignore")); got != config.LocalConfigFileName+"\n" {
		t.Errorf(".gitignore = %q, want %q", got, config.LocalConfigFileName+"\n")
	}
}

// A failed overwrite prompt is an error, not a silent "no".
func TestConfirmOverwrite_ReturnsPromptError(t *testing.T) {
	in, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_ = w.Close() // the prompt reads EOF
	t.Cleanup(func() { _ = in.Close() })
	out, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = out.Close() })

	if _, err := confirmOverwrite("config.toml", survey.WithStdio(in, out, out)); err == nil {
		t.Fatal("confirmOverwrite() error = nil, want the prompt's read error")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
