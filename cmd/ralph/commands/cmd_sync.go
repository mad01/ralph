package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/packages"
	"github.com/mad01/ralph/internal/report"
	"github.com/spf13/cobra"
)

var (
	syncSpecificPackage string
	syncNoPull          bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync dotfiles repo and remote packages",
	Long:  `Pulls latest changes for the dotfiles repository and clones/pulls remote packages. Does not build or install packages — run 'ralph apply' after syncing to build.`,
	Run: func(cmd *cobra.Command, args []string) {
		var w = io.Writer(io.Discard)
		if verbose {
			w = os.Stdout
		}

		fmt.Println("Syncing dotfiles repo and remote packages...")

		if dryRun {
			color.Cyan("\n*** DRY RUN MODE ENABLED ***")
			color.Cyan("No actual changes will be made.")
			color.Cyan("****************************\n")
		}

		rpt := &report.Report{Command: "sync"}

		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error loading configuration: %v", err))
			cfgPhase := rpt.AddPhase("Configuration")
			cfgPhase.AddFail("config", "failed to load", err)
			rpt.PrintSummary(os.Stdout, summaryVerbosity())
			os.Exit(1)
		}

		currentHost := config.GetCurrentHost()

		// Pull dotfiles repo before processing packages
		pullPhase := rpt.AddPhase("Dotfiles repo")
		if syncNoPull {
			fmt.Fprintf(w, "  Skipping dotfiles repo pull (--no-pull)\n")
			pullPhase.AddSkip("dotfiles-repo", "skipped (--no-pull)")
		} else {
			expandedRepoPath, err := config.ExpandPath(cfg.DotfilesRepoPath)
			if err != nil {
				fmt.Fprintln(os.Stderr, color.RedString("Error expanding dotfiles_repo_path: %v", err))
				pullPhase.AddFail("dotfiles-repo", "failed to expand path", err)
			} else {
				fmt.Fprintf(w, "  Pulling dotfiles repo: %s\n", expandedRepoPath)
				if err := packages.GitPull(w, expandedRepoPath, dryRun); err != nil {
					fmt.Fprintln(os.Stderr, color.RedString("Error pulling dotfiles repo: %v", err))
					pullPhase.AddFail("dotfiles-repo", "pull failed", err)
				} else {
					if dryRun {
						pullPhase.AddOK("dotfiles-repo", "[DRY RUN] would pull")
					} else {
						pullPhase.AddOK("dotfiles-repo", "pulled")
					}
				}
			}
		}

		remotePhase := rpt.AddPhase("Packages (remote)")
		localPhase := rpt.AddPhase("Packages (local)")

		if len(cfg.Packages) == 0 {
			fmt.Println("No packages configured.")
			remotePhase.AddOK("packages", "none configured")
			rpt.PrintSummary(os.Stdout, summaryVerbosity())
			return
		}

		if syncSpecificPackage != "" {
			if _, exists := cfg.Packages[syncSpecificPackage]; !exists {
				fmt.Fprintln(os.Stderr, color.RedString("Package '%s' not found in configuration", syncSpecificPackage))
				remotePhase.AddFail(syncSpecificPackage, "not found in configuration", nil)
				rpt.PrintSummary(os.Stdout, summaryVerbosity())
				os.Exit(1)
			}
		}

		opts := packages.SyncOptions{
			DryRun:          dryRun,
			SpecificPackage: syncSpecificPackage,
			Verbose:         verbose,
		}

		results := packages.SyncPackages(w, cfg.Packages, cfg.PackagesDir, currentHost, opts)

		// Report results, routing to the correct phase by source type.
		for _, r := range results {
			source := cfg.Packages[r.Name].Source
			if source == "" {
				source = "local"
			}
			phase := remotePhase
			if source == "local" {
				phase = localPhase
			}

			switch r.Action {
			case "error":
				fmt.Fprintln(os.Stderr, color.RedString("  %s: %s: %v", r.Name, r.Message, r.Err))
				phase.AddFail(r.Name, r.Message, r.Err)
			case "skipped":
				phase.AddSkip(r.Name, r.Message)
			default:
				fmt.Fprintf(w, "  %s: %s\n", r.Name, r.Message)
				phase.AddOK(r.Name, r.Message)
			}
		}

		fmt.Println("")
		if dryRun {
			color.Cyan("DRY RUN: Sync finished. No actual changes were made.")
		} else {
			color.Green("Sync complete. Run 'ralph apply' to build packages.")
		}

		rpt.PrintSummary(os.Stdout, summaryVerbosity())
		os.Exit(rpt.ExitCode())
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().StringVar(&syncSpecificPackage, "package", "", "Sync only the specified package")
	syncCmd.Flags().BoolVar(&syncNoPull, "no-pull", false, "Skip pulling the dotfiles repo before syncing packages")
}
