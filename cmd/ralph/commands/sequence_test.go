package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/gitutil"
	"github.com/mad01/ralph/internal/testutil"
)

// applyPhases is the order of the apply sequence's phases when every item
// type is configured and cleanup is on.
var applyPhases = []string{
	"Pre-apply hooks",
	"Directories",
	"Dirs mirror",
	"Repositories",
	"Dotfiles",
	"Shell config",
	"Tools",
	"Builds",
	"Packages",
	"Post-apply hooks",
	"Cleanup",
}

// sequenceFixture is a config with one item of every apply phase, a dotfiles
// repo cloned from a local origin, and an origin commit the clone has not
// pulled yet.
type sequenceFixture struct {
	home   string
	repo   string // the dotfiles repo, a clone of origin
	origin string
	log    string // hooks, the build and the package append their name here
}

// newSequenceFixture writes the fixture config where isolateHome points.
func newSequenceFixture(t *testing.T) sequenceFixture {
	t.Helper()
	home := isolateHome(t)
	f := sequenceFixture{
		home:   home,
		origin: filepath.Join(t.TempDir(), "origin"),
		repo:   filepath.Join(home, "dotfiles"),
		log:    filepath.Join(home, "run.log"),
	}
	testutil.InitGitRepo(t, f.origin)
	testutil.RunGitCmd(t, home, "clone", "-q", f.origin, f.repo)
	writeRepoFile(t, filepath.Join(f.repo, "rc.txt"), "rc")
	writeRepoFile(t, filepath.Join(f.repo, "mirror", "tool.conf"), "conf")
	advanceOrigin(t, f.origin)
	pkgDir := filepath.Join(home, "pkg-src")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeConfig(t, fmt.Sprintf(`dotfiles_repo_path = %[1]q

[hooks]
pre_apply = ["echo pre >> %[2]s"]
post_apply = ["echo post >> %[2]s"]

[hooks.builds.marker]
commands = ["echo build >> %[2]s"]
run = "always"

[packages.lib]
source = "local"
working_dir = %[3]q
build = ["echo package >> %[2]s"]
depends_on = ["builds.marker"]

[directories.made]
target = %[4]q

[dirs_mirror.conf]
source = "mirror"
target = %[5]q

[repos.extra]
url = %[6]q
target = %[7]q

[dotfiles.rc]
source = "rc.txt"
target = %[8]q

[[tools]]
name = "sh"
check_command = "true"
`,
		f.repo, f.log, pkgDir,
		filepath.Join(home, "made"),
		filepath.Join(home, "mirror-out"),
		f.origin, filepath.Join(home, "extra"),
		filepath.Join(home, ".rc"),
	))
	return f
}

// synced reports whether the run pulled the dotfiles repo up to origin.
func (f sequenceFixture) synced() bool {
	return gitutil.GetGitHash(f.repo) == gitutil.GetGitHash(f.origin)
}

// applied reports whether the run created the directory and ran the build.
func (f sequenceFixture) applied(t *testing.T) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(f.home, "made"))
	return err == nil && slices.Contains(f.logLines(t), "build")
}

