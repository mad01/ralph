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
	defer func() { _ = lock.Release() }()

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
	defer func() { _ = lock.Release() }()

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
	defer func() { _ = lock.Release() }()

	// flock is per open file description, so a second open of the same path
	// conflicts even within one process — this models a concurrent ralph run.
	second, err := Acquire()
	if err == nil {
		_ = second.Release()
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
	_ = again.Release()
}

// A dry run creates nothing: with no lock file yet, neither the file nor its
// directory appears, and the returned lock still releases cleanly.
func TestAcquireExistingCreatesNothing(t *testing.T) {
	p := withTempLockPath(t)

	lock, err := AcquireExisting()
	if err != nil {
		t.Fatalf("AcquireExisting() error: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release() error: %v", err)
	}

	if _, err := os.Lstat(filepath.Dir(p)); !os.IsNotExist(err) {
		t.Errorf("AcquireExisting created %s (stat err %v)", filepath.Dir(p), err)
	}
}

func TestAcquireExistingFailsWhileLockHeld(t *testing.T) {
	withTempLockPath(t)

	lock, err := Acquire()
	if err != nil {
		t.Fatalf("Acquire() error: %v", err)
	}
	defer func() { _ = lock.Release() }()

	second, err := AcquireExisting()
	if err == nil {
		_ = second.Release()
		t.Fatal("AcquireExisting() succeeded while the lock was held, want ErrLocked")
	}
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("AcquireExisting() error = %v, want ErrLocked", err)
	}
}

// With a lock file left by an earlier run, a dry run takes the lock without
// writing to the file, and a real run can take it after the release.
func TestAcquireExistingLeavesFileUnchanged(t *testing.T) {
	p := withTempLockPath(t)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	const earlier = "12345\n"
	if err := os.WriteFile(p, []byte(earlier), 0o644); err != nil {
		t.Fatal(err)
	}

	lock, err := AcquireExisting()
	if err != nil {
		t.Fatalf("AcquireExisting() error: %v", err)
	}
	if other, err := Acquire(); !errors.Is(err, ErrLocked) {
		_ = other.Release()
		t.Errorf("Acquire() during a dry run error = %v, want ErrLocked", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release() error: %v", err)
	}

	if data, _ := os.ReadFile(p); string(data) != earlier {
		t.Errorf("lock file content = %q, want it left at %q", data, earlier)
	}
	again, err := Acquire()
	if err != nil {
		t.Fatalf("Acquire() after the dry run released: %v", err)
	}
	_ = again.Release()
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
