package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A recipe's delete_behavior must be "delete", "abandon" or unset. Cleanup
// keeps orphans only on an exact "abandon", so any other value has to fail
// loading rather than delete what the recipe owned.
func TestProcessRecipes_DeleteBehavior(t *testing.T) {
	tests := []struct {
		name    string
		line    string // the delete_behavior line in [recipe], or "" for none
		wantErr string // "" when the recipe must load
	}{
		{name: "unset", line: ""},
		{name: "delete", line: `delete_behavior = "delete"`},
		{name: "abandon", line: `delete_behavior = "abandon"`},
		{name: "typo", line: `delete_behavior = "abandn"`, wantErr: "abandn"},
		{name: "wrong case", line: `delete_behavior = "Abandon"`, wantErr: "Abandon"},
		{name: "empty string", line: `delete_behavior = ""`},
		{name: "other word", line: `delete_behavior = "keep"`, wantErr: "keep"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			repo := t.TempDir()
			recipeDir := filepath.Join(repo, "editor")
			if err := os.MkdirAll(recipeDir, 0o755); err != nil {
				t.Fatal(err)
			}
			content := "[recipe]\nname = \"editor\"\n" + tt.line + "\n"
			if err := os.WriteFile(filepath.Join(recipeDir, RecipeFileName), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg := &Config{
				DotfilesRepoPath: repo,
				Recipes:          []RecipeRef{{Path: "editor/recipe.toml"}},
			}

			err := ProcessRecipes(cfg, "test-host")

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ProcessRecipes() returned error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("ProcessRecipes() accepted an invalid delete_behavior")
			}
			for _, want := range []string{"'editor'", "'" + tt.wantErr + "'", "'delete'", "'abandon'"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %s", err, want)
				}
			}
			if len(cfg.LoadedRecipes) != 0 {
				t.Errorf("a rejected recipe was recorded as loaded: %+v", cfg.LoadedRecipes)
			}
		})
	}
}
