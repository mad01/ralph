// Package lockfile serializes ralph runs with an advisory flock(2) on
// ~/.config/ralph/.lock. Both state files (.builds_state, .recipe_state) are
// written atomically, but two concurrent runs still race the read-modify-write
// cycle: each reads the same baseline and the last writer silently discards
// the other's updates. Holding the lock for the whole run (up/apply/clean)
// closes that window. Contention fails fast rather than blocking — overlapping
// runs are operator error (a cron/loop invocation crossing an interactive
// run), not a queue.
package lockfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
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
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}
	return filepath.Join(home, ".config", "ralph", ".lock"), nil
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
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf(
				"%w (lock held on %s) — wait for it to finish and retry",
				ErrLocked,
				p,
			)
		}
		return nil, fmt.Errorf("flock %s: %w", p, err)
	}
	// Best-effort PID breadcrumb so a human inspecting the file can find the
	// holder; correctness rests on the flock alone.
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return &Lock{f: f}, nil
}

// Release drops the lock. Safe to call more than once.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	if err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN); err != nil {
		l.f.Close()
		l.f = nil
		return fmt.Errorf("unlock: %w", err)
	}
	err := l.f.Close()
	l.f = nil
	return err
}
