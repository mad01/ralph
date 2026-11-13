package apply

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/cleanup"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

// testRecipe is one recipe in a test config: each link becomes a symlink
// dotfile it owns, and its pre_uninstall hook appends one byte to hookLog.
type testRecipe struct {
	name    string
	source  string
	links   []string
	hookLog string
}

// orphanHome points HOME (and with it the recipe-state file) at a temp dir.
func orphanHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// makeLink creates home/name as a symlink to a real file and returns its path.
func makeLink(t *testing.T, home, name string) string {
	t.Helper()
	target := filepath.Join(home, name+".src")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, name)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return link
}

// configWith builds a loaded config in which every given recipe is active.
func configWith(recipes ...testRecipe) *config.Config {
	cfg := &config.Config{Dotfiles: map[string]config.Dotfile{}}
	for _, r := range recipes {
		for i, link := range r.links {
			cfg.Dotfiles[fmt.Sprintf("%s-%d", r.name, i)] = config.Dotfile{
				Source:      "unused",
				Target:      link,
				OwnerRecipe: r.name,
			}
		}
		info := config.LoadedRecipeInfo{
			Name:           r.name,
			RecipeSource:   r.source,
			DeleteBehavior: config.DeleteBehaviorDelete,
		}
		if r.hookLog != "" {
			info.PreUninstall = []string{"printf x >> " + r.hookLog}
		}
		cfg.LoadedRecipes = append(cfg.LoadedRecipes, info)
	}
	return cfg
}

// record runs the apply sequence's last phase over cfg: build the intended
// manifest, then record it, removing orphans when opts.Cleanup is set.
func record(t *testing.T, cfg *config.Config, opts cleanup.Options) {
	t.Helper()
	opts.Out = io.Discard
	recordManifest(cfg, "anyhost", opts, &report.Report{Command: "test"})
}

func savedState(t *testing.T) *state.RecipeState {
	t.Helper()
	s, err := state.Load()
	if err != nil {
		t.Fatalf("load recipe state: %v", err)
	}
	return s
}

func linkExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// readFile returns the contents of path.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// findStep returns the StepResult for name in the named phase, or nil.
func findStep(rpt *report.Report, phase, name string) *report.StepResult {
	for i := range rpt.Phases {
		if rpt.Phases[i].Name != phase {
			continue
		}
		for j := range rpt.Phases[i].Steps {
			if rpt.Phases[i].Steps[j].Name == name {
				return &rpt.Phases[i].Steps[j]
			}
		}
	}
	return nil
}

// hookRuns counts how many times a testRecipe's pre_uninstall hook ran.
func hookRuns(t *testing.T, hookLog string) int {
	t.Helper()
	data, err := os.ReadFile(hookLog)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(data)
}

