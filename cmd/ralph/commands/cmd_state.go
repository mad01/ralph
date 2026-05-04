package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/state"
	"github.com/spf13/cobra"
)

var (
	stateJSON bool
)

var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Inspect ralph's per-recipe artifact manifest",
}

var stateShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the current recipe-state manifest",
	Long: `Prints the contents of ~/.config/ralph/.recipe_state. Each entry shows
the artifacts a recipe has installed (symlinks, copies, directories,
shell aliases/functions, packages, builds, install_paths) and the
recorded delete_behavior. Use --json for machine-readable output.`,
	Run: func(cmd *cobra.Command, args []string) {
		s, err := state.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error loading recipe state: %v", err))
			os.Exit(1)
		}

		if stateJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(s); err != nil {
				fmt.Fprintln(os.Stderr, color.RedString("Error encoding state: %v", err))
				os.Exit(1)
			}
			return
		}

		if len(s.Recipes) == 0 {
			fmt.Println("(empty — no recipes have been applied yet)")
			return
		}

		names := make([]string, 0, len(s.Recipes))
		for n := range s.Recipes {
			names = append(names, n)
		}
		sort.Strings(names)

		bold := color.New(color.Bold).SprintFunc()
		dim := color.New(color.Faint).SprintFunc()
		for _, n := range names {
			art := s.Recipes[n]
			fmt.Printf("%s  %s\n", bold(n), dim("(applied "+art.AppliedAt.Format("2006-01-02 15:04")+", delete_behavior="+art.DeleteBehavior+")"))
			emit := func(label string, vs []string) {
				if len(vs) == 0 {
					return
				}
				fmt.Printf("  %s:\n", label)
				for _, v := range vs {
					fmt.Printf("    %s\n", v)
				}
			}
			emit("symlinks", art.Symlinks)
			emit("dir_symlinks", art.DirSymlinks)
			emit("copies", art.Copies)
			emit("directories", art.Directories)
			emit("install_paths", art.InstallPaths)
			emit("repos", art.Repos)
			emit("shell_aliases", art.ShellAliases)
			emit("shell_functions", art.ShellFunctions)
			emit("shell_env", art.ShellEnv)
			emit("packages", art.Packages)
			emit("builds", art.Builds)
		}
	},
}

func init() {
	rootCmd.AddCommand(stateCmd)
	stateCmd.AddCommand(stateShowCmd)
	stateShowCmd.Flags().BoolVar(&stateJSON, "json", false, "Output state as JSON")
}
