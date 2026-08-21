package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/gitutil"
	"github.com/mad01/ralph/internal/report"
)

// syncRecipeSources pulls the latest state of update=true recipe sources.
// Sources pinned to a tag or commit (detached HEAD) are skipped: their state
// is fully described by the ref, and EnsureSourceCheckout moves them when the
// pin changes. Returns true when every attempted pull succeeded.
func syncRecipeSources(
	w io.Writer,
	cfg *config.Config,
	phase *report.Phase,
	isDryRun bool,
) bool {
	sourcesDir, err := config.SourcesDir()
	if err != nil {
		phase.AddFail("sources", "failed to expand sources dir", err)
		return false
	}

	ok := true
	for _, src := range cfg.RecipeSources {
		if !config.IsEnabled(src.Enable) {
			phase.AddSkip(src.Name, "disabled")
			continue
		}
		if !config.ShouldApplyForProfiles(src.Profiles, cfg.Profiles) {
			phase.AddSkip(src.Name, "profile mismatch")
			continue
		}
		if !src.Update {
			phase.AddSkip(src.Name, "update disabled")
			continue
		}
		dir := config.SourceCheckoutPath(sourcesDir, src)
		if gitutil.CurrentBranch(dir) == "" {
			phase.AddSkip(src.Name, fmt.Sprintf("pinned to %s", src.Ref))
			continue
		}
		if isDryRun {
			fmt.Fprintf(w, "  [DRY RUN] Would pull recipe source '%s' in '%s'\n", src.Name, dir)
			phase.AddSkip(src.Name, "dry run")
			continue
		}
		fmt.Fprintf(w, "  Pulling recipe source '%s'...\n", src.Name)
		if err := gitutil.Pull(w, dir); err != nil {
			phase.AddFail(src.Name, "pull failed", err)
			ok = false
			continue
		}
		phase.AddOK(src.Name, "pulled")
	}
	return ok
}

// syncFingerprint captures the dotfiles repo HEAD plus every enabled, active
// recipe source checkout HEAD. A change between two fingerprints means the
// recipes or config on disk moved under the running process, so the merged
// config must be reloaded before applying.
func syncFingerprint(cfg *config.Config) string {
	parts := []string{dotfilesRepoHead(cfg)}
	if sourcesDir, err := config.SourcesDir(); err == nil {
		for _, src := range cfg.RecipeSources {
			if !config.IsEnabled(src.Enable) {
				continue
			}
			if !config.ShouldApplyForProfiles(src.Profiles, cfg.Profiles) {
				continue
			}
			parts = append(parts, gitutil.GetGitHash(config.SourceCheckoutPath(sourcesDir, src)))
		}
	}
	return strings.Join(parts, ",")
}
