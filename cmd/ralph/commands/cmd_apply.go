package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/dotfile"
	"github.com/mad01/ralph/internal/hooks"
	"github.com/mad01/ralph/internal/packages"
	"github.com/mad01/ralph/internal/progress"
	"github.com/mad01/ralph/internal/repo"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/shell"
	"github.com/mad01/ralph/internal/state"
	"github.com/mad01/ralph/internal/tool"
	"github.com/spf13/cobra"
)

var (
	overwriteExisting bool
	skipExisting      bool
	forceBuilds       bool
	specificBuild     string
	resetBuilds       bool
	enableCleanup     bool
)

// applyContext holds the shared state threaded through every apply phase.
type applyContext struct {
	cfg         *config.Config
	currentHost string
	dryRun      bool
	verbose     bool
	w           io.Writer
	rpt         *report.Report
	caveats     []recipeCaveat
}

type recipeCaveat struct {
	recipe string
	text   string
}

func (ctx *applyContext) collectCaveat(recipeName string) {
	for _, r := range ctx.cfg.LoadedRecipes {
		if r.Name == recipeName && r.Caveats != "" {
			ctx.caveats = append(ctx.caveats, recipeCaveat{recipe: recipeName, text: r.Caveats})
			return
		}
	}
}

var applyCmd = &cobra.Command{
	Use:        "apply",
	Short:      "Apply ralph configurations",
	Long:       `Applies the configurations defined in your ralph config file. This includes symlinking dotfiles, setting up shell environments, etc.`,
	Deprecated: "use 'ralph up --no-sync' instead",
	RunE: func(cmd *cobra.Command, args []string) error {
		w := verboseWriter(verbose, dryRun)

		// Auto-migrate from legacy dotter config
		if err := config.MigrateFromLegacy(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, color.YellowString("Warning: legacy migration failed: %v", err))
		}

		fmt.Println("Applying ralph configurations...")

		if dryRun {
			printDryRunBanner(os.Stdout)
		}

		rpt := &report.Report{Command: "apply"}

		// Handle --reset-builds flag
		if resetBuilds {
			if dryRun {
				fmt.Fprintln(w, "[DRY RUN] Would reset all build state.")
			} else {
				if err := hooks.ResetBuildState(os.Stdout); err != nil {
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
			rpt.PrintSummary(os.Stdout, summaryVerbosity())
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		// Get current hostname for host filtering
		currentHost := config.GetCurrentHost()

		symlinkAction := dotfile.SymlinkActionBackup // Default action
		if overwriteExisting {
			symlinkAction = dotfile.SymlinkActionOverwrite
			fmt.Fprintln(w, "Symlink action: Overwrite existing files.")
		} else if skipExisting {
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

		// Execute pre-apply hooks
		if len(cfg.Hooks.PreApply) > 0 {
			prePhase := rpt.AddPhase("Pre-apply hooks")
			preContext := &hooks.HookContext{
				DryRun: dryRun,
			}
			if err := hooks.RunHooks(w, cfg.Hooks.PreApply, hooks.PreApply, preContext, dryRun); err != nil {
				fmt.Fprintln(os.Stderr, color.RedString("Error executing pre-apply hooks: %v", err))
				prePhase.AddFail("pre-apply", err.Error(), err)
				rpt.PrintSummary(os.Stdout, summaryVerbosity())
				return fmt.Errorf("pre-apply hooks failed: %w", err)
			}
			prePhase.AddOK("pre-apply", "completed")
		}

		// Phase execution
		applyDirectories(ctx)
		applyDirsMirror(ctx, symlinkAction)
		applyRepos(ctx)
		applyDotfiles(ctx, symlinkAction)
		applyShellConfig(ctx)
		applyTools(ctx)
		applyBuildsAndPackages(ctx, hooks.BuildOptions{
			DryRun:        dryRun,
			Force:         forceBuilds,
			SpecificBuild: specificBuild,
			Verbose:       verbose,
		}, forceBuilds)

		// Execute post-apply hooks
		if len(cfg.Hooks.PostApply) > 0 {
			postPhase := rpt.AddPhase("Post-apply hooks")
			postContext := &hooks.HookContext{
				DryRun: dryRun,
			}
			if err := hooks.RunHooks(w, cfg.Hooks.PostApply, hooks.PostApply, postContext, dryRun); err != nil {
				fmt.Fprintln(os.Stderr, color.YellowString("Warning: post-apply hooks failed: %v", err))
				postPhase.AddWarn("post-apply", err.Error())
			} else {
				postPhase.AddOK("post-apply", "completed")
			}
		}

		// Recipe cleanup: triggered by --enable-cleanup flag or auto_cleanup config.
		shouldCleanup := enableCleanup || cfg.RecipesConfig.AutoCleanup
		if shouldCleanup {
			cleanupPhase := rpt.AddPhase("Cleanup")
			cleanupBanner(w)

			next, manifestErr := buildIntendedManifest(cfg, currentHost, time.Now())
			if manifestErr != nil {
				fmt.Fprintln(os.Stderr, color.YellowString("Warning: skipping cleanup, could not build manifest: %v", manifestErr))
				cleanupPhase.AddWarn("manifest", manifestErr.Error())
			} else {
				prev, loadErr := state.Load()
				if loadErr != nil {
					fmt.Fprintln(os.Stderr, color.YellowString("Warning: could not load recipe state: %v", loadErr))
					cleanupPhase.AddWarn("load", loadErr.Error())
				} else {
					carryForwardFrozenRecipes(prev, next, frozenRecipeSet(cfg))
					if len(prev.Recipes) == 0 && cfg.RecipesConfig.AutoCleanup {
						_, _ = fmt.Fprintln(w, color.CyanString("First run with auto_cleanup: seeding state baseline (no artifacts will be removed)."))
						cleanupPhase.AddOK("baseline", "initial state recorded")
					} else {
						runCleanup(prev, next, dryRun, w, cleanupPhase)
					}
				}
				if !dryRun {
					if err := state.Save(next); err != nil {
						fmt.Fprintln(os.Stderr, color.YellowString("Warning: could not save recipe state: %v", err))
						cleanupPhase.AddWarn("save", err.Error())
					}
				}
			}
		}

		printApplyResult(rpt, ctx, dryRun, verbose)
		if code := rpt.ExitCode(); code != 0 {
			return &ExitError{Code: code}
		}
		return nil
	},
}

// applyDirectories creates configured directories, honoring enable and host filters.
func applyDirectories(ctx *applyContext) {
	dirPhase := ctx.rpt.AddPhase("Directories")
	if len(ctx.cfg.Directories) == 0 {
		return
	}

	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	dirNames := make([]string, 0, len(ctx.cfg.Directories))
	for name := range ctx.cfg.Directories {
		dirNames = append(dirNames, name)
	}
	sort.Strings(dirNames)
	prog := progress.New("Directories", len(dirNames))
	if ctx.verbose || ctx.dryRun {
		prog = progress.NewQuiet()
	}
	for _, name := range dirNames {
		prog.TickWith(name)
		dir := ctx.cfg.Directories[name]
		if !config.IsEnabled(dir.Enable) {
			fmt.Fprintf(ctx.w, "  %s %s\n", color.CyanString("skip"), dim(name+" (disabled)"))
			dirPhase.AddSkip(name, "disabled")
			continue
		}
		if !config.ShouldApplyForHost(dir.Hosts, ctx.currentHost) {
			fmt.Fprintf(ctx.w, "  %s %s\n", color.CyanString("skip"), dim(name+" (host filter)"))
			dirPhase.AddSkip(name, "host filter")
			continue
		}
		fmt.Fprintf(ctx.w, "  %s\n", bold(name))
		fmt.Fprintf(ctx.w, "    %s\n", dim(dir.Target))
		if err := dotfile.CreateDirectory(ctx.w, dir, ctx.dryRun); err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("    error: %s: %v", name, err))
			dirPhase.AddFail(name, err.Error(), err)
		} else {
			dirPhase.AddOK(name, "")
		}
	}
	prog.Done()
}

// applyDirsMirror processes dirs_mirror entries, creating symlinks for each
// entry (file or subdirectory) found in the source directory.
func applyDirsMirror(ctx *applyContext, symlinkAction dotfile.SymlinkAction) {
	if len(ctx.cfg.DirsMirror) == 0 {
		return
	}

	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	dmPhase := ctx.rpt.AddPhase("Dirs mirror")
	dmNames := make([]string, 0, len(ctx.cfg.DirsMirror))
	for name := range ctx.cfg.DirsMirror {
		dmNames = append(dmNames, name)
	}
	sort.Strings(dmNames)

	prog := progress.New("Dirs mirror", len(dmNames))
	if ctx.verbose || ctx.dryRun {
		prog = progress.NewQuiet()
	}

	for _, name := range dmNames {
		prog.TickWith(name)
		dm := ctx.cfg.DirsMirror[name]

		if !config.IsEnabled(dm.Enable) {
			fmt.Fprintf(ctx.w, "  %s %s\n", color.CyanString("skip"), dim(name+" (disabled)"))
			dmPhase.AddSkip(name, "disabled")
			continue
		}
		if !config.ShouldApplyForHost(dm.Hosts, ctx.currentHost) {
			fmt.Fprintf(ctx.w, "  %s %s\n", color.CyanString("skip"), dim(name+" (host filter)"))
			dmPhase.AddSkip(name, "host filter")
			continue
		}

		fmt.Fprintf(ctx.w, "  %s\n", bold(name))

		// Resolve source directory
		absoluteSource, err := config.ExpandPath(filepath.Join(ctx.cfg.DotfilesRepoPath, dm.Source))
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("    error: %s: failed to expand source path: %v", name, err))
			dmPhase.AddFail(name, fmt.Sprintf("expand source: %v", err), err)
			continue
		}

		// Ensure source directory exists
		srcInfo, err := os.Stat(absoluteSource)
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("    error: %s: source directory '%s' does not exist: %v", name, absoluteSource, err))
			dmPhase.AddFail(name, fmt.Sprintf("source not found: %v", err), err)
			continue
		}
		if !srcInfo.IsDir() {
			err := fmt.Errorf("source '%s' is not a directory", absoluteSource)
			fmt.Fprintln(os.Stderr, color.RedString("    error: %s: %v", name, err))
			dmPhase.AddFail(name, err.Error(), err)
			continue
		}

		// Read entries from source directory
		entries, err := os.ReadDir(absoluteSource)
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("    error: %s: failed to read source directory: %v", name, err))
			dmPhase.AddFail(name, fmt.Sprintf("read source dir: %v", err), err)
			continue
		}

		expandedTarget, err := config.ExpandPath(dm.Target)
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("    error: %s: failed to expand target path: %v", name, err))
			dmPhase.AddFail(name, fmt.Sprintf("expand target: %v", err), err)
			continue
		}

		// Ensure target directory exists
		if !ctx.dryRun {
			if err := os.MkdirAll(expandedTarget, 0755); err != nil {
				fmt.Fprintln(os.Stderr, color.RedString("    error: %s: failed to create target directory: %v", name, err))
				dmPhase.AddFail(name, fmt.Sprintf("create target dir: %v", err), err)
				continue
			}
		}

		action := dm.Action
		if action == "" {
			action = "symlink"
		}

		linked := 0
		failed := 0
		for _, entry := range entries {
			// Skip hidden files/dirs
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			entrySource := filepath.Join(dm.Source, entry.Name())
			entryTarget := filepath.Join(dm.Target, entry.Name())

			fmt.Fprintf(ctx.w, "    %s → %s\n", dim(entryTarget), dim(entrySource))

			df := config.Dotfile{
				Source: entrySource,
				Target: entryTarget,
			}

			var symlinkErr error
			switch action {
			case "symlink_dir":
				symlinkErr = dotfile.CreateDirSymlink(ctx.w, df, ctx.cfg.DotfilesRepoPath, symlinkAction, ctx.dryRun)
			default: // "symlink"
				symlinkErr = dotfile.CreateSymlink(ctx.w, df, ctx.cfg.DotfilesRepoPath, symlinkAction, ctx.dryRun)
			}

			if errors.Is(symlinkErr, dotfile.ErrSkipped) {
				linked++ // already in place, count as success
			} else if symlinkErr != nil {
				fmt.Fprintln(os.Stderr, color.RedString("    error: %s/%s: %v", name, entry.Name(), symlinkErr))
				failed++
			} else {
				linked++
			}
		}

		if failed > 0 {
			dmPhase.AddFail(name, fmt.Sprintf("%d linked, %d failed", linked, failed), fmt.Errorf("%d entries failed", failed))
		} else {
			dmPhase.AddOK(name, fmt.Sprintf("%d entries linked", linked))
		}
	}
	prog.Done()
}

