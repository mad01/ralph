package apply

import (
	"time"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/cleanup"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
)

// recordAndCleanup records the recipe-state manifest on every apply, so the
// cleanup baseline never goes stale, and removes orphans when --enable-cleanup
// or auto_cleanup asks for it. The banner names the toggle that is active.
func recordAndCleanup(s *sequence) {
	autoCleanup := s.cfg.RecipesConfig.AutoCleanup
	if banner := cleanupBanner(s.opts.EnableCleanup, autoCleanup); banner != "" {
		_, _ = color.New(color.FgCyan).Fprintln(s.out, banner)
	}
	opts := cleanup.Options{
		Cleanup:    s.opts.EnableCleanup || autoCleanup,
		DryRun:     s.opts.DryRun,
		SyncFailed: s.opts.SyncFailed,
		Out:        s.out,
		Err:        s.errOut,
	}
	recordManifest(s.cfg, s.opts.Host, opts, s.rpt)
}

// cleanupBanner is the line that announces the cleanup phase and the toggle
// that turned it on, or "" when cleanup is off. The --enable-cleanup flag
// wins over auto_cleanup.
func cleanupBanner(flag, autoCleanup bool) string {
	switch {
	case flag:
		return "\nProcessing recipe cleanup (--enable-cleanup)..."
	case autoCleanup:
		return "\nProcessing recipe cleanup (auto_cleanup=true)..."
	}
	return ""
}

// recordManifest builds the intended manifest of cfg on host and hands it to
// cleanup.Record. A manifest that cannot be built is never diffed nor saved.
func recordManifest(cfg *config.Config, host string, opts cleanup.Options, rpt *report.Report) {
	next, unowned, err := IntendedManifest(cfg, host, time.Now())
	if err != nil {
		cleanup.SkipRecord(opts, rpt, "manifest",
			"skipping recipe-state update, could not build manifest", err)
		return
	}
	cleanup.Record(cfg, next, unowned, opts, rpt)
}
