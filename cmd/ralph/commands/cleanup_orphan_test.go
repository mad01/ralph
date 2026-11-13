package commands

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
	"github.com/mad01/ralph/internal/testutil"
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

func record(t *testing.T, cfg *config.Config, opts manifestOptions) {
	t.Helper()
	recordManifestAndCleanup(cfg, "anyhost", opts, io.Discard, &report.Report{Command: "test"})
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
			record(t, configWith(tt.prev...), manifestOptions{})

			// This run loads no recipes at all; one origin's dir is missing.
			cfg := configWith()
			cfg.MissingRecipeDirs = []config.MissingRecipeDir{tt.missing}
			record(t, cfg, manifestOptions{cleanup: true})

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
	record(t, configWith(gone, active), manifestOptions{})

	rpt := &report.Report{Command: "test"}
	recordManifestAndCleanup(
		configWith(active),
		"anyhost",
		manifestOptions{cleanup: true, syncFailed: true},
		io.Discard,
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
	record(t, configWith(foo), manifestOptions{})

	// ralph disable foo, then a plain ralph up.
	record(t, configWith(), manifestOptions{})
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

	record(t, configWith(), manifestOptions{cleanup: true})
	if linkExists(foo.links[0]) {
		t.Error("cleanup run did not remove the disabled recipe's artifact")
	}
	if got := hookRuns(t, foo.hookLog); got != 1 {
		t.Errorf("uninstall hook ran %d time(s), want 1", got)
	}
	if _, ok := savedState(t).Recipes["foo"]; ok {
		t.Error("cleaned recipe is still recorded in state")
	}

	record(t, configWith(), manifestOptions{cleanup: true})
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
		manifestOptions{})

	after := testRecipe{name: "svc", links: []string{keep}, hookLog: hookLog}
	record(t, configWith(after), manifestOptions{})
	if !slices.Contains(savedState(t).Recipes["svc"].Symlinks, drop) {
		t.Fatal("plain run dropped the orphaned artifact from state")
	}

	record(t, configWith(after), manifestOptions{cleanup: true})
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
		manifestOptions{})
	record(t, configWith(), manifestOptions{})

	newHook := filepath.Join(home, "new.log")
	foo := testRecipe{name: "foo", links: links, hookLog: newHook}
	record(t, configWith(foo), manifestOptions{})
	record(t, configWith(foo), manifestOptions{cleanup: true})

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

// useConfigFile points config loading at a config.toml whose dotfiles repo is
// home/dotfiles (created) and returns the repo path. extra is appended to the
// file body.
func useConfigFile(t *testing.T, home, extra string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", "")
	repo := filepath.Join(home, "dotfiles")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "config.toml")
	body := fmt.Sprintf("dotfiles_repo_path = %q\n\n%s", repo, extra)
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := config.GetDefaultConfigPath
	config.GetDefaultConfigPath = func() (string, error) { return configPath, nil }
	t.Cleanup(func() { config.GetDefaultConfigPath = orig })
	return repo
}

// runClean runs `ralph clean`, scoped to one recipe when recipe is set.
func runClean(t *testing.T, recipe string) {
	t.Helper()
	setDryRun(t, false)
	orig := cleanRecipe
	cleanRecipe = recipe
	t.Cleanup(func() { cleanRecipe = orig })
	if err := cleanCmd.RunE(cleanCmd, nil); err != nil {
		t.Fatalf("clean: %v", err)
	}
}

const autoDiscover = "[recipes_config]\nauto_discover = true\n"

// MAD-587: `ralph clean` freezes the recipes of a missing recipes directory
// exactly like `ralph up --enable-cleanup`.
func TestClean_MissingLocalRecipesDirFreezesRecipes(t *testing.T) {
	home := orphanHome(t)
	editors := testRecipe{
		name:    "editors",
		links:   []string{makeLink(t, home, "editors.link")},
		hookLog: filepath.Join(home, "editors.log"),
	}
	record(t, configWith(editors), manifestOptions{})

	// The dotfiles repo exists but its recipes dir is gone.
	useConfigFile(t, home, autoDiscover)
	runClean(t, "")

	if !linkExists(editors.links[0]) {
		t.Error("clean removed an artifact of a recipe whose recipes dir is missing")
	}
	if got := hookRuns(t, editors.hookLog); got != 0 {
		t.Errorf("uninstall hook ran %d time(s), want 0", got)
	}
	if _, ok := savedState(t).Recipes["editors"]; !ok {
		t.Error("clean dropped the frozen recipe from state")
	}
}

