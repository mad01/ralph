package commands

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/report"
)

// runDoctor writes a scratch config.toml (dotfiles_repo_path, then body; its
// directory is the state dir), points HOME at a temp dir, runs `ralph doctor`
// with the given output format, and returns stdout, the state dir and the
// exit code.
func runDoctor(
	t *testing.T,
	body, format string,
	stateFiles map[string]string,
) (string, string, int) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	body = "dotfiles_repo_path = \"~/dots\"\n" + body
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("RALPH_CONFIG", cfgPath)
	for name, content := range stateFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	var runErr error
	out := captureStdout(t, func() {
		withOutputFormat(format, func() { runErr = doctorCmd.RunE(doctorCmd, nil) })
	})
	code := 0
	var exitErr *ExitError
	if errors.As(runErr, &exitErr) {
		code = exitErr.Code
	} else if runErr != nil {
		t.Fatalf("doctor RunE: %v", runErr)
	}
	return out, dir, code
}

// Config loading already rejects depends_on cycles and unknown dependency
// names, so doctor reports them as the config load failure, with the reason.
func TestDoctor_DependsOnErrorsSurfaceAsConfigFailure(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"cycle": {
			body: `
[hooks.builds.a]
commands = ["true"]
run = "always"
depends_on = ["builds.b"]

[hooks.builds.b]
commands = ["true"]
run = "always"
depends_on = ["builds.a"]
`,
			want: "dependency cycle detected among: ",
		},
		"unknown name": {
			body: `
[hooks.builds.a]
commands = ["true"]
run = "always"
depends_on = ["builds.missing"]
`,
			want: "depends_on references 'builds.missing'",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out, _, code := runDoctor(t, tc.body, "json", nil)
			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			step := jsonStep(t, out, "Configuration", "config")
			if step.Status != "fail" || !strings.Contains(step.Message, tc.want) {
				t.Errorf("config step = %+v, want a fail containing %q", step, tc.want)
			}
		})
	}
}

// In JSON mode a new check lands in the report on stdout, which stays one
// JSON document, and it drives the exit code.
func TestDoctor_JSONReportsCorruptState(t *testing.T) {
	out, dir, code := runDoctor(t, "", "json", map[string]string{".recipe_state": "{not json"})

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	step := jsonStep(t, out, "State", ".recipe_state")
	if step.Status != "fail" ||
		!strings.Contains(step.Message, filepath.Join(dir, ".recipe_state")) {
		t.Errorf(".recipe_state step = %+v, want a fail naming the file", step)
	}
}

// jsonStep parses a doctor JSON report and returns the named step.
func jsonStep(t *testing.T, out, phase, name string) report.JSONStep {
	t.Helper()
	var doc report.JSONReport
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, out)
	}
	for _, p := range doc.Phases {
		if p.Name != phase {
			continue
		}
		for _, s := range p.Steps {
			if s.Name == name {
				return s
			}
		}
	}
	t.Fatalf("no %s/%s step in:\n%s", phase, name, out)
	return report.JSONStep{}
}
