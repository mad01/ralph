// Package execx runs external commands so that a timeout or an interrupt
// stops them, and so that waiting for a command is always bounded.
//
// Command starts a command in a new session, which makes it the leader of its
// own process group: one signal to the group reaches a shell, the make it runs
// and anything either left in the background. The new session also detaches
// the command from ralph's controlling terminal, so the terminal's Ctrl-C
// reaches ralph only (the CLI forwards it with TerminateAll), and a command
// that opens /dev/tty to prompt fails at once instead of stopping on SIGTTIN
// until its timeout. Builds, packages, hooks and tool checks use it.
//
// TerminalCommand keeps the command in ralph's process group with the
// terminal, so an ssh passphrase, host-key or credential prompt works. Git
// uses it. A timeout signals the command's own process only; WaitDelay still
// bounds Wait when a helper such as ssh keeps the output pipe open.
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

// killGrace is how long a stopped command gets to exit after SIGTERM before it
// gets SIGKILL. A var so tests can shorten it.
var killGrace = 2 * time.Second

// waitDelay bounds how long Wait keeps reading a command's output once the
// command exited (or was stopped) while a process it started still holds the
// output pipe. For a TerminalCommand it is also the grace between SIGTERM and
// SIGKILL on a timeout. A var so tests can shorten it.
var waitDelay = 2 * time.Second

// ErrStopped is returned by Start once TerminateAll has run: ralph is exiting
// and must not start a command it would leave behind.
var ErrStopped = errors.New("not starting command: ralph is stopping")

// running holds every started command that has not been waited for, so
// TerminateAll can reach them. Start registers under the same lock that
// TerminateAll takes to set stopping, so a command either is in the snapshot
// TerminateAll stops or is refused.
var running = struct {
	sync.Mutex
	stopping bool
	cmds     map[*Cmd]struct{}
}{cmds: map[*Cmd]struct{}{}}

// Cmd is an exec.Cmd whose context stops it with SIGTERM, then SIGKILL, and
// whose Wait is bounded even when a leftover process holds the output pipe.
// Use Cmd's own Start, Wait, Run, Output and CombinedOutput: they track the
// command for TerminateAll.
type Cmd struct {
	*exec.Cmd
	stop func()        // stops the running command for TerminateAll
	done chan struct{} // closed when Wait returns
}

// Command returns a Cmd that runs name with args under ctx in its own session
// and process group. A timeout stops the whole group. Callers set Dir, Env,
// Stdout and Stderr as on an exec.Cmd; Stdin stays /dev/null unless set.
func Command(ctx context.Context, name string, args ...string) *Cmd {
	c := newCmd(ctx, name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	c.Cancel = func() error { return terminateGroup(c.Process.Pid) }
	c.stop = func() { _ = terminateGroup(c.Process.Pid) }
	return c
}

// TerminalCommand returns a Cmd that runs name with args under ctx in ralph's
// own process group, keeping the controlling terminal so the command can
// prompt. A timeout sends SIGTERM to the command's process, and SIGKILL after
// waitDelay; processes it started are not signalled.
func TerminalCommand(ctx context.Context, name string, args ...string) *Cmd {
	c := newCmd(ctx, name, args...)
	c.Cancel = func() error { return c.Process.Signal(syscall.SIGTERM) }
	c.stop = c.stopProcess
	return c
}

func newCmd(ctx context.Context, name string, args ...string) *Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = waitDelay
	return &Cmd{Cmd: cmd, done: make(chan struct{})}
}

// Start starts the command and records it for TerminateAll.
func (c *Cmd) Start() error {
	running.Lock()
	defer running.Unlock()
	if running.stopping {
		return ErrStopped
	}
	if err := c.Cmd.Start(); err != nil {
		return err
	}
	running.cmds[c] = struct{}{}
	return nil
}

// Wait waits for the command to exit. A command that exited 0 while a process
// it left in the background still held its output past waitDelay counts as a
// success: the command itself finished, and the leftover process is its own.
func (c *Cmd) Wait() error {
	err := c.Cmd.Wait()
	running.Lock()
	if _, ok := running.cmds[c]; ok {
		delete(running.cmds, c)
		close(c.done)
	}
	running.Unlock()
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

// TerminateAll stops every command still running and refuses to start new
// ones. A Command's whole process group gets SIGTERM, then SIGKILL after
// killGrace; a TerminalCommand's own process gets the same. It returns once
// each has exited or been sent SIGKILL. The CLI calls it when ralph receives
// SIGINT, SIGTERM or SIGHUP.
func TerminateAll() {
	running.Lock()
	running.stopping = true
	cmds := make([]*Cmd, 0, len(running.cmds))
	for c := range running.cmds {
		cmds = append(cmds, c)
	}
	running.Unlock()

	var wg sync.WaitGroup
	for _, c := range cmds {
		wg.Go(c.stop)
	}
	wg.Wait()
}

// stopProcess sends SIGTERM to the command's process and SIGKILL if Wait has
// not returned within killGrace. os.Process tracks a reaped process, so a
// signal never reaches a recycled PID.
func (c *Cmd) stopProcess() {
	_ = c.Process.Signal(syscall.SIGTERM)
	select {
	case <-c.done:
	case <-time.After(killGrace):
		_ = c.Process.Kill()
	}
}

// terminateGroup sends SIGTERM to process group pgid, waits up to killGrace
// for the group to exit, and sends SIGKILL to whatever is left. It returns
// os.ErrProcessDone when the group had already exited, which tells exec.Cmd
// not to report a cancellation for a command that finished on its own.
func terminateGroup(pgid int) error {
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
