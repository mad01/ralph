package commands

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
)

// checkGit reports whether git is on PATH, but only when the config has
// something 'ralph up' runs git for. Without git those steps fail one by one,
// each with its own error; this line names the one cause.
func checkGit(rpt *report.Report, cfg *config.Config) {
	users := gitUsers(cfg)
	if len(users) == 0 {
		return
	}
	phase := rpt.AddPhase("Prerequisites")
	path, err := exec.LookPath("git")
	if err != nil {
		phase.AddResult("git", "", report.StatusFail, fmt.Sprintf(
			"not found on PATH; needed for %s",
			strings.Join(users, ", "),
		), err)
		return
	}
	phase.AddResult("git", "", report.StatusOK, path, nil)
}

// gitUsers lists the parts of cfg that need git on this machine: the dotfiles
// repo 'ralph up' pulls, and the active repos, remote and make packages, and
// recipe sources.
func gitUsers(cfg *config.Config) []string {
	var users []string
	if cfg.DotfilesRepoPath != "" {
		users = append(users, "the dotfiles repo")
	}
	host := config.GetCurrentHost()
	for _, rp := range cfg.Repos {
		if itemActive(rp.Enable, rp.Hosts, rp.Profiles, host, cfg.Profiles) {
			users = append(users, "repos")
			break
		}
	}
	for _, pkg := range cfg.Packages {
		remote := pkg.Source == "remote" || pkg.Source == "make"
		if remote && itemActive(pkg.Enable, pkg.Hosts, pkg.Profiles, host, cfg.Profiles) {
			users = append(users, "remote packages")
			break
		}
	}
	for _, src := range cfg.RecipeSources {
		if itemActive(src.Enable, nil, src.Profiles, host, cfg.Profiles) {
			users = append(users, "recipe sources")
			break
		}
	}
	return users
}

// itemActive reports whether an item applies on this machine: enabled, and
// not filtered out by its hosts or profiles.
func itemActive(
	enable *bool,
	hosts, profiles []string,
	host string,
	machineProfiles []string,
) bool {
	return config.IsEnabled(enable) &&
		config.ShouldApplyForHost(hosts, host) &&
		config.ShouldApplyForProfiles(profiles, machineProfiles)
}
