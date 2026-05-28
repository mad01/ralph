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

func TestProbe_ReturnsVersionFromConventionalOutput(t *testing.T) {
	dir := t.TempDir()
	// Honour the convention only for `version -o json`; anything else is noise.
	bin := writeFakeBinary(t, dir, "mytool", `
if [ "$1" = "version" ] && [ "$2" = "-o" ] && [ "$3" = "json" ]; then
  echo '{"version":"deadbeefcafe"}'
  exit 0
fi
echo "usage" >&2
exit 1
`)

	got, ok := Probe(bin)
	if !ok {
		t.Fatal("expected ok=true for a tool implementing the convention")
	}
	if got != "deadbeefcafe" {
		t.Errorf("Probe() = %q, want %q", got, "deadbeefcafe")
	}
}

func TestProbe_MissingBinary(t *testing.T) {
	if _, ok := Probe(filepath.Join(t.TempDir(), "does-not-exist")); ok {
		t.Error("expected ok=false for a missing binary")
	}
}

func TestProbe_NonJSONOutput(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBinary(t, dir, "oldtool", `echo "v1.2.3"`)
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
