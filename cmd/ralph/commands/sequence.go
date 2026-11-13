package commands

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/dotfile"
	"github.com/mad01/ralph/internal/gitutil"
	"github.com/mad01/ralph/internal/hooks"
	"github.com/mad01/ralph/internal/lockfile"
	"github.com/mad01/ralph/internal/packages"
	"github.com/mad01/ralph/internal/progress"
	"github.com/mad01/ralph/internal/report"
)

// applyOptions carries the flags the apply sequence reads. `ralph up` and
// `ralph apply` each fill it from their own flag variables.
type applyOptions struct {
	overwrite     bool   // --overwrite: replace existing files at symlink targets
	skip          bool   // --skip: leave existing files at symlink targets alone
	force         bool   // --force: re-run 'once' builds and rebuild packages
	build         string // --build: run only this build
	enableCleanup bool   // --enable-cleanup: remove orphaned recipe artifacts
}

// applyCommand describes one run of `ralph up` or `ralph apply`.
type applyCommand struct {
	name        string // report command name: "up" or "apply"
	header      string // first line printed, before the dry-run banner
	resetBuilds bool   // --reset-builds: clear all build state first
	sync        bool   // run the sync sequence before the apply sequence
	opts        applyOptions
}

// runApplyCommand is the body of `ralph up` and `ralph apply`. It holds the
// run lock, migrates a legacy config, loads the config, runs the sync
// sequence when c.sync is set, then the apply sequence.
func runApplyCommand(c applyCommand) error {
	runLock, err := acquireRunLock(dryRun)
	if err != nil {
		return err
	}
	defer func() { _ = runLock.Release() }()

	w := verboseWriter(verbose, dryRun)
	startRun(c.header)
	rpt := &report.Report{Command: c.name}
	if c.resetBuilds {
		if err := resetBuildState(w); err != nil {
			return err
		}
	}
	cfg, err := loadConfigOrReport(rpt)
	if err != nil {
		return err
	}
	host := config.GetCurrentHost()
	syncFailed := false
	if c.sync {
		cfg, syncFailed = syncAndReload(w, cfg, host, rpt)
	}
	ctx := &applyContext{
		cfg:         cfg,
		currentHost: host,
		dryRun:      dryRun,
		verbose:     verbose,
		w:           w,
		rpt:         rpt,
	}
	return runApplySequence(ctx, c.opts, syncFailed)
}

// acquireRunLock takes the run lock that keeps up, apply, sync and clean from
// running side by side. A dry run takes it without creating the lock file, so
// it writes nothing, yet still fails fast while a real run holds it.
func acquireRunLock(isDryRun bool) (*lockfile.Lock, error) {
	if isDryRun {
		return lockfile.AcquireExisting()
	}
	return lockfile.Acquire()
}

// startRun migrates a legacy dotter config, then prints the run header and,
// on a dry run, the dry-run banner.
func startRun(header string) {
	if err := config.MigrateFromLegacy(uiOut()); err != nil {
		fmt.Fprintln(os.Stderr, color.YellowString("Warning: legacy migration failed: %v", err))
	}
	fmt.Fprintln(uiOut(), header)
	if dryRun {
		printDryRunBanner(uiOut())
	}
}

// resetBuildState clears all build state for --reset-builds; a dry run only
// says it would.
func resetBuildState(w io.Writer) error {
	if dryRun {
		fmt.Fprintln(w, "[DRY RUN] Would reset all build state.")
		return nil
	}
	if err := hooks.ResetBuildState(uiOut()); err != nil {
		fmt.Fprintln(os.Stderr, color.RedString("Error resetting build state: %v", err))
		return fmt.Errorf("failed to reset build state: %w", err)
	}
	return nil
}

// loadConfigOrReport loads the config. On failure it records the error in rpt
// and prints the end-of-run report before returning it.
func loadConfigOrReport(rpt *report.Report) (*config.Config, error) {
	cfg, err := loadRunConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, color.RedString("Error loading configuration: %v", err))
		rpt.AddPhase("Configuration").AddFail("config", "failed to load", err)
		finishReport(rpt, nil, dryRun, verbose)
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	return cfg, nil
}

// loadRunConfig loads the config for up, apply, sync and clean. A dry run
// loads it read-only, so a preview never clones a recipe source or moves one
// to its pinned ref.
func loadRunConfig() (*config.Config, error) {
	return config.LoadConfigWithOptions(config.LoadOptions{ReadOnly: dryRun})
}

