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
	"github.com/spf13/cobra"
)

var (
	upOverwrite     bool
	upSkip          bool
	upForce         bool
	upBuild         string
	upResetBuilds   bool
	upNoSync        bool
	upEnableCleanup bool
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Sync and apply ralph configurations",
	Long: `Pulls the dotfiles repo, syncs remote packages, and then applies all
configurations (symlinks, shell config, builds, packages, cleanup).
Replaces the separate 'ralph sync' + 'ralph apply' workflow.

Use --no-sync to skip the sync step and only apply.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		runLock, err := lockfile.Acquire()
		if err != nil {
			return err
		}
		defer func() { _ = runLock.Release() }()

		w := verboseWriter(verbose, dryRun)

		if err := config.MigrateFromLegacy(uiOut()); err != nil {
			fmt.Fprintln(os.Stderr, color.YellowString("Warning: legacy migration failed: %v", err))
		}

		fmt.Fprintln(uiOut(), "Ralph up...")

		if dryRun {
			printDryRunBanner(uiOut())
		}

		rpt := &report.Report{Command: "up"}

		if upResetBuilds {
			if dryRun {
				fmt.Fprintln(w, "[DRY RUN] Would reset all build state.")
			} else {
				if err := hooks.ResetBuildState(uiOut()); err != nil {
					fmt.Fprintln(os.Stderr, color.RedString("Error resetting build state: %v", err))
					return fmt.Errorf("failed to reset build state: %w", err)
				}
			}
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error loading configuration: %v", err))
			cfgPhase := rpt.AddPhase("Configuration")
			cfgPhase.AddFail("config", "failed to load", err)
			finishReport(rpt, nil, dryRun, verbose)
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		currentHost := config.GetCurrentHost()

		// --- Sync phase ---
		if !upNoSync {
			headBefore := dotfilesRepoHead(cfg)
			runSyncPhase(w, cfg, currentHost, rpt)
			// If the pull advanced the dotfiles repo, the recipes/config on disk
			// changed under us — reload so this same run applies the just-pulled
			// state instead of the pre-pull snapshot (otherwise cross-machine
			// edits always land one `ralph up` late).
			if headAfter := dotfilesRepoHead(cfg); headAfter != "" && headAfter != headBefore {
				if reloaded, err := config.LoadConfig(); err != nil {
					fmt.Fprintln(
						os.Stderr,
						color.YellowString(
							"Warning: dotfiles repo updated but config reload failed; applying pre-pull config: %v",
							err,
						),
					)
				} else {
					cfg = reloaded
					fmt.Fprintln(w, color.CyanString("Dotfiles repo advanced during sync; reloaded config."))
				}
			}
		}

		// --- Apply phase ---
		symlinkAction := dotfile.SymlinkActionBackup
		if upOverwrite {
			symlinkAction = dotfile.SymlinkActionOverwrite
			fmt.Fprintln(w, "Symlink action: Overwrite existing files.")
		} else if upSkip {
			symlinkAction = dotfile.SymlinkActionSkip
			fmt.Fprintln(w, "Symlink action: Skip existing files.")
		} else {
			fmt.Fprintln(w, "Symlink action: Backup existing files.")
		}

		ctx := &applyContext{
			cfg:         cfg,
			currentHost: currentHost,
			dryRun:      dryRun,
			verbose:     verbose,
			w:           w,
			rpt:         rpt,
		}

		if len(cfg.Hooks.PreApply) > 0 {
			prePhase := rpt.AddPhase("Pre-apply hooks")
			preContext := &hooks.HookContext{DryRun: dryRun}
			if err := hooks.RunHooks(w, cfg.Hooks.PreApply, hooks.PreApply, preContext, dryRun); err != nil {
				fmt.Fprintln(os.Stderr, color.RedString("Error executing pre-apply hooks: %v", err))
				prePhase.AddFail("pre-apply", err.Error(), err)
				finishReport(rpt, ctx, dryRun, verbose)
				return fmt.Errorf("pre-apply hooks failed: %w", err)
			}
			prePhase.AddOK("pre-apply", "completed")
		}

		applyDirectories(ctx)
		applyDirsMirror(ctx, symlinkAction)
		applyRepos(ctx)
		applyDotfiles(ctx, symlinkAction)
		applyShellConfig(ctx)
		applyTools(ctx)
		applyBuildsAndPackages(ctx, hooks.BuildOptions{
			DryRun:        dryRun,
			Force:         upForce,
			SpecificBuild: upBuild,
			Verbose:       verbose,
		}, upForce)

		if len(cfg.Hooks.PostApply) > 0 {
			postPhase := rpt.AddPhase("Post-apply hooks")
			postContext := &hooks.HookContext{DryRun: dryRun}
			if err := hooks.RunHooks(w, cfg.Hooks.PostApply, hooks.PostApply, postContext, dryRun); err != nil {
				fmt.Fprintln(
					os.Stderr,
					color.YellowString("Warning: post-apply hooks failed: %v", err),
				)
				postPhase.AddWarn("post-apply", err.Error())
			} else {
				postPhase.AddOK("post-apply", "completed")
			}
		}

		// Record the recipe-state manifest on every apply (so the cleanup
		// baseline never goes stale) and run the cleanup phase when enabled via
		// --enable-cleanup or auto_cleanup. The banner is printed here because
		// the wording depends on which toggle is active.
		shouldCleanup := upEnableCleanup || cfg.RecipesConfig.AutoCleanup
		if shouldCleanup {
			if cfg.RecipesConfig.AutoCleanup && !upEnableCleanup {
				color.New(color.FgCyan).
					Fprintln(w, "\nProcessing recipe cleanup (auto_cleanup=true)...")
			} else {
				cleanupBanner(w)
			}
		}
		recordManifestAndCleanup(cfg, currentHost, shouldCleanup, dryRun, w, rpt)

		finishReport(rpt, ctx, dryRun, verbose)
		if code := rpt.ExitCode(); code != 0 {
			return &ExitError{Code: code}
		}
		return nil
	},
}

// dotfilesRepoHead returns the dotfiles repo's current HEAD commit, or "" if it
// can't be determined (path missing, not a git repo). Used to detect whether the
// sync phase advanced the repo so the config can be reloaded.
func dotfilesRepoHead(cfg *config.Config) string {
	expanded, err := config.ExpandPath(cfg.DotfilesRepoPath)
	if err != nil {
		return ""
	}
	return gitutil.GetGitHash(expanded)
}

// runSyncPhase pulls the dotfiles repo and syncs remote packages.
func runSyncPhase(w io.Writer, cfg *config.Config, currentHost string, rpt *report.Report) {
	pullPhase := rpt.AddPhase("Dotfiles repo")
	pullOK := true

	expandedRepoPath, err := config.ExpandPath(cfg.DotfilesRepoPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, color.RedString("Error expanding dotfiles_repo_path: %v", err))
		pullPhase.AddFail("dotfiles-repo", "failed to expand path", err)
		pullOK = false
	} else {
		fmt.Fprintf(w, "  Pulling dotfiles repo: %s\n", expandedRepoPath)
		if err := packages.GitPull(context.Background(), w, expandedRepoPath, dryRun, verbose); err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error pulling dotfiles repo: %v", err))
			pullPhase.AddFail("dotfiles-repo", "pull failed", err)
			pullOK = false
		} else {
			pullPhase.AddOK("dotfiles-repo", "pulled")
		}
	}
	if !verbose && !dryRun {
		progress.StatusLine("Dotfiles repo", pullOK)
	}

	if len(cfg.Packages) > 0 {
		remotePhase := rpt.AddPhase("Packages (sync)")
		opts := packages.SyncOptions{
			DryRun:  dryRun,
			Verbose: verbose,
		}
		results := packages.SyncPackages(
			context.Background(),
			w,
			cfg.Packages,
			cfg.PackagesDir,
			currentHost,
			opts,
		)
		for _, r := range results {
			switch r.Action {
			case "error":
				fmt.Fprintln(os.Stderr, color.RedString("  %s: %s: %v", r.Name, r.Message, r.Err))
				remotePhase.AddFail(r.Name, r.Message, r.Err)
			case "skipped":
				remotePhase.AddSkip(r.Name, r.Message)
			default:
				fmt.Fprintf(w, "  %s: %s\n", r.Name, r.Message)
				remotePhase.AddOK(r.Name, r.Message)
			}
		}
	}
}

func init() {
	rootCmd.AddCommand(upCmd)
	upCmd.Flags().
		BoolVar(&upOverwrite, "overwrite", false, "Overwrite existing files at target locations")
	upCmd.Flags().BoolVar(&upSkip, "skip", false, "Skip symlinking if target file already exists")
	upCmd.Flags().BoolVar(&upForce, "force", false, "Force re-run of 'once' builds")
	upCmd.Flags().StringVar(&upBuild, "build", "", "Run only the specified build")
	upCmd.Flags().
		BoolVar(&upResetBuilds, "reset-builds", false, "Clear all build state before running")
	upCmd.Flags().
		BoolVar(&upNoSync, "no-sync", false, "Skip syncing (pull + package sync) and only apply")
	upCmd.Flags().
		BoolVar(&upEnableCleanup, "enable-cleanup", false, "Remove orphaned artifacts from removed/disabled recipes")
}
