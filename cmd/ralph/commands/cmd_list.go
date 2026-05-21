package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/packages"
	"github.com/mad01/ralph/internal/tool"
	"github.com/spf13/cobra"
)

var listSourceFilter string

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List managed dotfiles, tools, and shell configurations",
	Long:  `Displays a list of all items currently managed by ralph, along with their status.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(color.CyanString("Listing managed items..."))

		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error loading configuration: %v", err))
			fmt.Fprintln(os.Stderr, color.YellowString("Consider running 'ralph init' if you haven't already."))
			os.Exit(1)
		}

		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nManaged Dotfiles:"))
		if len(cfg.Dotfiles) == 0 {
			fmt.Println(color.YellowString("  No dotfiles configured."))
		} else {
			dfNames := make([]string, 0, len(cfg.Dotfiles))
			for k := range cfg.Dotfiles {
				dfNames = append(dfNames, k)
			}
			sort.Strings(dfNames)
			for _, name := range dfNames {
				df := cfg.Dotfiles[name]
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

		// Managed Packages
		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nManaged Packages:"))
		if len(cfg.Packages) == 0 {
			fmt.Println(color.YellowString("  No packages configured."))
		} else {
			currentHost := config.GetCurrentHost()
			statuses := packages.CheckPackageStatuses(cfg.Packages, cfg.PackagesDir, currentHost)

			shown := 0
			for _, s := range statuses {
				if listSourceFilter != "" && s.Source != listSourceFilter {
					continue
				}
				shown++

				var statusMsg string
				statusColor := color.New(color.FgYellow)

				if !s.Enabled {
					statusMsg = "Disabled"
					statusColor = color.New(color.FgRed)
				} else if !s.HostMatch {
					statusMsg = "Skipped (host filter)"
				} else if s.NeedsBuild {
					statusMsg = fmt.Sprintf("Needs update (%s)", s.NeedReason)
				} else if s.NeedReason == "working_dir missing" {
					statusMsg = fmt.Sprintf("Warning (%s)", s.NeedReason)
				} else if s.LastBuiltAt != nil {
					statusMsg = fmt.Sprintf("Up to date (built %s)", s.LastBuiltAt.Format("2006-01-02 15:04:05"))
					statusColor = color.New(color.FgGreen)
				} else {
					statusMsg = "Up to date"
					statusColor = color.New(color.FgGreen)
				}

				hashInfo := ""
				if s.CurrentHash != "" {
					hashInfo = fmt.Sprintf("\n      Git Hash: %s", shortHash(s.CurrentHash))
				}

				fmt.Printf("  - %s:\n      Source: %s\n      Working Dir: %s%s\n      Status: %s\n",
					color.New(color.Bold).Sprint(s.Name),
					s.Source, s.WorkingDir, hashInfo,
					statusColor.Sprint(statusMsg))

				if s.NeedsBuild && s.Enabled && s.HostMatch {
					fmt.Printf("      Run: ralph apply (or ralph apply --force)\n")
				}
			}

			if shown == 0 && listSourceFilter != "" {
				fmt.Println(color.YellowString("  No packages match source filter '%s'.", listSourceFilter))
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
			aliasNames := make([]string, 0, len(cfg.Shell.Aliases))
			for k := range cfg.Shell.Aliases {
				aliasNames = append(aliasNames, k)
			}
			sort.Strings(aliasNames)
			for _, name := range aliasNames {
				alias := cfg.Shell.Aliases[name]
				fmt.Printf("  - %s: %s\n", color.New(color.Bold).Sprint(name), alias.Command)
			}
		}

		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("\nDefined Shell Functions:"))
		if len(cfg.Shell.Functions) == 0 {
			fmt.Println(color.YellowString("  No shell functions defined."))
		} else {
			fnNames := make([]string, 0, len(cfg.Shell.Functions))
			for k := range cfg.Shell.Functions {
				fnNames = append(fnNames, k)
			}
			sort.Strings(fnNames)
			for _, name := range fnNames {
				fn := cfg.Shell.Functions[name]
				fmt.Printf("  - %s\n", color.New(color.Bold).Sprint(name))
				// Could print fn.Body or a summary if desired, for now just the name
				_ = fn // to satisfy linter if fn is not used
			}
		}
		fmt.Println("\n" + color.CyanString("Listing complete."))
	},
}

func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().StringVar(&listSourceFilter, "source", "", "Filter packages by source type: 'local' or 'remote'")
}
