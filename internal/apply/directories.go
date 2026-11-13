package apply

import (
	"fmt"
	"sort"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/dotfile"
	"github.com/mad01/ralph/internal/progress"
	"github.com/mad01/ralph/internal/state"
)

// applyDirectories creates configured directories, honoring enable and host filters.
func applyDirectories(s *sequence) {
	dirPhase := s.rpt.AddPhase("Directories")
	if len(s.cfg.Directories) == 0 {
		return
	}

	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	dirNames := make([]string, 0, len(s.cfg.Directories))
	for name := range s.cfg.Directories {
		dirNames = append(dirNames, name)
	}
	sort.Strings(dirNames)
	prog := progress.New("Directories", len(dirNames))
	if s.opts.Verbose || s.opts.DryRun {
		prog = progress.NewQuiet()
	}
	for _, name := range dirNames {
		prog.TickWith(name)
		dir := s.cfg.Directories[name]
		if reason := s.skipReason(dir.Enable, dir.Hosts, dir.Profiles); reason != "" {
			fmt.Fprintf(s.out, "  %s %s\n", color.CyanString("skip"), dim(name+" ("+reason+")"))
			dirPhase.AddSkip(name, reason)
			continue
		}
		fmt.Fprintf(s.out, "  %s\n", bold(name))
		fmt.Fprintf(s.out, "    %s\n", dim(dir.Target))
		if err := dotfile.CreateDirectory(s.out, dir, s.opts.DryRun); err != nil {
			fmt.Fprintln(s.errOut, color.RedString("    error: %s: %v", name, err))
			dirPhase.AddFail(name, err.Error(), err)
		} else {
			dirPhase.AddOK(name, "")
		}
	}
	prog.Done()
}

// declareDirectories records the target of every applying directory, the
// path dotfile.CreateDirectory creates.
func declareDirectories(m *manifest) {
	for _, dir := range m.cfg.Directories {
		if !m.applies(dir.Enable, dir.Hosts, dir.Profiles) {
			continue
		}
		target, err := config.ExpandPath(dir.Target)
		if err != nil {
			continue
		}
		m.track(dir.OwnerRecipe, state.KindDirectory, target)
	}
}