// runApplySequence applies ctx.cfg: pre-apply hooks, directories, dirs_mirror,
// repos, dotfiles, shell config, tools, builds and packages, post-apply hooks,
// the recipe-state manifest with optional cleanup, and the end-of-run report.
// syncFailed (a sync step failed earlier in this run) makes cleanup keep its
// orphans. It returns the error the command exits with.
func runApplySequence(ctx *applyContext, opts applyOptions, syncFailed bool) error {
	symlinkAction := opts.symlinkAction(ctx.w)
	if err := runPreApplyHooks(ctx); err != nil {
		finishReport(ctx.rpt, ctx, ctx.dryRun, ctx.verbose)
		return err
	}
	applyDirectories(ctx)
	applyDirsMirror(ctx, symlinkAction)
	applyRepos(ctx)
	applyDotfiles(ctx, symlinkAction)
	applyShellConfig(ctx)
	applyTools(ctx)
	applyBuildsAndPackages(ctx, hooks.BuildOptions{
		DryRun:        ctx.dryRun,
		Force:         opts.force,
		SpecificBuild: opts.build,
		Verbose:       ctx.verbose,
	}, opts.force)
	runPostApplyHooks(ctx)
	recordAndCleanup(ctx, opts.enableCleanup, syncFailed)

	finishReport(ctx.rpt, ctx, ctx.dryRun, ctx.verbose)
	if code := ctx.rpt.ExitCode(); code != 0 {
		return &ExitError{Code: code}
	}
	return nil
}

// symlinkAction picks what linking a dotfile does with an existing target:
// --overwrite wins over --skip, and backup is the default.
func (o applyOptions) symlinkAction(w io.Writer) dotfile.SymlinkAction {
	switch {
	case o.overwrite:
		fmt.Fprintln(w, "Symlink action: Overwrite existing files.")
		return dotfile.SymlinkActionOverwrite
	case o.skip:
		fmt.Fprintln(w, "Symlink action: Skip existing files.")
		return dotfile.SymlinkActionSkip
	}
	fmt.Fprintln(w, "Symlink action: Backup existing files.")
	return dotfile.SymlinkActionBackup
}

// runPreApplyHooks runs the pre-apply hooks. A failure stops the run.
func runPreApplyHooks(ctx *applyContext) error {
	if len(ctx.cfg.Hooks.PreApply) == 0 {
		return nil
	}
	phase := ctx.rpt.AddPhase("Pre-apply hooks")
	hookCtx := &hooks.HookContext{DryRun: ctx.dryRun}
	err := hooks.RunHooks(ctx.w, ctx.cfg.Hooks.PreApply, hooks.PreApply, hookCtx, ctx.dryRun)
	if err != nil {
		fmt.Fprintln(os.Stderr, color.RedString("Error executing pre-apply hooks: %v", err))
		phase.AddFail("pre-apply", err.Error(), err)
		return fmt.Errorf("pre-apply hooks failed: %w", err)
	}
	phase.AddOK("pre-apply", "completed")
	return nil
}

// runPostApplyHooks runs the post-apply hooks. A failure is a warning.
func runPostApplyHooks(ctx *applyContext) {
	if len(ctx.cfg.Hooks.PostApply) == 0 {
		return
	}
	phase := ctx.rpt.AddPhase("Post-apply hooks")
	hookCtx := &hooks.HookContext{DryRun: ctx.dryRun}
	err := hooks.RunHooks(ctx.w, ctx.cfg.Hooks.PostApply, hooks.PostApply, hookCtx, ctx.dryRun)
	if err != nil {
		fmt.Fprintln(os.Stderr, color.YellowString("Warning: post-apply hooks failed: %v", err))
		phase.AddWarn("post-apply", err.Error())
		return
	}
	phase.AddOK("post-apply", "completed")
}

// recordAndCleanup records the recipe-state manifest on every apply, so the
// cleanup baseline never goes stale, and removes orphans when --enable-cleanup
// or auto_cleanup asks for it. The banner names the toggle that is active.
func recordAndCleanup(ctx *applyContext, enableCleanup, syncFailed bool) {
	autoCleanup := ctx.cfg.RecipesConfig.AutoCleanup
	cleanup := enableCleanup || autoCleanup
	switch {
	case cleanup && autoCleanup && !enableCleanup:
		_, _ = color.New(color.FgCyan).
			Fprintln(ctx.w, "\nProcessing recipe cleanup (auto_cleanup=true)...")
	case cleanup:
		cleanupBanner(ctx.w)
	}
	opts := manifestOptions{cleanup: cleanup, dryRun: ctx.dryRun, syncFailed: syncFailed}
	recordManifestAndCleanup(ctx.cfg, ctx.currentHost, opts, ctx.w, ctx.rpt)
}

// syncAndReload runs the sync sequence for `ralph up`. When the pull advanced
// the dotfiles repo or a recipe source, the recipes and config on disk changed
// under this process, so it reloads the config and this same run applies the
// just-pulled state (otherwise cross-machine edits land one `ralph up` late).
// It returns the config to apply and whether a sync step failed.
func syncAndReload(
	w io.Writer,
	cfg *config.Config,
	host string,
	rpt *report.Report,
) (*config.Config, bool) {
	before := syncFingerprint(cfg)
	failed := !runSyncSequence(w, cfg, host, syncOptions{}, rpt)
	if syncFingerprint(cfg) == before {
		return cfg, failed
	}
	reloaded, err := loadRunConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, color.YellowString(
			"Warning: dotfiles repo or a recipe source updated but config reload failed; applying pre-pull config: %v",
			err,
		))
		return cfg, failed
	}
	fmt.Fprintln(
		w,
		color.CyanString("Dotfiles repo or recipe source advanced during sync; reloaded config."),
	)
	return reloaded, failed
}

