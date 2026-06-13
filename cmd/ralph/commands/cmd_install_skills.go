package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/skills"
	"github.com/spf13/cobra"
)

var forceSkills bool

var installSkillsCmd = &cobra.Command{
	Use:   "install-skills",
	Short: "Install ralph's Claude Code skills",
	Long: `Installs ralph's bundled Claude Code skills into ~/.claude/skills/.

Skills help Claude understand how to work with ralph configurations,
recipes, and troubleshooting.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		w := io.Writer(io.Discard)
		if verbose {
			w = os.Stdout
		}

		fmt.Println("Installing ralph Claude Code skills...")

		if dryRun {
			printDryRunBanner(os.Stdout)
		}

		rpt := &report.Report{Command: "install-skills"}

		opts := skills.InstallOptions{
			DryRun: dryRun,
			Force:  forceSkills,
		}

		results := skills.Install(w, opts)

		phase := rpt.AddPhase("Skills")
		for _, r := range results {
			switch r.Action {
			case "error":
				fmt.Fprintln(os.Stderr, color.RedString("  %s: %s: %v", r.Name, r.Message, r.Err))
				phase.AddFail(r.Name, r.Message, r.Err)
			case "skipped":
				phase.AddSkip(r.Name, r.Message)
			default:
				phase.AddOK(r.Name, r.Message)
			}
		}

		fmt.Println("")
		if dryRun {
			color.Cyan("DRY RUN: Install finished. No actual changes were made.")
		} else {
			color.Green("Skills installed.")
		}

		rpt.PrintSummary(os.Stdout, summaryVerbosity())
		if code := rpt.ExitCode(); code != 0 {
			return &ExitError{Code: code}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installSkillsCmd)
	installSkillsCmd.Flags().
		BoolVar(&forceSkills, "force", false, "Overwrite existing skill symlinks or directories")
}