// MAD-587: the freeze is for a missing directory only. A recipes directory
// that exists and is empty means the recipes were removed on purpose.
func TestClean_EmptyRecipesDirStillOrphans(t *testing.T) {
	home := orphanHome(t)
	editors := testRecipe{
		name:    "editors",
		links:   []string{makeLink(t, home, "editors.link")},
		hookLog: filepath.Join(home, "editors.log"),
	}
	record(t, configWith(editors), manifestOptions{})

	repo := useConfigFile(t, home, autoDiscover)
	if err := os.MkdirAll(filepath.Join(repo, config.DefaultRecipesDir), 0o755); err != nil {
		t.Fatal(err)
	}
	runClean(t, "")

	if linkExists(editors.links[0]) {
		t.Error("clean kept the artifact of a recipe removed from an existing recipes dir")
	}
	if got := hookRuns(t, editors.hookLog); got != 1 {
		t.Errorf("uninstall hook ran %d time(s), want 1", got)
	}
	if _, ok := savedState(t).Recipes["editors"]; ok {
		t.Error("cleaned recipe is still recorded in state")
	}
}

// `ralph clean --recipe X` saves X's outcome, so its uninstall hooks do not
// run again on the next cleanup, and leaves every other recipe as recorded.
func TestClean_RecipeScopeSavesOnlyThatRecipe(t *testing.T) {
	home := orphanHome(t)
	foo := testRecipe{
		name:    "foo",
		links:   []string{makeLink(t, home, "foo.link")},
		hookLog: filepath.Join(home, "foo.log"),
	}
	bar := testRecipe{
		name:    "bar",
		links:   []string{makeLink(t, home, "bar.link")},
		hookLog: filepath.Join(home, "bar.log"),
	}
	record(t, configWith(foo, bar), manifestOptions{})
	// Both recipes are disabled, then a plain run keeps them pending.
	record(t, configWith(), manifestOptions{})
	before := savedState(t).Recipes["bar"]

	useConfigFile(t, home, "")
	runClean(t, "foo")

	if linkExists(foo.links[0]) {
		t.Error("clean --recipe foo did not remove foo's artifact")
	}
	saved := savedState(t)
	if _, ok := saved.Recipes["foo"]; ok {
		t.Error("clean --recipe foo left foo recorded; its hooks would run again")
	}
	after, ok := saved.Recipes["bar"]
	if !ok || !slices.Equal(after.Symlinks, before.Symlinks) ||
		!slices.Equal(after.PreUninstall, before.PreUninstall) ||
		after.PendingRemoval != before.PendingRemoval {
		t.Errorf("clean --recipe foo changed bar's entry: got %+v, want %+v", after, before)
	}
	if !linkExists(bar.links[0]) || hookRuns(t, bar.hookLog) != 0 {
		t.Error("clean --recipe foo touched bar")
	}

	runClean(t, "")
	if got := hookRuns(t, foo.hookLog); got != 1 {
		t.Errorf("foo's uninstall hook ran %d time(s) in total, want 1", got)
	}
	if linkExists(bar.links[0]) || hookRuns(t, bar.hookLog) != 1 {
		t.Error("the full clean did not remove bar")
	}
}

// A path the config now declares with no owner recipe (straight in
// config.toml) is protected: a kept orphan that once owned it must not delete
// the live artifact.
func TestRecordManifest_UnownedConfigPathIsProtected(t *testing.T) {
	home := orphanHome(t)
	vimrc := makeLink(t, home, "vimrc")
	record(t, configWith(testRecipe{name: "vim", links: []string{vimrc}}), manifestOptions{})

	// The recipe is deleted and the same dotfile moves into config.toml.
	cfg := configWith()
	cfg.Dotfiles["vimrc"] = config.Dotfile{Source: "unused", Target: vimrc}
	record(t, cfg, manifestOptions{})
	record(t, cfg, manifestOptions{cleanup: true})

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
	record(t, configWith(foo), manifestOptions{})
	record(t, configWith(), manifestOptions{})
	if !savedState(t).Recipes["foo"].PendingRemoval {
		t.Fatal("setup: disabled recipe is not pending removal")
	}

	cfg := configWith()
	cfg.MissingRecipeDirs = []config.MissingRecipeDir{{Path: "/dotfiles/recipes"}}
	record(t, cfg, manifestOptions{})

	rec, ok := savedState(t).Recipes["foo"]
	if !ok {
		t.Fatal("frozen recipe was dropped from state")
	}
	if rec.PendingRemoval {
		t.Error("frozen recipe is still marked pending removal")
	}
}