// dotfilesRepoHead returns the dotfiles repo's current HEAD commit, or "" if it
// can't be determined (path missing, not a git repo). Used to detect whether the
// sync sequence advanced the repo so the config can be reloaded.
func dotfilesRepoHead(cfg *config.Config) string {
	expanded, err := config.ExpandPath(cfg.DotfilesRepoPath)
	if err != nil {
		return ""
	}
	return gitutil.GetGitHash(expanded)
}

// syncOptions narrows the sync sequence the way `ralph sync` flags can. The
// zero value is the full sync `ralph up` runs.
type syncOptions struct {
	noPull      bool   // sync --no-pull: leave the dotfiles repo and recipe sources as they are
	onlyPackage string // sync --package: sync just this package
}

// runSyncSequence pulls the dotfiles repo, then the update=true recipe
// sources, then clones or pulls remote packages, recording each step in rpt.
// It returns false when any step failed.
func runSyncSequence(
	w io.Writer,
	cfg *config.Config,
	host string,
	opts syncOptions,
	rpt *report.Report,
) bool {
	ok := pullDotfilesRepo(w, cfg, opts.noPull, rpt.AddPhase("Dotfiles repo"))
	if !opts.noPull && len(cfg.RecipeSources) > 0 {
		srcOK := syncRecipeSources(w, cfg, rpt.AddPhase("Recipe sources"), dryRun)
		syncStatusLine("Recipe sources", srcOK)
		ok = ok && srcOK
	}
	if len(cfg.Packages) > 0 {
		pkgOK := syncPackages(w, cfg, host, opts.onlyPackage, rpt.AddPhase("Packages (sync)"))
		ok = ok && pkgOK
	}
	return ok
}

// pullDotfilesRepo pulls the dotfiles repo into phase, or records the skip
// when skip is set. It returns false when the pull failed.
func pullDotfilesRepo(w io.Writer, cfg *config.Config, skip bool, phase *report.Phase) bool {
	ok := true
	if skip {
		fmt.Fprintf(w, "  Skipping dotfiles repo pull (--no-pull)\n")
		phase.AddSkip("dotfiles-repo", "skipped (--no-pull)")
	} else {
		ok = pullDotfiles(w, cfg, phase)
	}
	syncStatusLine("Dotfiles repo", ok)
	return ok
}

// pullDotfiles runs git pull in the dotfiles repo and records the outcome.
func pullDotfiles(w io.Writer, cfg *config.Config, phase *report.Phase) bool {
	repoPath, err := config.ExpandPath(cfg.DotfilesRepoPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, color.RedString("Error expanding dotfiles_repo_path: %v", err))
		phase.AddFail("dotfiles-repo", "failed to expand path", err)
		return false
	}
	fmt.Fprintf(w, "  Pulling dotfiles repo: %s\n", repoPath)
	if err := packages.GitPull(context.Background(), w, repoPath, dryRun, verbose); err != nil {
		fmt.Fprintln(os.Stderr, color.RedString("Error pulling dotfiles repo: %v", err))
		phase.AddFail("dotfiles-repo", "pull failed", err)
		return false
	}
	msg := "pulled"
	if dryRun {
		msg = "[DRY RUN] would pull"
	}
	phase.AddOK("dotfiles-repo", msg)
	return true
}

// syncPackages clones or pulls the configured packages (only the named one
// when only is set) and records each result in phase. It returns false when a
// package failed or only names no configured package.
func syncPackages(w io.Writer, cfg *config.Config, host, only string, phase *report.Phase) bool {
	if unknownPackage(cfg, only) {
		fmt.Fprintln(os.Stderr, color.RedString("Package '%s' not found in configuration", only))
		phase.AddFail(only, "not found in configuration", nil)
		return false
	}
	opts := packages.SyncOptions{DryRun: dryRun, SpecificPackage: only, Verbose: verbose}
	results := packages.SyncPackages(
		context.Background(), w, cfg.Packages, cfg.PackagesDir, host, cfg.Profiles, opts,
	)
	ok := true
	for _, r := range results {
		switch r.Action {
		case "error":
			fmt.Fprintln(os.Stderr, color.RedString("  %s: %s: %v", r.Name, r.Message, r.Err))
			phase.AddFail(r.Name, r.Message, r.Err)
			ok = false
		case "skipped":
			phase.AddSkip(r.Name, r.Message)
		default:
			fmt.Fprintf(w, "  %s: %s\n", r.Name, r.Message)
			phase.AddOK(r.Name, r.Message)
		}
	}
	return ok
}

// unknownPackage reports whether name, the package `ralph sync --package`
// asks for, is set while cfg has packages and none of them is name. With no
// packages configured the package step does not run, so no name is unknown.
func unknownPackage(cfg *config.Config, name string) bool {
	_, known := cfg.Packages[name]
	return name != "" && len(cfg.Packages) > 0 && !known
}

// syncStatusLine prints a one-line sync step status unless verbose or
// dry-run output already shows the step.
func syncStatusLine(label string, ok bool) {
	if !verbose && !dryRun {
		progress.StatusLine(label, ok)
	}
}
