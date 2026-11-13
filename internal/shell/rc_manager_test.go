package shell

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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

	_ = os.MkdirAll(filepath.Join(tempHome, ".config", "fish"), 0o755)
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
				_ = os.MkdirAll(tt.zdotdir, 0o755) // Ensure ZDOTDIR exists if set
				defer os.RemoveAll(tt.zdotdir)
			} else {
				// Ensure ZDOTDIR is unset if test requires it
				origZdotdir, zdotdirWasSet = os.LookupEnv("ZDOTDIR")
				if zdotdirWasSet {
					_ = os.Unsetenv("ZDOTDIR")
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
		_ = os.Unsetenv("XDG_CONFIG_HOME")
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
				_ = os.Unsetenv("XDG_CONFIG_HOME")
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

// mustEnsure runs ensureRalphBlock on input that must not be malformed.
func mustEnsure(t *testing.T, lines, contentLines []string) ([]string, bool) {
	t.Helper()
	got, modified, err := ensureRalphBlock(lines, contentLines)
	if err != nil {
		t.Fatalf("ensureRalphBlock: %v", err)
	}
	return got, modified
}

func TestEnsureRalphBlock_EmptyFile(t *testing.T) {
	got, modified := mustEnsure(t, []string{""}, []string{"source ~/.x"})
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
	got, modified := mustEnsure(t, lines, []string{"source ~/.x"})
	if !modified {
		t.Fatal("expected modified=true when no block present")
	}
	if got[0] != "export FOO=1" {
		t.Errorf("existing content must be preserved, got %v", got)
	}
}

func TestEnsureRalphBlock_IdempotentWhenAlreadyCorrect(t *testing.T) {
	content := []string{"source ~/.x", "source ~/.y"}
	first, _ := mustEnsure(t, []string{""}, content)
	// Re-running with the same content must be a no-op.
	_, modified := mustEnsure(t, first, content)
	if modified {
		t.Errorf("expected idempotent no-op on identical content, got modified=true")
	}
}

func TestEnsureRalphBlock_IdempotentWithCommentsAndBlanks(t *testing.T) {
	// Content lines that include a comment and a blank line: re-applying must
	// not rewrite the block every run.
	content := []string{"# ralph", "", "source ~/.x"}
	first, _ := mustEnsure(t, []string{"export FOO=1"}, content)
	_, modified := mustEnsure(t, first, content)
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
	got, modified := mustEnsure(t, lines, []string{"source ~/.x"})
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

func TestEnsureRalphBlock_MissingEndMarkerIsAnError(t *testing.T) {
	lines := []string{
		"export FOO=1",
		RalphBlockBeginMarker,
		"source ~/.old",
		"alias keep='yes'",
		// no end marker
	}
	_, _, err := ensureRalphBlock(lines, []string{"source ~/.x"})
	if !errors.Is(err, ErrMalformedBlock) {
		t.Fatalf("want ErrMalformedBlock for a BEGIN without END, got %v", err)
	}
}

func TestEnsureRalphBlock_RewritesOnContentChange(t *testing.T) {
	first, _ := mustEnsure(t, []string{""}, []string{"source ~/.x"})
	got, modified, err := ensureRalphBlock(first, []string{"source ~/.x", "source ~/.y"})
	if err != nil {
		t.Fatalf("ensureRalphBlock: %v", err)
	}
	if !modified {
		t.Fatal("expected modified=true when content changes")
	}
	if !strings.Contains(strings.Join(got, "\n"), "source ~/.y") {
		t.Errorf("new content not written: %v", got)
	}
}

// bashRCInTempHome points HOME at a fresh temp dir, so InjectSourceLines
// resolves the bash rc file inside it, and returns the home dir and rc path.
func bashRCInTempHome(t *testing.T) (home, rc string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	return home, filepath.Join(home, ".bashrc")
}

func writeTestFile(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	// WriteFile applies the umask; set the mode the test asks for.
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// dirNames returns the sorted names in dir.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

func assertDirNames(t *testing.T, dir string, want ...string) {
	t.Helper()
	if got := dirNames(t, dir); !slices.Equal(got, want) {
		t.Errorf("files in %s = %v, want %v", dir, got, want)
	}
}

// A file whose markers do not pair up into one block is never edited: ralph
// cannot tell its own lines from the user's, so it refuses with an error that
// names the file and the fix, and writes nothing (no rc change, no backup).
func TestInjectSourceLines_MalformedBlockRefusesAndWritesNothing(t *testing.T) {
	const (
		begin = RalphBlockBeginMarker
		end   = RalphBlockEndMarker
	)
	tests := []struct {
		name       string
		content    string
		wantLine   string
		wantPhrase string
	}{
		{
			name:       "missing END",
			content:    "export FOO=1\n" + begin + "\nsource ~/.old\nalias keep='yes'\nexport AFTER=1\n",
			wantLine:   "line 2",
			wantPhrase: "after it",
		},
		{
			name:       "missing BEGIN",
			content:    "export FOO=1\nsource ~/.old\n" + end + "\nalias keep='yes'\n",
			wantLine:   "line 3",
			wantPhrase: "before it",
		},
		{
			name:       "END before BEGIN",
			content:    end + "\nalias keep='yes'\n" + begin + "\nsource ~/.old\n",
			wantLine:   "line 1",
			wantPhrase: "before it",
		},
		{
			name:       "nested BEGIN",
			content:    begin + "\nalias keep='yes'\n" + begin + "\nsource ~/.old\n" + end + "\n",
			wantLine:   "line 3",
			wantPhrase: "still open",
		},
		{
			name: "duplicated block",
			content: begin + "\nsource ~/.a\n" + end + "\nalias keep='yes'\n" +
				begin + "\nsource ~/.b\n" + end + "\n",
			wantLine:   "line 5",
			wantPhrase: "second block",
		},
		{
			name:       "duplicated END",
			content:    begin + "\nsource ~/.a\n" + end + "\nalias keep='yes'\n" + end + "\n",
			wantLine:   "line 5",
			wantPhrase: "before it",
		},
	}
	for _, tt := range tests {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dry-run=%t", tt.name, dryRun), func(t *testing.T) {
				home, rc := bashRCInTempHome(t)
				writeTestFile(t, rc, tt.content, 0o644)

				err := InjectSourceLines(io.Discard, Bash, []string{"source ~/.x"}, dryRun)
				switch {
				case err == nil:
					t.Error("want an error for a malformed block, got nil")
				case !errors.Is(err, ErrMalformedBlock):
					t.Errorf("want ErrMalformedBlock, got %v", err)
				default:
					for _, want := range []string{
						rc,
						tt.wantLine,
						tt.wantPhrase,
						"restore the missing marker or delete the stray one",
					} {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("error %q does not mention %q", err, want)
						}
					}
				}
				if got := readTestFile(t, rc); got != tt.content {
					t.Errorf("rc file changed:\n got %q\nwant %q", got, tt.content)
				}
				assertDirNames(t, home, ".bashrc")
			})
		}
	}
}

