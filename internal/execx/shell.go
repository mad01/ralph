package execx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
)

// Shell runs the shell steps of one build or package. Each step's stdout goes
// to Out. Its stderr streams to os.Stderr when Verbose is set; otherwise it
// is kept, and everything kept so far is written to os.Stderr when a step
// fails, so a quiet run stays quiet unless something breaks. A step stopped
// by its context's deadline replays nothing: the caller reports the timeout.
type Shell struct {
	Out     io.Writer
	Dir     string // working directory; "" runs in ralph's own
	Verbose bool

	stderr bytes.Buffer
}

// Run runs sh with args under ctx as a Command.
func (s *Shell) Run(ctx context.Context, args ...string) error {
	cmd := Command(ctx, "sh", args...)
	cmd.Stdout = s.Out
	cmd.Stderr = io.Writer(os.Stderr)
	if !s.Verbose {
		cmd.Stderr = &s.stderr
	}
	if s.Dir != "" {
		cmd.Dir = s.Dir
	}
	err := cmd.Run()
	if err == nil || TimedOut(ctx) {
		return err
	}
	if !s.Verbose && s.stderr.Len() > 0 {
		_, _ = os.Stderr.Write(s.stderr.Bytes())
	}
	return err
}

// TimedOut reports whether ctx ended because its deadline passed.
func TimedOut(ctx context.Context) bool {
	return errors.Is(ctx.Err(), context.DeadlineExceeded)
}
