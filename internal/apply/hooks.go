package apply

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/hooks"
)

// runPreApplyHooks runs the pre-apply hooks. A failure stops the run.
func runPreApplyHooks(s *sequence) error {
	if len(s.cfg.Hooks.PreApply) == 0 {
		return nil
	}
	phase := s.rpt.AddPhase("Pre-apply hooks")
	hookCtx := &hooks.HookContext{DryRun: s.opts.DryRun}
	err := hooks.RunHooks(s.out, s.cfg.Hooks.PreApply, hooks.PreApply, hookCtx, s.opts.DryRun)
	if err != nil {
		fmt.Fprintln(s.errOut, color.RedString("Error executing pre-apply hooks: %v", err))
		phase.AddFail("pre-apply", err.Error(), err)
		return fmt.Errorf("pre-apply hooks failed: %w", err)
	}
	phase.AddOK("pre-apply", "completed")
	return nil
}

// runPostApplyHooks runs the post-apply hooks. A failure is a warning.
func runPostApplyHooks(s *sequence) {
	if len(s.cfg.Hooks.PostApply) == 0 {
		return
	}
	phase := s.rpt.AddPhase("Post-apply hooks")
	hookCtx := &hooks.HookContext{DryRun: s.opts.DryRun}
	err := hooks.RunHooks(s.out, s.cfg.Hooks.PostApply, hooks.PostApply, hookCtx, s.opts.DryRun)
	if err != nil {
		fmt.Fprintln(s.errOut, color.YellowString("Warning: post-apply hooks failed: %v", err))
		phase.AddWarn("post-apply", err.Error())
		return
	}
	phase.AddOK("post-apply", "completed")
}
