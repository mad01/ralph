package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunBuildVerify_ExitZero(t *testing.T) {
	ok, out, err := runBuildVerify("printf 'all good\\n'", "")
	if !ok {
		t.Fatalf("expected ok=true, got false (err=%v)", err)
	}
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if out != "all good" {
		t.Fatalf("expected output %q, got %q", "all good", out)
	}
}

func TestRunBuildVerify_NonZeroReportsDrift(t *testing.T) {
	ok, out, err := runBuildVerify("echo drift detected; exit 1", "")
	if ok {
		t.Fatal("expected ok=false for non-zero exit")
	}
	if err == nil {
		t.Fatal("expected non-nil err for non-zero exit")
	}
	if out != "drift detected" {
		t.Fatalf("expected output %q, got %q", "drift detected", out)
	}
}

func TestRunBuildVerify_RunsInWorkingDir(t *testing.T) {
	dir := t.TempDir()
	// `pwd -P` resolves symlinks so the comparison holds on macOS (/tmp -> /private/tmp).
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	ok, out, err := runBuildVerify("pwd -P", dir)
	if !ok {
		t.Fatalf("expected ok=true, got false (err=%v)", err)
	}
	if out != resolved {
		t.Fatalf("expected pwd %q, got %q", resolved, out)
	}
}

func TestFirstLine(t *testing.T) {
	cases := map[string]string{
		"":                   "",
		"single":             "single",
		"first\nsecond":      "first",
		"\n\n  trimmed  \nx": "trimmed",
		"   \n  real line\n": "real line",
	}
	for in, want := range cases {
		if got := firstLine(in); got != want {
			t.Errorf("firstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

// Sanity: a verify command that reads a file in the working dir behaves like
// the real sync-settings drift check (file present = in sync, exit 0).
func TestRunBuildVerify_WorkingDirFileCheck(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	ok, _, err := runBuildVerify("test -f marker", dir)
	if !ok || err != nil {
		t.Fatalf("expected marker present → ok, got ok=%v err=%v", ok, err)
	}
}
