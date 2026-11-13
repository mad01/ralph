package paths_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/lockfile"
	"github.com/mad01/ralph/internal/paths"
	"github.com/mad01/ralph/internal/shell"
	"github.com/mad01/ralph/internal/state"
)

// TestScratchConfigLeavesRealStateAlone reproduces the scratch-config bug: with
// RALPH_CONFIG pointing at a config outside ~/.config/ralph, loading and saving
// recipe state and build state and taking the run lock used to read and write
// the real files under ~/.config/ralph. Now every state path lands in the
// scratch config's directory and the real files stay byte-identical.
func TestScratchConfigLeavesRealStateAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv(paths.StateDirEnv, "")

	realDir := filepath.Join(home, ".config", "ralph")
	realFiles := map[string][]byte{
		filepath.Join(realDir, ".recipe_state"): []byte(
			`{"version":1,"recipes":{"real":{"symlinks":["/home/real/.zshrc"]}}}`,
		),
		filepath.Join(realDir, ".builds_state"): []byte(
			`{"builds":{"real-build":{"completed_at":"2026-01-01T00:00:00Z"}}}`,
		),
	}
	for p, data := range realFiles {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	scratch := t.TempDir()
	t.Setenv(paths.ConfigEnv, filepath.Join(scratch, "config.toml"))

	rs, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if _, ok := rs.Recipes["real"]; ok {
		t.Fatal("state.Load read the real recipe state")
	}
	rs.AddArtifact("scratch", state.KindSymlink, filepath.Join(scratch, "link"))
	if err := state.Save(rs); err != nil {
		t.Fatalf("state.Save: %v", err)
	}

	bs, err := buildstate.LoadBuildState()
	if err != nil {
		t.Fatalf("LoadBuildState: %v", err)
	}
	if _, ok := bs.Builds["real-build"]; ok {
		t.Fatal("LoadBuildState read the real build state")
	}
	bs.Builds["scratch-build"] = buildstate.BuildRecord{CompletedAt: time.Now()}
	if err := buildstate.SaveBuildState(bs); err != nil {
		t.Fatalf("SaveBuildState: %v", err)
	}

	lock, err := lockfile.Acquire()
	if err != nil {
		t.Fatalf("lockfile.Acquire: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	for _, name := range []string{".recipe_state", ".builds_state", ".lock"} {
		if _, err := os.Stat(filepath.Join(scratch, name)); err != nil {
			t.Errorf("%s not written to the scratch dir: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(realDir, ".lock")); !os.IsNotExist(err) {
		t.Errorf("lock created in the real dir (stat err = %v)", err)
	}
	for p, want := range realFiles {
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("reading %s: %v", p, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s changed:\n got %s\nwant %s", p, got, want)
		}
	}

	// The other paths under the state directory follow it too, so a scratch
	// run never rewrites the real generated scripts or package and source
	// caches.
	dirs := map[string]func() (string, error){
		"generated dir": shell.GetRalphGeneratedDir,
		"sources dir":   config.SourcesDir,
		"packages dir":  config.DefaultPackagesDir,
	}
	for name, resolve := range dirs {
		got, err := resolve()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.HasPrefix(got, scratch+string(os.PathSeparator)) {
			t.Errorf("%s = %s, want it under the scratch dir %s", name, got, scratch)
		}
	}
}
