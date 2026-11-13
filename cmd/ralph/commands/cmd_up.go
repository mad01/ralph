package commands

import (
	"github.com/mad01/ralph/internal/apply"
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
	Long: `Pulls the dotfiles repo and recipe sources, syncs remote packages, and
then applies all configurations (symlinks, shell config, builds, packages, cleanup).
Replaces the separate 'ralph sync' + 'ralph apply' workflow.

Use --no-sync to skip the sync step and only apply.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runApplyCommand(applyCommand{
			name:        "up",
			header:      "Ralph up...",
			resetBuilds: upResetBuilds,
			sync:        !upNoSync,
			opts: apply.Options{
				Overwrite:     upOverwrite,
				Skip:          upSkip,
				Force:         upForce,
				Build:         upBuild,
				EnableCleanup: upEnableCleanup,
			},
		})
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
	supportJSONOutput(upCmd)
	upCmd.Flags().
		BoolVar(&upOverwrite, "overwrite", false, "Overwrite existing files at target locations")
	upCmd.Flags().BoolVar(&upSkip, "skip", false, "Skip symlinking if target file already exists")
	upCmd.Flags().BoolVar(&upForce, "force", false, "Force re-run of 'once' builds")
	upCmd.Flags().StringVar(&upBuild, "build", "", "Run only the specified build")
	upCmd.Flags().
		BoolVar(&upResetBuilds, "reset-builds", false, "Clear all build state before running")
	upCmd.Flags().
		BoolVar(&upNoSync, "no-sync", false, "Skip the sync steps (dotfiles repo pull, recipe sources pull, package sync) and only apply")
	upCmd.Flags().
		BoolVar(&upEnableCleanup, "enable-cleanup", false, "Remove orphaned artifacts from removed/disabled recipes")
}
