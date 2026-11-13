package commands

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

const corruptStateBody = "{not json"

// corruptStateHome points HOME at a temp dir, writes a state file that does
// not parse, and returns the home dir and the state file path.
func corruptStateHome(t *testing.T) (string, string) {
	t.Helper()
	home := orphanHome(t)
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

// MAD-613: a state file that does not parse is moved aside, the run records a
// fresh one, and with no previous state nothing counts as an orphan.
func TestRecordManifest_CorruptStateIsMovedAside(t *testing.T) {
	home, p := corruptStateHome(t)
	active := testRecipe{name: "active", links: []string{makeLink(t, home, ".active")}}
	rpt := &report.Report{Command: "test"}

	recordManifestAndCleanup(
		configWith(active),
		"anyhost",
		manifestOptions{cleanup: true},
		io.Discard,
		rpt,
	)

	if _, ok := savedState(t).Recipes["active"]; !ok {
		t.Error("fresh state was not recorded: active recipe missing")
	}
	backups := corruptBackups(t, p)
	if len(backups) != 1 {
		t.Fatalf("backups = %v, want exactly one", backups)
	}
	if got := readFile(t, backups[0]); got != corruptStateBody {
		t.Errorf("backup holds %q, want the old bytes %q", got, corruptStateBody)
	}
	if !linkExists(active.links[0]) {
		t.Error("an active recipe's link was removed")
	}
	step := findStep(rpt, "Cleanup", "load")
	if step == nil || step.Status != report.StatusWarn ||
		!strings.Contains(step.Message, backups[0]) {
		t.Errorf("cleanup load step = %+v, want a warn naming %s", step, backups[0])
	}
}

func TestRecordManifest_CorruptStateDryRunMovesNothing(t *testing.T) {
	home, p := corruptStateHome(t)
	active := testRecipe{name: "active", links: []string{makeLink(t, home, ".active")}}
	rpt := &report.Report{Command: "test"}

	recordManifestAndCleanup(
		configWith(active),
		"anyhost",
		manifestOptions{cleanup: true, dryRun: true},
		io.Discard,
		rpt,
	)

	if got := readFile(t, p); got != corruptStateBody {
		t.Errorf("dry run changed the state file to %q", got)
	}
	if backups := corruptBackups(t, p); len(backups) != 0 {
		t.Errorf("dry run created backups %v", backups)
	}
	step := findStep(rpt, "Cleanup", "load")
	if step == nil || !strings.Contains(step.Message, "a real run moves it") {
		t.Errorf("cleanup load step = %+v, want a note on what a real run does", step)
	}
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

// A state file that cannot be moved aside keeps today's behaviour: warn,
// leave the file alone, and skip the save.
func TestRecordManifest_CorruptStateMoveFailsKeepsFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	home, p := corruptStateHome(t)
	active := testRecipe{name: "active", links: []string{makeLink(t, home, ".active")}}
	dir := filepath.Dir(p)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	rpt := &report.Report{Command: "test"}

	recordManifestAndCleanup(
		configWith(active),
		"anyhost",
		manifestOptions{cleanup: true},
		io.Discard,
		rpt,
	)

	if got := readFile(t, p); got != corruptStateBody {
		t.Errorf("state file changed to %q although the move failed", got)
	}
	step := findStep(rpt, "Cleanup", "load")
	if step == nil || !strings.Contains(step.Message, "moving it aside failed") {
		t.Errorf("cleanup load step = %+v, want the move failure reason", step)
	}
}
