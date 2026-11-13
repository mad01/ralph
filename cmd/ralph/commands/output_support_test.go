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
// captures what it writes to stdout and stderr. cobra's writers are left
// unset, so its own output lands on the same streams as in the binary.
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
			_, res.err = rootCmd.ExecuteC()
		})
	})
	return res
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
	// the walk covers them too.
	rootCmd.InitDefaultHelpCmd()
	rootCmd.InitDefaultCompletionCmd()

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
	cfg := `dotfiles_repo_path = "` + filepath.Join(home, "dotfiles") + `"

[packages.mytool]
source = "local"
working_dir = "` + filepath.Join(home, "src", "mytool") + `"
build = ["true"]
enable = false
`
	cfgPath := os.Getenv("RALPH_CONFIG")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

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