// MAD-587: a recipes directory that discovery would scan but finds missing
// (moved or unmounted dotfiles repo, a source that lost its recipes dir)
// loads as zero recipes. Cleanup must freeze what the previous state recorded
// for that origin, and only that origin.
func TestRecordManifest_MissingRecipesDirFreezesItsRecipes(t *testing.T) {
	tests := []struct {
		name    string
		prev    []testRecipe
		missing config.MissingRecipeDir
		kept    []string
	}{
		{
			name:    "local recipes dir",
			prev:    []testRecipe{{name: "editors"}, {name: "shell"}},
			missing: config.MissingRecipeDir{Path: "/dotfiles/recipes"},
			kept:    []string{"editors", "shell"},
		},
		{
			name: "one remote source",
			prev: []testRecipe{
				{name: "moon/app", source: "moon"},
				{name: "sun/app", source: "sun"},
				{name: "editors"},
			},
			missing: config.MissingRecipeDir{Source: "moon", Path: "/sources/moon/recipes"},
			kept:    []string{"moon/app"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := orphanHome(t)
			for i := range tt.prev {
				r := &tt.prev[i]
				r.links = []string{makeLink(t, home, fmt.Sprintf("link%d", i))}
				r.hookLog = filepath.Join(home, fmt.Sprintf("hook%d.log", i))
			}
			record(t, configWith(tt.prev...), cleanup.Options{})

			// This run loads no recipes at all; one origin's dir is missing.
			cfg := configWith()
			cfg.MissingRecipeDirs = []config.MissingRecipeDir{tt.missing}
			record(t, cfg, cleanup.Options{Cleanup: true})

			saved := savedState(t)
			for _, r := range tt.prev {
				keep := slices.Contains(tt.kept, r.name)
				if got := linkExists(r.links[0]); got != keep {
					t.Errorf("%s: link exists = %v, want %v", r.name, got, keep)
				}
				wantRuns := 1
				if keep {
					wantRuns = 0
				}
				if got := hookRuns(t, r.hookLog); got != wantRuns {
					t.Errorf("%s: uninstall hook ran %d time(s), want %d", r.name, got, wantRuns)
				}
				if _, ok := saved.Recipes[r.name]; ok != keep {
					t.Errorf("%s: recorded in saved state = %v, want %v", r.name, ok, keep)
				}
			}
		})
	}
}

// MAD-587: after a failed sync the config on disk may be stale or partial, so
// cleanup must not remove anything, and the state must still be saved.
func TestRecordManifest_SyncFailureSkipsRemoval(t *testing.T) {
	home := orphanHome(t)
	gone := testRecipe{
		name:    "gone",
		links:   []string{makeLink(t, home, "gone.link")},
		hookLog: filepath.Join(home, "gone.log"),
	}
	active := testRecipe{name: "active", links: []string{makeLink(t, home, "active.link")}}
	record(t, configWith(gone, active), cleanup.Options{})

	rpt := &report.Report{Command: "test"}
	recordManifest(
		configWith(active),
		"anyhost",
		cleanup.Options{Cleanup: true, SyncFailed: true, Out: io.Discard},
		rpt,
	)

	if !linkExists(gone.links[0]) {
		t.Error("orphan was removed although a sync step failed")
	}
	if got := hookRuns(t, gone.hookLog); got != 0 {
		t.Errorf("uninstall hook ran %d time(s) although a sync step failed", got)
	}
	saved := savedState(t)
	if _, ok := saved.Recipes["active"]; !ok {
		t.Error("state was not saved: active recipe missing")
	}
	if _, ok := saved.Recipes["gone"]; !ok {
		t.Error("skipped orphan was dropped from state; the next cleanup cannot find it")
	}
	if !rpt.HasWarnings() {
		t.Error("expected a cleanup-phase warning for the skipped removal")
	}
}

// MAD-588: a run without cleanup must keep the orphans it saw in state, so a
// later cleanup run still removes them and runs their uninstall hooks once.
func TestRecordManifest_DisabledRecipeIsCleanedAfterPlainRun(t *testing.T) {
	home := orphanHome(t)
	foo := testRecipe{
		name:    "foo",
		links:   []string{makeLink(t, home, "foo.link")},
		hookLog: filepath.Join(home, "foo.log"),
	}
	record(t, configWith(foo), cleanup.Options{})

	// ralph disable foo, then a plain ralph up.
	record(t, configWith(), cleanup.Options{})
	carried, ok := savedState(t).Recipes["foo"]
	if !ok {
		t.Fatal("plain run dropped the disabled recipe from state")
	}
	if !carried.PendingRemoval {
		t.Error("carried recipe is not marked pending removal")
	}
	if !linkExists(foo.links[0]) {
		t.Fatal("plain run removed an artifact")
	}

	record(t, configWith(), cleanup.Options{Cleanup: true})
	if linkExists(foo.links[0]) {
		t.Error("cleanup run did not remove the disabled recipe's artifact")
	}
	if got := hookRuns(t, foo.hookLog); got != 1 {
		t.Errorf("uninstall hook ran %d time(s), want 1", got)
	}
	if _, ok := savedState(t).Recipes["foo"]; ok {
		t.Error("cleaned recipe is still recorded in state")
	}

	record(t, configWith(), cleanup.Options{Cleanup: true})
	if got := hookRuns(t, foo.hookLog); got != 1 {
		t.Errorf("uninstall hook ran %d time(s) after a second cleanup, want 1", got)
	}
}

