package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

// MAD-630: a plain `ralph up -o json` that repairs a .recipe_state that does
// not parse reports the repair as a warn step in the JSON on stdout, and keeps
// the warning on stderr. A cleanup run reports it the same way.
func TestUp_OutputJSONReportsCorruptStateRepair(t *testing.T) {
	for _, extra := range [][]string{nil, {"--enable-cleanup"}} {
		t.Run(fmt.Sprintf("flags %q", extra), func(t *testing.T) {
			home := isolateHome(t)
			dots := filepath.Join(home, "dots")
			if err := os.MkdirAll(dots, 0o755); err != nil {
				t.Fatal(err)
			}
			writeConfig(t, fmt.Sprintf("dotfiles_repo_path = %q\n", dots))
			writeCorruptRecipeState(t)

			args := append([]string{"up", "--no-sync", "-o", "json"}, extra...)
			res := runRalph(t, args...)

			var exitErr *ExitError
			if !errors.As(res.err, &exitErr) || exitErr.Code != 2 {
				t.Fatalf("err = %v, want exit code 2 (warnings)\nstderr: %s", res.err, res.stderr)
			}
			var jr report.JSONReport
			if err := json.Unmarshal([]byte(res.stdout), &jr); err != nil {
				t.Fatalf("stdout is not pure JSON: %v\n%s", err, res.stdout)
			}
			step := findJSONStep(jr, "Cleanup", "load")
			if step == nil || step.Status != "warn" ||
				!strings.Contains(step.Message, "moved it to") {
				t.Errorf("Cleanup/load step = %+v, want a warn naming the backup", step)
			}
			if !strings.Contains(res.stderr, "moved it to") {
				t.Errorf("stderr lost the repair warning:\n%s", res.stderr)
			}
		})
	}
}

// writeCorruptRecipeState writes a .recipe_state that does not parse.
func writeCorruptRecipeState(t *testing.T) {
	t.Helper()
	p, err := state.GetStatePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// findJSONStep returns the first step named step in a phase named phase.
func findJSONStep(jr report.JSONReport, phase, step string) *report.JSONStep {
	for _, p := range jr.Phases {
		if p.Name != phase {
			continue
		}
		for i := range p.Steps {
			if p.Steps[i].Name == step {
				return &p.Steps[i]
			}
		}
	}
	return nil
}