func TestInjectSourceLines_ReplacesWellFormedBlockAndKeepsUserLines(t *testing.T) {
	home, rc := bashRCInTempHome(t)
	before := "export FOO=1\n" +
		RalphBlockBeginMarker + "\nsource ~/.old\n" + RalphBlockEndMarker +
		"\nalias after='kept'\nexport LAST=1\n"
	writeTestFile(t, rc, before, 0o640)

	if err := InjectSourceLines(io.Discard, Bash, []string{"source ~/.new"}, false); err != nil {
		t.Fatalf("InjectSourceLines: %v", err)
	}

	want := "export FOO=1\n" +
		RalphBlockBeginMarker + "\nsource ~/.new\n" + RalphBlockEndMarker +
		"\nalias after='kept'\nexport LAST=1\n"
	if got := readTestFile(t, rc); got != want {
		t.Errorf("rc file:\n got %q\nwant %q", got, want)
	}
	if mode := fileMode(t, rc); mode != 0o640 {
		t.Errorf("rc file mode = %o, want 640 (kept from the original)", mode)
	}
	// Only the rc file and its backup: no temp file is left behind.
	assertDirNames(t, home, ".bashrc", ".bashrc.ralph.bak")
}

// Dotfiles users often link ~/.bashrc into their repo. The write goes to the
// link's destination, so the link stays a link, and the backup stays next to
// the link instead of landing in the repo.
func TestInjectSourceLines_SymlinkedRCFileStaysALink(t *testing.T) {
	home, rc := bashRCInTempHome(t)
	repo := t.TempDir()
	dest := filepath.Join(repo, "bashrc")
	before := "export FOO=1\n"
	writeTestFile(t, dest, before, 0o640)
	if err := os.Symlink(dest, rc); err != nil {
		t.Fatal(err)
	}

	if err := InjectSourceLines(io.Discard, Bash, []string{"source ~/.a"}, false); err != nil {
		t.Fatalf("InjectSourceLines: %v", err)
	}

	info, err := os.Lstat(rc)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("rc file was replaced by a %v, want the symlink kept", info.Mode().Type())
	}
	if link, _ := os.Readlink(rc); link != dest {
		t.Errorf("symlink now points at %q, want %q", link, dest)
	}
	got := readTestFile(t, dest)
	if !strings.HasPrefix(got, before) || !strings.Contains(got, "source ~/.a") {
		t.Errorf("link destination not updated in place: %q", got)
	}
	if mode := fileMode(t, dest); mode != 0o640 {
		t.Errorf("destination mode = %o, want 640", mode)
	}
	assertDirNames(t, repo, "bashrc")
	assertDirNames(t, home, ".bashrc", ".bashrc.ralph.bak")
	if bak := readTestFile(t, rc+".ralph.bak"); bak != before {
		t.Errorf("backup = %q, want the previous content %q", bak, before)
	}
}

