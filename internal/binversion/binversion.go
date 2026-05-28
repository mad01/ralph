// Package binversion probes an installed binary for the build version it
// reports, following ralph's cross-tool convention: a tool exposes a `version`
// subcommand that, with `-o json`, prints a single JSON object of the form
//
//	{"version": "<git sha the binary was built from>"}
//
// This lets tooling ask a binary what build it is in a uniform way. ralph's own
// `ralph version -o json` emits this shape; sibling tools are expected to do the
// same so a single probe works across the whole fleet.
package binversion

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

// probeTimeout bounds how long we wait for a binary to answer `version -o json`.
// A misbehaving or hanging binary must not stall the caller.
const probeTimeout = 5 * time.Second

// versionDoc is the expected JSON shape emitted by `<tool> version -o json`.
type versionDoc struct {
	Version string `json:"version"`
}

// Probe runs `<binaryPath> version -o json` and returns the reported version.
// ok is false when the binary is missing, does not implement the convention,
// times out, or emits output that is not the expected JSON shape — callers
// should treat that as "version unknown", never as an error to surface.
func Probe(binaryPath string) (version string, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, binaryPath, "version", "-o", "json").Output()
	if err != nil {
		return "", false
	}

	var doc versionDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		return "", false
	}
	if doc.Version == "" {
		return "", false
	}
	return doc.Version, true
}
