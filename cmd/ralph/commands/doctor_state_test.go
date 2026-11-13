package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/paths"
	"github.com/mad01/ralph/internal/report"
)

// withStateDir pins the state dir to a temp dir and writes each named state
// file into it.
func withStateDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(paths.StateDirEnv, dir)
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// stateStep runs checkStateFiles and returns the step for one state file.
func stateStep(t *testing.T, name string) *report.StepResult {
	t.Helper()
	rpt := &report.Report{Command: "doctor"}
	checkStateFiles(rpt)
	step := findStep(rpt, "State", name)
	if step == nil {
		t.Fatalf("no State step for %s", name)
	}
	return step
}

func TestCheckStateFiles_CorruptRecipeStateFails(t *testing.T) {
	dir := withStateDir(t, map[string]string{".recipe_state": "{not json"})

	step := stateStep(t, ".recipe_state")
	if step.Status != report.StatusFail {
		t.Fatalf("status = %v, want fail (msg=%q)", step.Status, step.Message)
	}
	for _, want := range []string{filepath.Join(dir, ".recipe_state"), "move it aside"} {
		if !strings.Contains(step.Message, want) {
			t.Errorf("message %q should contain %q", step.Message, want)
		}
	}
}

func TestCheckStateFiles_NewerRecipeStateVersionFails(t *testing.T) {
	withStateDir(t, map[string]string{".recipe_state": `{"version": 99, "recipes": {}}`})

	step := stateStep(t, ".recipe_state")
	if step.Status != report.StatusFail || !strings.Contains(step.Message, "version 99") {
		t.Fatalf("step = %+v, want a fail naming version 99", step)
	}
}

func TestCheckStateFiles_CorruptBuildStateFails(t *testing.T) {
	dir := withStateDir(t, map[string]string{".builds_state": "{not json"})

	step := stateStep(t, ".builds_state")
	if step.Status != report.StatusFail {
		t.Fatalf("status = %v, want fail (msg=%q)", step.Status, step.Message)
	}
	for _, want := range []string{filepath.Join(dir, ".builds_state"), "ralph up --reset-builds"} {
		if !strings.Contains(step.Message, want) {
			t.Errorf("message %q should contain %q", step.Message, want)
		}
	}
}

// A fresh install has no state files yet; that is fine.
func TestCheckStateFiles_MissingFilesOK(t *testing.T) {
	withStateDir(t, nil)

	for _, name := range []string{".recipe_state", ".builds_state"} {
		step := stateStep(t, name)
		if step.Status != report.StatusOK || !strings.HasPrefix(step.Message, "not created yet: ") {
			t.Errorf("%s = %+v, want OK 'not created yet'", name, step)
		}
	}
}

func TestCheckStateFiles_ValidFilesOK(t *testing.T) {
	dir := withStateDir(t, map[string]string{
		".recipe_state": `{"version": 1, "recipes": {}}`,
		".builds_state": `{"builds": {}}`,
	})

	for _, name := range []string{".recipe_state", ".builds_state"} {
		step := stateStep(t, name)
		if step.Status != report.StatusOK || step.Message != filepath.Join(dir, name) {
			t.Errorf("%s = %+v, want OK with its path", name, step)
		}
	}
}
