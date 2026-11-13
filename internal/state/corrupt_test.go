package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeStateFile points HOME at a temp dir, writes body as the state file
// and returns its path.
func writeStateFile(t *testing.T, body string) string {
	t.Helper()
	withHome(t)
	p, err := GetStatePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLoad_ParseErrorIsCorrupt(t *testing.T) {
	writeStateFile(t, "{not json")

	if _, err := Load(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Load error = %v, want one wrapping ErrCorrupt", err)
	}
}

// A read error may hide a good file, so it must not count as corrupt.
func TestLoad_ReadErrorIsNotCorrupt(t *testing.T) {
	withHome(t)
	p, err := GetStatePath()
	if err != nil {
		t.Fatal(err)
	}
	// A directory at the state path fails the read, not the parse.
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = Load()
	if err == nil {
		t.Fatal("Load succeeded on a directory")
	}
	if errors.Is(err, ErrCorrupt) {
		t.Fatalf("Load error = %v, a read error must not wrap ErrCorrupt", err)
	}
}

func TestMoveCorruptAside_NamesBackupAfterUTCTime(t *testing.T) {
	p := writeStateFile(t, "{not json")
	now := time.Date(2026, 11, 13, 13, 4, 5, 0, time.FixedZone("CET", 3600))

	backup, err := MoveCorruptAside(now)
	if err != nil {
		t.Fatalf("MoveCorruptAside: %v", err)
	}
	if want := p + ".corrupt-20261113T120405Z"; backup != want {
		t.Errorf("backup = %s, want %s", backup, want)
	}
	if got := readFile(t, backup); got != "{not json" {
		t.Errorf("backup holds %q, want the old bytes", got)
	}
	if _, err := os.Lstat(p); !os.IsNotExist(err) {
		t.Errorf("state file still present after the move (err=%v)", err)
	}
}

func TestMoveCorruptAside_KeepsEarlierBackup(t *testing.T) {
	p := writeStateFile(t, "first")
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

	first, err := MoveCorruptAside(now)
	if err != nil {
		t.Fatalf("first move: %v", err)
	}
	if err := os.WriteFile(p, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := MoveCorruptAside(now)
	if err != nil {
		t.Fatalf("second move: %v", err)
	}

	if second == first {
		t.Fatalf("second move reused the backup name %s", first)
	}
	if got := readFile(t, first); got != "first" {
		t.Errorf("earlier backup holds %q, want %q", got, "first")
	}
	if got := readFile(t, second); got != "second" {
		t.Errorf("new backup holds %q, want %q", got, "second")
	}
}
