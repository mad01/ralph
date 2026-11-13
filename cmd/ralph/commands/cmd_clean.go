package commands

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/apply"
	"github.com/mad01/ralph/internal/cleanup"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
	"github.com/spf13/cobra"
)

var cleanRecipe string

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove artifacts owned by recipes that are gone or disabled",
	Long: `Compare the recorded recipe-state manifest against the current config and
remove orphans (artifacts present in the manifest but absent from the
intended config). Honors per-recipe delete_behavior: "delete" (default)
removes; "abandon" leaves artifacts in place with a log line.

Use --recipe to scope cleanup to a single recipe — the recipe still must
have entries in the previous manifest. Use --dry-run to preview without
touching disk. SafeRemove rails apply: no globs, only HOME-prefixed paths,
kind-specific verification, repos always abandoned.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		runLock, err := acquireRunLock(dryRun)
		if err != nil {
			return err
		}
		defer func() { _ = runLock.Release() }()

		diff, err := loadCleanDiff()
		if err != nil {
			return err
		}

		rpt := &report.Report{Command: "clean"}
		phase := rpt.AddPhase("Cleanup")

		if cleanRecipe != "" && !diff.scopeTo(cleanRecipe) {
			fmt.Fprintf(
				os.Stderr,
				"Recipe '%s' is not present in the previous manifest; nothing to clean.\n",
				cleanRecipe,
			)
			// A JSON reader still gets a report: an empty Cleanup phase.
			if outputJSON() {
				finishReport(rpt, nil, dryRun, verbose)
			}
			return nil
		}

		if dryRun {
			printDryRunBanner(uiOut())
		}
		diff.removeOrphans(phase)

		finishReport(rpt, nil, dryRun, verbose)
		if code := rpt.ExitCode(); code != 0 {
			return &ExitError{Code: code}
		}
		return nil
	},
}

// cleanDiff is what one `ralph clean` compares: the recorded manifest (prev)
// against the intended one (next). onDisk is the state file as loaded, which a
// --recipe run saves back with only that recipe's entry replaced.
type cleanDiff struct {
	prev, next, onDisk *state.RecipeState
	// protected is every path the active config still declares, taken from
	// the full intended manifest before any --recipe scoping.
	protected map[string]bool
	recipe    string // the --recipe the diff is scoped to; "" for all
}

// loadCleanDiff builds the intended manifest from the config and loads the
// recorded one, with the same freeze as `ralph up`.
func loadCleanDiff() (*cleanDiff, error) {
	cfg, err := loadRunConfig()
	if err != nil {
		return nil, fmt.Errorf("loading configuration: %w", err)
	}
	next, unowned, err := apply.IntendedManifest(cfg, config.GetCurrentHost(), time.Now())
	if err != nil {
		return nil, fmt.Errorf("building intended manifest: %w", err)
	}
	prev, err := loadRecordedState()
	if err != nil {
		return nil, err
	}
	// Same freeze as `ralph up`: filtered recipes and the recipes of a
	// missing recipes directory are carried forward, never cleaned.
	cleanup.KeepFrozenRecipes(cfg, prev, next, os.Stderr)

	// Compute the cross-recipe protected set from the FULL intended manifest
	// BEFORE any --recipe filtering. Otherwise `clean --recipe X` would strip
	// every other recipe from next, leaving the protected set empty, and a
	// binary still declared by another recipe (e.g. after a rename) would be
	// deleted. Protection must always reflect the whole manifest.
	protected := cleanup.ProtectedPaths(next, unowned)
	return &cleanDiff{prev: prev, next: next, onDisk: prev, protected: protected}, nil
}

// loadRecordedState loads the recipe-state manifest the last run recorded.
// Unlike `ralph up`, clean refuses a state file that does not parse.
func loadRecordedState() (*state.RecipeState, error) {
	prev, err := state.Load()
	if errors.Is(err, state.ErrCorrupt) {
		return nil, fmt.Errorf(
			"loading recipe state: %w (run 'ralph up' to move it aside and record a fresh one)",
			err,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("loading recipe state: %w", err)
	}
	return prev, nil
}

// scopeTo narrows the diff to one recipe by stripping the others from both
// sides of the comparison. Other recipes' state is preserved on disk
// untouched. It reports false when the previous manifest has no entry for
// the recipe, so there is nothing to clean.
func (d *cleanDiff) scopeTo(name string) bool {
	d.recipe = name
	d.prev = filterRecipe(d.prev, name)
	d.next = filterRecipe(d.next, name)
	_, ok := d.prev.Recipes[name]
	return ok
}

// removeOrphans removes the orphans of the diff and, outside a dry run, saves
// the resulting state with failed removals re-tracked for the next run.
func (d *cleanDiff) removeOrphans(phase *report.Phase) {
	failed := cleanup.Run(d.prev, d.next, cleanup.RunOptions{
		Protected: d.protected,
		DryRun:    dryRun,
		Out:       uiOut(),
		Phase:     phase,
	})
	if dryRun {
		return
	}
	for name, art := range failed {
		d.next.MergeRetry(name, art)
	}

	// Under --recipe only that recipe was re-evaluated, so save the
	// on-disk state with just its entry replaced. Without the save its
	// uninstall hooks would run again on the next cleanup.
	toSave := d.next
	if d.recipe != "" {
		toSave = scopedState(d.onDisk, d.next, d.recipe)
	}
	if toSave == nil {
		return
	}
	if err := state.Save(toSave); err != nil {
		fmt.Fprintln(
			os.Stderr,
			color.YellowString("Warning: could not save recipe state: %v", err),
		)
	}
}

// filterRecipe returns a copy of `s` containing only the named recipe.
// Used by `ralph clean --recipe <name>` to scope the diff.
func filterRecipe(s *state.RecipeState, name string) *state.RecipeState {
	out := &state.RecipeState{
		Version: state.CurrentRecipeStateVersion,
		Recipes: map[string]state.RecipeArtifacts{},
	}
	if s == nil {
		return out
	}
	out.Version = s.Version
	if art, ok := s.Recipes[name]; ok {
		out.Recipes[name] = art
	}
	return out
}

// scopedState returns the on-disk state with only the named recipe's entry
// replaced by its outcome in cleaned: what it still declares plus what failed
// removal, or no entry when it is gone. Every other recipe stays as recorded.
// It returns nil for a state older than the current version: Save would stamp
// the current version on entries the one-time provenance migration has not
// seen (see cleanup.KeepFrozenRecipes), so that state is left to a full run.
func scopedState(onDisk, cleaned *state.RecipeState, name string) *state.RecipeState {
	if onDisk.Version < state.CurrentRecipeStateVersion {
		return nil
	}
	if art, ok := cleaned.Recipes[name]; ok {
		onDisk.Recipes[name] = art
	} else {
		delete(onDisk.Recipes, name)
	}
	return onDisk
}

func init() {
	rootCmd.AddCommand(cleanCmd)
	supportJSONOutput(cleanCmd)
	cleanCmd.Flags().
		StringVar(&cleanRecipe, "recipe", "", "Clean only the named recipe (others untouched)")
}