// MAD-588: an artifact dropped from a still-active recipe survives a plain
// run in state; the later cleanup removes only it, with no uninstall hooks.
func TestRecordManifest_PartialOrphanIsCleanedAfterPlainRun(t *testing.T) {
	home := orphanHome(t)
	keep := makeLink(t, home, "keep.link")
	drop := makeLink(t, home, "drop.link")
	hookLog := filepath.Join(home, "svc.log")
	record(t, configWith(testRecipe{name: "svc", links: []string{keep, drop}, hookLog: hookLog}),
		cleanup.Options{})

	after := testRecipe{name: "svc", links: []string{keep}, hookLog: hookLog}
	record(t, configWith(after), cleanup.Options{})
	if !slices.Contains(savedState(t).Recipes["svc"].Symlinks, drop) {
		t.Fatal("plain run dropped the orphaned artifact from state")
	}

	record(t, configWith(after), cleanup.Options{Cleanup: true})
	if linkExists(drop) {
		t.Error("cleanup run did not remove the dropped artifact")
	}
	if !linkExists(keep) {
		t.Error("cleanup run removed a still-declared artifact")
	}
	if got := hookRuns(t, hookLog); got != 0 {
		t.Errorf("uninstall hook ran %d time(s) for a partial orphan, want 0", got)
	}
}

// MAD-588: a recipe re-enabled after a plain run is active again. Nothing of
// it is removed, and its state entry is the fresh one, not the carried one.
func TestRecordManifest_ReenabledRecipeIsNotOrphaned(t *testing.T) {
	home := orphanHome(t)
	links := []string{makeLink(t, home, "a.link"), makeLink(t, home, "b.link")}
	oldHook := filepath.Join(home, "old.log")
	record(t, configWith(testRecipe{name: "foo", links: links, hookLog: oldHook}),
		cleanup.Options{})
	record(t, configWith(), cleanup.Options{})

	newHook := filepath.Join(home, "new.log")
	foo := testRecipe{name: "foo", links: links, hookLog: newHook}
	record(t, configWith(foo), cleanup.Options{})
	record(t, configWith(foo), cleanup.Options{Cleanup: true})

	for _, l := range links {
		if !linkExists(l) {
			t.Errorf("re-enabled recipe lost %s", l)
		}
	}
	if got := hookRuns(t, oldHook) + hookRuns(t, newHook); got != 0 {
		t.Errorf("uninstall hooks ran %d time(s) for a re-enabled recipe, want 0", got)
	}
	rec := savedState(t).Recipes["foo"]
	if rec.PendingRemoval {
		t.Error("re-enabled recipe is still marked pending removal")
	}
	if want := []string{"printf x >> " + newHook}; !slices.Equal(rec.PreUninstall, want) {
		t.Errorf("pre_uninstall = %v, want the fresh %v", rec.PreUninstall, want)
	}
}

// A path the config now declares with no owner recipe (straight in
// config.toml) is protected: a kept orphan that once owned it must not delete
// the live artifact.
func TestRecordManifest_UnownedConfigPathIsProtected(t *testing.T) {
	home := orphanHome(t)
	vimrc := makeLink(t, home, "vimrc")
	record(t, configWith(testRecipe{name: "vim", links: []string{vimrc}}), cleanup.Options{})

	// The recipe is deleted and the same dotfile moves into config.toml.
	cfg := configWith()
	cfg.Dotfiles["vimrc"] = config.Dotfile{Source: "unused", Target: vimrc}
	record(t, cfg, cleanup.Options{})
	record(t, cfg, cleanup.Options{Cleanup: true})

	if !linkExists(vimrc) {
		t.Error("cleanup removed a path the config still declares")
	}
	saved := savedState(t)
	if _, ok := saved.Recipes["vim"]; ok {
		t.Error("the deleted recipe is still recorded after cleanup")
	}
	if _, ok := saved.Recipes[""]; ok {
		t.Error("an ownerless entry was recorded in state")
	}
}

