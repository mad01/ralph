// Package cleanup records the recipe-state manifest and removes orphans: the
// artifacts the previous run recorded that the intended manifest no longer
// declares. The caller builds the intended manifest (see apply.IntendedManifest)
// and passes it in; this package diffs it against the recorded one, honours
// each recipe's delete_behavior, freezes recipes the config may only be
// missing for now, and saves the result for the next run.
//
// Every removal goes through state.SafeRemove. A path the active config still
// declares, whether a recipe owns it or it sits straight in config.toml, is
// never removed (see ProtectedPaths).
package cleanup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

// Options controls one Record call.
type Options struct {
	Cleanup    bool      // remove orphans: --enable-cleanup or auto_cleanup
	DryRun     bool      // say what would be removed; remove and save nothing
	SyncFailed bool      // a sync step failed earlier in this run
	Out        io.Writer // per-artifact log lines; nil discards them
	Err        io.Writer // warnings; nil means os.Stderr
}

// out returns the writer for per-artifact log lines.
func (o Options) out() io.Writer {
	if o.Out == nil {
		return io.Discard
	}
	return o.Out
}

// errOut returns the writer for warnings.
func (o Options) errOut() io.Writer {
	if o.Err == nil {
		return os.Stderr
	}
	return o.Err
}

// Record saves next as the recipe-state manifest after every non-dry-run
// apply, so the cleanup baseline never goes stale even on runs without
// cleanup enabled. next is the intended manifest and unowned the paths that
// items with no owner recipe declare. Removal of orphans is gated by
// opts.Cleanup; recording ownership is not. A run that does not remove its
// orphans keeps them in the saved state (see settleOrphans), so a recipe
// disabled or dropped between two cleanup runs is still cleaned by the next.
// A recorded state that does not parse is moved aside and replaced by a fresh
// one (see loadPreviousState) instead of blocking every later run; the report
// gets a warn step for it with cleanup on or off (see reportStateRepair).
//
// Recipes cfg freezes (host or profile filtered, or from a recipes directory
// discovery found missing) are carried forward untouched. The caller prints
// the cleanup banner before calling when opts.Cleanup is true.
func Record(
	cfg *config.Config,
	next *state.RecipeState,
	unowned map[string]bool,
	opts Options,
	rpt *report.Report,
) {
	prev, loadNote, loadErr := loadPreviousState(opts.DryRun, opts.errOut())
	if loadErr != nil {
		SkipRecord(opts, rpt, "load", "could not load recipe state", loadErr)
		return
	}
	KeepFrozenRecipes(cfg, prev, next, opts.errOut())

	cleanupPhase := settleOrphans(cfg, prev, next, ProtectedPaths(next, unowned), opts, rpt)
	reportStateRepair(rpt, cleanupPhase, loadNote)

	if !opts.DryRun {
		if err := state.Save(next); err != nil {
			fmt.Fprintln(
				opts.errOut(),
				color.YellowString("Warning: could not save recipe state: %v", err),
			)
			if cleanupPhase != nil {
				cleanupPhase.AddWarn("save", err.Error())
			}
		}
	}
}

// SkipRecord warns that this run records nothing and why, and under cleanup
// adds the reason as a warn step named step. A caller that cannot build the
// intended manifest calls it instead of Record: an incomplete manifest must
// never be diffed (live artifacts would look like orphans) nor saved (it
// would clobber a good baseline).
func SkipRecord(opts Options, rpt *report.Report, step, what string, err error) {
	fmt.Fprintln(opts.errOut(), color.YellowString("Warning: %s: %v", what, err))
	if opts.Cleanup {
		rpt.AddPhase("Cleanup").AddWarn(step, err.Error())
	}
}

// ProtectedPaths returns every filesystem path the active config still
// declares: the intended manifest's recipe-owned paths plus the paths of items
// with no owner recipe. Cleanup never removes a protected path, whichever
// recipe the previous state attributed it to. Take it before any orphan is
// carried into next: a carried path is not "still declared".
func ProtectedPaths(next *state.RecipeState, unowned map[string]bool) map[string]bool {
	protected := next.AllPaths()
	for p := range unowned {
		protected[p] = true
	}
	return protected
}

