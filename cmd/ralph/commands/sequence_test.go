package commands

import (
	"encoding/json"
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
