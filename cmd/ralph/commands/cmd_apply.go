package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/dotfile"
	"github.com/mad01/ralph/internal/hooks"
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
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply ralph configurations",
	Long:  `Applies the configurations defined in your ralph config file. This includes symlinking dotfiles, setting up shell environments, etc.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Per-item output: visible only with --verbose, otherwise discarded
		var w io.Writer = io.Discard
		if verbose {
			w = os.Stdout
		}

		// Auto-migrate from legacy dotter config
		if err := config.MigrateFromLegacy(); err != nil {
			fmt.Fprintln(os.Stderr, color.YellowString("Warning: legacy migration failed: %v", err))
		}

		fmt.Println("Applying ralph configurations...")

		if dryRun {
			color.Cyan("\n*** DRY RUN MODE ENABLED ***")
			color.Cyan("No actual changes will be made.")
			color.Cyan("****************************\n")
		}

		rpt := &report.Report{Command: "apply"}
		bold := color.New(color.Bold).SprintFunc()
		dim := color.New(color.Faint).SprintFunc()

		// Handle --reset-builds flag
		if resetBuilds {
			if dryRun {
				fmt.Fprintln(w, "[DRY RUN] Would reset all build state.")
			} else {
				if err := hooks.ResetBuildState(); err != nil {
					fmt.Fprintln(os.Stderr, color.RedString("Error resetting build state: %v", err))
					os.Exit(1)
				}
			}
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error loading configuration: %v", err))
			cfgPhase := rpt.AddPhase("Configuration")
			cfgPhase.AddFail("config", "failed to load", err)
			rpt.PrintSummary(os.Stdout, summaryVerbosity())
			os.Exit(1)
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
				os.Exit(1)
			}
			prePhase.AddOK("pre-apply", "completed")
		}

		// Process directories
		dirPhase := rpt.AddPhase("Directories")
		if len(cfg.Directories) > 0 {
			fmt.Fprintln(w, "\nProcessing directories...")
			for name, dir := range cfg.Directories {
				if !config.IsEnabled(dir.Enable) {
					fmt.Fprintf(w, "  %s %s\n", color.CyanString("skip"), dim(name+" (disabled)"))
					dirPhase.AddSkip(name, "disabled")
					continue
				}
				if !config.ShouldApplyForHost(dir.Hosts, currentHost) {
					fmt.Fprintf(w, "  %s %s\n", color.CyanString("skip"), dim(name+" (host filter)"))
					dirPhase.AddSkip(name, "host filter")
					continue
				}
				fmt.Fprintf(w, "  %s\n", bold(name))
				fmt.Fprintf(w, "    %s\n", dim(dir.Target))
				if err := dotfile.CreateDirectory(w, dir, dryRun); err != nil {
					fmt.Fprintln(os.Stderr, color.RedString("    error: %s: %v", name, err))
					dirPhase.AddFail(name, err.Error(), err)
				} else {
					dirPhase.AddOK(name, "")
				}
			}
		}

		// Process repositories
		if len(cfg.Repos) > 0 {
			repoPhase := rpt.AddPhase("Repositories")
			if err := repo.ProcessRepos(w, cfg.Repos, currentHost, dryRun); err != nil {
				fmt.Fprintln(os.Stderr, color.RedString("Error processing repositories: %v", err))
				repoPhase.AddFail("repos", err.Error(), err)
			} else {
				repoPhase.AddOK("repos", "processed")
			}
		}

		fmt.Fprintln(w, "\nProcessing dotfiles...")
		dotfilesApplied := 0
		dotfilesSkippedOrFailed := 0
		dfPhase := rpt.AddPhase("Dotfiles")

		for name, df := range cfg.Dotfiles {
			if !config.IsEnabled(df.Enable) {
				fmt.Fprintf(w, "  %s %s\n", color.CyanString("skip"), dim(name+" (disabled)"))
				dfPhase.AddSkip(name, "disabled")
				continue
			}
			if !config.ShouldApplyForHost(df.Hosts, currentHost) {
				fmt.Fprintf(w, "  %s %s\n", color.CyanString("skip"), dim(name+" (host filter)"))
				dfPhase.AddSkip(name, "host filter")
				continue
			}
			fmt.Fprintf(w, "  %s\n", bold(name))
			fmt.Fprintf(w, "    %s → %s\n", dim(df.Target), dim(df.Source))

			// Execute pre-link hooks for this specific dotfile
			if preHooks, exists := cfg.Hooks.PreLink[name]; exists && len(preHooks) > 0 {
				linkContext := &hooks.HookContext{
					DotfileName: name,
					SourcePath:  filepath.Join(cfg.DotfilesRepoPath, df.Source),
					TargetPath:  df.Target,
					DryRun:      dryRun,
				}
				if err := hooks.RunHooks(w, preHooks, hooks.PreLink, linkContext, dryRun); err != nil {
					fmt.Fprintln(os.Stderr, color.RedString("Error executing pre-link hooks for %s: %v", name, err))
					dotfilesSkippedOrFailed++
					dfPhase.AddFail(name, fmt.Sprintf("pre-link hook: %v", err), err)
					continue
				}
			}

			templateData := make(map[string]interface{})

			var symlinkErr error
			currentSourcePath := filepath.Join(cfg.DotfilesRepoPath, df.Source)
			dotfileToSymlink := df
			repoPathForSymlink := cfg.DotfilesRepoPath

			if df.IsTemplate {
				fmt.Fprintf(w, "    %s\n", dim("template: "+df.Source))
				var processedPath string
				var templateErr error
				if dryRun {
					processedPath, templateErr = dotfile.WriteProcessedTemplateToFile(w, currentSourcePath, cfg, templateData, true)
					if templateErr == nil && processedPath == "" { // dry run specific path
						processedPath = "/tmp/fake_processed_template_for_dry_run" // ensure it has a value for dry run
					}
				} else {
					processedPath, templateErr = dotfile.WriteProcessedTemplateToFile(w, currentSourcePath, cfg, templateData, false)
				}

				if templateErr != nil {
					fmt.Fprintln(os.Stderr, color.YellowString("    - Warning: Error processing template for %s: %v", name, templateErr))
					dotfilesSkippedOrFailed++
					dfPhase.AddWarn(name, fmt.Sprintf("template error: %v", templateErr))
					continue
				}
				dotfileToSymlink.Source = processedPath
				repoPathForSymlink = "" // Processed template is an absolute path
			}

			// Determine action based on action field
			switch df.Action {
			case "copy":
				symlinkErr = dotfile.CopyFile(w, dotfileToSymlink, repoPathForSymlink, symlinkAction, dryRun)
			case "symlink_dir":
				symlinkErr = dotfile.CreateDirSymlink(w, dotfileToSymlink, repoPathForSymlink, symlinkAction, dryRun)
			default:
				// Default to regular symlink
				symlinkErr = dotfile.CreateSymlink(w, dotfileToSymlink, repoPathForSymlink, symlinkAction, dryRun)
			}

			// Cleanup for templated files
			if df.IsTemplate && repoPathForSymlink == "" && !dryRun && dotfileToSymlink.Source != "/tmp/fake_processed_template_for_dry_run" {
				// Check if the source is in a temp-like directory before removing
				// This is a basic check; for more robust checks, consider if WriteProcessedTemplateToFile returns if it's a temp file.
				if strings.HasPrefix(dotfileToSymlink.Source, os.TempDir()) || strings.Contains(dotfileToSymlink.Source, "ralph-temp-") {
					if removeErr := os.Remove(dotfileToSymlink.Source); removeErr != nil {
						fmt.Fprintln(os.Stderr, color.YellowString("    - Warning: failed to remove temporary processed file %s: %v", dotfileToSymlink.Source, removeErr))
					}
				}
			}

			if symlinkErr != nil {
				fmt.Fprintln(os.Stderr, color.RedString("    error: %s: %v", name, symlinkErr))
				dotfilesSkippedOrFailed++
				dfPhase.AddFail(name, symlinkErr.Error(), symlinkErr)
			} else {
				if !dryRun { // only count as applied if not dry run
					dotfilesApplied++
				}

				// Execute post-link hooks for this specific dotfile if symlink was created successfully
				postHookFailed := false
				if postHooks, exists := cfg.Hooks.PostLink[name]; exists && len(postHooks) > 0 {
					linkContext := &hooks.HookContext{
						DotfileName: name,
						SourcePath:  filepath.Join(cfg.DotfilesRepoPath, df.Source),
						TargetPath:  df.Target,
						DryRun:      dryRun,
					}
					if err := hooks.RunHooks(w, postHooks, hooks.PostLink, linkContext, dryRun); err != nil {
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
		if dryRun {
			fmt.Fprintln(w, "  Dotfiles processing (dry run): Inspect messages above for intended actions.")
		} else {
			fmt.Fprintf(w, "  Dotfiles processed: %s applied, %s skipped/failed.\n", color.GreenString("%d", dotfilesApplied), color.YellowString("%d", dotfilesSkippedOrFailed))
		}

		fmt.Fprintln(w, "\nProcessing shell configurations...")
		shellPhase := rpt.AddPhase("Shell config")
		resolvedShells := shell.ResolveShell(cfg.Shell.Name)
		currentShell := resolvedShells[0]
		if len(resolvedShells) > 1 {
			// Fallback to all shells means we couldn't determine a single shell
			fmt.Fprintln(os.Stderr, color.YellowString("Could not determine current shell. Skipping shell configuration."))
			shellPhase.AddSkip("shell", "could not determine shell")
		} else {
			fmt.Fprintf(w, "  Detected shell: %s\n", currentShell)
			var aliasFile, funcFile string
			var genErr error

			aliasFile, funcFile, genErr = shell.GenerateShellConfigs(w, cfg, currentShell, dryRun)

			if genErr != nil {
				fmt.Fprintln(os.Stderr, color.RedString("  Error generating shell configs for %s: %v", currentShell, genErr))
				shellPhase.AddFail(string(currentShell), fmt.Sprintf("generate configs: %v", genErr), genErr)
			} else {
				linesToSource := []string{}
				if aliasFile != "" && (len(cfg.Shell.Aliases) > 0 || (dryRun && aliasFile != "")) {
					linesToSource = append(linesToSource, fmt.Sprintf("source %s", toPortablePath(aliasFile)))
				}
				if funcFile != "" && (len(cfg.Shell.Functions) > 0 || (dryRun && funcFile != "")) {
					linesToSource = append(linesToSource, fmt.Sprintf("source %s", toPortablePath(funcFile)))
				}

				if len(linesToSource) > 0 {
					fmt.Fprintf(w, "  Injecting source lines into %s rc file...\n", currentShell)
					if err := shell.InjectSourceLines(w, currentShell, linesToSource, dryRun); err != nil {
						fmt.Fprintln(os.Stderr, color.RedString("  Error injecting source lines into %s rc file: %v", currentShell, err))
						shellPhase.AddFail(string(currentShell), fmt.Sprintf("inject source lines: %v", err), err)
					} else {
						shellPhase.AddOK(string(currentShell), "")
					}
				} else {
					fmt.Fprintln(w, "  No shell aliases or functions configured to source.")
					shellPhase.AddOK(string(currentShell), "no aliases/functions to source")
				}
			}
		}

		// Tool management in apply (TODO based on config)
		toolPhase := rpt.AddPhase("Tools")
		if len(cfg.Tools) > 0 {
			fmt.Fprintln(w, "\nChecking tool configurations (installation not performed by apply):")
			for _, t := range cfg.Tools {
				if !config.IsEnabled(t.Enable) {
					fmt.Fprintf(w, "  Skipping tool: %s (disabled)\n", t.Name)
					toolPhase.AddSkip(t.Name, "disabled")
					continue
				}
				if !config.ShouldApplyForHost(t.Hosts, currentHost) {
					fmt.Fprintf(w, "  Skipping tool: %s (host filter)\n", t.Name)
					toolPhase.AddSkip(t.Name, "host filter")
					continue
				}
				var statusColor func(format string, a ...interface{}) string
				status := "Not installed"
				if tool.CheckStatus(t.CheckCommand) {
					status = "Installed"
					statusColor = color.GreenString
					toolPhase.AddOK(t.Name, "installed")
				} else {
					statusColor = color.YellowString
					toolPhase.AddWarn(t.Name, "not installed")
				}
				fmt.Fprintf(w, "  - Tool '%s': %s. Install hint: %s\n", t.Name, statusColor(status), t.InstallHint)
			}
		}

		// Execute build hooks
		if len(cfg.Hooks.Builds) > 0 || specificBuild != "" {
			buildPhase := rpt.AddPhase("Builds")
			buildOpts := hooks.BuildOptions{
				DryRun:        dryRun,
				Force:         forceBuilds,
				SpecificBuild: specificBuild,
			}
			if err := hooks.RunBuilds(w, cfg.Hooks.Builds, currentHost, buildOpts); err != nil {
				fmt.Fprintln(os.Stderr, color.RedString("Error executing builds: %v", err))
				buildPhase.AddFail("builds", err.Error(), err)
			} else {
				buildPhase.AddOK("builds", "completed")
			}
		}

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

		fmt.Println("") // Add a newline for spacing
		if dryRun {
			color.Cyan("DRY RUN: Ralph apply finished. No actual changes were made.")
		} else {
			color.Green("Ralph apply complete.")
		}

		rpt.PrintSummary(os.Stdout, summaryVerbosity())
		os.Exit(rpt.ExitCode())
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().BoolVar(&overwriteExisting, "overwrite", false, "Overwrite existing files at target locations for symlinks")
	applyCmd.Flags().BoolVar(&skipExisting, "skip", false, "Skip symlinking if target file already exists")
	applyCmd.Flags().BoolVar(&forceBuilds, "force", false, "Force re-run of 'once' builds even if previously completed")
	applyCmd.Flags().StringVar(&specificBuild, "build", "", "Run only the specified build (works with 'manual' builds too)")
	applyCmd.Flags().BoolVar(&resetBuilds, "reset-builds", false, "Clear all build state before running")
	// Note: --overwrite and --skip are mutually exclusive in behavior.
	// Cobra doesn't enforce this directly, would need custom validation or be handled by logic choosing one if both true.
	// Current logic: if overwrite is true, it takes precedence over skip.
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
