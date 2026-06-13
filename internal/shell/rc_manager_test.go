package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helper to set/unset env vars for testing
func setEnvVar(t *testing.T, key, value string) (originalValue string, wasSet bool) {
	t.Helper()
	originalValue, wasSet = os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("Failed to set env var %s: %v", key, err)
	}
	return
}

func unsetEnvVar(t *testing.T, key string, originalValue string, wasSet bool) {
	t.Helper()
	if wasSet {
		if err := os.Setenv(key, originalValue); err != nil {
			t.Fatalf("Failed to restore env var %s: %v", key, err)
		}
	} else {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Failed to unset env var %s: %v", key, err)
		}
	}
}

func TestGetRCFilePath(t *testing.T) {
	origHome, homeWasSet := os.LookupEnv("HOME")
	const tempHome = "/tmp/fakehome_rc"
	setEnvVar(t, "HOME", tempHome)
	defer unsetEnvVar(t, "HOME", origHome, homeWasSet)

	os.MkdirAll(filepath.Join(tempHome, ".config", "fish"), 0o755)
	defer os.RemoveAll(tempHome) // Clean up fake home

	tests := []struct {
		name         string
		shell        SupportedShell
		zdotdir      string // value for ZDOTDIR, empty means unset
		wantError    bool
		expectedPath string
	}{
		{"bash", Bash, "", false, filepath.Join(tempHome, ".bashrc")},
		{"zsh_no_zdotdir", Zsh, "", false, filepath.Join(tempHome, ".zshrc")},
		{
			"zsh_with_zdotdir",
			Zsh,
			filepath.Join(tempHome, ".myzdotdir"),
			false,
			filepath.Join(tempHome, ".myzdotdir", ".zshrc"),
		},
		{"fish", Fish, "", false, filepath.Join(tempHome, ".config", "fish", "config.fish")},
		{"unsupported", SupportedShell("powershell"), "", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var origZdotdir string
			var zdotdirWasSet bool
			if tt.zdotdir != "" {
				origZdotdir, zdotdirWasSet = setEnvVar(t, "ZDOTDIR", tt.zdotdir)
				os.MkdirAll(tt.zdotdir, 0o755) // Ensure ZDOTDIR exists if set
				defer os.RemoveAll(tt.zdotdir)
			} else {
				// Ensure ZDOTDIR is unset if test requires it
				origZdotdir, zdotdirWasSet = os.LookupEnv("ZDOTDIR")
				if zdotdirWasSet {
					os.Unsetenv("ZDOTDIR")
				}
			}

			gotPath, err := GetRCFilePath(tt.shell)

			// Restore ZDOTDIR if it was manipulated for this test case
			if tt.zdotdir != "" ||
				(zdotdirWasSet && tt.zdotdir == "") { // tt.zdotdir == "" implies we might have unset it
				unsetEnvVar(t, "ZDOTDIR", origZdotdir, zdotdirWasSet)
			}

			if (err != nil) != tt.wantError {
				t.Errorf(
					"GetRCFilePath() for %s error = %v, wantError %v",
					tt.shell,
					err,
					tt.wantError,
				)
				return
			}
			if !tt.wantError && gotPath != tt.expectedPath {
				t.Errorf("GetRCFilePath() for %s = %s, want %s", tt.shell, gotPath, tt.expectedPath)
			}
		})
	}
}

func TestGetRalphGeneratedDir(t *testing.T) {
	origHome, homeWasSet := os.LookupEnv("HOME")
	const tempHome = "/tmp/fakehome_generated"
	setEnvVar(t, "HOME", tempHome)
	defer unsetEnvVar(t, "HOME", origHome, homeWasSet)

	origXdgConfig, xdgConfigWasSet := os.LookupEnv("XDG_CONFIG_HOME")
	// Unset XDG_CONFIG_HOME for the first case
	if xdgConfigWasSet {
		os.Unsetenv("XDG_CONFIG_HOME")
	}

	tests := []struct {
		name          string
		xdgConfigHome string // value for XDG_CONFIG_HOME, empty means unset for this test run after initial unset
		setXdg        bool   // whether to explicitly set it for this test case
		expectedDir   string
	}{
		{
			name:        "no_xdg_config_home",
			setXdg:      false,
			expectedDir: filepath.Join(tempHome, ".config", "ralph", "generated"),
		},
		{
			name:          "with_xdg_config_home",
			xdgConfigHome: filepath.Join(tempHome, "custom_xdg", "config"),
			setXdg:        true,
			expectedDir:   filepath.Join(tempHome, "custom_xdg", "config", "ralph", "generated"),
		},
		{
			name:          "xdg_config_home_is_empty_string", // Should fallback to default
			xdgConfigHome: "",
			setXdg:        true,
			expectedDir:   filepath.Join(tempHome, ".config", "ralph", "generated"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setXdg {
				setEnvVar(t, "XDG_CONFIG_HOME", tt.xdgConfigHome)
				// No defer here, restored after all tests from origXdgConfig
			} else {
				// Ensure it's unset if already manipulated by a previous test case
				os.Unsetenv("XDG_CONFIG_HOME")
			}

			gotDir, err := GetRalphGeneratedDir()
			if err != nil {
				t.Fatalf("GetRalphGeneratedDir() error: %v", err)
			}
			if gotDir != tt.expectedDir {
				t.Errorf("GetRalphGeneratedDir() = %s, want %s", gotDir, tt.expectedDir)
			}
		})
	}
	// Restore original XDG_CONFIG_HOME after all tests in this function are done
	unsetEnvVar(t, "XDG_CONFIG_HOME", origXdgConfig, xdgConfigWasSet)
}

