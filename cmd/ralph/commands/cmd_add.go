package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add items to ralph management (not yet implemented)",
	Long:  `Planned command for adding new configurations or items to be managed by ralph. Not yet implemented — edit config.toml directly.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), "error: ralph add is not yet implemented — edit config.toml directly")
		return fmt.Errorf("not implemented")
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}