// applyRepos clones or updates configured git repositories.
func applyRepos(ctx *applyContext) {
	if len(ctx.cfg.Repos) == 0 {
		return
	}
	repoPhase := ctx.rpt.AddPhase("Repositories")
	defer func() {
		if !ctx.verbose && !ctx.dryRun {
			_, _, fail, _ := repoPhase.Counts()
			progress.StatusLine("Repositories", fail == 0)
		}
	}()
	if err := repo.ProcessRepos(ctx.w, ctx.cfg.Repos, ctx.currentHost, ctx.dryRun); err != nil {
		fmt.Fprintln(os.Stderr, color.RedString("Error processing repositories: %v", err))
		repoPhase.AddFail("repos", err.Error(), err)
	} else {
		repoPhase.AddOK("repos", "processed")
	}
}

// applyDotfiles processes dotfile symlinks, copies, and templates with pre/post link hooks.
func applyDotfiles(ctx *applyContext, symlinkAction dotfile.SymlinkAction) {
	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	fmt.Fprintln(ctx.w, "\nProcessing dotfiles...")
	dotfilesApplied := 0
	dotfilesSkipped := 0
	dotfilesFailed := 0
	dfPhase := ctx.rpt.AddPhase("Dotfiles")

	dfNames := make([]string, 0, len(ctx.cfg.Dotfiles))
	for name := range ctx.cfg.Dotfiles {
		dfNames = append(dfNames, name)
	}
	sort.Strings(dfNames)
	dfProg := progress.New("Dotfiles", len(dfNames))
	if ctx.verbose || ctx.dryRun {
		dfProg = progress.NewQuiet()
	}
	for _, name := range dfNames {
		dfProg.TickWith(name)
		df := ctx.cfg.Dotfiles[name]
		if !config.IsEnabled(df.Enable) {
			fmt.Fprintf(ctx.w, "  %s %s\n", color.CyanString("skip"), dim(name+" (disabled)"))
			dfPhase.AddSkip(name, "disabled")
			continue
		}
		if !config.ShouldApplyForHost(df.Hosts, ctx.currentHost) {
			fmt.Fprintf(ctx.w, "  %s %s\n", color.CyanString("skip"), dim(name+" (host filter)"))
			dfPhase.AddSkip(name, "host filter")
			continue
		}
		fmt.Fprintf(ctx.w, "  %s\n", bold(name))
		fmt.Fprintf(ctx.w, "    %s → %s\n", dim(df.Target), dim(df.Source))

		// Execute pre-link hooks for this specific dotfile
		if preHooks, exists := ctx.cfg.Hooks.PreLink[name]; exists && len(preHooks) > 0 {
			linkContext := &hooks.HookContext{
				DotfileName: name,
				SourcePath:  filepath.Join(ctx.cfg.DotfilesRepoPath, df.Source),
				TargetPath:  df.Target,
				DryRun:      ctx.dryRun,
			}
			if err := hooks.RunHooks(ctx.w, preHooks, hooks.PreLink, linkContext, ctx.dryRun); err != nil {
				fmt.Fprintln(os.Stderr, color.RedString("Error executing pre-link hooks for %s: %v", name, err))
				dotfilesFailed++
				dfPhase.AddFail(name, fmt.Sprintf("pre-link hook: %v", err), err)
				continue
			}
		}

		templateData := make(map[string]any)

		var symlinkErr error
		currentSourcePath := filepath.Join(ctx.cfg.DotfilesRepoPath, df.Source)
		dotfileToSymlink := df
		repoPathForSymlink := ctx.cfg.DotfilesRepoPath

		if df.IsTemplate {
			fmt.Fprintf(ctx.w, "    %s\n", dim("template: "+df.Source))
			var processedPath string
			var templateErr error
			if ctx.dryRun {
				processedPath, templateErr = dotfile.WriteProcessedTemplateToFile(ctx.w, currentSourcePath, ctx.cfg, templateData, true)
				if templateErr == nil && processedPath == "" { // dry run specific path
					processedPath = "/tmp/fake_processed_template_for_dry_run" // ensure it has a value for dry run
				}
			} else {
				processedPath, templateErr = dotfile.WriteProcessedTemplateToFile(ctx.w, currentSourcePath, ctx.cfg, templateData, false)
			}

			if templateErr != nil {
				fmt.Fprintln(os.Stderr, color.YellowString("    - Warning: Error processing template for %s: %v", name, templateErr))
				dotfilesFailed++
				dfPhase.AddWarn(name, fmt.Sprintf("template error: %v", templateErr))
				continue
			}
			dotfileToSymlink.Source = processedPath
			repoPathForSymlink = "" // Processed template is an absolute path
		}

		// Determine action based on action field
		switch df.Action {
		case "copy":
			symlinkErr = dotfile.CopyFile(ctx.w, dotfileToSymlink, repoPathForSymlink, symlinkAction, ctx.dryRun)
		case "symlink_dir":
			symlinkErr = dotfile.CreateDirSymlink(ctx.w, dotfileToSymlink, repoPathForSymlink, symlinkAction, ctx.dryRun)
		default:
			// Default to regular symlink
			symlinkErr = dotfile.CreateSymlink(ctx.w, dotfileToSymlink, repoPathForSymlink, symlinkAction, ctx.dryRun)
		}

		// Cleanup for templated files
		if df.IsTemplate && repoPathForSymlink == "" && !ctx.dryRun && dotfileToSymlink.Source != "/tmp/fake_processed_template_for_dry_run" {
			// Check if the source is in a temp-like directory before removing
			// This is a basic check; for more robust checks, consider if WriteProcessedTemplateToFile returns if it's a temp file.
			if strings.HasPrefix(dotfileToSymlink.Source, os.TempDir()) || strings.Contains(dotfileToSymlink.Source, "ralph-temp-") {
				if removeErr := os.Remove(dotfileToSymlink.Source); removeErr != nil {
					fmt.Fprintln(os.Stderr, color.YellowString("    - Warning: failed to remove temporary processed file %s: %v", dotfileToSymlink.Source, removeErr))
				}
			}
		}

		if errors.Is(symlinkErr, dotfile.ErrSkipped) {
			dotfilesSkipped++
			dfPhase.AddSkip(name, "target exists")
		} else if symlinkErr != nil {
			fmt.Fprintln(os.Stderr, color.RedString("    error: %s: %v", name, symlinkErr))
			dotfilesFailed++
			dfPhase.AddFail(name, symlinkErr.Error(), symlinkErr)
		} else {
			dotfilesApplied++

			// Execute post-link hooks for this specific dotfile if symlink was created successfully
			postHookFailed := false
			if postHooks, exists := ctx.cfg.Hooks.PostLink[name]; exists && len(postHooks) > 0 {
				linkContext := &hooks.HookContext{
					DotfileName: name,
					SourcePath:  filepath.Join(ctx.cfg.DotfilesRepoPath, df.Source),
					TargetPath:  df.Target,
					DryRun:      ctx.dryRun,
				}
				if err := hooks.RunHooks(ctx.w, postHooks, hooks.PostLink, linkContext, ctx.dryRun); err != nil {
					fmt.Fprintln(os.Stderr, color.YellowString("Warning: post-link hook for %s failed: %v", name, err))
					dfPhase.AddWarn(name+"/post-hook", err.Error())
					postHookFailed = true
				}
			}
			if !postHookFailed {
				dfPhase.AddOK(name, "")
			}
		}
	}
	dfProg.Done()
	if ctx.dryRun {
		fmt.Fprintf(ctx.w, "  Dotfiles (dry run): would apply %s, %s skipped, %s failed.\n",
			color.GreenString("%d", dotfilesApplied), color.CyanString("%d", dotfilesSkipped), color.YellowString("%d", dotfilesFailed))
	} else {
		fmt.Fprintf(ctx.w, "  Dotfiles processed: %s applied, %s skipped, %s failed.\n",
			color.GreenString("%d", dotfilesApplied), color.CyanString("%d", dotfilesSkipped), color.YellowString("%d", dotfilesFailed))
	}
}

