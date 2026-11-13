package paths

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// isolate gives the test a fresh HOME and clears every variable the resolver
// reads, so the caller's shell environment never leaks in.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv(ConfigEnv, "")
	t.Setenv(StateDirEnv, "")
	return home
}

// resetLegacyWarning lets a test observe the once-per-process warning again.
func resetLegacyWarning(t *testing.T) {
	t.Helper()
	legacyWarning = sync.Once{}
	t.Cleanup(func() { legacyWarning = sync.Once{} })
}

func TestConfigFile(t *testing.T) {
	home := isolate(t)
	xdg := filepath.Join(home, "xdg")

	tests := []struct {
		name      string
		ralphConf string
		xdgHome   string
		want      string
	}{
		{"default", "", "", filepath.Join(home, ".config", "ralph", "config.toml")},
		{"xdg", "", xdg, filepath.Join(xdg, "ralph", "config.toml")},
		{"ralph config wins over xdg", "/tmp/scratch/my.toml", xdg, "/tmp/scratch/my.toml"},
		{
			"ralph config expands ~",
			"~/dots/ralph.toml",
			"",
			filepath.Join(home, "dots", "ralph.toml"),
		},
		{"ralph config need not exist", "/does/not/exist.toml", "", "/does/not/exist.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ConfigEnv, tt.ralphConf)
			t.Setenv("XDG_CONFIG_HOME", tt.xdgHome)
			got, err := ConfigFile()
			if err != nil {
				t.Fatalf("ConfigFile() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ConfigFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestStateDir pins the three resolution rules: RALPH_STATE_DIR, else the
// directory of RALPH_CONFIG, else the directory of the default config file.
func TestStateDir(t *testing.T) {
	home := isolate(t)
	scratch := t.TempDir()
	explicit := t.TempDir()
	xdg := filepath.Join(home, "xdg")

	tests := []struct {
		name      string
		stateDir  string
		ralphConf string
		xdgHome   string
		want      string
	}{
		{"default", "", "", "", filepath.Join(home, ".config", "ralph")},
		{"default honours xdg", "", "", xdg, filepath.Join(xdg, "ralph")},
		{
			"ralph config picks its directory",
			"", filepath.Join(scratch, "cfg.toml"), "",
			scratch,
		},
		{
			"ralph config inside the default directory",
			"", filepath.Join(home, ".config", "ralph", "other.toml"), "",
			filepath.Join(home, ".config", "ralph"),
		},
		{
			"ralph state dir wins over ralph config",
			explicit, filepath.Join(scratch, "cfg.toml"), "",
			explicit,
		},
		{"ralph state dir expands ~", "~/state", "", "", filepath.Join(home, "state")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(StateDirEnv, tt.stateDir)
			t.Setenv(ConfigEnv, tt.ralphConf)
			t.Setenv("XDG_CONFIG_HOME", tt.xdgHome)
			got, err := StateDir()
			if err != nil {
				t.Fatalf("StateDir() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("StateDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStateDir_RelativePathsBecomeAbsolute(t *testing.T) {
	isolate(t)
	cwd := t.TempDir()
	t.Chdir(cwd)
	wantCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv(ConfigEnv, "cfg.toml")
	if got, err := StateDir(); err != nil || got != wantCwd {
		t.Errorf("StateDir() with relative RALPH_CONFIG = %q, %v; want %q", got, err, wantCwd)
	}

	t.Setenv(StateDirEnv, "state")
	want := filepath.Join(wantCwd, "state")
	if got, err := StateDir(); err != nil || got != want {
		t.Errorf("StateDir() with relative RALPH_STATE_DIR = %q, %v; want %q", got, err, want)
	}
}

func TestWarnLegacyState(t *testing.T) {
	home := isolate(t)
	legacy := filepath.Join(home, ".config", "ralph")

	tests := []struct {
		name         string
		legacyState  bool
		dir          string
		dirHasState  bool
		wantWarnings bool
	}{
		{"legacy state, new dir empty", true, t.TempDir(), false, true},
		{"legacy state, new dir has state", true, t.TempDir(), true, false},
		{"no legacy state", false, t.TempDir(), false, false},
		{"new dir is the legacy dir", true, legacy, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.RemoveAll(legacy)
			if tt.legacyState {
				writeFile(t, filepath.Join(legacy, RecipeStateFile))
			}
			if tt.dirHasState {
				writeFile(t, filepath.Join(tt.dir, RecipeStateFile))
			}
			var buf bytes.Buffer
			warnLegacyState(&buf, tt.dir)
			if got := buf.Len() > 0; got != tt.wantWarnings {
				t.Fatalf("warned = %v, want %v; output %q", got, tt.wantWarnings, buf.String())
			}
			if !tt.wantWarnings {
				return
			}
			for _, want := range []string{tt.dir, legacy, StateDirEnv + "=" + legacy} {
				if !strings.Contains(buf.String(), want) {
					t.Errorf("warning missing %q:\n%s", want, buf.String())
				}
			}
		})
	}
}

// TestStateDir_LegacyWarningOncePerRunOnStderr checks the warning reaches
// stderr exactly once however often a run resolves the state directory, never
// stdout (which must stay pure JSON under -o json), and not at all when
// RALPH_STATE_DIR names the directory.
func TestStateDir_LegacyWarningOncePerRunOnStderr(t *testing.T) {
	home := isolate(t)
	writeFile(t, filepath.Join(home, ".config", "ralph", RecipeStateFile))
	t.Setenv(ConfigEnv, filepath.Join(t.TempDir(), "cfg.toml"))

	t.Run("ralph config elsewhere", func(t *testing.T) {
		resetLegacyWarning(t)
		stdout, stderr := captureStdio(t, func() {
			for range 3 {
				if _, err := StateDir(); err != nil {
					t.Fatalf("StateDir() error = %v", err)
				}
			}
		})
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		if n := strings.Count(stderr, "Warning:"); n != 1 {
			t.Errorf("stderr carries %d warnings, want 1:\n%s", n, stderr)
		}
	})

	t.Run("ralph state dir set", func(t *testing.T) {
		resetLegacyWarning(t)
		t.Setenv(StateDirEnv, t.TempDir())
		stdout, stderr := captureStdio(t, func() {
			if _, err := StateDir(); err != nil {
				t.Fatalf("StateDir() error = %v", err)
			}
		})
		if stdout != "" || stderr != "" {
			t.Errorf("stdout = %q, stderr = %q; want both empty", stdout, stderr)
		}
	})
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// captureStdio runs fn with os.Stdout and os.Stderr redirected to pipes and
// returns what each received.
func captureStdio(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()

	fn()

	_ = outW.Close()
	_ = errW.Close()
	outBytes, _ := io.ReadAll(outR)
	errBytes, _ := io.ReadAll(errR)
	return string(outBytes), string(errBytes)
}
