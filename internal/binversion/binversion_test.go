package binversion

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
	return path
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

func TestProbe_JSONWithoutVersionKey(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBinary(t, dir, "wrongtool", `echo '{"commit":"abc"}'`)
	if _, ok := Probe(bin); ok {
		t.Error("expected ok=false when JSON lacks a non-empty version field")
	}
}