// A link whose destination is gone is refused: writing through it would
// create a file wherever it points, and replacing it would drop the link.
func TestInjectSourceLines_DanglingSymlinkRefuses(t *testing.T) {
	home, rc := bashRCInTempHome(t)
	dest := filepath.Join(t.TempDir(), "bashrc")
	if err := os.Symlink(dest, rc); err != nil {
		t.Fatal(err)
	}

	err := InjectSourceLines(io.Discard, Bash, []string{"source ~/.a"}, false)
	if err == nil || !strings.Contains(err.Error(), rc) {
		t.Fatalf("want an error naming %s, got %v", rc, err)
	}
	if _, statErr := os.Lstat(dest); !os.IsNotExist(statErr) {
		t.Errorf("a file was created at the link destination %s", dest)
	}
	if link, _ := os.Readlink(rc); link != dest {
		t.Errorf("symlink changed: now %q, want %q", link, dest)
	}
	assertDirNames(t, home, ".bashrc")
}

func TestInjectSourceLines_BackupHoldsPreviousContent(t *testing.T) {
	home, rc := bashRCInTempHome(t)
	bak := rc + ".ralph.bak"
	original := "export SECRET=1\n"
	writeTestFile(t, rc, original, 0o600)

	if err := InjectSourceLines(io.Discard, Bash, []string{"source ~/.a"}, false); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if got := readTestFile(t, bak); got != original {
		t.Errorf("backup after first run = %q, want %q", got, original)
	}
	if mode := fileMode(t, bak); mode != 0o600 {
		t.Errorf("backup mode = %o, want 600 (same as the rc file)", mode)
	}

	// The backup has one fixed name: the next changing run overwrites it
	// with the content that run replaced.
	second := readTestFile(t, rc)
	if err := InjectSourceLines(io.Discard, Bash, []string{"source ~/.b"}, false); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if got := readTestFile(t, bak); got != second {
		t.Errorf("backup after second run = %q, want %q", got, second)
	}
	assertDirNames(t, home, ".bashrc", ".bashrc.ralph.bak")
}

func TestInjectSourceLines_NoBackupWhenNothingChanges(t *testing.T) {
	home, rc := bashRCInTempHome(t)
	bak := rc + ".ralph.bak"
	upToDate := "export FOO=1\n\n" +
		RalphBlockBeginMarker + "\nsource ~/.a\n" + RalphBlockEndMarker + "\n"
	writeTestFile(t, rc, upToDate, 0o644)

	// Already up to date: no write, no backup.
	if err := InjectSourceLines(io.Discard, Bash, []string{"source ~/.a"}, false); err != nil {
		t.Fatalf("no-op run: %v", err)
	}
	if got := readTestFile(t, rc); got != upToDate {
		t.Errorf("rc file changed on a no-op run: %q", got)
	}
	assertDirNames(t, home, ".bashrc")

	// A changing run backs up; the no-op run after it leaves that backup be.
	if err := InjectSourceLines(io.Discard, Bash, []string{"source ~/.b"}, false); err != nil {
		t.Fatalf("changing run: %v", err)
	}
	changed := readTestFile(t, rc)
	if err := InjectSourceLines(io.Discard, Bash, []string{"source ~/.b"}, false); err != nil {
		t.Fatalf("second no-op run: %v", err)
	}
	if got := readTestFile(t, rc); got != changed {
		t.Errorf("rc file changed on a no-op run: %q", got)
	}
	if got := readTestFile(t, bak); got != upToDate {
		t.Errorf("backup = %q, want the content before the changing run %q", got, upToDate)
	}
}

// When the backup cannot be written, the rc file is not touched either.
func TestInjectSourceLines_FailedBackupLeavesRCFileUnchanged(t *testing.T) {
	_, rc := bashRCInTempHome(t)
	original := "export FOO=1\n"
	writeTestFile(t, rc, original, 0o644)
	// A non-empty directory at the backup path makes the rename fail.
	blocker := filepath.Join(rc+".ralph.bak", "keep")
	if err := os.MkdirAll(blocker, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := InjectSourceLines(io.Discard, Bash, []string{"source ~/.a"}, false); err == nil {
		t.Fatal("want an error when the backup cannot be written, got nil")
	}
	if got := readTestFile(t, rc); got != original {
		t.Errorf("rc file changed although the backup failed: %q", got)
	}
}

func TestInjectSourceLines_DryRunWritesNothing(t *testing.T) {
	home, rc := bashRCInTempHome(t)
	original := "export FOO=1\n"
	writeTestFile(t, rc, original, 0o644)

	var out bytes.Buffer
	if err := InjectSourceLines(&out, Bash, []string{"source ~/.a"}, true); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if got := readTestFile(t, rc); got != original {
		t.Errorf("dry run changed the rc file: %q", got)
	}
	assertDirNames(t, home, ".bashrc")
	if !strings.Contains(out.String(), "[DRY RUN] Would update rc file") {
		t.Errorf("dry run output does not say it would update: %s", out.String())
	}
}
