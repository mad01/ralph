package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/hooks"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/shell"
	"github.com/mad01/ralph/internal/state"
	"github.com/spf13/cobra"
)

var (
	downForce bool
	downYes   bool
)

var downCmd = &cobra.Command{
	Use:   "down <recipe-name>",
	Short: "Uninstall a recipe and remove its effects",
	Long: `Remove all artifacts installed by the named recipe (symlinks, copies,
directories, shell aliases/functions, build state) and disable it in
config.toml. The recipe's pre_uninstall and post_uninstall hooks run
around the cleanup phase.

Requires confirmation before proceeding. Use --yes/-y to skip the prompt.
Use --force to bypass the dependency guard and pre_uninstall failures.
Use --dry-run to preview what would be removed without touching disk.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		recipeName := args[0]

		w := verboseWriter(verbose, dryRun)

		rpt := &report.Report{Command: "down"}

		if dryRun {
			printDryRunBanner(uiOut())
		}

		// --- Step 1: Load config and find recipe ---
		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error loading configuration: %v", err))
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		var recipeInfo *config.LoadedRecipeInfo
		for i := range cfg.LoadedRecipes {
			if cfg.LoadedRecipes[i].Name == recipeName {
				recipeInfo = &cfg.LoadedRecipes[i]
				break
			}
		}
		if recipeInfo == nil {
			// Recipe may be disabled — try to find it by directory path.
			recipesDir := cfg.RecipesConfig.Dir
			if recipesDir == "" {
				recipesDir = config.DefaultRecipesDir
			}
			candidatePath := filepath.Join(recipesDir, recipeName, config.RecipeFileName)
			expandedRepoPath, expandErr := config.ExpandPath(cfg.DotfilesRepoPath)
			if expandErr == nil {
				fullPath := filepath.Join(expandedRepoPath, candidatePath)
				if _, statErr := os.Stat(fullPath); statErr == nil {
					recipeInfo = &config.LoadedRecipeInfo{
						Path: candidatePath,
						Dir:  filepath.Join(recipesDir, recipeName),
						Name: recipeName,
					}
					fmt.Fprintln(os.Stderr, color.YellowString("Note: recipe '%s' is disabled or filtered; proceeding with uninstall.", recipeName))
				}
			}
		}
		if recipeInfo == nil {
			fmt.Fprintln(os.Stderr, color.RedString("Recipe '%s' not found. Check 'ralph list recipes' for available recipes.", recipeName))
			return fmt.Errorf("recipe '%s' not found", recipeName)
		}

		// --- Step 2: Dependency guard ---
		depPhase := rpt.AddPhase("Dependency check")
		var dependents []string

		for name, build := range cfg.Hooks.Builds {
			if build.OwnerRecipe == recipeName {
				continue
			}
			for _, dep := range build.DependsOn {
				if isOwnedByRecipe(dep, cfg, recipeName) {
					dependents = append(dependents, fmt.Sprintf("build '%s' depends on %s", name, dep))
				}
			}
		}
		for name, pkg := range cfg.Packages {
			if pkg.OwnerRecipe == recipeName {
				continue
			}
			for _, dep := range pkg.DependsOn {
				if isOwnedByRecipe(dep, cfg, recipeName) {
					dependents = append(dependents, fmt.Sprintf("package '%s' depends on %s", name, dep))
				}
			}
		}

		if len(dependents) > 0 {
			fmt.Fprintln(os.Stderr, color.YellowString("Warning: other items depend on artifacts from recipe '%s':", recipeName))
			for _, d := range dependents {
				fmt.Fprintf(os.Stderr, "  - %s\n", d)
			}
			if !downForce {
				fmt.Fprintln(os.Stderr, color.RedString("Aborting. Use --force to proceed anyway."))
				depPhase.AddFail("dependency-guard", fmt.Sprintf("%d dependent item(s) found", len(dependents)), fmt.Errorf("dependency guard"))
				finishReport(rpt, nil, dryRun, verbose)
				return fmt.Errorf("dependency guard: %d dependent item(s) found", len(dependents))
			}
			depPhase.AddWarn("dependency-guard", fmt.Sprintf("%d dependent item(s) found (--force)", len(dependents)))
		} else {
			depPhase.AddOK("dependency-guard", "no dependents")
		}

		// --- Confirmation prompt ---
		if !dryRun && !downYes {
			fmt.Fprintf(uiOut(), "\nThis will remove tracked symlinks, copies, shell aliases/functions/env vars,\n")
			fmt.Fprintf(uiOut(), "and build state for recipe '%s', then set enable=false in config.toml.\n", recipeName)
			fmt.Fprintf(uiOut(), "Tip: run with --dry-run first to see exactly what will be removed.\n\n")
			fmt.Fprint(uiOut(), "Continue? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				fmt.Fprintln(uiOut(), "Aborted.")
				return nil
			}
		}

		// --- Step 3: Load recipe state ---
		prev, err := state.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error loading recipe state: %v", err))
			return fmt.Errorf("failed to load recipe state: %w", err)
		}
		if _, ok := prev.Recipes[recipeName]; !ok {
			fmt.Fprintln(os.Stderr, color.YellowString("Warning: recipe '%s' has no tracked state (continuing anyway).", recipeName))
		}

		// --- Step 4: Load raw recipe file for uninstall hooks ---
		expandedRepoPath, err := config.ExpandPath(cfg.DotfilesRepoPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error expanding dotfiles repo path: %v", err))
			return fmt.Errorf("failed to expand dotfiles repo path: %w", err)
		}
		recipeFilePath := filepath.Join(expandedRepoPath, recipeInfo.Path)

		rawRecipe, err := config.LoadRecipe(recipeFilePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, color.YellowString("Warning: could not load raw recipe file '%s': %v", recipeFilePath, err))
			rawRecipe = &config.Recipe{} // continue with empty recipe
		}

		// --- Step 5: Run pre_uninstall hooks ---
		if len(rawRecipe.Hooks.PreUninstall) > 0 {
			hookPhase := rpt.AddPhase("Pre-uninstall hooks")
			hookCtx := &hooks.HookContext{DryRun: dryRun}
			if err := hooks.RunHooks(w, rawRecipe.Hooks.PreUninstall, hooks.PreUninstall, hookCtx, dryRun); err != nil {
				if !downForce {
					fmt.Fprintln(os.Stderr, color.RedString("Pre-uninstall hooks failed: %v", err))
					fmt.Fprintln(os.Stderr, color.RedString("Aborting. Use --force to proceed anyway."))
					hookPhase.AddFail("pre-uninstall", err.Error(), err)
					finishReport(rpt, nil, dryRun, verbose)
					return fmt.Errorf("pre-uninstall hooks failed: %w", err)
				}
				fmt.Fprintln(os.Stderr, color.YellowString("Warning: pre-uninstall hooks failed: %v (continuing with --force)", err))
				hookPhase.AddWarn("pre-uninstall", fmt.Sprintf("failed: %v (--force)", err))
			} else {
				hookPhase.AddOK("pre-uninstall", "completed")
			}
		}

		// --- Step 6: Cleanup artifacts ---
		cleanupPhase := rpt.AddPhase("Cleanup")
		prevScoped := filterRecipe(prev, recipeName)
		nextScoped := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
		runCleanup(prevScoped, nextScoped, dryRun, w, cleanupPhase)

		// --- Step 7: Regenerate shell config ---
		shellPhase := rpt.AddPhase("Shell config")
		filtered := configWithoutRecipe(cfg, recipeName)
		resolvedShells := shell.ResolveShell(filtered.Shell.Name)

		// Generate env file once (shell-agnostic, shared path).
		envPath, envPathErr := shell.GetEnvFilePath()
		if envPathErr != nil {
			fmt.Fprintln(os.Stderr, color.YellowString("Warning: failed to get env file path: %v", envPathErr))
			shellPhase.AddWarn("env", fmt.Sprintf("env file path: %v", envPathErr))
		} else if envErr := shell.GenerateEnvFile(w, filtered.Shell.Env, envPath, dryRun); envErr != nil {
			fmt.Fprintln(os.Stderr, color.YellowString("Warning: failed to regenerate env file: %v", envErr))
			shellPhase.AddWarn("env", fmt.Sprintf("generate env file: %v", envErr))
			envPath = ""
		}

		for _, currentShell := range resolvedShells {
			aliasFile, funcFile, genErr := shell.GenerateShellConfigs(w, filtered, currentShell, dryRun)
			if genErr != nil {
				fmt.Fprintln(os.Stderr, color.YellowString("Warning: failed to regenerate shell configs for %s: %v", currentShell, genErr))
				shellPhase.AddWarn(string(currentShell), fmt.Sprintf("generate configs: %v", genErr))
				continue
			}
			linesToSource := []string{}
			if envPath != "" && len(filtered.Shell.Env) > 0 {
				linesToSource = append(linesToSource, fmt.Sprintf("source %s", toPortablePath(envPath)))
			}
			if aliasFile != "" && len(filtered.Shell.Aliases) > 0 {
				linesToSource = append(linesToSource, fmt.Sprintf("source %s", toPortablePath(aliasFile)))
			}
			if funcFile != "" && len(filtered.Shell.Functions) > 0 {
				linesToSource = append(linesToSource, fmt.Sprintf("source %s", toPortablePath(funcFile)))
			}
			if len(linesToSource) > 0 {
				if err := shell.InjectSourceLines(w, currentShell, linesToSource, dryRun); err != nil {
					fmt.Fprintln(os.Stderr, color.YellowString("Warning: failed to inject source lines for %s: %v", currentShell, err))
					shellPhase.AddWarn(string(currentShell), fmt.Sprintf("inject source lines: %v", err))
				} else {
					shellPhase.AddOK(string(currentShell), "regenerated")
				}
			} else {
				shellPhase.AddOK(string(currentShell), "no shell items remaining")
			}
		}

		// --- Step 8: Clean build state ---
		buildStatePhase := rpt.AddPhase("Build state")
		resetCount := 0
		for name := range rawRecipe.Hooks.Builds {
			if !dryRun {
				if err := hooks.ResetBuildStateForName(uiOut(), name); err != nil {
					fmt.Fprintln(os.Stderr, color.YellowString("Warning: failed to reset build state for '%s': %v", name, err))
				} else {
					resetCount++
				}
			} else {
				fmt.Fprintf(w, "[DRY RUN] Would reset build state for '%s'\n", name)
				resetCount++
			}
		}
		for name := range rawRecipe.Packages {
			key := "pkg:" + name
			if !dryRun {
				if err := hooks.ResetBuildStateForName(uiOut(), key); err != nil {
					fmt.Fprintln(os.Stderr, color.YellowString("Warning: failed to reset build state for '%s': %v", key, err))
				} else {
					resetCount++
				}
			} else {
				fmt.Fprintf(w, "[DRY RUN] Would reset build state for '%s'\n", key)
				resetCount++
			}
		}
		if resetCount > 0 {
			buildStatePhase.AddOK("build-state", fmt.Sprintf("reset %d entries", resetCount))
		} else {
			buildStatePhase.AddOK("build-state", "nothing to reset")
		}

		// --- Step 9: Run post_uninstall hooks ---
		if len(rawRecipe.Hooks.PostUninstall) > 0 {
			postPhase := rpt.AddPhase("Post-uninstall hooks")
			hookCtx := &hooks.HookContext{DryRun: dryRun}
			if err := hooks.RunHooks(w, rawRecipe.Hooks.PostUninstall, hooks.PostUninstall, hookCtx, dryRun); err != nil {
				fmt.Fprintln(os.Stderr, color.YellowString("Warning: post-uninstall hooks failed: %v", err))
				postPhase.AddWarn("post-uninstall", err.Error())
			} else {
				postPhase.AddOK("post-uninstall", "completed")
			}
		}

		// --- Step 10: Update state ---
		statePhase := rpt.AddPhase("State")
		if !dryRun {
			prev.DeleteRecipe(recipeName)
			if err := state.Save(prev); err != nil {
				fmt.Fprintln(os.Stderr, color.YellowString("Warning: could not save recipe state: %v", err))
				statePhase.AddWarn("save", err.Error())
			} else {
				statePhase.AddOK("save", "recipe removed from state")
			}
		} else {
			fmt.Fprintf(w, "[DRY RUN] Would remove recipe '%s' from state\n", recipeName)
			statePhase.AddOK("save", "[DRY RUN] would remove from state")
		}

		// --- Step 11: Disable in config.toml ---
		configPhase := rpt.AddPhase("Config")
		if !dryRun {
			configPath, err := config.GetDefaultConfigPath()
			if err != nil {
				fmt.Fprintln(os.Stderr, color.YellowString("Warning: could not determine config path: %v", err))
				configPhase.AddWarn("disable", fmt.Sprintf("config path: %v", err))
			} else {
				if err := config.SetRecipeOverride(configPath, recipeName, false); err != nil {
					fmt.Fprintln(os.Stderr, color.YellowString("Warning: could not disable recipe in config.toml: %v", err))
					configPhase.AddWarn("disable", err.Error())
				} else {
					configPhase.AddOK("disable", fmt.Sprintf("recipe '%s' disabled in config.toml", recipeName))
				}
			}
		} else {
			fmt.Fprintf(w, "[DRY RUN] Would disable recipe '%s' in config.toml\n", recipeName)
			configPhase.AddOK("disable", "[DRY RUN] would disable in config.toml")
		}

		// --- Step 12: Print report ---
		if outputJSON() {
			_ = rpt.WriteJSON(os.Stdout, dryRun)
		} else {
			fmt.Println("")
			if dryRun {
				color.Cyan("DRY RUN: ralph down finished. No actual changes were made.")
			} else {
				printReportSummary(rpt)
			}
			if rpt.HasFailures() || rpt.HasWarnings() || verbose || dryRun {
				rpt.PrintSummary(os.Stdout, summaryVerbosity())
			}
		}
		if code := rpt.ExitCode(); code != 0 {
			return &ExitError{Code: code}
		}
		return nil
	},
}

// isOwnedByRecipe checks whether a dependency reference (e.g. "builds.foo" or
// "packages.bar") refers to an item owned by the given recipe.
func isOwnedByRecipe(dep string, cfg *config.Config, recipeName string) bool {
	parts := strings.SplitN(dep, ".", 2)
	if len(parts) != 2 {
		return false
	}
	kind, name := parts[0], parts[1]

	switch kind {
	case "builds":
		if b, ok := cfg.Hooks.Builds[name]; ok {
			return b.OwnerRecipe == recipeName
		}
	case "packages":
		if p, ok := cfg.Packages[name]; ok {
			return p.OwnerRecipe == recipeName
		}
	}
	return false
}

// configWithoutRecipe returns a shallow copy of cfg with all items owned by
// recipeName removed from shell aliases and functions. Used to regenerate
// shell config files without the recipe's contributions.
func configWithoutRecipe(cfg *config.Config, recipeName string) *config.Config {
	filtered := *cfg

	// Filter aliases
	if len(cfg.Shell.Aliases) > 0 {
		aliases := make(map[string]config.ShellAlias, len(cfg.Shell.Aliases))
		for name, alias := range cfg.Shell.Aliases {
			if alias.OwnerRecipe != recipeName {
				aliases[name] = alias
			}
		}
		filtered.Shell.Aliases = aliases
	}

	// Filter functions
	if len(cfg.Shell.Functions) > 0 {
		functions := make(map[string]config.ShellFunction, len(cfg.Shell.Functions))
		for name, fn := range cfg.Shell.Functions {
			if fn.OwnerRecipe != recipeName {
				functions[name] = fn
			}
		}
		filtered.Shell.Functions = functions
	}

	// Filter env vars using the EnvOwners map populated during recipe merge.
	if len(cfg.Shell.Env) > 0 && len(cfg.Shell.EnvOwners) > 0 {
		env := make(map[string]string, len(cfg.Shell.Env))
		for name, val := range cfg.Shell.Env {
			if cfg.Shell.EnvOwners[name] != recipeName {
				env[name] = val
			}
		}
		filtered.Shell.Env = env
	}

	return &filtered
}

func init() {
	rootCmd.AddCommand(downCmd)
	downCmd.Flags().BoolVar(&downForce, "force", false, "Bypass dependency guard and pre_uninstall failures")
	downCmd.Flags().BoolVarP(&downYes, "yes", "y", false, "Skip confirmation prompt")
}
