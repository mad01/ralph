package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/lockfile"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
	"github.com/spf13/cobra"
)

var (
	cleanRecipe string
)

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
		runLock, err := lockfile.Acquire()
		if err != nil {
			return err
		}
		defer func() { _ = runLock.Release() }()

		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
		currentHost := config.GetCurrentHost()

		next, err := buildIntendedManifest(cfg, currentHost, time.Now())
		if err != nil {
			return fmt.Errorf("building intended manifest: %w", err)
		}
		prev, err := state.Load()
		if err != nil {
			return fmt.Errorf("loading recipe state: %w", err)
		}
		carryForwardFrozenRecipes(prev, next, frozenRecipeSet(cfg))

		// Compute the cross-recipe protected set from the FULL intended manifest
		// BEFORE any --recipe filtering. Otherwise `clean --recipe X` would strip
		// every other recipe from next, leaving the protected set empty — and a
		// binary still declared by another recipe (e.g. after a rename) would be
		// deleted. Protection must always reflect the whole manifest.
		protected := next.AllPaths()

		// --recipe scopes the diff to a single recipe by stripping the
		// others from both sides of the comparison. Other recipes' state
		// is preserved on disk untouched.
		if cleanRecipe != "" {
			prev = filterRecipe(prev, cleanRecipe)
			next = filterRecipe(next, cleanRecipe)
			if _, ok := prev.Recipes[cleanRecipe]; !ok {
				fmt.Fprintf(os.Stderr, "Recipe '%s' is not present in the previous manifest; nothing to clean.\n", cleanRecipe)
				return nil
			}
		}

		rpt := &report.Report{Command: "clean"}
		phase := rpt.AddPhase("Cleanup")

		if dryRun {
			printDryRunBanner(uiOut())
		}

		failed := runCleanup(prev, next, protected, dryRun, uiOut(), phase)
		if !dryRun {
			for name, art := range failed {
				next.MergeRetry(name, art)
			}
		}

		// Persist updated state only when scoped to a single recipe is
		// NOT in play AND we actually mutated disk. When scoped, only
		// the targeted recipe's slice has been re-evaluated; merging it
		// back into the on-disk state would risk dropping unrelated
		// recipes' entries. v1: refuse to save under --recipe.
		if !dryRun && cleanRecipe == "" {
			if err := state.Save(next); err != nil {
				fmt.Fprintln(os.Stderr, color.YellowString("Warning: could not save recipe state: %v", err))
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
	out := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	if s == nil {
		return out
	}
	if art, ok := s.Recipes[name]; ok {
		out.Recipes[name] = art
	}
	return out
}

func init() {
	rootCmd.AddCommand(cleanCmd)
	cleanCmd.Flags().StringVar(&cleanRecipe, "recipe", "", "Clean only the named recipe (others untouched)")
}
