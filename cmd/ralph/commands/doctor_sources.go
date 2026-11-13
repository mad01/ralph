package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
)

// localRecipesName names the dotfiles repo's own recipes directory in the
// "Recipe sources" phase; state records its recipes with an empty source.
const localRecipesName = "dotfiles repo"

// checkRecipeSources reports each [[recipe_sources]] checkout and every
// recipes directory discovery found missing. Config loading has already
// cloned a missing source checkout (or failed to load), so a checkout that
// is absent here cannot happen; what can is a cache directory that is not a
// git checkout, which loading accepts when the source pins no ref.
func checkRecipeSources(rpt *report.Report, cfg *config.Config) {
	if len(cfg.RecipeSources) == 0 && len(cfg.MissingRecipeDirs) == 0 {
		return
	}
	phase := rpt.AddPhase("Recipe sources")

	if len(cfg.RecipeSources) > 0 {
		sourcesDir, err := config.SourcesDir()
		if err != nil {
			phase.AddResult("sources dir", "", report.StatusFail, err.Error(), err)
		} else {
			for _, src := range cfg.RecipeSources {
				status, msg := sourceCheckoutStatus(src, sourcesDir, cfg.Profiles)
				phase.AddResult(src.Name, "", status, msg, nil)
			}
		}
	}

	for _, missing := range cfg.MissingRecipeDirs {
		name := missing.Source
		if name == "" {
			name = localRecipesName
		}
		phase.AddResult(name, "", report.StatusWarn, fmt.Sprintf(
			"recipes dir missing: %s; no recipes load from it and cleanup keeps "+
				"the artifacts its recipes recorded",
			missing.Path,
		), nil)
	}
}

// sourceCheckoutStatus checks one recipe source's cached checkout, honoring
// the enable flag and the profile gate that config loading applies.
func sourceCheckoutStatus(
	src config.RecipeSource,
	sourcesDir string,
	profiles []string,
) (report.Status, string) {
	if !config.IsEnabled(src.Enable) {
		return report.StatusSkip, "disabled"
	}
	if !config.ShouldApplyForProfiles(src.Profiles, profiles) {
		return report.StatusSkip, "other profile"
	}
	checkout := config.SourceCheckoutPath(sourcesDir, src)
	if _, err := os.Stat(filepath.Join(checkout, ".git")); err != nil {
		return report.StatusWarn, fmt.Sprintf(
			"%s is not a git checkout, so 'ralph up' cannot update it; move it "+
				"aside and run 'ralph up' to clone it again",
			checkout,
		)
	}
	return report.StatusOK, checkout
}
