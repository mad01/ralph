// Package binversion probes an installed binary for the build it reports,
// following ralph's cross-tool convention: a tool exposes a `version`
// subcommand that, with `-o json`, prints a single JSON object of the form
//
//	{
//	  "version": "<short sha, or release tag>",
//	  "commit": "<full sha the binary was built from>",
//	  "tag": "<last release tag>",
//	  "build_time": "<UTC RFC3339>"
//	}
//
// This lets tooling ask a binary what build it is in a uniform way. ralph's own
// `ralph version -o json` emits this shape (see internal/buildinfo); sibling
// tools are expected to do the same so a single probe works across the whole
// fleet. Tools predating the wider contract emit version alone — the extra
// fields simply come back empty, which every caller must tolerate.
package binversion

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"time"

	"github.com/mad01/ralph/internal/buildinfo"
)

// probeTimeout bounds one attempt at `version -o json`. A misbehaving or
// hanging binary must not stall the caller; with the one retry a timeout
// earns, a binary that hangs every time costs two of these.
const probeTimeout = 5 * time.Second

// probeWaitDelay bounds the wait for the binary's output pipes to close after
// a timeout kills it, in case a child it started still holds them.
const probeWaitDelay = time.Second

// Probe runs `<binaryPath> version -o json` and returns the build the binary
// reports. ok is false when the binary is missing, does not implement the
// convention, times out, or emits output that is not the expected JSON shape —
// callers should treat that as "version unknown", never as an error to
// surface. Fields the binary did not report come back empty.
func Probe(binaryPath string) (info buildinfo.Info, ok bool) {
	return probe(binaryPath, probeTimeout)
}

// probe is Probe with the per-attempt timeout as a parameter. An attempt that
// ends by its timeout is tried once more: macOS checks a new executable on its
// first exec, and under load that check alone can outlast the timeout, while
// the second exec finds it done. A non-zero exit or bad output is an answer,
// so it is not retried.
func probe(binaryPath string, timeout time.Duration) (info buildinfo.Info, ok bool) {
	out, err := runVersion(binaryPath, timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		out, err = runVersion(binaryPath, timeout)
	}
	if err != nil {
		return buildinfo.Info{}, false
	}

	// Unknown keys are ignored, so a tool that reports more than the contract
	// still probes cleanly.
	if err := json.Unmarshal(out, &info); err != nil {
		return buildinfo.Info{}, false
	}
	if info.Version == "" {
		return buildinfo.Info{}, false
	}
	return info, true
}

// runVersion runs `<binaryPath> version -o json` once and returns its stdout.
// When the attempt fails because it ran out of time, the error is
// context.DeadlineExceeded, whatever the kill made the process report.
func runVersion(binaryPath string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "version", "-o", "json")
	cmd.WaitDelay = probeWaitDelay
	out, err := cmd.Output()
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, context.DeadlineExceeded
	}
	return out, err
}