// MAD-587: one warning per missing recipes directory that had recipes kept,
// naming the directory and the count; none when nothing was kept.
func TestKeepFrozenRecipes_WarnsPerMissingDirWithKeptRecipes(t *testing.T) {
	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{
		"editors":  {Symlinks: []string{"/home/u/.vimrc"}},
		"shell":    {Symlinks: []string{"/home/u/.zshrc"}},
		"moon/app": {RecipeSource: "moon", Symlinks: []string{"/home/u/.app"}},
	}}
	cfg := &config.Config{MissingRecipeDirs: []config.MissingRecipeDir{
		{Path: "/dotfiles/recipes"},
		{Source: "sun", Path: "/sources/sun/recipes"},
	}}
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	var errW bytes.Buffer
	keepFrozenRecipes(cfg, prev, next, &errW)

	lines := strings.Split(strings.TrimSpace(errW.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d warning line(s), want 1: %q", len(lines), errW.String())
	}
	if !strings.Contains(lines[0], "/dotfiles/recipes") || !strings.Contains(lines[0], "2 recipe") {
		t.Errorf("warning does not name the directory and the count: %q", lines[0])
	}
	if strings.Contains(errW.String(), "/sources/sun/recipes") {
		t.Errorf("warned about a directory with nothing kept: %q", errW.String())
	}
}

// clonedRepo returns a git checkout that pulls cleanly from a local origin.
func clonedRepo(t *testing.T) string {
	t.Helper()
	origin := filepath.Join(t.TempDir(), "origin")
	testutil.InitGitRepo(t, origin)
	clone := filepath.Join(t.TempDir(), "clone")
	testutil.RunGitCmd(t, filepath.Dir(clone), "clone", origin, clone)
	return clone
}

// MAD-587: runSyncPhase reports whether every sync step succeeded, so `ralph
// up` can skip cleanup removal after a failed pull.
func TestRunSyncPhase_ReportsFailure(t *testing.T) {
	disabled := false
	tests := []struct {
		name       string
		dryRun     bool
		brokenRepo bool
		sources    []config.RecipeSource
		packages   map[string]config.Package
		want       bool
	}{
		{name: "dotfiles pull fails", brokenRepo: true, want: false},
		{name: "dry run pulls nothing", dryRun: true, brokenRepo: true, want: true},
		{name: "clean pull, nothing else to sync", want: true},
		{
			name:    "recipe source pull fails",
			sources: []config.RecipeSource{{Name: "moon", Update: true}},
			want:    false,
		},
		{
			name:    "disabled recipe source is skipped",
			sources: []config.RecipeSource{{Name: "moon", Update: true, Enable: &disabled}},
			want:    true,
		},
		{
			name:     "package sync fails",
			packages: map[string]config.Package{"tool": {Source: "remote", Repo: "/nonexistent/tool.git"}},
			want:     false,
		},
		{
			name:     "disabled package is skipped",
			packages: map[string]config.Package{"tool": {Source: "remote", Repo: "/nonexistent/tool.git", Enable: &disabled}},
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := orphanHome(t)
			setDryRun(t, tt.dryRun)
			cfg := &config.Config{
				DotfilesRepoPath: filepath.Join(t.TempDir(), "missing"),
				RecipeSources:    tt.sources,
				Packages:         tt.packages,
				PackagesDir:      filepath.Join(home, "pkg"),
			}
			if !tt.brokenRepo {
				cfg.DotfilesRepoPath = clonedRepo(t)
			}
			// A source checkout on a branch with no remote: its pull fails.
			for _, src := range tt.sources {
				testutil.InitGitRepo(t, filepath.Join(home, ".config", "ralph", "sources", src.Name))
			}
			got := runSyncPhase(io.Discard, cfg, "anyhost", &report.Report{Command: "test"})
			if got != tt.want {
				t.Errorf("runSyncPhase() = %v, want %v", got, tt.want)
			}
		})
	}
}

// MAD-587: `ralph up --enable-cleanup` passes a failed sync on to cleanup, so
// an orphan survives a run whose dotfiles pull failed.
func TestUp_FailedPullSkipsOrphanRemoval(t *testing.T) {
	home := orphanHome(t)
	gone := testRecipe{
		name:    "gone",
		links:   []string{makeLink(t, home, "gone.link")},
		hookLog: filepath.Join(home, "gone.log"),
	}
	record(t, configWith(gone), manifestOptions{})

	// The dotfiles repo is a plain directory, so its pull fails.
	useConfigFile(t, home, "")
	setDryRun(t, false)
	origCleanup, origNoSync := upEnableCleanup, upNoSync
	upEnableCleanup, upNoSync = true, false
	t.Cleanup(func() { upEnableCleanup, upNoSync = origCleanup, origNoSync })

	// The failed pull makes the run exit non-zero; only the outcome matters.
	_ = upCmd.RunE(upCmd, nil)

	if !linkExists(gone.links[0]) {
		t.Error("up removed an orphan although the dotfiles pull failed")
	}
	if got := hookRuns(t, gone.hookLog); got != 0 {
		t.Errorf("uninstall hook ran %d time(s) although the dotfiles pull failed", got)
	}
	if _, ok := savedState(t).Recipes["gone"]; !ok {
		t.Error("the skipped orphan was dropped from state")
	}
}
