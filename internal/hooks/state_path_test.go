package hooks

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/buildstate"
)

func TestGetStateFilePath_Overridable(t *testing.T) {
	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "custom_state")

	orig := buildstate.GetStateFilePath
	buildstate.GetStateFilePath = func() (string, error) { return customPath, nil }
	t.Cleanup(func() { buildstate.GetStateFilePath = orig })

	state := &BuildState{
		Builds: map[string]BuildRecord{
			"test": {CompletedAt: time.Now(), GitHash: "abc"},
		},
	}

	if err := SaveBuildState(state); err != nil {
		t.Fatalf("save: %v", err)
	}

	if _, err := os.Stat(customPath); os.IsNotExist(err) {
		t.Error("expected state file at custom path")
	}

	loaded, err := LoadBuildState()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Builds["test"].GitHash != "abc" {
		t.Errorf("expected git hash 'abc', got %q", loaded.Builds["test"].GitHash)
	}
}
