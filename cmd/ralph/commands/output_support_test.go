package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/progress"
	"github.com/mad01/ralph/internal/state"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// jsonOutputCommands is the documented set of commands that honour -o json
// (docs/commands.md, "JSON output"). Every other runnable command must reject
// it, so a command added without opting in fails the walk below.
var jsonOutputCommands = []string{
	"ralph apply",
	"ralph clean",
	"ralph config",
	"ralph doctor",
	"ralph outdated",
	"ralph state show",
	"ralph up",
	"ralph version",
}

// isolateHome points every path ralph resolves (state, config, lock) at a
// fresh temp dir, so an in-process run never reads or writes the real ones.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("RALPH_CONFIG", filepath.Join(home, ".config", "ralph", "config.toml"))
	return home
}

// resetFlags puts every flag in the tree back to its default. Flag values are
// package globals, so without this one run's --json leaks into the next.
func resetFlags(cmd *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace(nil)
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	}
	cmd.Flags().VisitAll(reset)
	cmd.PersistentFlags().VisitAll(reset)
	for _, c := range cmd.Commands() {
		resetFlags(c)
	}
}

// runResult is what one in-process ralph invocation produced.
type runResult struct {
	stdout string
	stderr string
	err    error
}

// runRalph executes the root command with args from a clean flag state and
// captures what it writes to stdout and stderr. It goes through executeRoot,
// the path the binary takes, and cobra's writers are left unset, so its own
// output lands on the same streams as in the binary.
func runRalph(t *testing.T, args ...string) runResult {
	t.Helper()
	resetFlags(rootCmd)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		resetFlags(rootCmd)
		rootCmd.SetArgs(nil)
		progress.SetSilent(false)
	})

	var res runResult
	res.stderr = captureFile(t, &os.Stderr, func() {
		res.stdout = captureStdout(t, func() {
			res.err = executeRoot()
		})
	})
	return res
}