// applyShellConfig generates shell alias, function, and env files, then injects source lines.
func applyShellConfig(ctx *applyContext) {
	fmt.Fprintln(ctx.w, "\nProcessing shell configurations...")
	shellPhase := ctx.rpt.AddPhase("Shell config")
	defer func() {
		if !ctx.verbose && !ctx.dryRun {
			_, _, fail, _ := shellPhase.Counts()
			progress.StatusLine("Shell config", fail == 0)
		}
	}()
	resolvedShells := shell.ResolveShell(ctx.cfg.Shell.Name)
	currentShell := resolvedShells[0]
	if len(resolvedShells) > 1 {
		// Fallback to all shells means we couldn't determine a single shell
		fmt.Fprintln(os.Stderr, color.YellowString("Could not determine current shell. Skipping shell configuration."))
		shellPhase.AddSkip("shell", "could not determine shell")
		return
	}

	fmt.Fprintf(ctx.w, "  Detected shell: %s\n", currentShell)
	aliasFile, funcFile, genErr := shell.GenerateShellConfigs(ctx.w, ctx.cfg, currentShell, ctx.dryRun)

	if genErr != nil {
		fmt.Fprintln(os.Stderr, color.RedString("  Error generating shell configs for %s: %v", currentShell, genErr))
		shellPhase.AddFail(string(currentShell), fmt.Sprintf("generate configs: %v", genErr), genErr)
		return
	}

	// Generate env file -- sourced before aliases and functions so they can reference env vars.
	envFilePath, envPathErr := shell.GetEnvFilePath()
	if envPathErr != nil {
		fmt.Fprintln(os.Stderr, color.RedString("  Error getting env file path: %v", envPathErr))
		shellPhase.AddFail(string(currentShell), fmt.Sprintf("env file path: %v", envPathErr), envPathErr)
		return
	}
	if envErr := shell.GenerateEnvFile(ctx.w, ctx.cfg.Shell.Env, envFilePath, ctx.dryRun); envErr != nil {
		fmt.Fprintln(os.Stderr, color.RedString("  Error generating env file: %v", envErr))
		shellPhase.AddFail(string(currentShell), fmt.Sprintf("generate env file: %v", envErr), envErr)
		return
	}

	linesToSource := []string{}
	// Env vars sourced first so aliases/functions can reference them.
	if len(ctx.cfg.Shell.Env) > 0 {
		linesToSource = append(linesToSource, fmt.Sprintf("source %s", toPortablePath(envFilePath)))
	}
	if aliasFile != "" && (len(ctx.cfg.Shell.Aliases) > 0 || (ctx.dryRun && aliasFile != "")) {
		linesToSource = append(linesToSource, fmt.Sprintf("source %s", toPortablePath(aliasFile)))
	}
	if funcFile != "" && (len(ctx.cfg.Shell.Functions) > 0 || (ctx.dryRun && funcFile != "")) {
		linesToSource = append(linesToSource, fmt.Sprintf("source %s", toPortablePath(funcFile)))
	}

	if len(linesToSource) == 0 {
		fmt.Fprintln(ctx.w, "  No shell aliases, functions, or env vars configured to source.")
		shellPhase.AddOK(string(currentShell), "nothing to source")
		return
	}

	fmt.Fprintf(ctx.w, "  Injecting source lines into %s rc file...\n", currentShell)
	if err := shell.InjectSourceLines(ctx.w, currentShell, linesToSource, ctx.dryRun); err != nil {
		fmt.Fprintln(os.Stderr, color.RedString("  Error injecting source lines into %s rc file: %v", currentShell, err))
		shellPhase.AddFail(string(currentShell), fmt.Sprintf("inject source lines: %v", err), err)
	} else {
		shellPhase.AddOK(string(currentShell), "")
	}
}

