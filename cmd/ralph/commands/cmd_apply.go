package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/apply"
	"github.com/mad01/ralph/internal/report"
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
			opts: apply.Options{
				Overwrite:     overwriteExisting,
				Skip:          skipExisting,
				Force:         forceBuilds,
				Build:         specificBuild,
				EnableCleanup: enableCleanup,
			},
		})
	},
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

// printApplyResult prints the final summary line, caveats, and report. A
// recipe's caveat prints once, however many of its packages changed.
func printApplyResult(rpt *report.Report, caveats []apply.Caveat, isDryRun, isVerbose bool) {
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
	if len(caveats) > 0 {
		fmt.Println("")
		fmt.Println(color.YellowString("==> Caveats"))
		seen := map[string]bool{}
		for _, c := range caveats {
			if seen[c.Recipe] {
				continue
			}
			seen[c.Recipe] = true
			fmt.Printf("\n%s:\n", color.YellowString(c.Recipe))
			for _, line := range strings.Split(strings.TrimSpace(c.Text), "\n") {
				fmt.Printf("  %s\n", line)
			}
		}
		fmt.Println("")
	}
	if rpt.HasFailures() || rpt.HasWarnings() || isVerbose {
		rpt.PrintSummary(os.Stdout, summaryVerbosity())
	}
}
