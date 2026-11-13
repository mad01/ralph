package cleanup

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/state"
)

// KeepFrozenRecipes carries every recipe cfg freezes from prev into next (see
// carryForwardFrozenRecipes) and warns on errW once per missing recipes
// directory whose recorded recipes it kept, so a moved or unmounted dotfiles
// repo is visible instead of silently loading as zero recipes.
func KeepFrozenRecipes(cfg *config.Config, prev, next *state.RecipeState, errW io.Writer) {
	tracked := make(map[string]bool, len(next.Recipes))
	for name := range next.Recipes {
		tracked[name] = true
	}
	carryForwardFrozenRecipes(prev, next, frozenRecipeSet(cfg), frozenRecipeSourceSet(cfg))

	for _, dir := range cfg.MissingRecipeDirs {
		kept := 0
		for name, art := range next.Recipes {
			if !tracked[name] && art.RecipeSource == dir.Source {
				kept++
			}
		}
		if kept == 0 {
			continue
		}
		fmt.Fprintln(errW, color.YellowString(
			"Warning: recipes directory %s is missing; kept %d recipe(s) recorded from it, nothing removed. "+
				"If you removed it on purpose, an empty directory there (or auto_discover = false) lets cleanup proceed.",
			dir.Path,
			kept,
		))
	}
}

// frozenRecipeSet returns exact recipe names that are filtered on this host.
// Their artifacts must not be treated as orphans during cleanup.
func frozenRecipeSet(cfg *config.Config) map[string]bool {
	if len(cfg.HostFilteredRecipes) == 0 {
		return nil
	}
	frozen := make(map[string]bool, len(cfg.HostFilteredRecipes))
	for _, name := range cfg.HostFilteredRecipes {
		frozen[name] = true
	}
	return frozen
}

// frozenRecipeSourceSet returns the recipe sources whose previously recorded
// recipes freeze this run: remote sources filtered by this machine's
// profiles, and the origin of every recipes directory discovery found
// missing. The empty name is the local dotfiles repo, matching the empty
// source local recipes record. Persisted recipe provenance determines which
// prior recipes freeze.
func frozenRecipeSourceSet(cfg *config.Config) map[string]bool {
	if len(cfg.ProfileFilteredRecipeSources) == 0 && len(cfg.MissingRecipeDirs) == 0 {
		return nil
	}
	frozen := make(
		map[string]bool,
		len(cfg.ProfileFilteredRecipeSources)+len(cfg.MissingRecipeDirs),
	)
	for _, name := range cfg.ProfileFilteredRecipeSources {
		frozen[name] = true
	}
	for _, dir := range cfg.MissingRecipeDirs {
		frozen[dir.Source] = true
	}
	return frozen
}

// carryForwardFrozenRecipes copies each frozen recipe's previously-recorded
// artifacts from prev into next when next doesn't already track them. Recipes
// freeze by exact name or by their persisted source provenance (the empty
// source is the local dotfiles repo). This keeps filtered recipes out of the
// orphan diff and preserves their entries in the saved state for future runs.
func carryForwardFrozenRecipes(
	prev, next *state.RecipeState,
	frozenRecipes, frozenSources map[string]bool,
) {
	if prev == nil || next == nil || (len(frozenRecipes) == 0 && len(frozenSources) == 0) {
		return
	}
	if next.Recipes == nil {
		next.Recipes = map[string]state.RecipeArtifacts{}
	}
	for name, art := range prev.Recipes {
		if _, alreadyTracked := next.Recipes[name]; alreadyTracked {
			continue
		}
		frozen := frozenRecipes[name] || frozenSources[art.RecipeSource]
		if !frozen && prev.Version < state.CurrentRecipeStateVersion &&
			art.RecipeSource == "" {
			// Version 0 predates explicit source provenance. Remote recipe
			// identities were already namespaced as <source>/<recipe>, so infer
			// provenance once and persist it in the upgraded state. Restrict the
			// fallback to legacy state so a current local recipe whose name
			// contains a slash never freezes by prefix.
			for source := range frozenSources {
				if strings.HasPrefix(name, source+"/") {
					art.RecipeSource = source
					frozen = true
					break
				}
			}
		}
		if !frozen {
			continue
		}
		// Frozen means cleanup will not touch it, whatever an earlier run
		// marked.
		art.PendingRemoval = false
		next.Recipes[name] = art
	}
}
