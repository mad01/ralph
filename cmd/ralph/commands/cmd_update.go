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
	updateForce           bool
	updateSpecificPackage string
	updateNoPull          bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update and rebuild managed packages",
	Long:  `Pulls latest changes for remote packages, detects changes for local packages, and rebuilds/installs as needed.`,
	Run: func(cmd *cobra.Command, args []string) {
		var w = io.Writer(io.Discard)
		if verbose {
			w = os.Stdout
		}

		fmt.Println("Updating managed packages...")

		if dryRun {
			color.Cyan("\n*** DRY RUN MODE ENABLED ***")
			color.Cyan("No actual changes will be made.")
			color.Cyan("****************************\n")
		}

		rpt := &report.Report{Command: "update"}

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
		if updateNoPull {
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

		pkgPhase := rpt.AddPhase("Packages")

		if len(cfg.Packages) == 0 {
			fmt.Println("No packages configured.")
			pkgPhase.AddOK("packages", "none configured")
			rpt.PrintSummary(os.Stdout, summaryVerbosity())
			return
		}

		if updateSpecificPackage != "" {
			if _, exists := cfg.Packages[updateSpecificPackage]; !exists {
				fmt.Fprintln(os.Stderr, color.RedString("Package '%s' not found in configuration", updateSpecificPackage))
				pkgPhase.AddFail(updateSpecificPackage, "not found in configuration", nil)
				rpt.PrintSummary(os.Stdout, summaryVerbosity())
				os.Exit(1)
			}
		}

		opts := packages.UpdateOptions{
			DryRun:          dryRun,
			Force:           updateForce,
			SpecificPackage: updateSpecificPackage,
		}

		results := packages.UpdatePackages(w, cfg.Packages, cfg.PackagesDir, currentHost, opts)

		// Report results
		for _, r := range results {
			switch r.Action {
			case "error":
				fmt.Fprintln(os.Stderr, color.RedString("  %s: %s: %v", r.Name, r.Message, r.Err))
				pkgPhase.AddFail(r.Name, r.Message, r.Err)
			case "skipped":
				pkgPhase.AddSkip(r.Name, r.Message)
			case "up-to-date":
				pkgPhase.AddOK(r.Name, r.Message)
			default:
				fmt.Fprintf(w, "  %s: %s\n", r.Name, r.Message)
				pkgPhase.AddOK(r.Name, r.Message)
			}
		}

		fmt.Println("")
		if dryRun {
			color.Cyan("DRY RUN: Package update finished. No actual changes were made.")
		} else {
			color.Green("Package update complete.")
		}

		rpt.PrintSummary(os.Stdout, summaryVerbosity())
		os.Exit(rpt.ExitCode())
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Force rebuild of all packages regardless of change detection")
	updateCmd.Flags().StringVar(&updateSpecificPackage, "package", "", "Update only the specified package")
	updateCmd.Flags().BoolVar(&updateNoPull, "no-pull", false, "Skip pulling the dotfiles repo before updating packages")
}
