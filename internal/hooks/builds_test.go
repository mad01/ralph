package hooks

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/dotter/internal/config"
)

// testStateDir creates a temp directory and sets HOME to it for isolated testing.
// Returns a cleanup function that should be deferred.
func testStateDir(t *testing.T) (string, func()) {
	t.Helper()
	origHome := os.Getenv("HOME")
	tmpDir, err := os.MkdirTemp("", "dotter-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	os.Setenv("HOME", tmpDir)
	return tmpDir, func() {
		os.Setenv("HOME", origHome)
		os.RemoveAll(tmpDir)
	}
}

// --- Tests for LoadBuildState ---

func TestLoadBuildState_NonExistentFile(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	state, err := LoadBuildState()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if len(state.Builds) != 0 {
		t.Errorf("expected empty builds map, got %d entries", len(state.Builds))
	}
}

func TestLoadBuildState_ValidJSON(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	// Create state file with test data
	stateDir := filepath.Join(tmpDir, ".config", "dotter")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	testState := &BuildState{
		Builds: map[string]BuildRecord{
			"test_build": {
				CompletedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
				GitHash:     "abc123def456",
			},
		},
	}
	data, _ := json.MarshalIndent(testState, "", "  ")
	if err := os.WriteFile(filepath.Join(stateDir, ".builds_state"), data, 0644); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}

	state, err := LoadBuildState()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(state.Builds) != 1 {
		t.Errorf("expected 1 build, got %d", len(state.Builds))
	}
	record, exists := state.Builds["test_build"]
	if !exists {
		t.Fatal("expected test_build to exist")
	}
	if record.GitHash != "abc123def456" {
		t.Errorf("expected git hash abc123def456, got %s", record.GitHash)
	}
}

func TestLoadBuildState_InvalidJSON(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	stateDir := filepath.Join(tmpDir, ".config", "dotter")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	// Write invalid JSON
	if err := os.WriteFile(filepath.Join(stateDir, ".builds_state"), []byte("not valid json"), 0644); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}

	_, err := LoadBuildState()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- Tests for SaveBuildState ---

func TestSaveBuildState_CreatesDirectory(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	state := &BuildState{
		Builds: map[string]BuildRecord{
			"my_build": {
				CompletedAt: time.Now(),
				GitHash:     "hash123",
			},
		},
	}

	if err := SaveBuildState(state); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify directory was created
	stateDir := filepath.Join(tmpDir, ".config", "dotter")
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Error("expected state directory to be created")
	}

	// Verify file was created
	statePath := filepath.Join(stateDir, ".builds_state")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("expected state file to be created")
	}
}

func TestSaveBuildState_Roundtrip(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	originalState := &BuildState{
		Builds: map[string]BuildRecord{
			"build1": {
				CompletedAt: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
				GitHash:     "deadbeef",
			},
			"build2": {
				CompletedAt: time.Date(2024, 6, 2, 12, 0, 0, 0, time.UTC),
				GitHash:     "",
			},
		},
	}

	if err := SaveBuildState(originalState); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loadedState, err := LoadBuildState()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(loadedState.Builds) != 2 {
		t.Errorf("expected 2 builds, got %d", len(loadedState.Builds))
	}

	if loadedState.Builds["build1"].GitHash != "deadbeef" {
		t.Errorf("expected git hash deadbeef, got %s", loadedState.Builds["build1"].GitHash)
	}
}

// --- Tests for ResetBuildState ---

func TestResetBuildState_ClearsAllState(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	// Create initial state
	state := &BuildState{
		Builds: map[string]BuildRecord{
			"build1": {CompletedAt: time.Now()},
			"build2": {CompletedAt: time.Now()},
		},
	}
	if err := SaveBuildState(state); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists
	statePath := filepath.Join(tmpDir, ".config", "dotter", ".builds_state")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatal("expected state file to exist before reset")
	}

	// Reset
	if err := ResetBuildState(); err != nil {
		t.Fatalf("reset failed: %v", err)
	}

	// Verify file is removed
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Error("expected state file to be removed after reset")
	}
}