// applyTools checks the installation status of configured tools.
func applyTools(ctx *applyContext) {
	toolPhase := ctx.rpt.AddPhase("Tools")
	if len(ctx.cfg.Tools) == 0 {
		return
	}

	prog := progress.New("Tools", len(ctx.cfg.Tools))
	if ctx.verbose || ctx.dryRun {
		prog = progress.NewQuiet()
	}

	fmt.Fprintln(ctx.w, "\nChecking tool configurations (installation not performed by apply):")
	for _, t := range ctx.cfg.Tools {
		prog.TickWith(t.Name)
		if !config.IsEnabled(t.Enable) {
			fmt.Fprintf(ctx.w, "  Skipping tool: %s (disabled)\n", t.Name)
			toolPhase.AddSkip(t.Name, "disabled")
			continue
		}
		if !config.ShouldApplyForHost(t.Hosts, ctx.currentHost) {
			fmt.Fprintf(ctx.w, "  Skipping tool: %s (host filter)\n", t.Name)
			toolPhase.AddSkip(t.Name, "host filter")
			continue
		}
		var statusColor func(format string, a ...any) string
		status := "Not installed"
		if tool.CheckStatus(t.CheckCommand) {
			status = "Installed"
			statusColor = color.GreenString
			toolPhase.AddOK(t.Name, "installed")
		} else {
			statusColor = color.YellowString
			toolPhase.AddWarn(t.Name, "not installed")
		}
		fmt.Fprintf(ctx.w, "  - Tool '%s': %s. Install hint: %s\n", t.Name, statusColor(status), t.InstallHint)
	}
	prog.Done()
}

