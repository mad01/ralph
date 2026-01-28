package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/mad01/dotter/internal/config"

	// "github.com/mad01/dotter/internal/dotfile" // For symlink status check - removing to clear linter
	"github.com/mad01/dotter/internal/tool" // Added for tool status check
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List managed dotfiles, tools, and shell configurations",
	Long:  `Displays a list of all items currently managed by dotter, along with their status.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(color.CyanString("Listing managed items..."))

		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error loading configuration: %v", err))
			fmt.Fprintln(os.Stderr, color.YellowString("Consider running 'dotter init' if you haven't already."))
			os.Exit(1)
		}

		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nManaged Dotfiles:"))
		if len(cfg.Dotfiles) == 0 {
			fmt.Println(color.YellowString("  No dotfiles configured."))
		} else {
			for name, df := range cfg.Dotfiles {
				var statusMsg string
				statusColor := color.New(color.FgYellow) // Default to yellow for warnings/unknown

				absoluteTarget, expandErr := config.ExpandPath(df.Target)
				if expandErr != nil {
					statusMsg = fmt.Sprintf("Error expanding target path: %v", expandErr)
					statusColor = color.New(color.FgRed)
				} else {
					targetInfo, statErr := os.Lstat(absoluteTarget)
					if os.IsNotExist(statErr) {
						statusMsg = "Not linked (target does not exist)"
					} else if statErr != nil {
						statusMsg = fmt.Sprintf("Error checking target: %v", statErr)
						statusColor = color.New(color.FgRed)
					} else {
						if targetInfo.Mode()&os.ModeSymlink != 0 {
							linkDest, readlinkErr := os.Readlink(absoluteTarget)
							if readlinkErr != nil {
								statusMsg = "Symlink (error reading destination)"
								statusColor = color.New(color.FgRed)
							} else {
								absoluteSource, _ := config.ExpandPath(filepath.Join(cfg.DotfilesRepoPath, df.Source))
								var expectedLinkDest string
								if df.IsTemplate {
									// For templates, the symlink points to a processed file which is absolute.
									// The actual check if it's the *correct* processed file is harder here
									// We rely on the `apply` command doing the right thing.
									// We check if the link destination exists.
									if _, err := os.Stat(linkDest); err == nil {
										statusMsg = fmt.Sprintf("Linked (templated) to: %s", linkDest)
										statusColor = color.New(color.FgGreen)
									} else {
										statusMsg = fmt.Sprintf("Linked (templated) but destination '%s' MISSING", linkDest)
									}
								} else {
									expectedLinkDest = absoluteSource
									if linkDest == expectedLinkDest {
										statusMsg = fmt.Sprintf("Correctly linked to: %s", linkDest)
										statusColor = color.New(color.FgGreen)
									} else {
										statusMsg = fmt.Sprintf("Symlinked to WRONG source: %s (expected %s)", linkDest, expectedLinkDest)
									}
								}
							}
						} else {
							statusMsg = "Exists but is NOT a symlink"
						}
					}
				}
				templateMarker := ""
				if df.IsTemplate {
					templateMarker = color.CyanString(" (template)")
				}
				fmt.Printf("  - %s%s:\n      Source: %s\n      Target: %s\n      Status: %s\n",
					color.New(color.Bold).Sprint(name), templateMarker,
					df.Source, df.Target,
					statusColor.Sprint(statusMsg))
			}
		}

		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nConfigured Tools:"))
		if len(cfg.Tools) == 0 {
			fmt.Println(color.YellowString("  No tools configured."))
		} else {
			for _, t := range cfg.Tools {
				var statusColor *color.Color
				status := "Not installed"
				if tool.CheckStatus(t.CheckCommand) {
					status = "Installed"
					statusColor = color.New(color.FgGreen)
				} else {
					statusColor = color.New(color.FgYellow)
				}
				fmt.Printf("  - %s (Check: '%s', Hint: '%s'): %s\n",
					color.New(color.Bold).Sprint(t.Name), t.CheckCommand, t.InstallHint, statusColor.Sprint(status))
			}
		}

		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nDefined Shell Aliases:"))
		if len(cfg.Shell.Aliases) == 0 {
			fmt.Println(color.YellowString("  No shell aliases defined."))
		} else {
			for name, alias := range cfg.Shell.Aliases {
				fmt.Printf("  - %s: %s\n", color.New(color.Bold).Sprint(name), alias.Command)
			}
		}

		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nDefined Shell Functions:"))
		if len(cfg.Shell.Functions) == 0 {
			fmt.Println(color.YellowString("  No shell functions defined."))
		} else {
			for name, fn := range cfg.Shell.Functions { // Iterate to get fn details if needed in future
				fmt.Printf("  - %s\n", color.New(color.Bold).Sprint(name))
				// Could print fn.Body or a summary if desired, for now just the name
				_ = fn // to satisfy linter if fn is not used
			}
		}
		fmt.Println("\n" + color.CyanString("Listing complete."))
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
