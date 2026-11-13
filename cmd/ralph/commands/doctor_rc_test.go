package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/shell"
)

const (
	rcBegin = shell.RalphBlockBeginMarker
	rcEnd   = shell.RalphBlockEndMarker
)

// configWithAlias returns a config with one alias, so ralph has something to
// inject into the rc file.
func configWithAlias() *config.Config {
	cfg := &config.Config{}
	cfg.Shell.Aliases = map[string]config.ShellAlias{"ll": {Command: "ls -l"}}
	return cfg
}

// runCheckRCFiles writes content to ~/.bashrc in a temp HOME and returns the
// doctor result for bash along with the rc file's path.
func runCheckRCFiles(
	t *testing.T,
	cfg *config.Config,
	content string,
) (*report.StepResult, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	rcPath := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(rcPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write rc file: %v", err)
	}
	cfg.Shell.Name = string(shell.Bash)

	rpt := &report.Report{Command: "doctor"}
	checkRCFiles(rpt, cfg)

	step := findStep(rpt, "RC files", string(shell.Bash))
	if step == nil {
		t.Fatal("expected an RC files result for bash")
	}
	return step, rcPath
}

// A managed block whose markers do not pair up is a doctor failure that names
// the file and the offending line and says how to repair it by hand. It must
// not send the user to apply, which refuses such a file.
func TestCheckRCFiles_MalformedBlockFails(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantLine string
	}{
		{
			name:     "end before begin",
			content:  "export A=1\n" + rcEnd + "\nsource ~/x.sh\n" + rcBegin + "\n",
			wantLine: "line 2",
		},
		{
			name:     "begin without end",
			content:  "export A=1\n" + rcBegin + "\nsource ~/x.sh\n",
			wantLine: "line 2",
		},
		{
			name:     "end without begin",
			content:  "export A=1\nsource ~/x.sh\n" + rcEnd + "\n",
			wantLine: "line 3",
		},
		{
			name:     "nested begin",
			content:  rcBegin + "\n" + rcBegin + "\nsource ~/x.sh\n" + rcEnd + "\n" + rcEnd + "\n",
			wantLine: "line 2",
		},
		{
			name: "duplicate block",
			content: rcBegin + "\nsource ~/x.sh\n" + rcEnd + "\n" +
				rcBegin + "\nsource ~/y.sh\n" + rcEnd + "\n",
			wantLine: "line 4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configWithAlias()

			step, rcPath := runCheckRCFiles(t, cfg, tt.content)

			if step.Status != report.StatusFail {
				t.Fatalf("status = %v, want fail (msg=%q)", step.Status, step.Message)
			}
			for _, want := range []string{
				rcPath,
				tt.wantLine,
				"restore the missing marker or delete the stray one",
			} {
				if !strings.Contains(step.Message, want) {
					t.Errorf("message %q does not contain %q", step.Message, want)
				}
			}
			if strings.Contains(step.Message, "apply") {
				t.Errorf("message %q tells the user to run apply", step.Message)
			}
		})
	}
}

// A well-formed block whose sourced files exist reports OK.
func TestCheckRCFiles_WellFormedBlockOK(t *testing.T) {
	sourced := filepath.Join(t.TempDir(), "aliases.sh")
	if err := os.WriteFile(sourced, nil, 0o644); err != nil {
		t.Fatalf("write sourced file: %v", err)
	}
	cfg := configWithAlias()

	step, _ := runCheckRCFiles(t, cfg,
		"export A=1\n"+rcBegin+"\nsource "+sourced+"\n"+rcEnd+"\n")

	if step.Status != report.StatusOK || step.Message != "" {
		t.Fatalf("got %v %q, want OK with no message", step.Status, step.Message)
	}
}

// Apply migrates legacy DOTTER markers before it checks the block, so a block
// that opens with a legacy marker and closes with a ralph one is well formed
// for doctor too.
func TestCheckRCFiles_LegacyMarkersReadAsBlock(t *testing.T) {
	sourced := filepath.Join(t.TempDir(), "aliases.sh")
	if err := os.WriteFile(sourced, nil, 0o644); err != nil {
		t.Fatalf("write sourced file: %v", err)
	}

	step, _ := runCheckRCFiles(t, configWithAlias(),
		"# BEGIN DOTTER MANAGED BLOCK\nsource "+sourced+"\n"+rcEnd+"\n")

	if step.Status != report.StatusOK || step.Message != "" {
		t.Fatalf("got %v %q, want OK with no message", step.Status, step.Message)
	}
}

// A well-formed block that sources a missing file is a failure.
func TestCheckRCFiles_WellFormedBlockMissingSourceFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.sh")

	step, _ := runCheckRCFiles(t, &config.Config{},
		rcBegin+"\nsource "+missing+"\n"+rcEnd+"\n")

	if step.Status != report.StatusFail || step.Message != "sourced file(s) missing" {
		t.Fatalf("got %v %q, want fail %q", step.Status, step.Message, "sourced file(s) missing")
	}
}

// A well-formed block with no source line warns when aliases are configured.
func TestCheckRCFiles_WellFormedBlockWithoutSourceWarns(t *testing.T) {
	cfg := configWithAlias()

	step, _ := runCheckRCFiles(t, cfg, rcBegin+"\n# nothing here\n"+rcEnd+"\n")

	want := "block found but no source commands detected"
	if step.Status != report.StatusWarn || step.Message != want {
		t.Fatalf("got %v %q, want warn %q", step.Status, step.Message, want)
	}
}

// An rc file without a block warns, and points at apply only when ralph has
// something to inject.
func TestCheckRCFiles_NoBlockWarns(t *testing.T) {
	withAliases := configWithAlias()

	tests := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{
			name: "aliases configured",
			cfg:  withAliases,
			want: "ralph block missing but aliases/functions configured (run apply to fix)",
		},
		{name: "nothing configured", cfg: &config.Config{}, want: "ralph block not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step, _ := runCheckRCFiles(t, tt.cfg, "export A=1\n")

			if step.Status != report.StatusWarn || step.Message != tt.want {
				t.Fatalf("got %v %q, want warn %q", step.Status, step.Message, tt.want)
			}
		})
	}
}