// applyBuildsAndPackages executes builds and packages in topologically sorted
// order so that items can depend on each other via depends_on. When a specific
// build is requested via --build, only that build runs (skipping topological sort).
func applyBuildsAndPackages(ctx *applyContext, buildOpts hooks.BuildOptions, force bool) {
	if len(ctx.cfg.Hooks.Builds) == 0 && len(ctx.cfg.Packages) == 0 && buildOpts.SpecificBuild == "" {
		return
	}

	// If a specific build is requested, just run that one (no topological sort).
	if buildOpts.SpecificBuild != "" {
		applyBuilds(ctx, buildOpts)
		return
	}

	buildPhase := ctx.rpt.AddPhase("Builds")
	pkgPhase := ctx.rpt.AddPhase("Packages")

	// Group by wave and execute each wave fully before the next.
	groups := config.GroupByWave(ctx.cfg.Hooks.Builds, ctx.cfg.Packages)
	waveNums := config.SortedWaveNumbers(groups)

	totalItems := len(ctx.cfg.Hooks.Builds) + len(ctx.cfg.Packages)
	prog := progress.New("Builds & Packages", totalItems)
	if ctx.verbose || ctx.dryRun {
		prog = progress.NewQuiet()
	}

	var buildFailures []hooks.BuildResult

	for _, waveNum := range waveNums {
		group := groups[waveNum]

		order, err := config.TopologicalSort(group.Builds, group.Packages)
		if err != nil {
			phase := ctx.rpt.AddPhase(fmt.Sprintf("Wave %d", waveNum))
			phase.AddFail("dependency-sort", err.Error(), err)
			fmt.Fprintln(os.Stderr, color.RedString("Error sorting wave %d dependencies: %v", waveNum, err))
			continue
		}

		if len(order) == 0 {
			continue
		}

		if ctx.verbose && len(waveNums) > 1 {
			fmt.Fprintf(ctx.w, "  Wave %d (%d items)\n", waveNum, len(order))
		}

		for _, key := range order {
			parts := strings.SplitN(key, ".", 2)
			kind := parts[0]
			name := parts[1]

			prog.TickWith(name)

			switch kind {
			case "builds":
				build := group.Builds[name]
				if err := hooks.RunBuild(context.Background(), ctx.w, name, build, ctx.currentHost, buildOpts); err != nil {
					buildFailures = append(buildFailures, hooks.BuildResult{Name: name, Err: err})
					buildPhase.AddFail(name, err.Error(), err)
				} else {
					buildPhase.AddOK(name, "completed")
				}

			case "packages":
				pkg := group.Packages[name]
				source := pkg.Source
				if source == "" {
					source = "local"
				}

				if !config.IsEnabled(pkg.Enable) {
					fmt.Fprintf(ctx.w, "  Skipping package: %s [%s] (disabled)\n", name, source)
					pkgPhase.AddSkip(name, fmt.Sprintf("disabled [%s]", source))
					continue
				}
				if !config.ShouldApplyForHost(pkg.Hosts, ctx.currentHost) {
					fmt.Fprintf(ctx.w, "  Skipping package: %s [%s] (host filter)\n", name, source)
					pkgPhase.AddSkip(name, fmt.Sprintf("host filter [%s]", source))
					continue
				}

				resolved := packages.ResolvePackagePaths(name, pkg, ctx.cfg.PackagesDir)
				pkgOpts := packages.BuildOptions{
					DryRun:  ctx.dryRun,
					Force:   force,
					Verbose: ctx.verbose,
				}
				r := packages.BuildPackage(context.Background(), ctx.w, name, resolved, pkgOpts)

				switch r.Action {
				case "error":
					fmt.Fprintln(os.Stderr, color.RedString("  %s: %s: %v", r.Name, r.Message, r.Err))
					pkgPhase.AddFail(r.Name, r.Message, r.Err)
				case "skipped":
					pkgPhase.AddSkip(r.Name, r.Message)
				case "up-to-date":
					pkgPhase.AddOK(r.Name, r.Message)
				default:
					fmt.Fprintf(ctx.w, "  %s: %s\n", r.Name, r.Message)
					pkgPhase.AddOK(r.Name, r.Message)
					ctx.collectCaveat(pkg.OwnerRecipe)
				}
			}
		}
	}
	prog.Done()

	for _, f := range buildFailures {
		fmt.Fprintf(os.Stderr, "  build %s: %v\n", f.Name, f.Err)
	}
}

