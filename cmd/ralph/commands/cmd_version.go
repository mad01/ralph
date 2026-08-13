package commands

import (
	"encoding/json"
	"fmt"

	"github.com/mad01/ralph/internal/buildinfo"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the ralph version",
	Long: `Print the ralph version (the git commit it was built from).

With -o json, prints the full build metadata — the cross-tool convention
sibling tools follow so a single probe can ask any of them what build it is:

  {
    "version": "2917e73",
    "commit": "2917e735a634884fa21ff45a833e2067dc2236be",
    "tag": "v0.1.0",
    "build_time": "2026-08-13T19:40:11Z"
  }

All four keys are always present; a value the build could not determine is
empty.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		info := buildinfo.Get()
		if outputJSON() {
			b, err := json.MarshalIndent(info, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		}
		fmt.Println(info.Version)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
