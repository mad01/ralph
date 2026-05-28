package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the ralph version",
	Long: `Print the ralph version (the git commit it was built from).

With -o json, prints {"version":"<sha>"} — the cross-tool convention sibling
tools follow so a single probe can ask any of them what build it is.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if outputJSON() {
			b, err := json.Marshal(map[string]string{"version": Version})
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		}
		fmt.Println(Version)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