// A recipe kept because it is frozen is not pending removal: cleanup will not
// touch it while the freeze lasts.
func TestRecordManifest_FrozenRecipeIsNotPendingRemoval(t *testing.T) {
	home := orphanHome(t)
	foo := testRecipe{name: "foo", links: []string{makeLink(t, home, "foo.link")}}
	record(t, configWith(foo), cleanup.Options{})
	record(t, configWith(), cleanup.Options{})
	if !savedState(t).Recipes["foo"].PendingRemoval {
		t.Fatal("setup: disabled recipe is not pending removal")
	}

	cfg := configWith()
	cfg.MissingRecipeDirs = []config.MissingRecipeDir{{Path: "/dotfiles/recipes"}}
	record(t, cfg, cleanup.Options{})

	rec, ok := savedState(t).Recipes["foo"]
	if !ok {
		t.Fatal("frozen recipe was dropped from state")
	}
	if rec.PendingRemoval {
		t.Error("frozen recipe is still marked pending removal")
	}
}

// C001 — the recipe-state manifest is persisted on EVERY successful apply, not
// only when cleanup is enabled. Without this, a recipe added and removed between
// two cleanup runs is invisible to the next cleanup and leaks forever.
func TestRecordManifest_SavesEvenWithoutCleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &config.Config{
		Dotfiles: map[string]config.Dotfile{
			"foo": {
				Source:      "foo.conf",
				Target:      filepath.Join(home, "foo.conf"),
				OwnerRecipe: "fooer",
			},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{{Name: "fooer", DeleteBehavior: "delete"}},
	}

	rpt := &report.Report{Command: "up"}
	// shouldCleanup=false: cleanup is OFF, but the manifest must still be saved.
	recordManifest(cfg, "anyhost", cleanup.Options{Out: &bytes.Buffer{}}, rpt)

	saved, err := state.Load()
	if err != nil {
		t.Fatalf("load saved state: %v", err)
	}
	rec, ok := saved.Recipes["fooer"]
	if !ok {
		t.Fatalf("expected recipe 'fooer' recorded without cleanup, got %v", saved.Recipes)
	}
	if len(rec.Symlinks) != 1 || rec.Symlinks[0] != filepath.Join(home, "foo.conf") {
		t.Errorf("expected dotfile target tracked, got %v", rec.Symlinks)
	}
}

// C001 — dry-run must never write the manifest.
func TestRecordManifest_DryRunDoesNotSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &config.Config{
		Dotfiles: map[string]config.Dotfile{
			"foo": {
				Source:      "foo.conf",
				Target:      filepath.Join(home, "foo.conf"),
				OwnerRecipe: "fooer",
			},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{{Name: "fooer", DeleteBehavior: "delete"}},
	}

	rpt := &report.Report{Command: "up"}
	recordManifest(cfg, "anyhost", cleanup.Options{DryRun: true, Out: &bytes.Buffer{}}, rpt)

	p, _ := state.GetStatePath()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("dry-run must not write the state file, but it exists (err=%v)", err)
	}
}

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

	recordManifest(
		configWith(active),
		"anyhost",
		cleanup.Options{Cleanup: true, Out: io.Discard},
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

	recordManifest(
		configWith(active),
		"anyhost",
		cleanup.Options{Cleanup: true, DryRun: true, Out: io.Discard},
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

	recordManifest(
		configWith(active),
		"anyhost",
		cleanup.Options{Cleanup: true, Out: io.Discard},
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
