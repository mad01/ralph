// Package execx runs external commands so that a timeout or an interrupt
// stops the command and every process it started, and so that waiting for a
// command is always bounded.
//
// Each command starts in a new session, which makes it the leader of its own
// process group: one signal to the group reaches a shell, the make it runs and
// anything either left in the background. The new session also detaches the
// command from ralph's controlling terminal, so the terminal's Ctrl-C reaches
// ralph only; the CLI forwards it with TerminateAll. A command that opens
// /dev/tty to prompt (sudo, an ssh host-key question, a git credential prompt)
// fails at once instead of stopping on SIGTTIN until its timeout.
package execx

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// DefaultTimeout bounds a command that has no timeout of its own: a build or
// package without a timeout field, a lifecycle hook, or a git network call.
const DefaultTimeout = 10 * time.Minute

// pollInterval is how often a terminating process group is checked for exit.
const pollInterval = 20 * time.Millisecond

// killGrace is how long a process group gets to exit after SIGTERM before it
// gets SIGKILL. A var so tests can shorten it.
var killGrace = 2 * time.Second

// waitDelay bounds how long Wait keeps reading a command's output once the
// command exited (or was stopped) while a process it started still holds the
// output pipe. A var so tests can shorten it.
var waitDelay = 2 * time.Second

// ErrStopped is returned by Start once TerminateAll has run: ralph is exiting
// and must not start a command it would leave behind.
var ErrStopped = errors.New("not starting command: ralph is stopping")

// running holds the process group of every started command that has not been
// waited for, so TerminateAll can reach them.
var running = struct {
	sync.Mutex
	stopping bool
	groups   map[int]struct{}
}{groups: map[int]struct{}{}}

// Cmd is an exec.Cmd that runs in its own session and process group. When its
// context is done the whole group gets SIGTERM, then SIGKILL after a short
// grace, and Wait returns within a bounded time even when a background
// process holds the output pipe. Use Cmd's own Start, Wait, Run, Output and
// CombinedOutput: they track the process for TerminateAll.
type Cmd struct {
	*exec.Cmd
}

// Command returns a Cmd that runs name with args under ctx. Callers set Dir,
// Env, Stdout and Stderr on it as on an exec.Cmd. Stdin stays /dev/null unless
// set.
func Command(ctx context.Context, name string, args ...string) *Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return terminate(cmd.Process.Pid) }
	cmd.WaitDelay = waitDelay
	return &Cmd{Cmd: cmd}
}

// Start starts the command and records its process group.
func (c *Cmd) Start() error {
	running.Lock()
	defer running.Unlock()
	if running.stopping {
		return ErrStopped
	}
	if err := c.Cmd.Start(); err != nil {
		return err
	}
	running.groups[c.Process.Pid] = struct{}{}
	return nil
}

// Wait waits for the command to exit. A command that exited 0 while a process
// it left in the background still held its output past waitDelay counts as a
// success: the command itself finished, and the leftover process is its own.
func (c *Cmd) Wait() error {
	err := c.Cmd.Wait()
	if c.Process != nil {
		running.Lock()
		delete(running.groups, c.Process.Pid)
		running.Unlock()
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		return nil
	}
	return err
}

// Run starts the command and waits for it to finish.
func (c *Cmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

// Output runs the command and returns its standard output. When Stderr is not
// set, a failure's *exec.ExitError carries the command's standard error, as
// with exec.Cmd.Output.
func (c *Cmd) Output() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	captureStderr := c.Stderr == nil
	if captureStderr {
		c.Stderr = &stderr
	}
	err := c.Run()
	var exitErr *exec.ExitError
	if captureStderr && errors.As(err, &exitErr) {
		exitErr.Stderr = stderr.Bytes()
	}
	return stdout.Bytes(), err
}

// CombinedOutput runs the command and returns its standard output and
// standard error together.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	if c.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out
	err := c.Run()
	return out.Bytes(), err
}

// TerminateAll stops the process group of every command still running and
// refuses to start new ones. It returns once each group has exited or has
// been sent SIGKILL. The CLI calls it when ralph receives SIGINT, SIGTERM or
// SIGHUP.
func TerminateAll() {
	running.Lock()
	running.stopping = true
	groups := make([]int, 0, len(running.groups))
	for pgid := range running.groups {
		groups = append(groups, pgid)
	}
	running.Unlock()

	var wg sync.WaitGroup
	for _, pgid := range groups {
		wg.Go(func() { _ = terminate(pgid) })
	}
	wg.Wait()
}

// terminate sends SIGTERM to process group pgid, waits up to killGrace for the
// group to exit, and sends SIGKILL to whatever is left. It returns
// os.ErrProcessDone when the group had already exited, which tells exec.Cmd
// not to report a cancellation for a command that finished on its own.
func terminate(pgid int) error {
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	deadline := time.Now().Add(killGrace)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH) {
			return nil
		}
		time.Sleep(pollInterval)
	}
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
