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

		cfg, err := loadRunConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
		currentHost := config.GetCurrentHost()

		next, unowned, err := apply.IntendedManifest(cfg, currentHost, time.Now())
		if err != nil {
			return fmt.Errorf("building intended manifest: %w", err)
		}
		prev, err := state.Load()
		if errors.Is(err, state.ErrCorrupt) {
			return fmt.Errorf(
				"loading recipe state: %w (run 'ralph up' to move it aside and record a fresh one)",
				err,
			)
		}
		if err != nil {
			return fmt.Errorf("loading recipe state: %w", err)
		}
		// Same freeze as `ralph up`: filtered recipes and the recipes of a
		// missing recipes directory are carried forward, never cleaned.
		cleanup.KeepFrozenRecipes(cfg, prev, next, os.Stderr)

		// Compute the cross-recipe protected set from the FULL intended manifest
		// BEFORE any --recipe filtering. Otherwise `clean --recipe X` would strip
		// every other recipe from next, leaving the protected set empty — and a
		// binary still declared by another recipe (e.g. after a rename) would be
		// deleted. Protection must always reflect the whole manifest.
		protected := cleanup.ProtectedPaths(next, unowned)

		// --recipe scopes the diff to a single recipe by stripping the
		// others from both sides of the comparison. Other recipes' state
		// is preserved on disk untouched.
		rpt := &report.Report{Command: "clean"}
		phase := rpt.AddPhase("Cleanup")

		onDisk := prev
		if cleanRecipe != "" {
			prev = filterRecipe(prev, cleanRecipe)
			next = filterRecipe(next, cleanRecipe)
			if _, ok := prev.Recipes[cleanRecipe]; !ok {
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
		}

		if dryRun {
			printDryRunBanner(uiOut())
		}

		failed := cleanup.Run(prev, next, protected, dryRun, uiOut(), phase)
		if !dryRun {
			for name, art := range failed {
				next.MergeRetry(name, art)
			}
		}

		// Under --recipe only that recipe was re-evaluated, so save the
		// on-disk state with just its entry replaced. Without the save its
		// uninstall hooks would run again on the next cleanup.
		toSave := next
		if cleanRecipe != "" {
			toSave = scopedState(onDisk, next, cleanRecipe)
		}
		if !dryRun && toSave != nil {
			if err := state.Save(toSave); err != nil {
				fmt.Fprintln(
					os.Stderr,
					color.YellowString("Warning: could not save recipe state: %v", err),
				)
			}
		}

		finishReport(rpt, nil, dryRun, verbose)
		if code := rpt.ExitCode(); code != 0 {
			return &ExitError{Code: code}
		}
		return nil
	},
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
