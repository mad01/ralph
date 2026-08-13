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
	"os/exec"
	"time"

	"github.com/mad01/ralph/internal/buildinfo"
)

// probeTimeout bounds how long we wait for a binary to answer `version -o json`.
// A misbehaving or hanging binary must not stall the caller.
const probeTimeout = 5 * time.Second

// Probe runs `<binaryPath> version -o json` and returns the build the binary
// reports. ok is false when the binary is missing, does not implement the
// convention, times out, or emits output that is not the expected JSON shape —
// callers should treat that as "version unknown", never as an error to
// surface. Fields the binary did not report come back empty.
func Probe(binaryPath string) (info buildinfo.Info, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, binaryPath, "version", "-o", "json").Output()
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
