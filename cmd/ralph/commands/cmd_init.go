package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/mad01/ralph/configs/examples"
	"github.com/mad01/ralph/internal/config"
	"github.com/spf13/cobra"
)

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
			overwrite, err := confirmOverwrite(defaultConfigPath)
			if err != nil {
				return err
			}
			if !overwrite {
				color.Green("Initialization cancelled. Existing configuration preserved.")
				return nil
			}
			color.Yellow("Existing configuration will be overwritten.")
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("checking for config file at %s: %w", defaultConfigPath, err)
		}

		expandedRepoPath, err := promptRepoPath()
		if err != nil {
			return err
		}
		fmt.Printf("Dotfiles repository path set to: %s\n", color.GreenString(expandedRepoPath))

		content := renderInitConfig(expandedRepoPath)
		if err := installInitConfig(defaultConfigPath, expandedRepoPath, content); err != nil {
			return err
		}
		if !dryRun {
			printInitNextSteps(defaultConfigPath, expandedRepoPath)
		}
		return nil
	},
}

// confirmOverwrite asks whether to replace the existing config at configPath.
// A failed prompt is returned as an error rather than read as "no".
func confirmOverwrite(configPath string, opts ...survey.AskOpt) (bool, error) {
	color.Yellow("Configuration file already exists at %s.", configPath)
	overwrite := false
	prompt := &survey.Confirm{Message: "Overwrite?"}
	if err := survey.AskOne(prompt, &overwrite, opts...); err != nil {
		return false, fmt.Errorf("reading overwrite answer: %w", err)
	}
	return overwrite, nil
}

// promptRepoPath asks for the dotfiles repository path and returns it expanded.
func promptRepoPath() (string, error) {
	input := ""
	suggestion, _ := config.ExpandPath("~/.dotfiles") // Best effort for suggestion
	prompt := &survey.Input{
		Message: color.New(color.FgWhite, color.Bold).
			Sprint("Enter the path to your dotfiles source repository:"),
		Default: suggestion,
		Help:    "This is where your actual dotfiles (e.g., .bashrc, .vimrc) are stored. Use ~ for home directory.",
	}
	if err := survey.AskOne(prompt, &input, survey.WithValidator(survey.Required)); err != nil {
		return "", fmt.Errorf("during survey: %w", err)
	}
	expanded, err := config.ExpandPath(input)
	if err != nil {
		return "", fmt.Errorf("expanding repository path '%s': %w", input, err)
	}
	return expanded, nil
}

// renderInitConfig returns the embedded starter config with dotfiles_repo_path
// set to repoPath.
func renderInitConfig(repoPath string) []byte {
	line := fmt.Sprintf("dotfiles_repo_path = %q", repoPath)
	return []byte(strings.Replace(examples.DefaultConfig, examples.DefaultRepoPathLine, line, 1))
}

// installInitConfig writes content to configPath, creating its directory, and
// gitignores the machine-local overlay in repoPath. Under --dry-run it prints
// what it would write and writes nothing.
func installInitConfig(configPath, repoPath string, content []byte) error {
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	if dryRun {
		fmt.Printf("Would write configuration to %s:\n%s\n", configPath, content)
		fmt.Printf(
			"Would add '%s' to %s if it is not listed there\n",
			config.LocalConfigFileName, gitignorePath,
		)
		return nil
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("creating config directory %s: %w", configDir, err)
	}
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		return fmt.Errorf("writing default configuration to %s: %w", configPath, err)
	}
	color.Green("Default configuration file created at %s", configPath)

	// Best-effort: keep the optional machine-local overlay out of version
	// control by gitignoring it in the dotfiles repo. Never fail init on this.
	if added, err := ensureGitignored(repoPath, config.LocalConfigFileName); err != nil {
		color.Yellow("Could not update .gitignore in %s: %v", repoPath, err)
	} else if added {
		color.Green("Added '%s' to %s", config.LocalConfigFileName, gitignorePath)
	}
	return nil
}

// printInitNextSteps tells a new user how to go on from a freshly written config.
func printInitNextSteps(configPath, repoPath string) {
	fmt.Println("\n" + color.New(color.FgCyan, color.Bold).Sprint("🎉 Next Steps:"))
	fmt.Printf(
		"1. %s your dotfiles repository at '%s'.\n",
		color.YellowString("Populate"),
		color.GreenString(repoPath),
	)
	fmt.Printf(
		"2. %s your '%s' with the dotfiles, tools, and shell settings you want to manage.\n",
		color.YellowString("Customize"),
		color.GreenString(configPath),
	)
	fmt.Printf("3. Run '%s' to apply your configurations.\n", color.YellowString("ralph up"))
	fmt.Println("\n" + color.New(color.FgWhite, color.Bold).Sprint("💡 Important:"))
	fmt.Println(
		"   It is highly recommended to commit your dotfiles source repository (and potentially",
	)
	fmt.Printf(
		"   this config file if you symlink it there from '%s') to version control (e.g., Git).\n",
		color.GreenString(repoPath),
	)
	fmt.Println("\n" + color.New(color.FgWhite, color.Bold).Sprint("✨ Tip:"))
	fmt.Printf(
		"   Consider version controlling your ralph config file by placing it in '%s' \n   and symlinking '%s' to '%s'.\n",
		color.GreenString(
			repoPath,
		),
		color.GreenString(filepath.Join(repoPath, "your-ralph-config.toml")),
		color.GreenString(configPath),
	)
}

// ensureGitignored adds entry to repoPath/.gitignore unless it is already
// ignored. It is a no-op (added=false, err=nil) when repoPath is not an
// existing directory, so it is safe to call before the repo is populated. The
// .gitignore is created if missing. A trailing newline is preserved.
func ensureGitignored(repoPath, entry string) (added bool, err error) {
	info, statErr := os.Stat(repoPath)
	if statErr != nil || !info.IsDir() {
		return false, nil //nolint:nilerr // missing repo dir is intentionally not an error here
	}

	gitignorePath := filepath.Join(repoPath, ".gitignore")
	content, readErr := os.ReadFile(gitignorePath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return false, fmt.Errorf("reading %s: %w", gitignorePath, readErr)
	}

	for line := range strings.SplitSeq(string(content), "\n") {
		if strings.TrimSpace(line) == entry {
			return false, nil
		}
	}

	var b strings.Builder
	b.Write(content)
	if len(content) > 0 && !bytes.HasSuffix(content, []byte("\n")) {
		b.WriteString("\n")
	}
	b.WriteString(entry)
	b.WriteString("\n")

	if err := os.WriteFile(gitignorePath, []byte(b.String()), 0o644); err != nil {
		return false, fmt.Errorf("writing %s: %w", gitignorePath, err)
	}
	return true, nil
}

func init() {
	rootCmd.AddCommand(initCmd)
}
