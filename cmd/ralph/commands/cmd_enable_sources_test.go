package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

func TestVerifyRecipeExists_SourceRecipe(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Fake a cached source checkout with one recipe.
	recipeDir := filepath.Join(home, ".config", "ralph", "sources", "moon", "recipes", "app")
	if err := os.MkdirAll(recipeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte("[recipe]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		DotfilesRepoPath: t.TempDir(),
		RecipeSources:    []config.RecipeSource{{Name: "moon", URL: "git@github.com:mad01/x.git"}},
	}

	if err := verifyRecipeExists(cfg, "moon/app"); err != nil {
		t.Errorf("existing source recipe rejected: %v", err)
	}
	if err := verifyRecipeExists(cfg, "moon/missing"); err == nil {
		t.Error("missing source recipe accepted")
	}
	if err := verifyRecipeExists(cfg, "nosuch/app"); err == nil {
		t.Error("undeclared source accepted")
	}
}
