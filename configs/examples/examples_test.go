package examples_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

// repoPathLine matches the dotfiles_repo_path assignment every shipped config
// declares, so the test can point it at the example's own directory.
var repoPathLine = regexp.MustCompile(`(?m)^\s*dotfiles_repo_path\s*=.*$`)

// TestShippedConfigsLoad loads every shipped config through config.LoadConfig,
// the path `ralph up` takes, so an example that no longer validates fails
// `go test` instead of failing a user's first run.
func TestShippedConfigsLoad(t *testing.T) {
	for _, path := range shippedConfigs(t) {
		t.Run(path, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

			exampleDir, err := filepath.Abs(filepath.Dir(path))
			if err != nil {
				t.Fatalf("resolving example dir: %v", err)
			}
			t.Setenv("RALPH_CONFIG", withRepoPath(t, path, exampleDir))

			cfg, err := config.LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}
			if len(cfg.MissingRecipeDirs) > 0 {
				t.Errorf("recipes directory not found: %+v", cfg.MissingRecipeDirs)
			}
			if want := countRecipes(t, exampleDir); len(cfg.LoadedRecipes) != want {
				t.Errorf("loaded %d recipes, want every recipe.toml in the example (%d)",
					len(cfg.LoadedRecipes), want)
			}
		})
	}
}

// shippedConfigs lists the top-level configs under configs/examples and
// examples/, relative to this package directory.
func shippedConfigs(t *testing.T) []string {
	t.Helper()
	var paths []string
	for _, pattern := range []string{"*.toml", "../../examples/*/config.toml"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		if len(matches) == 0 {
			t.Fatalf("no shipped configs match %s", pattern)
		}
		paths = append(paths, matches...)
	}
	return paths
}

// withRepoPath writes a copy of the config at path into a temp dir with
// dotfiles_repo_path set to repoPath, and returns the copy's path.
func withRepoPath(t *testing.T, path, repoPath string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if !repoPathLine.Match(body) {
		t.Fatalf("%s does not declare dotfiles_repo_path", path)
	}
	line := fmt.Sprintf("dotfiles_repo_path = %q", repoPath)
	body = repoPathLine.ReplaceAllLiteral(body, []byte(line))

	out := filepath.Join(t.TempDir(), config.DefaultConfigFileName)
	if err := os.WriteFile(out, body, 0o644); err != nil {
		t.Fatalf("writing %s: %v", out, err)
	}
	return out
}

// countRecipes counts the recipe files under dir.
func countRecipes(t *testing.T, dir string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == config.RecipeFileName {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return count
}
