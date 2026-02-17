package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/hooks"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/shell"
	"github.com/mad01/ralph/internal/tool"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the health of the ralph setup",
	Long:  `Performs a series of checks to ensure ralph is configured correctly and all managed items are in a healthy state.`,
	Run: func(cmd *cobra.Command, args []string) {
		color.Cyan("🩺 Running ralph doctor checks...")
		healthy := true
		rpt := &report.Report{Command: "doctor"}

		// 1. Verify config.toml readability and validity
		cfgPhase := rpt.AddPhase("Configuration")
		fmt.Print(color.New(color.FgWhite, color.Bold).Sprint("\nChecking configuration file... "))
		cfg, err := config.LoadConfig() // This already does validation
		if err != nil {
			color.Red("Error: %v", err)
			healthy = false
			cfgPhase.AddFail("config", fmt.Sprintf("failed to load: %v", err), err)
		} else {
			color.Green("OK")
			cfgPhase.AddOK("config", "")
		}

		if cfg == nil { // If config failed to load, cannot proceed with other checks
			fmt.Fprintln(os.Stderr, color.RedString("Cannot perform further checks due to configuration load failure."))
			rpt.PrintSummary(os.Stdout, summaryVerbosity())
			os.Exit(1)
		}

		// 2. Check for broken symlinks for managed dotfiles
		dfPhase := rpt.AddPhase("Dotfile symlinks")
		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nChecking managed dotfile symlinks:"))
		foundIssuesInSymlinks := false
		if len(cfg.Dotfiles) == 0 {
			color.Yellow("  No dotfiles configured to check.")
		} else {
			for name, df := range cfg.Dotfiles {
				templateMarker := ""
				if df.IsTemplate {
					templateMarker = color.CyanString(" (template)")
				}
				fmt.Printf("  - %s%s (Target: %s): ", color.New(color.Bold).Sprint(name), templateMarker, df.Target)
				absoluteTarget, expandErr := config.ExpandPath(df.Target)
				if expandErr != nil {
					color.Red("Error expanding target path: %v", expandErr)
					healthy = false
					foundIssuesInSymlinks = true
					dfPhase.AddFail(name, fmt.Sprintf("error expanding target path: %v", expandErr), expandErr)
					continue
				}

				targetInfo, statErr := os.Lstat(absoluteTarget)
				if os.IsNotExist(statErr) {
					color.Yellow("Not linked (target does not exist)")
					dfPhase.AddWarn(name, "not linked (target does not exist)")
				} else if statErr != nil {
					color.Red("Error checking target: %v", statErr)
					healthy = false
					foundIssuesInSymlinks = true
					dfPhase.AddFail(name, fmt.Sprintf("error checking target: %v", statErr), statErr)
				} else {
					if targetInfo.Mode()&os.ModeSymlink == 0 {
						color.Yellow("Exists but is NOT a symlink")
						foundIssuesInSymlinks = true // This is an issue if we expect a symlink
						dfPhase.AddWarn(name, "exists but is not a symlink")
					} else {
						linkDest, readlinkErr := os.Readlink(absoluteTarget)
						if readlinkErr != nil {
							color.Red("Symlink (error reading destination: %v)", readlinkErr)
							healthy = false
							foundIssuesInSymlinks = true
							dfPhase.AddFail(name, fmt.Sprintf("error reading symlink destination: %v", readlinkErr), readlinkErr)
						} else {
							var actualSourcePath string
							if df.IsTemplate {
								actualSourcePath = linkDest // For templates, linkDest is the absolute path to the processed file.
							} else {
								expandedRepoSource, _ := config.ExpandPath(filepath.Join(cfg.DotfilesRepoPath, df.Source))
								actualSourcePath = expandedRepoSource
								if linkDest != actualSourcePath {
									color.Yellow("WARN: Symlink points to '%s', but config expects '%s'. Checking existence of actual '%s'... ", linkDest, actualSourcePath, linkDest)
									actualSourcePath = linkDest // For broken check, use what it *actually* points to
								}
							}

							if _, err := os.Stat(actualSourcePath); os.IsNotExist(err) {
								color.Red("BROKEN SYMLINK (source '%s' does not exist)", actualSourcePath)
								healthy = false
								foundIssuesInSymlinks = true
								dfPhase.AddFail(name, fmt.Sprintf("broken symlink (source '%s' does not exist)", actualSourcePath), err)
							} else if err != nil {
								color.Red("Error stating symlink source '%s': %v", actualSourcePath, err)
								healthy = false
								foundIssuesInSymlinks = true
								dfPhase.AddFail(name, fmt.Sprintf("error stating source '%s': %v", actualSourcePath, err), err)
							} else {
								color.Green("OK")
								dfPhase.AddOK(name, "")
							}
						}
					}
				}
			}
			if !foundIssuesInSymlinks && len(cfg.Dotfiles) > 0 {
				color.Green("  All checked symlinks appear valid or target does not exist yet.")
			}
		}

		// Check configured directories
		dirPhase := rpt.AddPhase("Directories")
		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nChecking configured directories:"))
		if len(cfg.Directories) == 0 {
			color.Yellow("  No directories configured to check.")
		} else {
			for name, dir := range cfg.Directories {
				fmt.Printf("  - %s (Target: %s): ", color.New(color.Bold).Sprint(name), dir.Target)
				absoluteTarget, expandErr := config.ExpandPath(dir.Target)
				if expandErr != nil {
					color.Red("Error expanding target path: %v", expandErr)
					healthy = false
					dirPhase.AddFail(name, fmt.Sprintf("error expanding path: %v", expandErr), expandErr)
					continue
				}
				info, statErr := os.Stat(absoluteTarget)
				if os.IsNotExist(statErr) {
					color.Yellow("Does not exist (will be created on apply)")
					dirPhase.AddWarn(name, "does not exist")
				} else if statErr != nil {
					color.Red("Error checking: %v", statErr)
					healthy = false
					dirPhase.AddFail(name, fmt.Sprintf("error checking: %v", statErr), statErr)
				} else if !info.IsDir() {
					color.Red("Exists but is NOT a directory")
					healthy = false
					dirPhase.AddFail(name, "exists but is not a directory", nil)
				} else {
					color.Green("OK (exists)")
					dirPhase.AddOK(name, "")
				}
			}
		}

		// Check configured repositories
		repoPhase := rpt.AddPhase("Repositories")
		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nChecking configured repositories:"))
		if len(cfg.Repos) == 0 {
			color.Yellow("  No repositories configured to check.")
		} else {
			for name, rp := range cfg.Repos {
				fmt.Printf("  - %s (URL: %s): ", color.New(color.Bold).Sprint(name), rp.URL)
				absoluteTarget, expandErr := config.ExpandPath(rp.Target)
				if expandErr != nil {
					color.Red("Error expanding target path: %v", expandErr)
					healthy = false
					repoPhase.AddFail(name, fmt.Sprintf("error expanding path: %v", expandErr), expandErr)
					continue
				}
				info, statErr := os.Stat(absoluteTarget)
				if os.IsNotExist(statErr) {
					color.Yellow("Not cloned (will be cloned on apply)")
					repoPhase.AddWarn(name, "not cloned")
				} else if statErr != nil {
					color.Red("Error checking: %v", statErr)
					healthy = false
					repoPhase.AddFail(name, fmt.Sprintf("error checking: %v", statErr), statErr)
				} else if !info.IsDir() {
					color.Red("Target exists but is NOT a directory")
					healthy = false
					repoPhase.AddFail(name, "target exists but is not a directory", nil)
				} else {
					// Check if it's a git repository
					gitDir := filepath.Join(absoluteTarget, ".git")
					if _, gitErr := os.Stat(gitDir); os.IsNotExist(gitErr) {
						color.Yellow("Directory exists but is NOT a git repository")
						repoPhase.AddWarn(name, "directory exists but is not a git repository")
					} else {
						color.Green("OK (cloned)")
						repoPhase.AddOK(name, "")
					}
				}
			}
		}

		// Check configured builds
		buildPhase := rpt.AddPhase("Builds")
		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nChecking configured builds:"))
		if len(cfg.Hooks.Builds) == 0 {
			color.Yellow("  No builds configured to check.")
		} else {
			buildState, stateErr := hooks.LoadBuildState()
			if stateErr != nil {
				color.Red("  Error loading build state: %v", stateErr)
				healthy = false
				buildPhase.AddFail("build-state", fmt.Sprintf("error loading build state: %v", stateErr), stateErr)
			} else {
				for name, build := range cfg.Hooks.Builds {
					fmt.Printf("  - %s (run: %s): ", color.New(color.Bold).Sprint(name), build.Run)

					// Check working directory if specified
					if build.WorkingDir != "" {
						expandedDir, expandErr := config.ExpandPath(build.WorkingDir)
						if expandErr != nil {
							color.Red("Error expanding working_dir: %v", expandErr)
							healthy = false
							buildPhase.AddFail(name, fmt.Sprintf("error expanding working_dir: %v", expandErr), expandErr)
							continue
						}
						if _, statErr := os.Stat(expandedDir); os.IsNotExist(statErr) {
							color.Red("working_dir '%s' does not exist", expandedDir)
							healthy = false
							buildPhase.AddFail(name, fmt.Sprintf("working_dir '%s' does not exist", expandedDir), nil)
							continue
						}
					}

					// Check build state
					if record, exists := buildState.Builds[name]; exists {
						color.Green("Completed at %s", record.CompletedAt.Format("2006-01-02 15:04:05"))
						buildPhase.AddOK(name, fmt.Sprintf("completed at %s", record.CompletedAt.Format("2006-01-02 15:04:05")))
					} else {
						switch build.Run {
						case "once":
							color.Yellow("Not yet run (will run on next apply)")
							buildPhase.AddWarn(name, "not yet run")
						case "always":
							color.Cyan("Runs every apply")
							buildPhase.AddOK(name, "runs every apply")
						case "manual":
							color.Cyan("Manual (use --build=%s to run)", name)
							buildPhase.AddSkip(name, "manual")
						}
					}
				}
			}
		}

		// Add tool status checks to doctor command
		toolPhase := rpt.AddPhase("Tools")
		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nChecking configured tools:"))
		if len(cfg.Tools) == 0 {
			color.Yellow("  No tools configured to check.")
		} else {
			for _, t := range cfg.Tools {
				fmt.Printf("  - %s: ", color.New(color.Bold).Sprint(t.Name))
				if tool.CheckStatus(t.CheckCommand) {
					color.Green("Installed")
					toolPhase.AddOK(t.Name, "installed")
				} else {
					color.Yellow("Not Installed (or check failed)")
					fmt.Printf("      Install hint: %s\n", t.InstallHint)
					toolPhase.AddWarn(t.Name, "not installed")
				}
			}
		}

		// 3. Verify if rc file snippets are correctly sourced
		rcPhase := rpt.AddPhase("RC files")
		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nChecking RC file sourcing:"))
		shellsToTest := shell.ResolveShell(cfg.Shell.Name)
		foundRCIssues := false
		for _, s := range shellsToTest {
			fmt.Printf("  Shell '%s': ", color.New(color.Bold).Sprint(s))
			shellName := string(s)
			rcPath, err := shell.GetRCFilePath(s)
			if err != nil {
				color.Yellow("Could not get RC file path: %v", err)
				rcPhase.AddSkip(shellName, "could not get RC file path")
				continue
			}
			if _, err := os.Stat(rcPath); os.IsNotExist(err) {
				color.Yellow("RC file '%s' does not exist. Ralph block not present.", rcPath)
				rcPhase.AddSkip(shellName, "RC file does not exist")
				continue // Not an error for doctor if RC file itself is missing
			}
			content, err := os.ReadFile(rcPath)
			if err != nil {
				color.Red("Could not read RC file '%s': %v", rcPath, err)
				healthy = false
				foundRCIssues = true
				rcPhase.AddFail(shellName, fmt.Sprintf("could not read RC file: %v", err), err)
				continue
			}
			if strings.Contains(string(content), shell.RalphBlockBeginMarker) && strings.Contains(string(content), shell.RalphBlockEndMarker) {
				color.Green("Ralph managed block found.")
				blockStartIndex := strings.Index(string(content), shell.RalphBlockBeginMarker)
				blockEndIndex := strings.Index(string(content), shell.RalphBlockEndMarker)
				// Ensured blockStartIndex < blockEndIndex in previous implementation. It is implicitly handled by Index returning -1 if not found.
				// And the outer if checks both exist.
				blockContent := string(content)[blockStartIndex+len(shell.RalphBlockBeginMarker) : blockEndIndex]
				blockLines := strings.Split(blockContent, "\n")
				foundMissingSourceFiles := false
				sourcedFilesExpected := (len(cfg.Shell.Aliases) > 0 || len(cfg.Shell.Functions) > 0)
				sourcedFilesFoundInBlock := 0

				for _, line := range blockLines {
					trimmedLine := strings.TrimSpace(line)
					var sourcedFile string
					if strings.HasPrefix(trimmedLine, "source ") {
						sourcedFile = strings.TrimPrefix(trimmedLine, "source ")
					} else if strings.HasPrefix(trimmedLine, ". ") { // POSIX sh alternate for source
						sourcedFile = strings.TrimPrefix(trimmedLine, ". ")
					}

					if sourcedFile != "" {
						sourcedFilesFoundInBlock++
						expandedSourcedFile, expErr := config.ExpandPath(sourcedFile)
						if expErr != nil {
							color.Yellow("    -> Could not expand path for sourced file '%s': %v", sourcedFile, expErr)
							foundMissingSourceFiles = true
							healthy = false // Path expansion failure is an issue
							continue
						}
						if _, statErr := os.Stat(expandedSourcedFile); os.IsNotExist(statErr) {
							color.Red("    -> Sourced file '%s' does NOT exist.", shortenHome(expandedSourcedFile))
							foundMissingSourceFiles = true
							healthy = false
						} else if statErr != nil {
							color.Red("    -> Error checking sourced file '%s': %v", shortenHome(expandedSourcedFile), statErr)
							foundMissingSourceFiles = true
							healthy = false
						} else {
							color.Green("    -> Sourced file '%s' exists.", shortenHome(expandedSourcedFile))
						}
					}
				}
				if !foundMissingSourceFiles && sourcedFilesFoundInBlock > 0 {
					color.Green("    All detected source commands in block point to existing files.")
					rcPhase.AddOK(shellName, "")
				} else if sourcedFilesExpected && sourcedFilesFoundInBlock == 0 {
					color.Yellow("    Ralph block found, but no source commands for generated files detected, yet shell items are configured.")
					foundRCIssues = true
					rcPhase.AddWarn(shellName, "block found but no source commands detected")
				} else if !sourcedFilesExpected && sourcedFilesFoundInBlock == 0 {
					color.Green("    Ralph block found, and no shell items are configured (no source commands expected).")
					rcPhase.AddOK(shellName, "")
				}
				if foundMissingSourceFiles {
					foundRCIssues = true
					rcPhase.AddFail(shellName, "sourced file(s) missing", nil)
				}

			} else {
				color.Yellow("Ralph managed block NOT found.")
				if len(cfg.Shell.Aliases) > 0 || len(cfg.Shell.Functions) > 0 {
					color.Yellow("    Warning: Aliases/functions are configured but ralph block is missing in %s.", rcPath)
					foundRCIssues = true
					rcPhase.AddWarn(shellName, "ralph block missing but aliases/functions configured (run apply to fix)")
				} else {
					rcPhase.AddWarn(shellName, "ralph block not found")
				}
			}
		}
		if !foundRCIssues && len(shellsToTest) > 0 {
			// This message might be too broad if some shells were skipped due to no RC file
			// color.Green("  RC file checks passed for tested shells.")
		}

		fmt.Println("\n" + color.CyanString("Doctor checks complete."))
		if healthy {
			color.Green("Ralph setup appears to be healthy! ✅")
		} else {
			color.Red("Ralph setup has some issues. ❌ Please review the messages above.")
		}

		rpt.PrintSummary(os.Stdout, summaryVerbosity())
		os.Exit(rpt.ExitCode())
	},
}

func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
