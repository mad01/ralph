package apply

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/progress"
	"github.com/mad01/ralph/internal/repo"
	"github.com/mad01/ralph/internal/state"
)

// applyRepos clones or updates configured git repositories.
func applyRepos(s *sequence) {
	if len(s.cfg.Repos) == 0 {
		return
	}
	repoPhase := s.rpt.AddPhase("Repositories")
	defer func() {
		if !s.opts.Verbose && !s.opts.DryRun {
			_, _, fail, _ := repoPhase.Counts()
			progress.StatusLine("Repositories", fail == 0)
		}
	}()
	err := repo.ProcessRepos(s.out, s.cfg.Repos, s.opts.Host, s.cfg.Profiles, s.opts.DryRun)
	if err != nil {
		fmt.Fprintln(s.errOut, color.RedString("Error processing repositories: %v", err))
		repoPhase.AddFail("repos", err.Error(), err)
	} else {
		repoPhase.AddOK("repos", "processed")
	}
}

// declareRepos records the clone target of every applying repo a recipe owns,
// the path repo.ProcessRepos clones into. Repos are abandoned, never removed,
// so an ownerless repo is not protected either.
func declareRepos(m *manifest) {
	for _, r := range m.cfg.Repos {
		if r.OwnerRecipe == "" || !m.applies(r.Enable, r.Hosts, r.Profiles) {
			continue
		}
		target, err := config.ExpandPath(r.Target)
		if err != nil {
			continue
		}
		m.own(r.OwnerRecipe, state.KindRepo, target)
	}
}