// logLines returns what the hooks, the build and the package logged, in order.
func (f sequenceFixture) logLines(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(f.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

// writeRepoFile writes body to path, creating its parent directories.
func writeRepoFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// phaseNames decodes a JSON report and returns its phase names in order.
func phaseNames(t *testing.T, out string) []string {
	t.Helper()
	var rpt struct {
		Command string `json:"command"`
		Phases  []struct {
			Name string `json:"name"`
		} `json:"phases"`
	}
	if err := json.Unmarshal([]byte(out), &rpt); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	names := make([]string, 0, len(rpt.Phases))
	for _, p := range rpt.Phases {
		names = append(names, p.Name)
	}
	return names
}

// The apply sequence runs its phases in a fixed order, and its hooks, build
// and package take effect in that order.
func TestApply_RunsPhasesInOrder(t *testing.T) {
	f := newSequenceFixture(t)

	res := runRalph(t, "apply", "-o", "json", "--enable-cleanup")
	if res.err != nil {
		t.Fatalf("apply failed: %v\nstderr: %s", res.err, res.stderr)
	}

	if got := phaseNames(t, res.stdout); !slices.Equal(got, applyPhases) {
		t.Errorf("apply phases = %q\nwant %q", got, applyPhases)
	}
	want := []string{"pre", "build", "package", "post"}
	if got := f.logLines(t); !slices.Equal(got, want) {
		t.Errorf("effects ran in order %q, want %q", got, want)
	}
}

// `ralph up` is the sync sequence followed by the same apply sequence.
func TestUp_SyncsThenRunsApplySequence(t *testing.T) {
	newSequenceFixture(t)

	res := runRalph(t, "up", "-o", "json", "--enable-cleanup")
	if res.err != nil {
		t.Fatalf("up failed: %v\nstderr: %s", res.err, res.stderr)
	}

	want := append([]string{"Dotfiles repo", "Packages (sync)"}, applyPhases...)
	if got := phaseNames(t, res.stdout); !slices.Equal(got, want) {
		t.Errorf("up phases = %q\nwant %q", got, want)
	}
}

// A dry run pulls nothing, so the dotfiles repo step says it would pull. The
// step keeps its status; only the message differs from a real run.
func TestUp_DryRunSaysDotfilesRepoWouldPull(t *testing.T) {
	f := newSequenceFixture(t)

	res := runRalph(t, "up", "--dry-run", "-o", "json")
	if res.err != nil {
		t.Fatalf("up --dry-run failed: %v\nstderr: %s", res.err, res.stderr)
	}

	var rpt struct {
		Phases []struct {
			Name  string `json:"name"`
			Steps []struct {
				Name    string `json:"name"`
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"steps"`
		} `json:"phases"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &rpt); err != nil {
		t.Fatalf("decode report: %v\n%s", err, res.stdout)
	}
	if len(rpt.Phases) == 0 || len(rpt.Phases[0].Steps) != 1 {
		t.Fatalf("want a first phase with one step, got %+v", rpt.Phases)
	}
	step := rpt.Phases[0].Steps[0]
	if step.Name != "dotfiles-repo" || step.Status != "ok" ||
		step.Message != "[DRY RUN] would pull" {
		t.Errorf("dotfiles repo step = %+v, want dotfiles-repo, ok, %q",
			step, "[DRY RUN] would pull")
	}
	if f.synced() {
		t.Error("dry run pulled the dotfiles repo")
	}
}

// apply never syncs, sync never applies, and up does both unless --no-sync.
func TestCommands_SyncAndApplyHalves(t *testing.T) {
	tests := []struct {
		args        []string
		wantSynced  bool
		wantApplied bool
	}{
		{args: []string{"up"}, wantSynced: true, wantApplied: true},
		{args: []string{"up", "--no-sync"}, wantSynced: false, wantApplied: true},
		{args: []string{"apply"}, wantSynced: false, wantApplied: true},
		{args: []string{"sync"}, wantSynced: true, wantApplied: false},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			f := newSequenceFixture(t)

			res := runRalph(t, tt.args...)
			if res.err != nil {
				t.Fatalf("ralph %v failed: %v\nstderr: %s", tt.args, res.err, res.stderr)
			}

			if got := f.synced(); got != tt.wantSynced {
				t.Errorf("synced = %v, want %v", got, tt.wantSynced)
			}
			if got := f.applied(t); got != tt.wantApplied {
				t.Errorf("applied = %v, want %v", got, tt.wantApplied)
			}
		})
	}
}

// The cleanup banner names the toggle that turned cleanup on: the
// --enable-cleanup flag wins over auto_cleanup, and with neither there is no
// banner.
func TestUp_CleanupBannerNamesItsToggle(t *testing.T) {
	const flagBanner = "Processing recipe cleanup (--enable-cleanup)..."
	const autoBanner = "Processing recipe cleanup (auto_cleanup=true)..."
	tests := []struct {
		name string
		flag bool
		auto bool
		want string
	}{
		{name: "flag", flag: true, want: flagBanner},
		{name: "auto_cleanup", auto: true, want: autoBanner},
		{name: "flag and auto_cleanup", flag: true, auto: true, want: flagBanner},
		{name: "off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateHome(t)
			body := fmt.Sprintf("dotfiles_repo_path = %q\n", filepath.Join(home, "dotfiles"))
			if tt.auto {
				body += "\n[recipes_config]\nauto_cleanup = true\n"
			}
			writeConfig(t, body)
			args := []string{"up", "--no-sync", "--verbose"}
			if tt.flag {
				args = append(args, "--enable-cleanup")
			}

			res := runRalph(t, args...)
			if res.err != nil {
				t.Fatalf("ralph %v: %v\nstderr: %s", args, res.err, res.stderr)
			}

			got := strings.Count(res.stdout, "Processing recipe cleanup")
			switch {
			case tt.want == "" && got != 0:
				t.Errorf("cleanup is off but stdout has %d banner(s):\n%s", got, res.stdout)
			case tt.want != "" && (got != 1 || !strings.Contains(res.stdout, tt.want)):
				t.Errorf("stdout has %d banner(s), want one %q:\n%s", got, tt.want, res.stdout)
			}
		})
	}
}

// reloadFixture is a dotfiles repo cloned from a local origin and a config
// that links base.txt straight from config.toml and auto-discovers recipes.
// The origin then gains a commit adding recipes/pulled/recipe.toml, which
// the clone has not pulled yet.
func newReloadFixture(t *testing.T, recipe string) (home string) {
	t.Helper()
	home = isolateHome(t)
	origin := filepath.Join(t.TempDir(), "origin")
	testutil.InitGitRepo(t, origin)
	repo := filepath.Join(home, "dotfiles")
	testutil.RunGitCmd(t, home, "clone", "-q", origin, repo)

	writeRepoFile(t, filepath.Join(origin, "recipes", "pulled", "recipe.toml"), recipe)
	writeRepoFile(t, filepath.Join(origin, "recipes", "pulled", "pulled.txt"), "pulled")
	testutil.RunGitCmd(t, origin, "add", ".")
	testutil.RunGitCmd(t, origin, "commit", "-q", "-m", "add the pulled recipe")

	writeConfig(t, fmt.Sprintf(`dotfiles_repo_path = %q

[recipes_config]
auto_discover = true

[dotfiles.base]
source = "test.txt"
target = %q
`, repo, filepath.Join(home, ".base")))
	return home
}

// `ralph up` reloads the config when the pull moved the dotfiles repo, so the
// same run applies what the pull brought in. A config that no longer loads
// after the pull is not applied: the run applies the pre-pull config.
func TestUp_AppliesConfigReloadedAfterPull(t *testing.T) {
	tests := []struct {
		name       string
		recipe     string
		wantPulled bool
		wantOut    string
		wantErr    string
	}{
		{
			name: "reloaded config is applied",
			recipe: "[recipe]\nname = \"pulled\"\n\n" +
				"[dotfiles.pulled]\nsource = \"pulled.txt\"\ntarget = \"~/.pulled\"\n",
			wantPulled: true,
			wantOut:    "reloaded config",
		},
		{
			name:    "reload error keeps the pre-pull config",
			recipe:  "[recipe\nthis is not toml\n",
			wantErr: "config reload failed; applying pre-pull config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := newReloadFixture(t, tt.recipe)

			res := runRalph(t, "up", "--verbose")
			if res.err != nil {
				t.Fatalf("up: %v\nstdout: %s\nstderr: %s", res.err, res.stdout, res.stderr)
			}

			if _, err := os.Lstat(filepath.Join(home, ".base")); err != nil {
				t.Errorf("the pre-pull dotfile was not linked: %v", err)
			}
			_, err := os.Lstat(filepath.Join(home, ".pulled"))
			if got := err == nil; got != tt.wantPulled {
				t.Errorf("pulled recipe's dotfile linked = %v, want %v", got, tt.wantPulled)
			}
			if tt.wantOut != "" && !strings.Contains(res.stdout, tt.wantOut) {
				t.Errorf("stdout does not say %q:\n%s", tt.wantOut, res.stdout)
			}
			if tt.wantErr != "" && !strings.Contains(res.stderr, tt.wantErr) {
				t.Errorf("stderr does not say %q:\n%s", tt.wantErr, res.stderr)
			}
		})
	}
}

