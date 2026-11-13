package packages

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/testutil"
)

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what
// was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stderr = orig }()
	fn()
	_ = w.Close()
	return <-done
}

// A corrupt .builds_state makes every package rebuild on each run. The
// per-package notes go to the verbose writer only, so a normal run must still
// say why on stderr, once per run however many packages rebuild.
func TestBuildPackage_CorruptBuildStateWarnsOnceOnStderr(t *testing.T) {
	home := testutil.WithHome(t)
	statePath := filepath.Join(testutil.EnsureRalphDir(t, home), ".builds_state")
	if err := os.WriteFile(statePath, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt build state: %v", err)
	}
	buildStateWarning = new(sync.Once)
	t.Cleanup(func() { buildStateWarning = new(sync.Once) })

	stderr := captureStderr(t, func() {
		for _, name := range []string{"one", "two"} {
			pkg := config.Package{Source: "local", WorkingDir: t.TempDir(), Build: []string{"true"}}
			result := BuildPackage(context.Background(), io.Discard, name, pkg, BuildOptions{})
			if result.Action != "built" {
				t.Errorf(
					"package %s: action = %q (%v), want built",
					name,
					result.Action,
					result.Err,
				)
			}
		}
	})

	if n := strings.Count(stderr, statePath); n != 1 {
		t.Errorf("stderr names %s %d times, want once:\n%s", statePath, n, stderr)
	}
	if !strings.Contains(stderr, "ralph up --reset-builds") {
		t.Errorf("stderr warning should name the fix, got:\n%s", stderr)
	}
}
