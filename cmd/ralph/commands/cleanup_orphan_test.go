package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/apply"
	"github.com/mad01/ralph/internal/cleanup"
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

// record saves the recipe-state manifest cfg declares, as a plain `ralph up`
// (no cleanup) on host anyhost would.
func record(t *testing.T, cfg *config.Config) {
	t.Helper()
	next, unowned, err := apply.IntendedManifest(cfg, "anyhost", time.Now())
	if err != nil {
		t.Fatalf("intended manifest: %v", err)
	}
	cleanup.Record(cfg, next, unowned, cleanup.Options{Out: io.Discard},
		&report.Report{Command: "test"})
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
	record(t, configWith(editors))

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
	record(t, configWith(editors))

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
	record(t, configWith(foo, bar))
	// Both recipes are disabled, then a plain run keeps them pending.
	record(t, configWith())
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

// clonedRepo returns a git checkout that pulls cleanly from a local origin.
func clonedRepo(t *testing.T) string {
	t.Helper()
	origin := filepath.Join(t.TempDir(), "origin")
	testutil.InitGitRepo(t, origin)
	clone := filepath.Join(t.TempDir(), "clone")
	testutil.RunGitCmd(t, filepath.Dir(clone), "clone", origin, clone)
	return clone
}

// MAD-587: runSyncSequence reports whether every sync step succeeded, so `ralph
// up` can skip cleanup removal after a failed pull.
func TestRunSyncSequence_ReportsFailure(t *testing.T) {
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
			name: "package sync fails",
			packages: map[string]config.Package{
				"tool": {Source: "remote", Repo: "/nonexistent/tool.git"},
			},
			want: false,
		},
		{
			name: "disabled package is skipped",
			packages: map[string]config.Package{
				"tool": {Source: "remote", Repo: "/nonexistent/tool.git", Enable: &disabled},
			},
			want: true,
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
				testutil.InitGitRepo(
					t,
					filepath.Join(home, ".config", "ralph", "sources", src.Name),
				)
			}
			got := runSyncSequence(
				io.Discard, cfg, "anyhost", syncOptions{}, &report.Report{Command: "test"},
			)
			if got != tt.want {
				t.Errorf("runSyncSequence() = %v, want %v", got, tt.want)
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
	record(t, configWith(gone))

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
