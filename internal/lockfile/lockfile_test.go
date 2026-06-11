package lockfile

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// withTempLockPath points GetLockPath at a file under t.TempDir() so tests
// never touch the real ~/.config/ralph/.lock.
func withTempLockPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ralph", ".lock")
	orig := GetLockPath
	GetLockPath = func() (string, error) { return p, nil }
	t.Cleanup(func() { GetLockPath = orig })
	return p
}

func TestAcquireCreatesLockDirAndFile(t *testing.T) {
	p := withTempLockPath(t)

	lock, err := Acquire()
	if err != nil {
		t.Fatalf("Acquire() error: %v", err)
	}
	defer lock.Release()

	if _, err := os.Stat(p); err != nil {
		t.Fatalf("lock file not created at %s: %v", p, err)
	}
}

func TestAcquireWritesPID(t *testing.T) {
	p := withTempLockPath(t)

	lock, err := Acquire()
	if err != nil {
		t.Fatalf("Acquire() error: %v", err)
	}
	defer lock.Release()

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading lock file: %v", err)
	}
	want := strconv.Itoa(os.Getpid()) + "\n"
	if string(data) != want {
		t.Errorf("lock file content = %q, want %q", data, want)
	}
}

func TestSecondAcquireFailsWithErrLocked(t *testing.T) {
	withTempLockPath(t)

	lock, err := Acquire()
	if err != nil {
		t.Fatalf("first Acquire() error: %v", err)
	}
	defer lock.Release()

	// flock is per open file description, so a second open of the same path
	// conflicts even within one process — this models a concurrent ralph run.
	second, err := Acquire()
	if err == nil {
		second.Release()
		t.Fatal("second Acquire() succeeded, want ErrLocked")
	}
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("second Acquire() error = %v, want ErrLocked", err)
	}
}

func TestReleaseAllowsReacquire(t *testing.T) {
	withTempLockPath(t)

	lock, err := Acquire()
	if err != nil {
		t.Fatalf("Acquire() error: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release() error: %v", err)
	}

	again, err := Acquire()
	if err != nil {
		t.Fatalf("re-Acquire() after Release error: %v", err)
	}
	again.Release()
}

func TestReleaseIsIdempotent(t *testing.T) {
	withTempLockPath(t)

	lock, err := Acquire()
	if err != nil {
		t.Fatalf("Acquire() error: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("first Release() error: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("second Release() error: %v", err)
	}
}
