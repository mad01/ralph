package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// A mistyped table in a recipe is legal TOML that applies nothing — three
// overlay recipes once shipped a [symlinks] table and silently dropped every
// item. These tests pin the guard: unknown keys warn on stderr, and the
// recipe still loads with the keys it does declare.

// captureStderr runs fn while capturing everything written to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestLoadRecipe_WarnsOnUnknownTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recipe.toml")
	body := `
[recipe]
name = "git"

[dotfiles.known]
source = "gitconfig.local"
target = "~/.gitconfig.local"

[symlinks.gitconfig_local]
source = "gitconfig.local"
target = "~/.gitconfig.local"
action = "symlink"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var recipe *Recipe
	var loadErr error
	stderr := captureStderr(t, func() {
		recipe, loadErr = LoadRecipe(path)
	})
	if loadErr != nil {
		t.Fatalf("LoadRecipe: %v", loadErr)
	}
	if !strings.Contains(stderr, "unknown keys ignored") ||
		!strings.Contains(stderr, "symlinks.gitconfig_local") {
		t.Errorf("expected an unknown-key warning naming the table, got: %q", stderr)
	}
	// The declared items must survive the warning untouched.
	if recipe.Recipe.Name != "git" || len(recipe.Dotfiles) != 1 {
		t.Errorf(
			"recipe should still load: name=%q dotfiles=%d",
			recipe.Recipe.Name,
			len(recipe.Dotfiles),
		)
	}
}

func TestLoadRecipe_NoWarningOnCleanRecipe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recipe.toml")
	body := `
[recipe]
name = "clean"

[dotfiles.file]
source = "f"
target = "~/f"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stderr := captureStderr(t, func() {
		if _, err := LoadRecipe(path); err != nil {
			t.Errorf("LoadRecipe: %v", err)
		}
	})
	if stderr != "" {
		t.Errorf("expected no warning for a clean recipe, got: %q", stderr)
	}
}

func TestUndecodedSummary(t *testing.T) {
	cases := []struct {
		name string
		keys []toml.Key
		want string
	}{
		{"empty", nil, ""},
		{
			"fields collapse to their table",
			[]toml.Key{
				{"symlinks", "x", "source"},
				{"symlinks", "x", "target"},
				{"symlinks", "x", "action"},
			},
			"symlinks.x",
		},
		{
			"distinct keys sorted",
			[]toml.Key{{"zeta"}, {"alpha", "one"}},
			"alpha.one, zeta",
		},
		{
			"caps at five",
			[]toml.Key{{"a"}, {"b"}, {"c"}, {"d"}, {"e"}, {"f"}, {"g"}},
			"a, b, c, d, e, +2 more",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := undecodedSummary(tc.keys); got != tc.want {
				t.Errorf("undecodedSummary() = %q, want %q", got, tc.want)
			}
		})
	}
}
