package binversion

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// writeFakeBinary writes an executable shell script at dir/name whose behaviour
// is the given script body. Skips on Windows (no shebang execution).
func writeFakeBinary(t *testing.T, dir, name, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binaries are not executable on Windows")
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	warmUp(t, path)
	return path
}

// warmUp runs a new executable once, untimed, before Probe sees it. macOS
// checks a new executable on its first exec, and under load that check alone
// can outlast Probe's timeout; later execs of the same file skip it. The run
// gets no arguments and its output is discarded; only a failure to start
// counts.
func warmUp(t *testing.T, path string) {
	t.Helper()
	var exitErr *exec.ExitError
	if err := exec.Command(path).Run(); err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("warm up %s: %v", path, err)
	}
}

func TestProbe_ReturnsBuildFromConventionalOutput(t *testing.T) {
	dir := t.TempDir()
	// Honour the convention only for `version -o json`; anything else is noise.
	bin := writeFakeBinary(t, dir, "mytool", `
if [ "$1" = "version" ] && [ "$2" = "-o" ] && [ "$3" = "json" ]; then
  cat <<'JSON'
{
  "version": "deadbee",
  "commit": "deadbeefcafe1234567890abcdef1234567890ab",
  "tag": "keep/v0.4.0",
  "build_time": "2026-08-13T19:40:11Z"
}
JSON
  exit 0
fi
echo "usage" >&2
exit 1
`)

	got, ok := Probe(bin)
	if !ok {
		t.Fatal("expected ok=true for a tool implementing the convention")
	}
	if got.Version != "deadbee" {
		t.Errorf("Version = %q, want %q", got.Version, "deadbee")
	}
	if got.Commit != "deadbeefcafe1234567890abcdef1234567890ab" {
		t.Errorf("Commit = %q, want the full sha", got.Commit)
	}
	if got.Tag != "keep/v0.4.0" {
		t.Errorf("Tag = %q, want %q", got.Tag, "keep/v0.4.0")
	}
	if got.BuildTime != "2026-08-13T19:40:11Z" {
		t.Errorf("BuildTime = %q, want %q", got.BuildTime, "2026-08-13T19:40:11Z")
	}
}

// Tools built before the four-field contract report version alone. They must
// still probe cleanly, with the fields they never emitted left empty.
func TestProbe_LegacyVersionOnlyTool(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBinary(t, dir, "oldtool", `echo '{"version":"deadbeefcafe"}'`)

	got, ok := Probe(bin)
	if !ok {
		t.Fatal("expected ok=true for a tool emitting only the version field")
	}
	if got.Version != "deadbeefcafe" {
		t.Errorf("Version = %q, want %q", got.Version, "deadbeefcafe")
	}
	if got.Commit != "" || got.Tag != "" || got.BuildTime != "" {
		t.Errorf("expected the unreported fields to be empty, got %+v", got)
	}
}

// A tool reporting more than the contract must not break the probe.
func TestProbe_IgnoresUnknownFields(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBinary(t, dir, "richtool", `echo '{"version":"abc1234","go":"1.26.5"}'`)

	got, ok := Probe(bin)
	if !ok || got.Version != "abc1234" {
		t.Errorf("Probe() = (%+v, %v), want version abc1234 and ok", got, ok)
	}
}

func TestProbe_MissingBinary(t *testing.T) {
	if _, ok := Probe(filepath.Join(t.TempDir(), "does-not-exist")); ok {
		t.Error("expected ok=false for a missing binary")
	}
}

func TestProbe_NonJSONOutput(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBinary(t, dir, "plaintool", `echo "v1.2.3"`)
	if _, ok := Probe(bin); ok {
		t.Error("expected ok=false when output is not the conventional JSON")
	}
}

// testTimeout stands in for probeTimeout in the retry tests, so a fake that
// sleeps past it costs a fraction of a second, not 5 s. A warmed-up script
// answers well inside it.
const testTimeout = 300 * time.Millisecond

// attempts returns how many times a fake binary logged a probe to path.
func attempts(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "\n")
}

// The first exec of a new binary can outlast the timeout while macOS checks
// it; the second exec finds the check done. One retry after a timeout covers
// that. The fake hangs on its first probe and answers on the second. `exec`
// replaces the shell, so the timeout's kill ends the sleep too.
func TestProbe_RetriesOnceAfterTimeout(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "seen")
	count := filepath.Join(dir, "count")
	bin := writeFakeBinary(t, dir, "slowstart", `
[ "$1" = "version" ] || exit 0
echo probe >> '`+count+`'
if [ ! -e '`+marker+`' ]; then
  : > '`+marker+`'
  exec sleep 5
fi
echo '{"version":"abc1234"}'
`)

	got, ok := probe(bin, testTimeout)
	if !ok || got.Version != "abc1234" {
		t.Errorf("probe() = (%+v, %v), want version abc1234 and ok", got, ok)
	}
	if n := attempts(t, count); n != 2 {
		t.Errorf("probed %d times, want 2", n)
	}
}

// A binary that hangs on every run is given up on after the retry.
func TestProbe_HangingBinaryGivesUpAfterTwoAttempts(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	bin := writeFakeBinary(t, dir, "hangtool", `
[ "$1" = "version" ] || exit 0
echo probe >> '`+count+`'
exec sleep 5
`)

	if _, ok := probe(bin, testTimeout); ok {
		t.Error("expected ok=false for a binary that always hangs")
	}
	if n := attempts(t, count); n != 2 {
		t.Errorf("probed %d times, want 2", n)
	}
}

// Only a timeout earns a retry: a binary that answers with a non-zero exit
// has answered, and asking again would get the same answer.
func TestProbe_NonZeroExitIsNotRetried(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	bin := writeFakeBinary(t, dir, "failtool", `
[ "$1" = "version" ] || exit 0
echo probe >> '`+count+`'
exit 3
`)

	if _, ok := Probe(bin); ok {
		t.Error("expected ok=false for a binary that exits non-zero")
	}
	if n := attempts(t, count); n != 1 {
		t.Errorf("probed %d times, want 1", n)
	}
}

func TestProbe_JSONWithoutVersionKey(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBinary(t, dir, "wrongtool", `echo '{"commit":"abc"}'`)
	if _, ok := Probe(bin); ok {
		t.Error("expected ok=false when JSON lacks a non-empty version field")
	}
}
