package commands

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
	"github.com/spf13/cobra"
)

var (
	syncSpecificPackage string
	syncNoPull          bool
)

var syncCmd = &cobra.Command{
	Use:        "sync",
	Short:      "Sync dotfiles repo and remote packages",
	Long:       `Pulls latest changes for the dotfiles repository and clones/pulls remote packages. Does not build or install packages — run 'ralph up' after syncing to build.`,
	Deprecated: "use 'ralph up' instead",
	RunE: func(cmd *cobra.Command, args []string) error {
		// sync pulls the same checkouts `ralph up` does, so it holds the same
		// run lock.
		runLock, err := acquireRunLock(dryRun)
		if err != nil {
			return err
		}
		defer func() { _ = runLock.Release() }()

		w := verboseWriter(verbose, dryRun)

		fmt.Println("Syncing dotfiles repo and remote packages...")

		if dryRun {
			printDryRunBanner(os.Stdout)
		}

		rpt := &report.Report{Command: "sync"}

		cfg, err := loadRunConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error loading configuration: %v", err))
			cfgPhase := rpt.AddPhase("Configuration")
			cfgPhase.AddFail("config", "failed to load", err)
			rpt.PrintSummary(os.Stdout, summaryVerbosity())
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		// sync never applies: it is the sync half of `ralph up`.
		opts := syncOptions{noPull: syncNoPull, onlyPackage: syncSpecificPackage}
		runSyncSequence(w, cfg, config.GetCurrentHost(), opts, rpt)

		printSyncResult(rpt)
		if unknownPackage(cfg, opts.onlyPackage) {
			return fmt.Errorf("package '%s' not found in configuration", opts.onlyPackage)
		}
		if code := rpt.ExitCode(); code != 0 {
			return &ExitError{Code: code}
		}
		return nil
	},
}

// printSyncResult prints the end-of-run summary of `ralph sync`.
func printSyncResult(rpt *report.Report) {
	fmt.Println("")
	if dryRun {
		color.Cyan("DRY RUN: Sync finished. No actual changes were made.")
		rpt.PrintSummary(os.Stdout, summaryVerbosity())
		return
	}
	printReportSummary(rpt)
	if rpt.HasFailures() || rpt.HasWarnings() || verbose {
		rpt.PrintSummary(os.Stdout, summaryVerbosity())
	}
}

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().
		StringVar(&syncSpecificPackage, "package", "", "Sync only the specified package")
	syncCmd.Flags().
		BoolVar(&syncNoPull, "no-pull", false, "Skip pulling the dotfiles repo and recipe sources before syncing packages")
}
