// Package lockfile serializes ralph runs with an advisory flock(2) on .lock in
// the state directory (paths.StateDir). Both state files (.builds_state, .recipe_state) are
// written atomically, but two concurrent runs still race the read-modify-write
// cycle: each reads the same baseline and the last writer silently discards
// the other's updates. Holding the lock for the whole run (up/apply/sync/clean)
// closes that window; sync holds it because it pulls the same checkouts up
// does. A dry run takes it with AcquireExisting, which writes nothing.
// Contention fails fast rather than blocking: overlapping
// runs are operator error (a cron/loop invocation crossing an interactive
// run), not a queue.
package lockfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/mad01/ralph/internal/paths"
)

// ErrLocked reports that another ralph process holds the run lock.
var ErrLocked = errors.New("another ralph run is in progress")

// Lock is a held run lock. Release it when the run finishes; the lock is also
// dropped automatically by the kernel if the process dies, so a crashed run
// never wedges subsequent runs.
type Lock struct {
	f *os.File
}

// GetLockPath returns the path of the run lock file.
// It is a variable to allow tests to override the path without mutating $HOME.
var GetLockPath = getLockPathInternal

func getLockPathInternal() (string, error) {
	dir, err := paths.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".lock"), nil
}

// Acquire takes an exclusive, non-blocking advisory lock. On contention it
// returns an error wrapping ErrLocked with a user-facing message; callers can
// pass it straight up. The lock file itself is never removed — only the flock
// matters, and deleting it would race other acquirers.
func Acquire() (*Lock, error) {
	p, err := GetLockPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := flock(f, p); err != nil {
		return nil, err
	}
	// Best-effort PID breadcrumb so a human inspecting the file can find the
	// holder; correctness rests on the flock alone.
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return &Lock{f: f}, nil
}

// AcquireExisting is Acquire for a dry run, which must write nothing. It
// opens the lock file read-only and never creates it or its directory, and it
// leaves the PID breadcrumb alone. Contention still fails with ErrLocked, so a
// dry run never previews state a real run is halfway through writing. With no
// lock file yet no run can hold the lock, so it returns a Lock that holds
// nothing; Release on it is a no-op.
func AcquireExisting() (*Lock, error) {
	p, err := GetLockPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return &Lock{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := flock(f, p); err != nil {
		return nil, err
	}
	return &Lock{f: f}, nil
}

// flock takes an exclusive, non-blocking flock on f, the lock file at p. On
// failure it closes f; contention comes back wrapping ErrLocked.
func flock(f *os.File, p string) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return nil
	}
	_ = f.Close()
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return fmt.Errorf(
			"%w (lock held on %s) — wait for it to finish and retry",
			ErrLocked,
			p,
		)
	}
	return fmt.Errorf("flock %s: %w", p, err)
}

// Release drops the lock. Safe to call more than once.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	if err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN); err != nil {
		_ = l.f.Close()
		l.f = nil
		return fmt.Errorf("unlock: %w", err)
	}
	err := l.f.Close()
	l.f = nil
	return err
}
