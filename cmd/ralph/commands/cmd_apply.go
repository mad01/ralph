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

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/dotfile"
	"github.com/mad01/ralph/internal/hooks"
	"github.com/mad01/ralph/internal/packages"
	"github.com/mad01/ralph/internal/progress"
	"github.com/mad01/ralph/internal/repo"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/shell"
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

// skipReason reports why an item with these filter fields does not apply on
// this run's host and profiles, or "" when it does (see config.SkipReason).
func (ctx *applyContext) skipReason(enable *bool, hosts, profiles []string) string {
	return config.SkipReason(enable, hosts, profiles, ctx.currentHost, ctx.cfg.Profiles)
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
		// apply never syncs: it is `ralph up --no-sync` under its old name.
		return runApplyCommand(applyCommand{
			name:        "apply",
			header:      "Applying ralph configurations...",
			resetBuilds: resetBuilds,
			opts: applyOptions{
				overwrite:     overwriteExisting,
				skip:          skipExisting,
				force:         forceBuilds,
				build:         specificBuild,
				enableCleanup: enableCleanup,
			},
		})
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
		if reason := ctx.skipReason(dir.Enable, dir.Hosts, dir.Profiles); reason != "" {
			fmt.Fprintf(ctx.w, "  %s %s\n", color.CyanString("skip"), dim(name+" ("+reason+")"))
			dirPhase.AddSkip(name, reason)
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

		if reason := ctx.skipReason(dm.Enable, dm.Hosts, dm.Profiles); reason != "" {
			fmt.Fprintf(ctx.w, "  %s %s\n", color.CyanString("skip"), dim(name+" ("+reason+")"))
			dmPhase.AddSkip(name, reason)
			continue
		}

		fmt.Fprintf(ctx.w, "  %s\n", bold(name))

		// Resolve source directory
		absoluteSource, err := config.ExpandPath(
			config.JoinSourcePath(ctx.cfg.DotfilesRepoPath, dm.Source),
		)
		if err != nil {
			fmt.Fprintln(
				os.Stderr,
				color.RedString("    error: %s: failed to expand source path: %v", name, err),
			)
			dmPhase.AddFail(name, fmt.Sprintf("expand source: %v", err), err)
			continue
		}

		// Ensure source directory exists
		srcInfo, err := os.Stat(absoluteSource)
		if err != nil {
			fmt.Fprintln(
				os.Stderr,
				color.RedString(
					"    error: %s: source directory '%s' does not exist: %v",
					name,
					absoluteSource,
					err,
				),
			)
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
			fmt.Fprintln(
				os.Stderr,
				color.RedString("    error: %s: failed to read source directory: %v", name, err),
			)
			dmPhase.AddFail(name, fmt.Sprintf("read source dir: %v", err), err)
			continue
		}

		expandedTarget, err := config.ExpandPath(dm.Target)
		if err != nil {
			fmt.Fprintln(
				os.Stderr,
				color.RedString("    error: %s: failed to expand target path: %v", name, err),
			)
			dmPhase.AddFail(name, fmt.Sprintf("expand target: %v", err), err)
			continue
		}

		// Ensure target directory exists
		if !ctx.dryRun {
			if err := os.MkdirAll(expandedTarget, 0o755); err != nil {
				fmt.Fprintln(
					os.Stderr,
					color.RedString(
						"    error: %s: failed to create target directory: %v",
						name,
						err,
					),
				)
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
				symlinkErr = dotfile.CreateDirSymlink(
					ctx.w,
					df,
					ctx.cfg.DotfilesRepoPath,
					symlinkAction,
					ctx.dryRun,
				)
			default: // "symlink"
				symlinkErr = dotfile.CreateSymlink(
					ctx.w,
					df,
					ctx.cfg.DotfilesRepoPath,
					symlinkAction,
					ctx.dryRun,
				)
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
			dmPhase.AddFail(
				name,
				fmt.Sprintf("%d linked, %d failed", linked, failed),
				fmt.Errorf("%d entries failed", failed),
			)
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
	if err := repo.ProcessRepos(ctx.w, ctx.cfg.Repos, ctx.currentHost, ctx.cfg.Profiles, ctx.dryRun); err != nil {
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
		if reason := ctx.skipReason(df.Enable, df.Hosts, df.Profiles); reason != "" {
			fmt.Fprintf(ctx.w, "  %s %s\n", color.CyanString("skip"), dim(name+" ("+reason+")"))
			dfPhase.AddSkip(name, reason)
			continue
		}
		fmt.Fprintf(ctx.w, "  %s\n", bold(name))
		fmt.Fprintf(ctx.w, "    %s → %s\n", dim(df.Target), dim(df.Source))

		// Execute pre-link hooks for this specific dotfile
		if preHooks, exists := ctx.cfg.Hooks.PreLink[name]; exists && len(preHooks) > 0 {
			linkContext := &hooks.HookContext{
				DotfileName: name,
				SourcePath:  config.JoinSourcePath(ctx.cfg.DotfilesRepoPath, df.Source),
				TargetPath:  df.Target,
				DryRun:      ctx.dryRun,
			}
			if err := hooks.RunHooks(ctx.w, preHooks, hooks.PreLink, linkContext, ctx.dryRun); err != nil {
				fmt.Fprintln(
					os.Stderr,
					color.RedString("Error executing pre-link hooks for %s: %v", name, err),
				)
				dotfilesFailed++
				dfPhase.AddFail(name, fmt.Sprintf("pre-link hook: %v", err), err)
				continue
			}
		}

		templateData := make(map[string]any)

		var symlinkErr error
		currentSourcePath := config.JoinSourcePath(ctx.cfg.DotfilesRepoPath, df.Source)
		dotfileToSymlink := df
		repoPathForSymlink := ctx.cfg.DotfilesRepoPath

		if df.IsTemplate {
			fmt.Fprintf(ctx.w, "    %s\n", dim("template: "+df.Source))
			// WriteProcessedTemplateToFile returns a real path in both modes (a
			// placeholder under os.TempDir() on dry-run), so no sentinel is
			// needed.
			processedPath, templateErr := dotfile.WriteProcessedTemplateToFile(
				ctx.w,
				currentSourcePath,
				ctx.cfg,
				templateData,
				ctx.dryRun,
			)

			if templateErr != nil {
				fmt.Fprintln(
					os.Stderr,
					color.YellowString(
						"    - Warning: Error processing template for %s: %v",
						name,
						templateErr,
					),
				)
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
			symlinkErr = dotfile.CopyFile(
				ctx.w,
				dotfileToSymlink,
				repoPathForSymlink,
				symlinkAction,
				ctx.dryRun,
			)
		case "symlink_dir":
			symlinkErr = dotfile.CreateDirSymlink(
				ctx.w,
				dotfileToSymlink,
				repoPathForSymlink,
				symlinkAction,
				ctx.dryRun,
			)
		default:
			// Default to regular symlink
			symlinkErr = dotfile.CreateSymlink(
				ctx.w,
				dotfileToSymlink,
				repoPathForSymlink,
				symlinkAction,
				ctx.dryRun,
			)
		}

		// Cleanup for templated files: the processed template was copied to the
		// target, so remove the temp file. Dry-run wrote nothing.
		if df.IsTemplate && repoPathForSymlink == "" && !ctx.dryRun {
			if strings.HasPrefix(dotfileToSymlink.Source, os.TempDir()) ||
				strings.Contains(dotfileToSymlink.Source, "ralph-temp-") {
				if removeErr := os.Remove(dotfileToSymlink.Source); removeErr != nil {
					fmt.Fprintln(
						os.Stderr,
						color.YellowString(
							"    - Warning: failed to remove temporary processed file %s: %v",
							dotfileToSymlink.Source,
							removeErr,
						),
					)
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
					SourcePath:  config.JoinSourcePath(ctx.cfg.DotfilesRepoPath, df.Source),
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
		fmt.Fprintf(
			ctx.w,
			"  Dotfiles (dry run): would apply %s, %s skipped, %s failed.\n",
			color.GreenString(
				"%d",
				dotfilesApplied,
			),
			color.CyanString("%d", dotfilesSkipped),
			color.YellowString("%d", dotfilesFailed),
		)
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
		fmt.Fprintln(
			os.Stderr,
			color.YellowString("Could not determine current shell. Skipping shell configuration."),
		)
		shellPhase.AddSkip("shell", "could not determine shell")
		return
	}

	fmt.Fprintf(ctx.w, "  Detected shell: %s\n", currentShell)
	aliasFile, funcFile, genErr := shell.GenerateShellConfigs(
		ctx.w,
		ctx.cfg,
		currentShell,
		ctx.dryRun,
	)

	if genErr != nil {
		fmt.Fprintln(
			os.Stderr,
			color.RedString("  Error generating shell configs for %s: %v", currentShell, genErr),
		)
		shellPhase.AddFail(
			string(currentShell),
			fmt.Sprintf("generate configs: %v", genErr),
			genErr,
		)
		return
	}

	// Generate env file -- sourced before aliases and functions so they can reference env vars.
	envFilePath, envPathErr := shell.GetEnvFilePath()
	if envPathErr != nil {
		fmt.Fprintln(os.Stderr, color.RedString("  Error getting env file path: %v", envPathErr))
		shellPhase.AddFail(
			string(currentShell),
			fmt.Sprintf("env file path: %v", envPathErr),
			envPathErr,
		)
		return
	}
	if envErr := shell.GenerateEnvFile(ctx.w, ctx.cfg.Shell.Env, envFilePath, ctx.dryRun); envErr != nil {
		fmt.Fprintln(os.Stderr, color.RedString("  Error generating env file: %v", envErr))
		shellPhase.AddFail(
			string(currentShell),
			fmt.Sprintf("generate env file: %v", envErr),
			envErr,
		)
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
		fmt.Fprintln(
			ctx.w,
			"  No shell aliases, functions, or env vars configured to source.",
		)
		shellPhase.AddOK(string(currentShell), "nothing to source")
		return
	}

	fmt.Fprintf(ctx.w, "  Injecting source lines into %s rc file...\n", currentShell)
	if err := shell.InjectSourceLines(ctx.w, currentShell, linesToSource, ctx.dryRun); err != nil {
		fmt.Fprintln(
			os.Stderr,
			color.RedString(
				"  Error injecting source lines into %s rc file: %v",
				currentShell,
				err,
			),
		)
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

	fmt.Fprintln(
		ctx.w,
		"\nChecking tool configurations (installation not performed by apply):",
	)
	for _, t := range ctx.cfg.Tools {
		prog.TickWith(t.Name)
		if reason := ctx.skipReason(t.Enable, t.Hosts, t.Profiles); reason != "" {
			fmt.Fprintf(ctx.w, "  Skipping tool: %s (%s)\n", t.Name, reason)
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
			ctx.w,
			"  - Tool '%s': %s. Install hint: %s\n",
			t.Name,
			statusColor(status),
			t.InstallHint,
		)
	}
	prog.Done()
}

// applyBuildsAndPackages executes builds and packages in topologically sorted
// order so that items can depend on each other via depends_on. When a specific
// build is requested via --build, only that build runs (skipping topological sort).
func applyBuildsAndPackages(ctx *applyContext, buildOpts hooks.BuildOptions, force bool) {
	if len(ctx.cfg.Hooks.Builds) == 0 && len(ctx.cfg.Packages) == 0 &&
		buildOpts.SpecificBuild == "" {
		return
	}

	// If a specific build is requested, just run that one (no topological sort).
	if buildOpts.SpecificBuild != "" {
		applyBuilds(ctx, buildOpts)
		return
	}

	r := &nodeRunner{
		ctx:        ctx,
		buildOpts:  buildOpts,
		force:      force,
		buildPhase: ctx.rpt.AddPhase("Builds"),
		pkgPhase:   ctx.rpt.AddPhase("Packages"),
		failed:     map[string]bool{},
	}

	// Group by wave and execute each wave fully before the next.
	groups := config.GroupByWave(ctx.cfg.Hooks.Builds, ctx.cfg.Packages)
	waveNums := config.SortedWaveNumbers(groups)

	totalItems := len(ctx.cfg.Hooks.Builds) + len(ctx.cfg.Packages)
	prog := progress.New("Builds & Packages", totalItems)
	if ctx.verbose || ctx.dryRun {
		prog = progress.NewQuiet()
	}

	// Cross-wave depends_on edges (a dependency in a LATER wave) are not
	// enforced by the per-wave topological sort — warn so a silently-unordered
	// dependency is visible.
	for _, warning := range config.CrossWaveDependencyWarnings(ctx.cfg.Hooks.Builds, ctx.cfg.Packages) {
		fmt.Fprintln(os.Stderr, color.YellowString("Warning: %s", warning))
	}

	for _, waveNum := range waveNums {
		r.runWave(waveNum, groups[waveNum], len(waveNums) > 1, prog)
	}
	prog.Done()

	for _, f := range r.buildFailures {
		fmt.Fprintf(os.Stderr, "  build %s: %v\n", f.Name, f.Err)
	}
}

// nodeRunner carries the state one builds+packages phase shares across waves.
type nodeRunner struct {
	ctx        *applyContext
	buildOpts  hooks.BuildOptions
	force      bool
	buildPhase *report.Phase
	pkgPhase   *report.Phase
	// failed tracks node keys ("builds.x"/"packages.x") that failed or were
	// skipped because a dependency failed, so dependents in this and later
	// waves skip instead of building against a missing artifact.
	failed        map[string]bool
	buildFailures []hooks.BuildResult
}

// runWave runs one wave's builds and packages in dependency order.
func (r *nodeRunner) runWave(
	waveNum int,
	group *config.WaveGroup,
	multiWave bool,
	prog *progress.Counter,
) {
	order, err := config.TopologicalSort(group.Builds, group.Packages)
	if err != nil {
		phase := r.ctx.rpt.AddPhase(fmt.Sprintf("Wave %d", waveNum))
		phase.AddFail("dependency-sort", err.Error(), err)
		fmt.Fprintln(
			os.Stderr,
			color.RedString("Error sorting wave %d dependencies: %v", waveNum, err),
		)
		return
	}
	if len(order) == 0 {
		return
	}
	if r.ctx.verbose && multiWave {
		fmt.Fprintf(r.ctx.w, "  Wave %d (%d items)\n", waveNum, len(order))
	}
	for _, key := range order {
		kind, name, _ := strings.Cut(key, ".")
		prog.TickWith(name)
		switch kind {
		case "builds":
			r.runBuild(key, name, group.Builds[name])
		case "packages":
			r.runPackage(key, name, group.Packages[name])
		}
	}
}

// runBuild runs one build unless a filter or a failed dependency skips it.
// Disabled and filtered builds are reported as skips, not "completed".
func (r *nodeRunner) runBuild(key, name string, build config.Build) {
	if reason := r.ctx.skipReason(build.Enable, build.Hosts, build.Profiles); reason != "" {
		fmt.Fprintf(r.ctx.w, "  Skipping build: %s (%s)\n", name, reason)
		r.buildPhase.AddSkip(name, reason)
		return
	}
	if dep := firstFailedDependency(build.DependsOn, r.failed); dep != "" {
		fmt.Fprintf(r.ctx.w, "  Skipping build: %s (dependency %s failed)\n", name, dep)
		r.buildPhase.AddSkip(name, fmt.Sprintf("dependency %s failed", dep))
		r.failed[key] = true // cascade to this build's own dependents
		return
	}
	ctx := r.ctx
	err := hooks.RunBuild(
		context.Background(), ctx.w, name, build, ctx.currentHost, ctx.cfg.Profiles, r.buildOpts,
	)
	if err != nil {
		r.buildFailures = append(r.buildFailures, hooks.BuildResult{Name: name, Err: err})
		r.buildPhase.AddFail(name, err.Error(), err)
		r.failed[key] = true
		return
	}
	r.buildPhase.AddOK(name, "completed")
}

// runPackage builds one package unless a filter or a failed dependency skips it.
func (r *nodeRunner) runPackage(key, name string, pkg config.Package) {
	source := pkg.Source
	if source == "" {
		source = "local"
	}
	if reason := r.ctx.skipReason(pkg.Enable, pkg.Hosts, pkg.Profiles); reason != "" {
		fmt.Fprintf(r.ctx.w, "  Skipping package: %s [%s] (%s)\n", name, source, reason)
		r.pkgPhase.AddSkip(name, fmt.Sprintf("%s [%s]", reason, source))
		return
	}
	if dep := firstFailedDependency(pkg.DependsOn, r.failed); dep != "" {
		fmt.Fprintf(
			r.ctx.w,
			"  Skipping package: %s [%s] (dependency %s failed)\n",
			name,
			source,
			dep,
		)
		r.pkgPhase.AddSkip(name, fmt.Sprintf("dependency %s failed", dep))
		r.failed[key] = true // cascade to this package's own dependents
		return
	}
	resolved := packages.ResolvePackagePaths(name, pkg, r.ctx.cfg.PackagesDir)
	opts := packages.BuildOptions{DryRun: r.ctx.dryRun, Force: r.force, Verbose: r.ctx.verbose}
	r.recordPackage(
		key,
		pkg,
		packages.BuildPackage(context.Background(), r.ctx.w, name, resolved, opts),
	)
}

// recordPackage reports one package build result and marks a failure so the
// package's dependents skip.
func (r *nodeRunner) recordPackage(key string, pkg config.Package, res packages.BuildResult) {
	switch res.Action {
	case "error":
		fmt.Fprintln(os.Stderr, color.RedString("  %s: %s: %v", res.Name, res.Message, res.Err))
		r.pkgPhase.AddFail(res.Name, res.Message, res.Err)
		r.failed[key] = true
	case "skipped":
		r.pkgPhase.AddSkip(res.Name, res.Message)
	case "up-to-date":
		r.pkgPhase.AddOK(res.Name, res.Message)
	default:
		fmt.Fprintf(r.ctx.w, "  %s: %s\n", res.Name, res.Message)
		r.pkgPhase.AddOK(res.Name, res.Message)
		r.ctx.collectCaveat(pkg.OwnerRecipe)
	}
}

// firstFailedDependency returns the first depends_on entry that is in the failed
// set (a build/package that errored or was itself skipped for a failed dep), or
// "" if none. Used to skip a node whose dependency didn't succeed rather than
// build it against a missing or stale artifact.
func firstFailedDependency(deps []string, failed map[string]bool) string {
	for _, dep := range deps {
		if failed[dep] {
			return dep
		}
	}
	return ""
}

// applyBuilds executes build hooks with the given options.
func applyBuilds(ctx *applyContext, buildOpts hooks.BuildOptions) {
	if len(ctx.cfg.Hooks.Builds) == 0 && buildOpts.SpecificBuild == "" {
		return
	}
	buildPhase := ctx.rpt.AddPhase("Builds")
	if err := hooks.RunBuilds(context.Background(), ctx.w, ctx.cfg.Hooks.Builds, ctx.currentHost, ctx.cfg.Profiles, buildOpts); err != nil {
		fmt.Fprintln(os.Stderr, color.RedString("Error executing builds: %v", err))
		buildPhase.AddFail("builds", err.Error(), err)
	} else {
		buildPhase.AddOK("builds", "completed")
	}
}

func init() {
	rootCmd.AddCommand(applyCmd)
	supportJSONOutput(applyCmd)
	applyCmd.Flags().
		BoolVar(&overwriteExisting, "overwrite", false, "Overwrite existing files at target locations for symlinks")
	applyCmd.Flags().
		BoolVar(&skipExisting, "skip", false, "Skip symlinking if target file already exists")
	applyCmd.Flags().
		BoolVar(&forceBuilds, "force", false, "Force re-run of 'once' builds even if previously completed")
	applyCmd.Flags().
		StringVar(&specificBuild, "build", "", "Run only the specified build (works with 'manual' builds too)")
	applyCmd.Flags().
		BoolVar(&resetBuilds, "reset-builds", false, "Clear all build state before running")
	applyCmd.Flags().
		BoolVar(&enableCleanup, "enable-cleanup", false, "Remove orphaned artifacts owned by recipes that disappeared or are disabled (honors per-recipe delete_behavior)")

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