func TestInjectSourceLines_DryRun_NoFile(t *testing.T) {
	tempDir := t.TempDir()
	// Point HOME to a temp dir to ensure GetRCFilePath resolves to a path we control
	origHome, homeWasSet := os.LookupEnv("HOME")
	setEnvVar(t, "HOME", tempDir)
	defer unsetEnvVar(t, "HOME", origHome, homeWasSet)

	rcFilePath := filepath.Join(tempDir, ".bashrc_dry_run_test")
	// Ensure file does not exist
	os.Remove(rcFilePath) // ignore error if not exists

	linesToInject := []string{"source /path/to/aliases.sh", "source /path/to/functions.sh"}

	var buf bytes.Buffer
	err := InjectSourceLines(&buf, Bash, linesToInject, true)
	output := buf.String()

	if err != nil {
		t.Errorf("InjectSourceLines (dry run, no file) returned error: %v", err)
	}

	// Check that RC file was NOT created
	if _, statErr := os.Stat(rcFilePath); !os.IsNotExist(statErr) {
		t.Errorf("InjectSourceLines dry run created rc file %s when it should not have", rcFilePath)
	}

	if !strings.Contains(output, "[DRY RUN] Would update rc file") {
		t.Errorf("Expected dry run output to contain 'Would update rc file', got: %s", output)
	}
	if !strings.Contains(output, linesToInject[0]) {
		t.Errorf(
			"Expected dry run output to contain injected line '%s', got: %s",
			linesToInject[0],
			output,
		)
	}
}

// More tests for InjectSourceLines (non-dry run, existing files, existing blocks, etc.)
// would go here. These require more complex file setup and content verification.

func TestEnsureRalphBlock_EmptyFile(t *testing.T) {
	got, modified := ensureRalphBlock([]string{""}, []string{"source ~/.x"})
	if !modified {
		t.Fatal("expected modified=true for empty file")
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, RalphBlockBeginMarker) ||
		!strings.Contains(joined, "source ~/.x") {
		t.Errorf("block not created correctly: %q", joined)
	}
}

func TestEnsureRalphBlock_AppendsWhenNoBlock(t *testing.T) {
	lines := []string{"export FOO=1", "alias ll='ls -l'"}
	got, modified := ensureRalphBlock(lines, []string{"source ~/.x"})
	if !modified {
		t.Fatal("expected modified=true when no block present")
	}
	if got[0] != "export FOO=1" {
		t.Errorf("existing content must be preserved, got %v", got)
	}
}

func TestEnsureRalphBlock_IdempotentWhenAlreadyCorrect(t *testing.T) {
	content := []string{"source ~/.x", "source ~/.y"}
	first, _ := ensureRalphBlock([]string{""}, content)
	// Re-running with the same content must be a no-op.
	_, modified := ensureRalphBlock(first, content)
	if modified {
		t.Errorf("expected idempotent no-op on identical content, got modified=true")
	}
}

func TestEnsureRalphBlock_IdempotentWithCommentsAndBlanks(t *testing.T) {
	// Content lines that include a comment and a blank line: re-applying must
	// not rewrite the block every run.
	content := []string{"# ralph", "", "source ~/.x"}
	first, _ := ensureRalphBlock([]string{"export FOO=1"}, content)
	_, modified := ensureRalphBlock(first, content)
	if modified {
		t.Errorf(
			"expected idempotent no-op even with comments/blanks in content, got modified=true",
		)
	}
}

func TestEnsureRalphBlock_MigratesLegacyMarkers(t *testing.T) {
	lines := []string{
		legacyBlockBeginMarker,
		"source ~/.x",
		legacyBlockEndMarker,
	}
	got, modified := ensureRalphBlock(lines, []string{"source ~/.x"})
	if !modified {
		t.Fatal("expected modified=true when migrating legacy markers")
	}
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, legacyBlockBeginMarker) {
		t.Errorf("legacy markers should be migrated to RALPH markers: %q", joined)
	}
	if !strings.Contains(joined, RalphBlockBeginMarker) {
		t.Errorf("expected RALPH markers after migration: %q", joined)
	}
}

func TestEnsureRalphBlock_RepairsMissingEndMarker(t *testing.T) {
	lines := []string{
		"export FOO=1",
		RalphBlockBeginMarker,
		"source ~/.old",
		// no end marker
	}
	got, modified := ensureRalphBlock(lines, []string{"source ~/.x"})
	if !modified {
		t.Fatal("expected modified=true for malformed block")
	}
	joined := strings.Join(got, "\n")
	if strings.Count(joined, RalphBlockBeginMarker) != 1 ||
		strings.Count(joined, RalphBlockEndMarker) != 1 {
		t.Errorf("expected exactly one well-formed block, got: %q", joined)
	}
	if strings.Contains(joined, "source ~/.old") {
		t.Errorf("stale content should be replaced: %q", joined)
	}
}

func TestEnsureRalphBlock_RewritesOnContentChange(t *testing.T) {
	first, _ := ensureRalphBlock([]string{""}, []string{"source ~/.x"})
	got, modified := ensureRalphBlock(first, []string{"source ~/.x", "source ~/.y"})
	if !modified {
		t.Fatal("expected modified=true when content changes")
	}
	if !strings.Contains(strings.Join(got, "\n"), "source ~/.y") {
		t.Errorf("new content not written: %v", got)
	}
}
