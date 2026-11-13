package commands

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/ralph/internal/lockfile"
	"github.com/mad01/ralph/internal/paths"
)

// lockingDryRuns are dry runs of every command that takes the run lock.
var lockingDryRuns = [][]string{
	{"up", "--dry-run"},
	{"apply", "--dry-run"},
	{"sync", "--dry-run"},
	{"clean", "--dry-run"},
}

// holdRunLock takes the run lock the way a concurrent real run would, and
// releases it when the test ends.
func holdRunLock(t *testing.T) {
	t.Helper()
	lock, err := lockfile.Acquire()
	if err != nil {
		t.Fatalf("hold run lock: %v", err)
	}
	t.Cleanup(func() { _ = lock.Release() })
}

// freshStateDir points the state directory at a path that does not exist yet.
func freshStateDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	t.Setenv(paths.StateDirEnv, dir)
	return dir
}

// `ralph sync` pulls the dotfiles repo, recipe sources and packages, so it
// must not run beside another run: with the lock held it fails fast and pulls
// nothing.
func TestSync_FailsFastWhileRunLockHeld(t *testing.T) {
	f := newSequenceFixture(t)
	holdRunLock(t)

	res := runRalph(t, "sync")

	if !errors.Is(res.err, lockfile.ErrLocked) {
		t.Errorf("sync error = %v, want ErrLocked", res.err)
	}
	if f.synced() {
		t.Error("sync pulled the dotfiles repo while another run held the lock")
	}
}

// A dry run writes nothing, the run lock file and its directory included.
func TestDryRun_CreatesNoLockFile(t *testing.T) {
	for _, args := range lockingDryRuns {
		t.Run(args[0], func(t *testing.T) {
			newSequenceFixture(t)
			dir := freshStateDir(t)

			res := runRalph(t, args...)
			if res.err != nil {
				t.Fatalf("ralph %v failed: %v\nstderr: %s", args, res.err, res.stderr)
			}

			if _, err := os.Lstat(dir); !os.IsNotExist(err) {
				entries, _ := os.ReadDir(dir)
				t.Errorf("dry run created the state dir %s (entries %v)", dir, entries)
			}
		})
	}
}

// A dry run still fails fast while a real run holds the lock, so its preview
// never reads state a real run is halfway through writing.
func TestDryRun_FailsFastWhileRunLockHeld(t *testing.T) {
	for _, args := range lockingDryRuns {
		t.Run(args[0], func(t *testing.T) {
			newSequenceFixture(t)
			holdRunLock(t)

			res := runRalph(t, args...)

			if !errors.Is(res.err, lockfile.ErrLocked) {
				t.Errorf("ralph %v error = %v, want ErrLocked", args, res.err)
			}
		})
	}
}

// A real run creates the lock file in a fresh state dir and holds it.
func TestRealRun_CreatesLockFile(t *testing.T) {
	newSequenceFixture(t)
	dir := freshStateDir(t)

	res := runRalph(t, "apply")
	if res.err != nil {
		t.Fatalf("apply failed: %v\nstderr: %s", res.err, res.stderr)
	}

	if _, err := os.Stat(filepath.Join(dir, ".lock")); err != nil {
		t.Errorf("apply did not create the run lock: %v", err)
	}
}
