package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

var listRecipesCmd = &cobra.Command{
	Use:   "recipes",
	Short: "List all discovered recipes and their status",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error loading configuration: %v", err))
			os.Exit(1)
		}

		expandedRepoPath, err := config.ExpandPath(cfg.DotfilesRepoPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error expanding dotfiles repo path: %v", err))
			os.Exit(1)
		}

		// Discover all recipes (including disabled ones)
		discovered, err := config.DiscoverRecipes(cfg.DotfilesRepoPath, cfg.RecipesConfig)
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error discovering recipes: %v", err))
			os.Exit(1)
		}

		if len(discovered) == 0 {
			fmt.Println(color.YellowString("No recipes found."))
			return
		}

		// Sort by name
		sort.Slice(discovered, func(i, j int) bool {
			return discovered[i].Name < discovered[j].Name
		})

		currentHost := config.GetCurrentHost()

		// Collect recipe info for display
		type recipeInfo struct {
			name    string
			enabled bool
			summary string
		}

		var recipes []recipeInfo
		maxNameLen := 0

		for _, ref := range discovered {
			enabled := config.IsEnabled(ref.Enable) && config.ShouldApplyForHost(ref.Hosts, currentHost)

			recipePath := filepath.Join(expandedRepoPath, ref.Path)
			recipe, loadErr := config.LoadRecipe(recipePath)

			name := ref.Name
			summary := ""

			if loadErr != nil {
				summary = fmt.Sprintf("(error: %v)", loadErr)
			} else {
				if recipe.Recipe.Name != "" {
					name = ref.Name // Use directory name for consistency in listing
				}

				// Check for special delete_behavior
				if recipe.Recipe.DeleteBehavior == config.DeleteBehaviorAbandon {
					summary = "(abandon on delete)"
				} else {
					summary = recipeItemSummary(recipe)
				}
			}

			if len(name) > maxNameLen {
				maxNameLen = len(name)
			}

			recipes = append(recipes, recipeInfo{
				name:    name,
				enabled: enabled,
				summary: summary,
			})
		}

		// Print header
		fmt.Println(color.New(color.FgWhite, color.Bold).Sprint("Recipes:"))

		enabledCount := 0
		disabledCount := 0

		for _, r := range recipes {
			var status string
			if r.enabled {
				enabledCount++
				status = color.GreenString("enabled")
			} else {
				disabledCount++
				status = color.YellowString("disabled")
			}

			padding := strings.Repeat(" ", maxNameLen-len(r.name)+2)
			fmt.Printf("  %s%s%-10s %s\n", r.name, padding, status, r.summary)
		}

		// Footer
		total := len(recipes)
		fmt.Printf("\n%d recipes (%d enabled, %d disabled)\n", total, enabledCount, disabledCount)
	},
}

// recipeItemSummary builds a parenthesized summary of item counts in a recipe.
func recipeItemSummary(recipe *config.Recipe) string {
	type itemCount struct {
		count int
		label string
	}

	var counts []itemCount

	if n := len(recipe.Dotfiles); n > 0 {
		label := "dotfiles"
		if n == 1 {
			label = "dotfile"
		}
		counts = append(counts, itemCount{n, label})
	}

	if n := len(recipe.Shell.Aliases); n > 0 {
		label := "aliases"
		if n == 1 {
			label = "alias"
		}
		counts = append(counts, itemCount{n, label})
	}

	if n := len(recipe.Shell.Functions); n > 0 {
		label := "functions"
		if n == 1 {
			label = "function"
		}
		counts = append(counts, itemCount{n, label})
	}

	if n := len(recipe.Shell.Env); n > 0 {
		label := "env vars"
		if n == 1 {
			label = "env var"
		}
		counts = append(counts, itemCount{n, label})
	}

	if n := len(recipe.Hooks.Builds); n > 0 {
		label := "builds"
		if n == 1 {
			label = "build"
		}
		counts = append(counts, itemCount{n, label})
	}

	if n := len(recipe.Packages); n > 0 {
		label := "pkgs"
		if n == 1 {
			label = "pkg"
		}
		counts = append(counts, itemCount{n, label})
	}

	if n := len(recipe.DirsMirror); n > 0 {
		label := "dir mirrors"
		if n == 1 {
			label = "dir mirror"
		}
		counts = append(counts, itemCount{n, label})
	}

	if n := len(recipe.Repos); n > 0 {
		label := "repos"
		if n == 1 {
			label = "repo"
		}
		counts = append(counts, itemCount{n, label})
	}

	if n := len(recipe.Directories); n > 0 {
		label := "dirs"
		if n == 1 {
			label = "dir"
		}
		counts = append(counts, itemCount{n, label})
	}

	if n := len(recipe.Tools); n > 0 {
		label := "tools"
		if n == 1 {
			label = "tool"
		}
		counts = append(counts, itemCount{n, label})
	}

	if len(counts) == 0 {
		return "(empty)"
	}

	parts := make([]string, len(counts))
	for i, c := range counts {
		parts[i] = fmt.Sprintf("%d %s", c.count, c.label)
	}
	return "(" + strings.Join(parts, ", ") + ")"
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
	listCmd.AddCommand(listRecipesCmd)
}