// writeConfig writes body as the config.toml that isolateHome points at.
func writeConfig(t *testing.T, body string) {
	t.Helper()
	cfgPath := os.Getenv("RALPH_CONFIG")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runnableCommands returns every command in the tree that cobra would run,
// cobra's own help and completion commands included.
func runnableCommands(cmd *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	if cmd.Runnable() {
		out = append(out, cmd)
	}
	for _, c := range cmd.Commands() {
		out = append(out, runnableCommands(c)...)
	}
	return out
}

// stubRun swaps the command's run function for one that only records that it
// ran, so the walk exercises flag parsing and the pre-run hooks without doing
// any of the command's work. The original is restored when the test ends.
func stubRun(t *testing.T, cmd *cobra.Command) *bool {
	t.Helper()
	ran := new(bool)
	origRun, origRunE := cmd.Run, cmd.RunE
	cmd.Run = nil
	cmd.RunE = func(*cobra.Command, []string) error {
		*ran = true
		return nil
	}
	t.Cleanup(func() { cmd.Run, cmd.RunE = origRun, origRunE })
	return ran
}

// acceptedArgs returns the shortest list of placeholder positional arguments
// the command's Args validator accepts, so the run reaches the pre-run hooks.
func acceptedArgs(t *testing.T, cmd *cobra.Command) []string {
	t.Helper()
	for n := range 4 {
		args := slices.Repeat([]string{"placeholder"}, n)
		if cmd.ValidateArgs(args) == nil {
			return args
		}
	}
	t.Fatalf("%s: no placeholder argument count satisfies its Args validator", cmd.CommandPath())
	return nil
}

// Every runnable command runs with -o text, and either honours -o json (it is
// in the documented set and runs) or rejects it before running, with nothing
// on stdout and an error that names the command. A new command that forgets
// to opt in lands in the reject branch, which is the safe default.
func TestOutputJSON_EveryCommandHonoursOrRejects(t *testing.T) {
	isolateHome(t)
	// cobra adds help and completion lazily at execute time; add them now so
	// the walk covers them too. Group commands become runnable the way
	// executeRoot makes them, so the walk covers those as well.
	rootCmd.InitDefaultHelpCmd()
	rootCmd.InitDefaultCompletionCmd()
	runGroupsAsHelp(rootCmd)

	cmds := runnableCommands(rootCmd)
	if len(cmds) < 20 {
		t.Fatalf("walk found only %d runnable commands; the tree did not load", len(cmds))
	}

	seen := map[string]bool{}
	for _, cmd := range cmds {
		path := cmd.CommandPath()
		seen[path] = true
		t.Run(path, func(t *testing.T) {
			ran := stubRun(t, cmd)
			args := append(strings.Fields(path)[1:], acceptedArgs(t, cmd)...)

			// Text output stays the default: the JSON gate never blocks it.
			if res := runRalph(t, append(args, "-o", "text")...); res.err != nil || !*ran {
				t.Fatalf("%s -o text did not run: err=%v ran=%v", path, res.err, *ran)
			}

			*ran = false
			res := runRalph(t, append(args, "-o", "json")...)

			if slices.Contains(jsonOutputCommands, path) {
				if res.err != nil || !*ran {
					t.Fatalf(
						"%s supports -o json but did not run: err=%v ran=%v",
						path,
						res.err,
						*ran,
					)
				}
				return
			}
			if res.err == nil {
				t.Fatalf(
					"%s accepted -o json without a JSON form (ran=%v, stdout=%q)",
					path,
					*ran,
					res.stdout,
				)
			}
			if *ran {
				t.Errorf("%s ran its work before rejecting -o json", path)
			}
			if res.stdout != "" {
				t.Errorf("%s wrote to stdout while rejecting -o json: %q", path, res.stdout)
			}
			want := `JSON output (-o json) is not supported for "` + path + `"`
			if !strings.Contains(res.err.Error(), want) {
				t.Errorf("%s rejection = %q, want it to contain %q", path, res.err.Error(), want)
			}
		})
	}

	for _, path := range jsonOutputCommands {
		if !seen[path] {
			t.Errorf("documented JSON command %q is not in the command tree", path)
		}
	}
}

// A group command (and bare ralph) has no JSON form either. cobra skips the
// pre-run hooks for a command with no run function, so this runs the real
// tree, not stubs: -o json fails with nothing on stdout, and without it a
// group still prints its help and succeeds.
func TestOutputJSON_GroupCommandsReject(t *testing.T) {
	isolateHome(t)
	for _, args := range [][]string{{}, {"state"}, {"profile"}, {"completion"}} {
		path := strings.Join(append([]string{"ralph"}, args...), " ")
		t.Run(path, func(t *testing.T) {
			res := runRalph(t, append(args, "-o", "json")...)
			want := `JSON output (-o json) is not supported for "` + path + `"`
			if res.err == nil || !strings.Contains(res.err.Error(), want) {
				t.Fatalf("%s -o json: err = %v, want it to contain %q", path, res.err, want)
			}
			if res.stdout != "" {
				t.Errorf("%s wrote to stdout while rejecting -o json: %q", path, res.stdout)
			}

			if len(args) == 0 {
				return // bare ralph prints a hint, not a group's help
			}
			res = runRalph(t, args...)
			if res.err != nil {
				t.Fatalf("%s without -o json: err = %v, want help and success", path, res.err)
			}
			if !strings.Contains(res.stdout, "Available Commands:") {
				t.Errorf("%s did not print its help on stdout:\n%s", path, res.stdout)
			}
		})
	}
}

// clean --recipe for a recipe the saved state does not know cleans nothing,
// but under -o json it must still print a report, not an empty stdout.
func TestClean_UnknownRecipeOutputJSON(t *testing.T) {
	home := isolateHome(t)
	dotfiles := filepath.Join(home, "dotfiles")
	if err := os.MkdirAll(dotfiles, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, `dotfiles_repo_path = "`+dotfiles+`"`+"\n")
	seeded := &state.RecipeState{
		Version: state.CurrentRecipeStateVersion,
		Recipes: map[string]state.RecipeArtifacts{
			"nvim": {DeleteBehavior: "delete", Symlinks: []string{filepath.Join(home, ".vimrc")}},
		},
	}
	if err := state.Save(seeded); err != nil {
		t.Fatalf("seeding state: %v", err)
	}
	const note = "Recipe 'gone' is not present in the previous manifest; nothing to clean."

	res := runRalph(t, "clean", "--recipe", "gone", "-o", "json")
	if res.err != nil {
		t.Fatalf("clean --recipe gone -o json: err = %v", res.err)
	}
	var got struct {
		Command string `json:"command"`
		Phases  []struct {
			Name  string `json:"name"`
			Steps []any  `json:"steps"`
		} `json:"phases"`
		ExitCode int `json:"exit_code"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &got); err != nil {
		t.Fatalf("stdout is not pure JSON: %v\n%s", err, res.stdout)
	}
	if got.Command != "clean" || got.ExitCode != 0 || len(got.Phases) != 1 ||
		got.Phases[0].Name != "Cleanup" || len(got.Phases[0].Steps) != 0 {
		t.Errorf("report = %+v, want command clean, exit 0, one empty Cleanup phase", got)
	}
	if !strings.Contains(res.stderr, note) {
		t.Errorf("stderr lost the note %q:\n%s", note, res.stderr)
	}

	// Text mode keeps today's output: the note on stderr, nothing on stdout.
	res = runRalph(t, "clean", "--recipe", "gone")
	if res.err != nil || res.stdout != "" || !strings.Contains(res.stderr, note) {
		t.Errorf(
			"clean --recipe gone: err=%v stdout=%q stderr=%q, want only the note",
			res.err,
			res.stdout,
			res.stderr,
		)
	}
}

// decodeJSON fails the test unless out is exactly one JSON document.
func decodeJSON(t *testing.T, label, out string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("%s is not pure JSON: %v\n%s", label, err, out)
	}
	return v
}

func TestStateShow_OutputJSONMatchesJSONFlag(t *testing.T) {
	isolateHome(t)
	seeded := &state.RecipeState{
		Version: state.CurrentRecipeStateVersion,
		Recipes: map[string]state.RecipeArtifacts{
			"nvim": {
				AppliedAt:      time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
				DeleteBehavior: "delete",
				Symlinks:       []string{"/tmp/example/.config/nvim"},
			},
		},
	}
	if err := state.Save(seeded); err != nil {
		t.Fatalf("seeding state: %v", err)
	}

	viaOutput := runRalph(t, "state", "show", "-o", "json")
	viaFlag := runRalph(t, "state", "show", "--json")
	if viaOutput.err != nil || viaFlag.err != nil {
		t.Fatalf("state show: -o json err=%v, --json err=%v", viaOutput.err, viaFlag.err)
	}

	got := decodeJSON(t, "state show -o json", viaOutput.stdout)
	decodeJSON(t, "state show --json", viaFlag.stdout)
	if viaOutput.stdout != viaFlag.stdout {
		t.Errorf(
			"-o json and --json differ:\n-o json:\n%s\n--json:\n%s",
			viaOutput.stdout,
			viaFlag.stdout,
		)
	}
	recipes, _ := got.(map[string]any)["recipes"].(map[string]any)
	if _, ok := recipes["nvim"]; !ok {
		t.Errorf("state show -o json lost the seeded recipe:\n%s", viaOutput.stdout)
	}
}

func TestOutdated_OutputJSONMatchesJSONFlag(t *testing.T) {
	home := isolateHome(t)
	// A disabled package is reported as skipped without any network or git
	// call, so the result is deterministic.
	writeConfig(t, `dotfiles_repo_path = "`+filepath.Join(home, "dotfiles")+`"

[packages.mytool]
source = "local"
working_dir = "`+filepath.Join(home, "src", "mytool")+`"
build = ["true"]
enable = false
`)

	viaOutput := runRalph(t, "outdated", "-o", "json")
	viaFlag := runRalph(t, "outdated", "--json")
	if viaOutput.err != nil || viaFlag.err != nil {
		t.Fatalf("outdated: -o json err=%v, --json err=%v", viaOutput.err, viaFlag.err)
	}

	got := decodeJSON(t, "outdated -o json", viaOutput.stdout)
	decodeJSON(t, "outdated --json", viaFlag.stdout)
	if viaOutput.stdout != viaFlag.stdout {
		t.Errorf(
			"-o json and --json differ:\n-o json:\n%s\n--json:\n%s",
			viaOutput.stdout,
			viaFlag.stdout,
		)
	}
	if results, _ := got.([]any); len(results) != 1 {
		t.Errorf("outdated -o json = %v, want one skipped package", got)
	}
}