// applyBuilds executes build hooks with the given options.
func applyBuilds(ctx *applyContext, buildOpts hooks.BuildOptions) {
	if len(ctx.cfg.Hooks.Builds) == 0 && buildOpts.SpecificBuild == "" {
		return
	}
	buildPhase := ctx.rpt.AddPhase("Builds")
	if err := hooks.RunBuilds(context.Background(), ctx.w, ctx.cfg.Hooks.Builds, ctx.currentHost, buildOpts); err != nil {
		fmt.Fprintln(os.Stderr, color.RedString("Error executing builds: %v", err))
		buildPhase.AddFail("builds", err.Error(), err)
	} else {
		buildPhase.AddOK("builds", "completed")
	}
}

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().BoolVar(&overwriteExisting, "overwrite", false, "Overwrite existing files at target locations for symlinks")
	applyCmd.Flags().BoolVar(&skipExisting, "skip", false, "Skip symlinking if target file already exists")
	applyCmd.Flags().BoolVar(&forceBuilds, "force", false, "Force re-run of 'once' builds even if previously completed")
	applyCmd.Flags().StringVar(&specificBuild, "build", "", "Run only the specified build (works with 'manual' builds too)")
	applyCmd.Flags().BoolVar(&resetBuilds, "reset-builds", false, "Clear all build state before running")
	applyCmd.Flags().BoolVar(&enableCleanup, "enable-cleanup", false, "Remove orphaned artifacts owned by recipes that disappeared or are disabled (honors per-recipe delete_behavior)")
	// Note: --overwrite and --skip are mutually exclusive in behavior.
	// Cobra doesn't enforce this directly, would need custom validation or be handled by logic choosing one if both true.
	// Current logic: if overwrite is true, it takes precedence over skip.
}

