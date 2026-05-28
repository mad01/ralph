package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetStatePath_Overridable(t *testing.T) {
	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "custom_recipe_state")

	orig := GetStatePath
	GetStatePath = func() (string, error) { return customPath, nil }
	t.Cleanup(func() { GetStatePath = orig })

	rs := &RecipeState{
		Recipes: map[string]RecipeArtifacts{
			"test-recipe": {
				AppliedAt:      time.Now(),
				DeleteBehavior: "delete",
				Symlinks:       []string{"/home/user/.bashrc"},
			},
		},
	}

	if err := Save(rs); err != nil {
		t.Fatalf("save: %v", err)
	}

	if _, err := os.Stat(customPath); os.IsNotExist(err) {
		t.Error("expected state file at custom path")
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Recipes["test-recipe"].Symlinks) != 1 {
		t.Errorf("expected 1 symlink, got %d", len(loaded.Recipes["test-recipe"].Symlinks))
	}
}
