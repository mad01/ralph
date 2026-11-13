package apply

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/progress"
	"github.com/mad01/ralph/internal/tool"
)

// applyTools checks the installation status of configured tools.
func applyTools(s *sequence) {
	toolPhase := s.rpt.AddPhase("Tools")
	if len(s.cfg.Tools) == 0 {
		return
	}

	prog := progress.NewOrQuiet("Tools", len(s.cfg.Tools), s.opts.Verbose || s.opts.DryRun)

	fmt.Fprintln(
		s.out,
		"\nChecking tool configurations (installation not performed by apply):",
	)
	for _, t := range s.cfg.Tools {
		prog.TickWith(t.Name)
		if reason := s.skipReason(t.Enable, t.Hosts, t.Profiles); reason != "" {
			fmt.Fprintf(s.out, "  Skipping tool: %s (%s)\n", t.Name, reason)
			toolPhase.AddSkip(t.Name, reason)
			continue
		}
		var statusColor func(format string, a ...any) string
		status := "Not installed"
		installed, checkErr := tool.CheckStatus(t.CheckCommand)
		if checkErr != nil {
			status = checkErr.Error()
			statusColor = color.YellowString
			toolPhase.AddWarn(t.Name, checkErr.Error())
		} else if installed {
			status = "Installed"
			statusColor = color.GreenString
			toolPhase.AddOK(t.Name, "installed")
		} else {
			statusColor = color.YellowString
			toolPhase.AddWarn(t.Name, "not installed")
		}
		fmt.Fprintf(
			s.out,
			"  - Tool '%s': %s. Install hint: %s\n",
			t.Name,
			statusColor(status),
			t.InstallHint,
		)
	}
	prog.Done()
}