// printApplyResult prints the final summary line, caveats, and report.
func printApplyResult(rpt *report.Report, ctx *applyContext, isDryRun, isVerbose bool) {
	fmt.Println("")
	if isDryRun {
		color.Cyan("DRY RUN: finished. No actual changes were made.")
		rpt.PrintSummary(os.Stdout, summaryVerbosity())
		return
	}
	ok, warn, fail, skip := rpt.TotalCounts()
	parts := []string{color.GreenString("%d ok", ok)}
	if warn > 0 {
		parts = append(parts, color.YellowString("%d warnings", warn))
	}
	if fail > 0 {
		parts = append(parts, color.RedString("%d failed", fail))
	}
	if skip > 0 {
		parts = append(parts, color.CyanString("%d skipped", skip))
	}
	fmt.Printf("Complete — %s\n", strings.Join(parts, "  "))
	if ctx != nil && len(ctx.caveats) > 0 {
		fmt.Println("")
		fmt.Println(color.YellowString("==> Caveats"))
		seen := map[string]bool{}
		for _, c := range ctx.caveats {
			if seen[c.recipe] {
				continue
			}
			seen[c.recipe] = true
			fmt.Printf("\n%s:\n", color.YellowString(c.recipe))
			for _, line := range strings.Split(strings.TrimSpace(c.text), "\n") {
				fmt.Printf("  %s\n", line)
			}
		}
		fmt.Println("")
	}
	if rpt.HasFailures() || rpt.HasWarnings() || isVerbose {
		rpt.PrintSummary(os.Stdout, summaryVerbosity())
	}
}

// toPortablePath converts an absolute path to use $HOME instead of the expanded home directory.
// This makes the path portable across different users/machines.
func toPortablePath(path string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, homeDir) {
		return "$HOME" + path[len(homeDir):]
	}
	return path
}