// loadPreviousState loads the manifest the last run recorded. A state file
// that does not parse is moved aside (a dry run only says it would be) and the
// run goes on from an empty state: nothing is an orphan, so the run removes
// nothing and records a fresh manifest. The returned note says what happened,
// for the cleanup phase; it is printed on errW too. A read error, or a move
// that fails, is returned instead, so the caller keeps the file as it is.
func loadPreviousState(dryRun bool, errW io.Writer) (*state.RecipeState, string, error) {
	prev, err := state.Load()
	if !errors.Is(err, state.ErrCorrupt) {
		return prev, "", err
	}
	note, err := setAsideCorruptState(err, dryRun)
	if err != nil {
		return nil, "", err
	}
	fmt.Fprintln(errW, color.YellowString("Warning: %s", note))
	return state.New(), note, nil
}

// reportStateRepair adds the note loadPreviousState returned as a warn step
// named "load" in the Cleanup phase, or does nothing when the note is empty.
// phase is the run's Cleanup phase, nil when cleanup is off; the step then
// opens one, so the repair reaches the report (and -o json) either way.
func reportStateRepair(rpt *report.Report, phase *report.Phase, note string) {
	if note == "" {
		return
	}
	if phase == nil {
		phase = rpt.AddPhase("Cleanup")
	}
	phase.AddWarn("load", note)
}

// setAsideCorruptState moves the corrupt state file aside, or under dry run
// only describes the move, and returns a note naming the backup.
func setAsideCorruptState(loadErr error, dryRun bool) (string, error) {
	const forgets = "cleanup forgets the artifacts recorded before"
	path, err := state.GetStatePath()
	if err != nil {
		return "", fmt.Errorf("%w; %w", loadErr, err)
	}
	if dryRun {
		return fmt.Sprintf("%s: %v; a real run moves it to %s and records a fresh one, so %s",
			path, loadErr, state.CorruptBackupPattern(path), forgets), nil
	}
	backup, err := state.MoveCorruptAside(time.Now())
	if err != nil {
		return "", fmt.Errorf("%w; moving it aside failed: %w", loadErr, err)
	}
	return fmt.Sprintf("%s: %v; moved it to %s and recording a fresh one, so %s",
		path, loadErr, backup, forgets), nil
}

// settleOrphans decides what happens to the orphans state.Diff(prev, next)
// reports. A cleanup run removes them (honoring delete_behavior) and re-tracks
// failed removals so they retry next run. A run without cleanup, or one whose
// sync failed (the config on disk may be stale or partial), removes nothing
// and carries the orphans into next, uninstall hooks included, so the next
// cleanup still finds them. protected (see ProtectedPaths) must be taken
// before this call, since it adds orphans to next. It returns the cleanup
// phase, or nil when cleanup is off.
func settleOrphans(
	cfg *config.Config,
	prev, next *state.RecipeState,
	protected map[string]bool,
	opts Options,
	rpt *report.Report,
) *report.Phase {
	if !opts.Cleanup {
		next.CarryOrphans(state.Diff(prev, next))
		return nil
	}

	phase := rpt.AddPhase("Cleanup")
	switch {
	case opts.SyncFailed:
		phase.AddWarn(
			"sync",
			"orphan removal skipped: a sync step failed earlier in this run; orphans stay recorded for the next cleanup (`ralph clean` removes them without syncing)",
		)
		next.CarryOrphans(state.Diff(prev, next))
	case len(prev.Recipes) == 0 && cfg.RecipesConfig.AutoCleanup:
		fmt.Fprintln(
			opts.out(),
			color.CyanString(
				"First run with auto_cleanup: seeding state baseline (no artifacts will be removed).",
			),
		)
		phase.AddOK("baseline", "initial state recorded")
	default:
		failed := Run(prev, next, RunOptions{
			Protected: protected,
			DryRun:    opts.DryRun,
			Out:       opts.out(),
			Phase:     phase,
		})
		if !opts.DryRun {
			for name, art := range failed {
				next.MergeRetry(name, art)
			}
		}
	}
	return phase
}
