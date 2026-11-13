package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// MAD-587: a recipes directory that discovery scans but finds missing is
// recorded on the config, so cleanup can tell "recipes dir gone" apart from
// "user removed these recipes".
func TestProcessRecipes_RecordsMissingLocalRecipesDir(t *testing.T) {
	tests := []struct {
		name         string
		autoDiscover bool
		makeDir      bool
		wantMissing  bool
	}{
		{name: "auto-discover, dir missing", autoDiscover: true, wantMissing: true},
		{name: "auto-discover, dir present", autoDiscover: true, makeDir: true},
		{name: "explicit mode does not scan", autoDiscover: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", "")
			repo := t.TempDir()
			recipesDir := filepath.Join(repo, DefaultRecipesDir)
			if tt.makeDir {
				if err := os.MkdirAll(recipesDir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &Config{
				DotfilesRepoPath: repo,
				RecipesConfig:    RecipesConfig{AutoDiscover: tt.autoDiscover},
			}
			if err := ProcessRecipes(cfg, "testhost"); err != nil {
				t.Fatalf("ProcessRecipes failed: %v", err)
			}

			var want []MissingRecipeDir
			if tt.wantMissing {
				want = []MissingRecipeDir{{Path: recipesDir}}
			}
			if !slices.Equal(cfg.MissingRecipeDirs, want) {
				t.Errorf("MissingRecipeDirs = %+v, want %+v", cfg.MissingRecipeDirs, want)
			}
		})
	}
}

func TestProcessRecipes_RecordsMissingSourceRecipesDir(t *testing.T) {
	src, _, _ := makeSourceRepo(t, "app")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg := &Config{
		DotfilesRepoPath: t.TempDir(),
		RecipeSources: []RecipeSource{
			{Name: "moon", URL: src, Ref: "main", RecipesDir: "gone"},
		},
	}
	if err := ProcessRecipes(cfg, "testhost"); err != nil {
		t.Fatalf("ProcessRecipes failed: %v", err)
	}

	want := []MissingRecipeDir{{
		Source: "moon",
		Path:   filepath.Join(home, ".config", "ralph", "sources", "moon", "gone"),
	}}
	if !slices.Equal(cfg.MissingRecipeDirs, want) {
		t.Errorf("MissingRecipeDirs = %+v, want %+v", cfg.MissingRecipeDirs, want)
	}
}
