// Package apply runs ralph's apply sequence: the ordered phases that make the
// machine match the merged config (pre-apply hooks, directories, dirs_mirror
// entries, repos, dotfiles, shell config, tool checks, builds and packages,
// post-apply hooks), then records the recipe-state manifest and, when asked,
// removes orphaned recipe artifacts through package cleanup.
//
// Each phase pairs a run step, which applies its items, with a declare step,
// which records the tracked artifacts those items produce. Run walks the
// phase list calling run; IntendedManifest walks the same list calling
// declare. Per item type, run and declare share the helper that derives the
// artifact, so the manifest cleanup diffs against cannot drift from what the
// phases create.
//
// The package prints per-item progress to Options.Out and warnings to
// Options.Err, and fills the report it is given; the caller prints the report.
package apply

import (
	"fmt"
	"io"
	"os"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/dotfile"
	"github.com/mad01/ralph/internal/report"
)

// Options configures one apply run.
type Options struct {
	Host          string    // this machine's host name, for host filters
	DryRun        bool      // say what would change; change nothing
	Verbose       bool      // print every item instead of progress counters
	Overwrite     bool      // --overwrite: replace existing files at link targets
	Skip          bool      // --skip: leave existing files at link targets alone
	Force         bool      // --force: re-run 'once' builds and rebuild packages
	Build         string    // --build: run only this build
	EnableCleanup bool      // --enable-cleanup: remove orphaned recipe artifacts
	SyncFailed    bool      // a sync step failed earlier in this run
	Out           io.Writer // per-item progress lines; nil discards them
	Err           io.Writer // warnings and errors; nil means os.Stderr
}

// Result is what a run leaves for the caller to print after the report.
type Result struct {
	// Caveats are the notes of recipes whose packages changed, in build
	// order. A recipe appears once per package it owns that changed.
	Caveats []Caveat
}

// Run applies cfg phase by phase, recording each item in rpt, then records
// the recipe-state manifest and removes orphans when opts or the config's
// auto_cleanup ask for it. It returns an error only when a pre-apply hook
// failed, which stops the run before any other phase; every other failure is
// a report step, and rpt.ExitCode says how the command should exit.
func Run(cfg *config.Config, rpt *report.Report, opts Options) (Result, error) {
	s := newSequence(cfg, rpt, opts)
	fmt.Fprintln(s.out, linkNote(s.link))
	for _, p := range phases {
		if err := p.run(s); err != nil {
			return s.result(), err
		}
	}
	recordAndCleanup(s)
	return s.result(), nil
}

// sequence is the state one run threads through its phases.
type sequence struct {
	cfg     *config.Config
	opts    Options
	rpt     *report.Report
	out     io.Writer             // opts.Out, or io.Discard
	errOut  io.Writer             // opts.Err, or os.Stderr
	link    dotfile.SymlinkAction // what linking does with an existing target
	caveats []Caveat
}

// newSequence returns the state for one run of cfg with opts, filling rpt.
func newSequence(cfg *config.Config, rpt *report.Report, opts Options) *sequence {
	s := &sequence{
		cfg:    cfg,
		opts:   opts,
		rpt:    rpt,
		out:    opts.Out,
		errOut: opts.Err,
		link:   opts.symlinkAction(),
	}
	if s.out == nil {
		s.out = io.Discard
	}
	if s.errOut == nil {
		s.errOut = os.Stderr
	}
	return s
}

// skipReason reports why an item with these filter fields does not apply on
// this run's host and profiles, or "" when it does (see config.SkipReason).
func (s *sequence) skipReason(enable *bool, hosts, profiles []string) string {
	return config.SkipReason(enable, hosts, profiles, s.opts.Host, s.cfg.Profiles)
}

// result returns what the run leaves for its caller.
func (s *sequence) result() Result {
	return Result{Caveats: s.caveats}
}

// phase is one step of the apply sequence. run applies it and returns an
// error only to stop the sequence. declare records the tracked artifacts the
// phase produces into the intended manifest; it is nil for a phase that
// produces none.
type phase struct {
	run     func(*sequence) error
	declare func(*manifest) error
}

// phases is the apply sequence in order. Run calls every run step and
// IntendedManifest every declare step, so a phase added here is both applied
// and declared, with no second list to keep in step.
var phases = []phase{
	{run: runPreApplyHooks},
	{run: infallible(applyDirectories), declare: infallible(declareDirectories)},
	{run: infallible(applyDirsMirror), declare: declareDirsMirror},
	{run: infallible(applyRepos), declare: infallible(declareRepos)},
	{run: infallible(applyDotfiles), declare: infallible(declareDotfiles)},
	{run: infallible(applyShellConfig), declare: infallible(declareShell)},
	{run: infallible(applyTools), declare: infallible(declareToolConfigFiles)},
	{run: infallible(applyBuildsAndPackages), declare: infallible(declareBuildsAndPackages)},
	{run: infallible(runPostApplyHooks)},
}

// infallible adapts a step that never stops the sequence to a phase step.
func infallible[T any](step func(T)) func(T) error {
	return func(v T) error {
		step(v)
		return nil
	}
}

// symlinkAction picks what linking a dotfile does with an existing target:
// --overwrite wins over --skip, and backup is the default.
func (o Options) symlinkAction() dotfile.SymlinkAction {
	switch {
	case o.Overwrite:
		return dotfile.SymlinkActionOverwrite
	case o.Skip:
		return dotfile.SymlinkActionSkip
	}
	return dotfile.SymlinkActionBackup
}

// linkNote is the line a run prints first, naming its symlink action.
func linkNote(action dotfile.SymlinkAction) string {
	switch action {
	case dotfile.SymlinkActionOverwrite:
		return "Symlink action: Overwrite existing files."
	case dotfile.SymlinkActionSkip:
		return "Symlink action: Skip existing files."
	}
	return "Symlink action: Backup existing files."
}