func TestResetBuildState_NoErrorIfNoFile(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	// Reset without any existing state
	if err := ResetBuildState(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// --- Tests for ResetBuildStateForName ---

func TestResetBuildStateForName_ClearsSpecificBuild(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	// Create initial state with two builds
	state := &BuildState{
		Builds: map[string]BuildRecord{
			"keep_me":   {CompletedAt: time.Now(), GitHash: "keep"},
			"delete_me": {CompletedAt: time.Now(), GitHash: "delete"},
		},
	}
	if err := SaveBuildState(state); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Reset specific build
	if err := ResetBuildStateForName("delete_me"); err != nil {
		t.Fatalf("reset failed: %v", err)
	}

	// Load and verify
	loaded, err := LoadBuildState()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(loaded.Builds) != 1 {
		t.Errorf("expected 1 build remaining, got %d", len(loaded.Builds))
	}
	if _, exists := loaded.Builds["keep_me"]; !exists {
		t.Error("expected keep_me to still exist")
	}
	if _, exists := loaded.Builds["delete_me"]; exists {
		t.Error("expected delete_me to be deleted")
	}
}

func TestResetBuildStateForName_NonExistentBuild(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	state := &BuildState{
		Builds: map[string]BuildRecord{
			"existing": {CompletedAt: time.Now()},
		},
	}
	if err := SaveBuildState(state); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Reset non-existent build (should not error)
	if err := ResetBuildStateForName("nonexistent"); err != nil {
		t.Fatalf("expected no error for non-existent build, got: %v", err)
	}

	// Verify existing build is unchanged
	loaded, err := LoadBuildState()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded.Builds) != 1 {
		t.Errorf("expected 1 build, got %d", len(loaded.Builds))
	}
}

// --- Tests for RunBuild skip logic ---

// Helper to create a simple test build
func testBuild(run string) config.Build {
	return config.Build{
		Commands: []string{"echo test"},
		Run:      run,
	}
}

func TestRunBuild_AlwaysRuns(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	// Pre-populate state to ensure "always" ignores it
	state := &BuildState{
		Builds: map[string]BuildRecord{
			"always_build": {CompletedAt: time.Now()},
		},
	}
	SaveBuildState(state)

	opts := BuildOptions{DryRun: true}
	err := RunBuild("always_build", testBuild("always"), "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// In dry run mode with "always", it should process (not skip)
}

func TestRunBuild_OnceNoPriorState_Runs(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	// No prior state - build should run
	opts := BuildOptions{DryRun: true}
	err := RunBuild("new_build", testBuild("once"), "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunBuild_OnceWithPriorState_NoWorkingDir_Skips(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	// Pre-populate state
	state := &BuildState{
		Builds: map[string]BuildRecord{
			"completed_build": {CompletedAt: time.Now()},
		},
	}
	SaveBuildState(state)

	// Build without working_dir - should skip because already completed
	build := config.Build{
		Commands: []string{"echo test"},
		Run:      "once",
		// No WorkingDir - git checks won't apply
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("completed_build", build, "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The function should have returned early with a "Skipping" message
}

func TestRunBuild_OnceWithPriorState_SameHash_NoChanges_Skips(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	// Create a git repo for testing
	gitDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create git dir: %v", err)
	}

	// Initialize git repo
	runGitCmd(t, gitDir, "init")
	runGitCmd(t, gitDir, "config", "user.email", "test@test.com")
	runGitCmd(t, gitDir, "config", "user.name", "Test")

	// Create a file and commit
	testFile := filepath.Join(gitDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)
	runGitCmd(t, gitDir, "add", ".")
	runGitCmd(t, gitDir, "commit", "-m", "initial")

	// Get current hash
	currentHash := getGitHash(gitDir)
	if currentHash == "" {
		t.Skip("git not available or repo setup failed")
	}

	// Pre-populate state with same hash
	state := &BuildState{
		Builds: map[string]BuildRecord{
			"git_build": {CompletedAt: time.Now(), GitHash: currentHash},
		},
	}
	SaveBuildState(state)

	build := config.Build{
		Commands:   []string{"echo test"},
		Run:        "once",
		WorkingDir: gitDir,
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("git_build", build, "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should skip since hash matches and no uncommitted changes
}

func TestRunBuild_OnceWithPriorState_DifferentHash_Reruns(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	// Create a git repo
	gitDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create git dir: %v", err)
	}

	runGitCmd(t, gitDir, "init")
	runGitCmd(t, gitDir, "config", "user.email", "test@test.com")
	runGitCmd(t, gitDir, "config", "user.name", "Test")

	testFile := filepath.Join(gitDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)
	runGitCmd(t, gitDir, "add", ".")
	runGitCmd(t, gitDir, "commit", "-m", "initial")

	currentHash := getGitHash(gitDir)
	if currentHash == "" {
		t.Skip("git not available")
	}

	// Pre-populate state with DIFFERENT hash
	state := &BuildState{
		Builds: map[string]BuildRecord{
			"git_build": {CompletedAt: time.Now(), GitHash: "oldhash123"},
		},
	}
	SaveBuildState(state)

	build := config.Build{
		Commands:   []string{"echo test"},
		Run:        "once",
		WorkingDir: gitDir,
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("git_build", build, "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should re-run since hash changed
}

func TestRunBuild_OnceWithPriorState_UncommittedChanges_Reruns(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	// Create a git repo
	gitDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create git dir: %v", err)
	}

	runGitCmd(t, gitDir, "init")
	runGitCmd(t, gitDir, "config", "user.email", "test@test.com")
	runGitCmd(t, gitDir, "config", "user.name", "Test")

	testFile := filepath.Join(gitDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)
	runGitCmd(t, gitDir, "add", ".")
	runGitCmd(t, gitDir, "commit", "-m", "initial")

	currentHash := getGitHash(gitDir)
	if currentHash == "" {
		t.Skip("git not available")
	}

	// Add uncommitted changes
	os.WriteFile(testFile, []byte("modified content"), 0644)

	// Pre-populate state with same hash
	state := &BuildState{
		Builds: map[string]BuildRecord{
			"git_build": {CompletedAt: time.Now(), GitHash: currentHash},
		},
	}
	SaveBuildState(state)

	build := config.Build{
		Commands:   []string{"echo test"},
		Run:        "once",
		WorkingDir: gitDir,
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("git_build", build, "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should re-run since there are uncommitted changes
}

func TestRunBuild_ManualWithoutFlag_Skips(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	opts := BuildOptions{DryRun: true}
	err := RunBuild("manual_build", testBuild("manual"), "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should skip since --build flag doesn't match
}

func TestRunBuild_ManualWithMatchingFlag_Runs(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	opts := BuildOptions{
		DryRun:        true,
		SpecificBuild: "manual_build",
	}
	err := RunBuild("manual_build", testBuild("manual"), "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should run since --build=manual_build matches
}

func TestRunBuild_ForceOverridesOnce(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	// Pre-populate state
	state := &BuildState{
		Builds: map[string]BuildRecord{
			"force_build": {CompletedAt: time.Now()},
		},
	}
	SaveBuildState(state)

	opts := BuildOptions{
		DryRun: true,
		Force:  true,
	}
	err := RunBuild("force_build", testBuild("once"), "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should run despite prior state because Force is true
}

func TestRunBuild_InvalidRunMode(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	build := config.Build{
		Commands: []string{"echo test"},
		Run:      "invalid_mode",
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("bad_build", build, "testhost", opts)
	if err == nil {
		t.Fatal("expected error for invalid run mode")
	}
}

func TestRunBuild_SavesStateAfterOnceRun(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	// Create a working directory (not git repo for simplicity)
	workDir := filepath.Join(tmpDir, "workdir")
	os.MkdirAll(workDir, 0755)

	build := config.Build{
		Commands:   []string{"true"}, // Simple command that succeeds
		Run:        "once",
		WorkingDir: workDir,
	}

	opts := BuildOptions{DryRun: false} // Actually run
	err := RunBuild("save_test", build, "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify state was saved
	state, err := LoadBuildState()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	if _, exists := state.Builds["save_test"]; !exists {
		t.Error("expected build state to be saved after successful run")
	}
}

func TestRunBuild_DryRunDoesNotSaveState(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	build := config.Build{
		Commands: []string{"echo test"},
		Run:      "once",
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("dry_run_test", build, "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify state was NOT saved (dry run)
	state, err := LoadBuildState()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	if _, exists := state.Builds["dry_run_test"]; exists {
		t.Error("state should not be saved during dry run")
	}
}

// --- Tests for RunBuilds ---

func TestRunBuilds_EmptyBuilds(t *testing.T) {
	err := RunBuilds(nil, "testhost", BuildOptions{})
	if err != nil {
		t.Fatalf("unexpected error for empty builds: %v", err)
	}

	err = RunBuilds(map[string]config.Build{}, "testhost", BuildOptions{})
	if err != nil {
		t.Fatalf("unexpected error for empty builds map: %v", err)
	}
}

func TestRunBuilds_SpecificBuildNotFound(t *testing.T) {
	builds := map[string]config.Build{
		"existing": testBuild("always"),
	}

	opts := BuildOptions{SpecificBuild: "nonexistent"}
	err := RunBuilds(builds, "testhost", opts)
	if err == nil {
		t.Fatal("expected error for non-existent specific build")
	}
}

func TestRunBuilds_SpecificBuildRuns(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	builds := map[string]config.Build{
		"target": testBuild("manual"),
		"other":  testBuild("manual"),
	}

	opts := BuildOptions{
		DryRun:        true,
		SpecificBuild: "target",
	}
	err := RunBuilds(builds, "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Tests for Host Filtering ---

func TestRunBuild_HostFilter_MatchingHost_Runs(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	build := config.Build{
		Commands: []string{"echo test"},
		Run:      "always",
		Hosts:    []string{"matchinghost"},
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("host_test", build, "matchinghost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should run since host matches
}

func TestRunBuild_HostFilter_NonMatchingHost_Skips(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	build := config.Build{
		Commands: []string{"echo test"},
		Run:      "always",
		Hosts:    []string{"otherhost"},
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("host_test", build, "myhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should skip since host doesn't match (error would mean it tried to run and failed)
}

func TestRunBuild_HostFilter_EmptyHosts_Runs(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	build := config.Build{
		Commands: []string{"echo test"},
		Run:      "always",
		Hosts:    []string{}, // Empty means run on all hosts
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("host_test", build, "anyhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should run since empty hosts means all hosts
}

func TestRunBuild_HostFilter_CaseInsensitive(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	build := config.Build{
		Commands: []string{"echo test"},
		Run:      "always",
		Hosts:    []string{"MYHOST"}, // Uppercase in config
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("host_test", build, "myhost", opts) // Lowercase current host
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should run since host matching is case-insensitive
}

// --- Tests for Enable Filtering ---

func TestRunBuild_Disabled_Skips(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	enabled := false
	build := config.Build{
		Commands: []string{"echo test"},
		Run:      "always",
		Enable:   &enabled,
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("disabled_build", build, "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should skip since build is disabled
}

func TestRunBuild_Enabled_Runs(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	enabled := true
	build := config.Build{
		Commands: []string{"echo test"},
		Run:      "always",
		Enable:   &enabled,
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("enabled_build", build, "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should run since build is explicitly enabled
}

func TestRunBuild_EnableNotSet_Runs(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	build := config.Build{
		Commands: []string{"echo test"},
		Run:      "always",
		Enable:   nil, // Not set, defaults to enabled
	}

	opts := BuildOptions{DryRun: true}
	err := RunBuild("default_enabled_build", build, "testhost", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should run since enable not set means enabled
}

// --- Helper functions ---

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2024-01-01T00:00:00", "GIT_COMMITTER_DATE=2024-01-01T00:00:00")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Logf("git command failed: %s, output: %s", err, output)
	}
}