// `ralph sync` exits non-zero when a sync step fails, and zero otherwise.
func TestSync_ExitCode(t *testing.T) {
	tests := []struct {
		name       string
		brokenRepo bool
		wantFail   bool
	}{
		{name: "every step succeeds"},
		{name: "dotfiles pull fails", brokenRepo: true, wantFail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateHome(t)
			repo := filepath.Join(home, "dotfiles")
			if tt.brokenRepo {
				// A plain directory, not a git checkout: its pull fails.
				if err := os.MkdirAll(repo, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				origin := filepath.Join(t.TempDir(), "origin")
				testutil.InitGitRepo(t, origin)
				testutil.RunGitCmd(t, home, "clone", "-q", origin, repo)
			}
			writeConfig(t, fmt.Sprintf("dotfiles_repo_path = %q\n", repo))

			res := runRalph(t, "sync")

			var exitErr *ExitError
			switch {
			case !tt.wantFail && res.err != nil:
				t.Errorf("sync: %v, want success\nstderr: %s", res.err, res.stderr)
			case tt.wantFail && !errors.As(res.err, &exitErr):
				t.Errorf("sync error = %v, want an *ExitError", res.err)
			case tt.wantFail && exitErr.Code == 0:
				t.Errorf("sync exit code = 0, want non-zero")
			}
		})
	}
}
