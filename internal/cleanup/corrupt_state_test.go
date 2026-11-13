package cleanup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/state"
)

const corruptStateBody = "{not json"

// corruptStateHome points HOME at a temp dir, writes a state file that does
// not parse, and returns the home dir and the state file path.
func corruptStateHome(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, err := state.GetStatePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(corruptStateBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return home, p
}

// corruptBackups lists the backups MoveCorruptAside left next to p.
func corruptBackups(t *testing.T, p string) []string {
	t.Helper()
	matches, err := filepath.Glob(p + ".corrupt-*")
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

// The repair is reported once on the error writer, naming the backup.
func TestLoadPreviousState_CorruptStateWarnsOnce(t *testing.T) {
	_, p := corruptStateHome(t)
	var errBuf bytes.Buffer

	prev, note, err := loadPreviousState(false, &errBuf)
	if err != nil {
		t.Fatalf("loadPreviousState: %v", err)
	}
	if prev == nil || len(prev.Recipes) != 0 {
		t.Fatalf("prev = %+v, want an empty state", prev)
	}
	backups := corruptBackups(t, p)
	if len(backups) != 1 || !strings.Contains(note, backups[0]) {
		t.Fatalf("note %q should name the one backup in %v", note, backups)
	}
	if got := strings.Count(errBuf.String(), "Warning:"); got != 1 {
		t.Errorf("stderr has %d warnings, want 1:\n%s", got, errBuf.String())
	}
}
