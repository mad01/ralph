package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/testutil"
)

// sourceConfig is a config.toml fragment declaring one recipe source, moon,
// cloned from a fresh local origin repo.
func sourceConfig(t *testing.T) string {
	t.Helper()
	origin := filepath.Join(t.TempDir(), "origin")
	testutil.InitGitRepo(t, origin)
	return "[[recipe_sources]]\nname = \"moon\"\nurl = \"" + origin + "\"\n"
}

// A dry run loads the config read-only: the recipe source is not cloned and
// its recipes dir is recorded missing. A real run of the same load clones it.
func TestLoadConfigOrReport_DryRunDoesNotCloneSource(t *testing.T) {
	home := orphanHome(t)
	useConfigFile(t, home, sourceConfig(t))
	sourcesDir := filepath.Join(home, ".config", "ralph", "sources")

	setDryRun(t, true)
	var cfgErr error
	stderr := captureFile(t, &os.Stderr, func() {
		cfg, err := loadConfigOrReport(&report.Report{Command: "up"})
		cfgErr = err
		if err == nil && (len(cfg.MissingRecipeDirs) != 1 ||
			cfg.MissingRecipeDirs[0].Source != "moon") {
			t.Errorf("MissingRecipeDirs = %+v, want moon's", cfg.MissingRecipeDirs)
		}
	})
	if cfgErr != nil {
		t.Fatalf("dry-run load: %v", cfgErr)
	}
	if _, err := os.Lstat(sourcesDir); !os.IsNotExist(err) {
		t.Errorf("dry-run load created %s (stat err %v)", sourcesDir, err)
	}
	if !strings.Contains(stderr, "recipe source 'moon' is not cloned") {
		t.Errorf("stderr does not say the source was not cloned:\n%s", stderr)
	}

	setDryRun(t, false)
	if _, err := loadConfigOrReport(&report.Report{Command: "up"}); err != nil {
		t.Fatalf("real load: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourcesDir, "moon", ".git")); err != nil {
		t.Errorf("real load did not clone the source: %v", err)
	}
}

// `ralph up --dry-run --enable-cleanup` with a source that is not cloned keeps
// the recipes state recorded from that source: none of them is an orphan.
func TestUp_DryRunCleanupKeepsUnclonedSourceRecipes(t *testing.T) {
	home := orphanHome(t)
	app := testRecipe{
		name:    "moon/app",
		source:  "moon",
		links:   []string{makeLink(t, home, "app.link")},
		hookLog: filepath.Join(home, "app.log"),
	}
	record(t, configWith(app))
	useConfigFile(t, home, sourceConfig(t))

	setDryRun(t, true)
	origCleanup, origNoSync := upEnableCleanup, upNoSync
	upEnableCleanup, upNoSync = true, true
	t.Cleanup(func() { upEnableCleanup, upNoSync = origCleanup, origNoSync })

	var runErr error
	out := captureStdout(t, func() {
		withOutputFormat("json", func() { runErr = upCmd.RunE(upCmd, nil) })
	})
	if runErr != nil {
		t.Fatalf("up --dry-run: %v\n%s", runErr, out)
	}
	if step := jsonStep(t, out, "Cleanup", "cleanup"); step.Message != "no orphans" {
		t.Errorf("cleanup step = %+v, want no orphans", step)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config", "ralph", "sources")); !os.IsNotExist(err) {
		t.Errorf("up --dry-run created the sources dir (stat err %v)", err)
	}
	if got := hookRuns(t, app.hookLog); got != 0 {
		t.Errorf("uninstall hook ran %d time(s) in a dry run", got)
	}
}

// Doctor loads read-only: an uncloned source stays uncloned, and its check
// line warns that doctor did not clone it.
func TestDoctor_DoesNotCloneRecipeSource(t *testing.T) {
	out, stateDir, _ := runDoctor(t, sourceConfig(t), "json", nil)

	if _, err := os.Lstat(filepath.Join(stateDir, "sources")); !os.IsNotExist(err) {
		t.Errorf("doctor created the sources dir (stat err %v)", err)
	}
	step := jsonStep(t, out, "Recipe sources", "moon")
	if step.Status != "warn" || !strings.Contains(step.Message, "not cloned") {
		t.Errorf("moon step = %+v, want a warning that it is not cloned", step)
	}
}

// A command that only reads loads the config read-only: a recipe source that
// is not cloned stays uncloned, and stderr says so. A real migrate still
// clones it, since it repoints symlinks at what the source ships.
func TestCommands_ReadOnlyLoadDoesNotCloneSource(t *testing.T) {
	runCmd := func(c *cobra.Command, args ...string) func() error {
		return func() error { return c.RunE(c, args) }
	}
	tests := []struct {
		name      string
		dryRun    bool
		status    bool
		run       func() error
		wantClone bool
	}{
		{name: "list", run: runCmd(listCmd)},
		{name: "list recipes", run: runCmd(listRecipesCmd)},
		{name: "graph", run: runCmd(graphCmd)},
		{name: "outdated", run: runCmd(outdatedCmd)},
		{name: "add", run: runCmd(addCmd, "fresh")},
		{name: "migrate --status", status: true, run: runCmd(migrateCmd)},
		{name: "migrate --dry-run", dryRun: true, run: runCmd(migrateCmd)},
		{name: "migrate", run: runCmd(migrateCmd), wantClone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := orphanHome(t)
			useConfigFile(t, home, sourceConfig(t))
			setDryRun(t, tt.dryRun)
			origStatus := migrateStatus
			migrateStatus = tt.status
			t.Cleanup(func() { migrateStatus = origStatus })

			var runErr error
			stderr := captureFile(t, &os.Stderr, func() {
				captureStdout(t, func() { runErr = tt.run() })
			})
			if runErr != nil {
				t.Fatalf("run: %v\nstderr:\n%s", runErr, stderr)
			}

			_, statErr := os.Stat(
				filepath.Join(home, ".config", "ralph", "sources", "moon", ".git"),
			)
			if cloned := statErr == nil; cloned != tt.wantClone {
				t.Errorf("source cloned = %v, want %v", cloned, tt.wantClone)
			}
			warned := strings.Contains(stderr, "recipe source 'moon' is not cloned")
			if warned == tt.wantClone {
				t.Errorf("not-cloned warning = %v, want %v:\n%s", warned, !tt.wantClone, stderr)
			}
		})
	}
}
