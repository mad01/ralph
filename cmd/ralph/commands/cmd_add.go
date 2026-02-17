package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add new items to ralph management (e.g., a tool)",
	Long:  `Helps add new configurations or items to be managed by ralph.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Adding item...")
		// TODO: Implement add logic
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}
