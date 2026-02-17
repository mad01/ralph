package commands

import (
	"bytes"
	"fmt"

	// "io/fs" // Unused import

	"os"
	"path/filepath"

	// For replacing content
	// For replacing content
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/spf13/cobra"
)

// Removing: //go:embed ../../configs/examples/default.config.toml
// var defaultConfigContentBytes []byte

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize ralph configuration",
	Long:  `Initializes a new ralph configuration file and provides guidance on next steps.`,
	Run: func(cmd *cobra.Command, args []string) {
		color.Cyan("Initializing ralph...")

		defaultConfigPath, err := config.GetDefaultConfigPath()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error: Could not determine default config path: %v", err))
			os.Exit(1)
		}

		if _, err := os.Stat(defaultConfigPath); err == nil {
			color.Yellow("Configuration file already exists at %s.", defaultConfigPath)
			overwrite := false
			prompt := &survey.Confirm{
				Message: "Overwrite?",
			}
			survey.AskOne(prompt, &overwrite)
			if !overwrite {
				color.Green("Initialization cancelled. Existing configuration preserved.")
				return
			}
			color.Yellow("Existing configuration will be overwritten.")
		} else if !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, color.RedString("Error checking for config file at %s: %v", defaultConfigPath, err))
			os.Exit(1)
		}

		dotfilesRepoPathInput := ""
		defaultRepoPathSuggestion, _ := config.ExpandPath("~/.dotfiles") // Best effort for suggestion
		promptRepo := &survey.Input{
			Message: color.New(color.FgWhite, color.Bold).Sprint("Enter the path to your dotfiles source repository:"),
			Default: defaultRepoPathSuggestion,
			Help:    "This is where your actual dotfiles (e.g., .bashrc, .vimrc) are stored. Use ~ for home directory.",
		}
		err = survey.AskOne(promptRepo, &dotfilesRepoPathInput, survey.WithValidator(survey.Required))
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error during survey: %v", err))
			os.Exit(1)
		}

		expandedRepoPath, err := config.ExpandPath(dotfilesRepoPathInput)
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error expanding repository path '%s': %v", dotfilesRepoPathInput, err))
			os.Exit(1)
		}
		fmt.Printf("Dotfiles repository path set to: %s\\n", color.GreenString(expandedRepoPath))

		var finalConfigContent []byte
		defaultConfigFilePath := "configs/examples/default.config.toml"
		templateBytes, err := os.ReadFile(defaultConfigFilePath)
		if err != nil {
			fmt.Fprintln(os.Stdout, color.YellowString("Warning: Could not read default config template from '%s' (%v). Using minimal hardcoded config.", defaultConfigFilePath, err))
			hardcodedConfig := fmt.Sprintf("dotfiles_repo_path = \"%s\"\n\n"+
				"# Example dotfile entry:\n"+
				"# [dotfiles.bashrc]\n"+
				"# source = \".bashrc\"\n"+
				"# target = \"~/.bashrc\"\n"+
				"# is_template = false\n\n"+
				"# Example tool entry:\n"+
				"# [[tools]]\n"+
				"# name = \"fzf\"\n"+
				"# check_command = \"command -v fzf\"\n"+
				"# install_hint = \"Install fzf from https://github.com/junegunn/fzf\"\n\n"+
				"# Example shell alias:\n"+
				"# [shell.aliases]\n"+
				"# ll = \"ls -alh\"\n\n"+
				"# Example shell function (POSIX):\n"+
				"# [shell.functions.myfunc]\n"+
				"# body = \"\"\"\n"+
				"# echo \\\"Hello from myfunc!\\\"\n"+
				"# echo \\\"Arguments: $@\\\"\n"+
				"# \"\"\"\n", expandedRepoPath)
			finalConfigContent = []byte(hardcodedConfig)
		} else {
			// Replace the placeholder in the template file
			placeholder := "dotfiles_repo_path = \"~/.dotfiles\"" // Must match placeholder in default.config.toml
			replacement := fmt.Sprintf("dotfiles_repo_path = \"%s\"", expandedRepoPath)
			finalConfigContent = bytes.ReplaceAll(templateBytes, []byte(placeholder), []byte(replacement))
			fmt.Println("Using default configuration template.")
		}

		configDir := filepath.Dir(defaultConfigPath)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error creating config directory %s: %v", configDir, err))
			os.Exit(1)
		}

		if err := os.WriteFile(defaultConfigPath, finalConfigContent, 0644); err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error writing default configuration to %s: %v", defaultConfigPath, err))
			os.Exit(1)
		}
		color.Green("Default configuration file created at %s", defaultConfigPath)

		fmt.Println("\n" + color.New(color.FgCyan, color.Bold).Sprint("🎉 Next Steps:"))
		fmt.Printf("1. %s your dotfiles repository at '%s'.\n", color.YellowString("Populate"), color.GreenString(expandedRepoPath))
		fmt.Printf("2. %s your '%s' with the dotfiles, tools, and shell settings you want to manage.\n", color.YellowString("Customize"), color.GreenString(defaultConfigPath))
		fmt.Printf("3. Run '%s' to apply your configurations.\n", color.YellowString("ralph apply"))
		fmt.Println("\n" + color.New(color.FgWhite, color.Bold).Sprint("💡 Important:"))
		fmt.Println("   It is highly recommended to commit your dotfiles source repository (and potentially")
		fmt.Printf("   this config file if you symlink it there from '%s') to version control (e.g., Git).\n", color.GreenString(expandedRepoPath))
		fmt.Println("\n" + color.New(color.FgWhite, color.Bold).Sprint("✨ Tip:"))
		fmt.Printf("   Consider version controlling your ralph config file by placing it in '%s' \n   and symlinking '%s' to '%s'.\n",
			color.GreenString(expandedRepoPath), color.GreenString(filepath.Join(expandedRepoPath, "your-ralph-config.toml")), color.GreenString(defaultConfigPath))
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

/*
// This function is no longer needed with the direct read/fallback approach
func getDefaultConfigTemplateContent() ([]byte, error) { ... }
*/
