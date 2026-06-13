package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	// "io/fs" // Unused import

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
	RunE: func(cmd *cobra.Command, args []string) error {
		color.Cyan("Initializing ralph...")

		defaultConfigPath, err := config.GetDefaultConfigPath()
		if err != nil {
			return fmt.Errorf("could not determine default config path: %w", err)
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
				return nil
			}
			color.Yellow("Existing configuration will be overwritten.")
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("checking for config file at %s: %w", defaultConfigPath, err)
		}

		dotfilesRepoPathInput := ""
		defaultRepoPathSuggestion, _ := config.ExpandPath(
			"~/.dotfiles",
		) // Best effort for suggestion
		promptRepo := &survey.Input{
			Message: color.New(color.FgWhite, color.Bold).
				Sprint("Enter the path to your dotfiles source repository:"),
			Default: defaultRepoPathSuggestion,
			Help:    "This is where your actual dotfiles (e.g., .bashrc, .vimrc) are stored. Use ~ for home directory.",
		}
		err = survey.AskOne(
			promptRepo,
			&dotfilesRepoPathInput,
			survey.WithValidator(survey.Required),
		)
		if err != nil {
			return fmt.Errorf("during survey: %w", err)
		}

		expandedRepoPath, err := config.ExpandPath(dotfilesRepoPathInput)
		if err != nil {
			return fmt.Errorf("expanding repository path '%s': %w", dotfilesRepoPathInput, err)
		}
		fmt.Printf("Dotfiles repository path set to: %s\\n", color.GreenString(expandedRepoPath))

		var finalConfigContent []byte
		defaultConfigFilePath := "configs/examples/default.config.toml"
		templateBytes, err := os.ReadFile(defaultConfigFilePath)
		if err != nil {
			fmt.Fprintln(
				os.Stdout,
				color.YellowString(
					"Warning: Could not read default config template from '%s' (%v). Using minimal hardcoded config.",
					defaultConfigFilePath,
					err,
				),
			)
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
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			return fmt.Errorf("creating config directory %s: %w", configDir, err)
		}

		if err := os.WriteFile(defaultConfigPath, finalConfigContent, 0o644); err != nil {
			return fmt.Errorf("writing default configuration to %s: %w", defaultConfigPath, err)
		}
		color.Green("Default configuration file created at %s", defaultConfigPath)

		fmt.Println("\n" + color.New(color.FgCyan, color.Bold).Sprint("🎉 Next Steps:"))
		fmt.Printf(
			"1. %s your dotfiles repository at '%s'.\n",
			color.YellowString("Populate"),
			color.GreenString(expandedRepoPath),
		)
		fmt.Printf(
			"2. %s your '%s' with the dotfiles, tools, and shell settings you want to manage.\n",
			color.YellowString("Customize"),
			color.GreenString(defaultConfigPath),
		)
		fmt.Printf("3. Run '%s' to apply your configurations.\n", color.YellowString("ralph up"))
		fmt.Println("\n" + color.New(color.FgWhite, color.Bold).Sprint("💡 Important:"))
		fmt.Println(
			"   It is highly recommended to commit your dotfiles source repository (and potentially",
		)
		fmt.Printf(
			"   this config file if you symlink it there from '%s') to version control (e.g., Git).\n",
			color.GreenString(expandedRepoPath),
		)
		fmt.Println("\n" + color.New(color.FgWhite, color.Bold).Sprint("✨ Tip:"))
		fmt.Printf(
			"   Consider version controlling your ralph config file by placing it in '%s' \n   and symlinking '%s' to '%s'.\n",
			color.GreenString(
				expandedRepoPath,
			),
			color.GreenString(filepath.Join(expandedRepoPath, "your-ralph-config.toml")),
			color.GreenString(defaultConfigPath),
		)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

/*
// This function is no longer needed with the direct read/fallback approach
func getDefaultConfigTemplateContent() ([]byte, error) { ... }
*/
