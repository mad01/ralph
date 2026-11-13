package cleanup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
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

// MAD-630: the repair reaches the report whether cleanup is on or off, as the
// same warn step, so a -o json consumer sees it. The stderr line stays.
func TestRecord_CorruptStateRepairIsReported(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opts     Options
		wantNote string
	}{
		{"plain run", Options{}, "moved it to"},
		{"cleanup run", Options{Cleanup: true}, "moved it to"},
		{"plain dry run", Options{DryRun: true}, "a real run moves it"},
		{"cleanup dry run", Options{Cleanup: true, DryRun: true}, "a real run moves it"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			corruptStateHome(t)
			var errBuf bytes.Buffer
			tc.opts.Err = &errBuf
			rpt := &report.Report{Command: "up"}

			Record(&config.Config{}, state.New(), nil, tc.opts, rpt)

			jr := rpt.ToJSON(tc.opts.DryRun)
			steps := jsonSteps(jr, "Cleanup", "load")
			if len(steps) != 1 {
				t.Fatalf("Cleanup/load steps = %+v, want one\nphases: %+v", steps, jr.Phases)
			}
			if steps[0].Status != "warn" || !strings.Contains(steps[0].Message, tc.wantNote) {
				t.Errorf("load step = %+v, want a warn containing %q", steps[0], tc.wantNote)
			}
			if !strings.Contains(errBuf.String(), "Warning: ") {
				t.Errorf("stderr lost the repair warning:\n%s", errBuf.String())
			}
		})
	}
}

// jsonSteps returns the steps named step in every phase named phase.
func jsonSteps(jr report.JSONReport, phase, step string) []report.JSONStep {
	var found []report.JSONStep
	for _, p := range jr.Phases {
		if p.Name != phase {
			continue
		}
		for _, s := range p.Steps {
			if s.Name == step {
				found = append(found, s)
			}
		}
	}
	return found
}
