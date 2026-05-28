package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

// buildIntendedManifest computes the artifact manifest a successful apply
// of `cfg` on `currentHost` would produce, scoped to items that came from
// a recipe (OwnerRecipe set). Items defined directly in the main config
// (no recipe) are intentionally excluded — there's no recipe to remove
// them when they go away.
//
// Disabled items are skipped, so they aren't tracked as owned and will be
// treated as orphans on the next apply (the desired behavior — disabling a
// recipe should clean it up). Recipe-level host-filtered recipes are handled
// separately via carryForwardFrozenRecipes: their artifacts are frozen, not
// orphaned, because they belong to other hosts.
//
// Returns an error if an intended artifact set cannot be enumerated (e.g. a
// dirs_mirror source directory is unreadable). Callers must abort cleanup on
// error rather than diff against an incomplete manifest, which would treat
// live artifacts as orphans and delete them.
func buildIntendedManifest(cfg *config.Config, currentHost string, now time.Time) (*state.RecipeState, error) {
	s := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	// Pre-populate per-recipe metadata so abandon/delete behavior is
	// preserved even for recipes whose entire artifact set was emptied
	// this apply (rare, but possible).
	for _, info := range cfg.LoadedRecipes {
		behavior := info.DeleteBehavior
		if behavior == "" {
			behavior = config.DeleteBehaviorDelete
		}
		s.SetMetadata(info.Name, now, behavior)
	}

	for name, df := range cfg.Dotfiles {
		if df.OwnerRecipe == "" || !config.IsEnabled(df.Enable) || !config.ShouldApplyForHost(df.Hosts, currentHost) {
			continue
		}
		target, err := config.ExpandPath(df.Target)
		if err != nil {
			continue
		}
		kind := state.KindSymlink
		switch df.Action {
		case "copy":
			kind = state.KindCopy
		case "symlink_dir":
			kind = state.KindDirSymlink
		}
		s.AddArtifact(df.OwnerRecipe, kind, target)
		_ = name
	}

	for _, dm := range cfg.DirsMirror {
		if dm.OwnerRecipe == "" || !config.IsEnabled(dm.Enable) || !config.ShouldApplyForHost(dm.Hosts, currentHost) {
			continue
		}
		// Walk the source directory to discover individual symlink targets
		expandedRepo, err := config.ExpandPath(cfg.DotfilesRepoPath)
		if err != nil {
			continue
		}
		absoluteSource := filepath.Join(expandedRepo, dm.Source)
		entries, err := os.ReadDir(absoluteSource)
		if err != nil {
			return nil, fmt.Errorf("cleanup: cannot read dirs_mirror source %q for recipe %q: %w", absoluteSource, dm.OwnerRecipe, err)
		}
		expandedTarget, err := config.ExpandPath(dm.Target)
		if err != nil {
			continue
		}
		action := dm.Action
		if action == "" {
			action = "symlink"
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			entryTarget := filepath.Join(expandedTarget, entry.Name())
			kind := state.KindSymlink
			if action == "symlink_dir" {
				kind = state.KindDirSymlink
			}
			s.AddArtifact(dm.OwnerRecipe, kind, entryTarget)
		}
	}

	for name, dir := range cfg.Directories {
		if dir.OwnerRecipe == "" || !config.IsEnabled(dir.Enable) || !config.ShouldApplyForHost(dir.Hosts, currentHost) {
			continue
		}
		target, err := config.ExpandPath(dir.Target)
		if err != nil {
			continue
		}
		s.AddArtifact(dir.OwnerRecipe, state.KindDirectory, target)
		_ = name
	}

	for name, r := range cfg.Repos {
		if r.OwnerRecipe == "" || !config.IsEnabled(r.Enable) || !config.ShouldApplyForHost(r.Hosts, currentHost) {
			continue
		}
		target, err := config.ExpandPath(r.Target)
		if err != nil {
			continue
		}
		s.AddArtifact(r.OwnerRecipe, state.KindRepo, target)
		_ = name
	}

	for i := range cfg.Tools {
		t := &cfg.Tools[i]
		if !config.IsEnabled(t.Enable) || !config.ShouldApplyForHost(t.Hosts, currentHost) {
			continue
		}
		for _, df := range t.ConfigFiles {
			if df.OwnerRecipe == "" || !config.IsEnabled(df.Enable) || !config.ShouldApplyForHost(df.Hosts, currentHost) {
				continue
			}
			target, err := config.ExpandPath(df.Target)
			if err != nil {
				continue
			}
			kind := state.KindSymlink
			switch df.Action {
			case "copy":
				kind = state.KindCopy
			case "symlink_dir":
				kind = state.KindDirSymlink
			}
			s.AddArtifact(df.OwnerRecipe, kind, target)
		}
	}

	for name, alias := range cfg.Shell.Aliases {
		if alias.OwnerRecipe == "" || !config.IsEnabled(alias.Enable) || !config.ShouldApplyForHost(alias.Hosts, currentHost) {
			continue
		}
		s.AddArtifact(alias.OwnerRecipe, state.KindShellAlias, name)
	}

	for name, fn := range cfg.Shell.Functions {
		if fn.OwnerRecipe == "" || !config.IsEnabled(fn.Enable) || !config.ShouldApplyForHost(fn.Hosts, currentHost) {
			continue
		}
		s.AddArtifact(fn.OwnerRecipe, state.KindShellFunc, name)
	}

	// shell.env is a flat map[string]string so there's no per-entry owner
	// today. Skipping for v1 — env vars are rarely orphaned in practice
	// and the right fix is to upgrade ShellConfig.Env to a typed struct
	// later.

	for name, pkg := range cfg.Packages {
		if pkg.OwnerRecipe == "" || !config.IsEnabled(pkg.Enable) || !config.ShouldApplyForHost(pkg.Hosts, currentHost) {
			continue
		}
		s.AddArtifact(pkg.OwnerRecipe, state.KindPackage, name)
		for _, ip := range pkg.InstallPaths {
			expanded, err := config.ExpandPath(ip)
			if err != nil {
				continue
			}
			s.AddArtifact(pkg.OwnerRecipe, state.KindInstallPath, expanded)
		}
	}

	for name, b := range cfg.Hooks.Builds {
		if b.OwnerRecipe == "" || !config.IsEnabled(b.Enable) || !config.ShouldApplyForHost(b.Hosts, currentHost) {
			continue
		}
		s.AddArtifact(b.OwnerRecipe, state.KindBuild, name)
		for _, ip := range b.InstallPaths {
			expanded, err := config.ExpandPath(ip)
			if err != nil {
				continue
			}
			s.AddArtifact(b.OwnerRecipe, state.KindInstallPath, expanded)
		}
	}

	return s, nil
}

// frozenRecipeSet returns the set of recipe names that are host-filtered on
// this host. Their artifacts must not be treated as orphans during cleanup.
func frozenRecipeSet(cfg *config.Config) map[string]bool {
	if len(cfg.HostFilteredRecipes) == 0 {
		return nil
	}
	frozen := make(map[string]bool, len(cfg.HostFilteredRecipes))
	for _, name := range cfg.HostFilteredRecipes {
		frozen[name] = true
	}
	return frozen
}

// carryForwardFrozenRecipes copies each frozen recipe's previously-recorded
// artifacts from prev into next when next doesn't already track them. This
// keeps host-filtered recipes (which were skipped this apply because they
// belong to other hosts) out of the orphan diff and preserves their entries
// in the saved state for future runs.
func carryForwardFrozenRecipes(prev, next *state.RecipeState, frozen map[string]bool) {
	if prev == nil || next == nil || len(frozen) == 0 {
		return
	}
	if next.Recipes == nil {
		next.Recipes = map[string]state.RecipeArtifacts{}
	}
	for name := range frozen {
		if _, alreadyTracked := next.Recipes[name]; alreadyTracked {
			continue
		}
		if art, ok := prev.Recipes[name]; ok {
			next.Recipes[name] = art
		}
	}
}

// runCleanup applies the diff between prev and next manifests, honoring
// each recipe's delete_behavior. Filesystem-removable kinds (symlinks,
// copies, directories, install_paths) go through state.SafeRemove.
// Repos are always abandoned (auto-removal disabled in v1).
// Shell aliases/functions/env, packages, and builds are tracked but their
// "removal" is "stop emitting" — handled implicitly by the corresponding
// generators (shell.GenerateShellConfigs, etc.) when the recipe drops out
// of the merged config. This phase logs them as abandoned-by-design so
// the user can see they were noticed.
//
// Reports counts via the provided phase. logger receives per-action lines
// (one per file removed/abandoned).
func runCleanup(prev, next *state.RecipeState, dryRun bool, logger io.Writer, phase *report.Phase) {
	orphans := state.Diff(prev, next)
	if len(orphans) == 0 {
		phase.AddOK("cleanup", "no orphans")
		return
	}

	// Stable iteration order for deterministic reports/logs.
	names := make([]string, 0, len(orphans))
	for n := range orphans {
		names = append(names, n)
	}
	sort.Strings(names)

	totalRemoved := 0
	totalAbandoned := 0
	for _, recipeName := range names {
		art := orphans[recipeName]
		behavior := resolveBehavior(recipeName, next, art)

		if behavior == config.DeleteBehaviorAbandon {
			abandonAll(logger, recipeName, art)
			totalAbandoned += countAll(art)
			// Configured intent, not an error condition — keep as OK so
			// the apply exit code stays clean.
			phase.AddOK(recipeName, fmt.Sprintf("abandoned %d artifact(s) (delete_behavior=abandon)", countAll(art)))
			continue
		}

		removed := 0
		abandoned := 0
		opts := state.SafeRemoveOptions{DryRun: dryRun, Logger: logger}

		removed += removeAll(opts, recipeName, state.KindSymlink, art.Symlinks, &abandoned, logger)
		removed += removeAll(opts, recipeName, state.KindDirSymlink, art.DirSymlinks, &abandoned, logger)
		removed += removeAll(opts, recipeName, state.KindCopy, art.Copies, &abandoned, logger)
		removed += removeAll(opts, recipeName, state.KindInstallPath, art.InstallPaths, &abandoned, logger)
		// Directories last: any nested artifacts above must be removed
		// first so the directory is empty when SafeRemove inspects it.
		removed += removeAll(opts, recipeName, state.KindDirectory, art.Directories, &abandoned, logger)

		// These kinds are tracked but never auto-removed — log them so
		// the user knows ralph noticed.
		for _, p := range art.Repos {
			fmt.Fprintf(logger, "abandoned repo: %s (recipe %s; auto-removal disabled in v1)\n", p, recipeName)
			abandoned++
		}
		for _, name := range art.ShellAliases {
			fmt.Fprintf(logger, "abandoned shell alias: %s (recipe %s; cleared on next shell config regeneration)\n", name, recipeName)
			abandoned++
		}
		for _, name := range art.ShellFunctions {
			fmt.Fprintf(logger, "abandoned shell function: %s (recipe %s; cleared on next shell config regeneration)\n", name, recipeName)
			abandoned++
		}
		for _, name := range art.ShellEnv {
			fmt.Fprintf(logger, "abandoned shell env var: %s (recipe %s)\n", name, recipeName)
			abandoned++
		}
		for _, name := range art.Packages {
			fmt.Fprintf(logger, "abandoned package: %s (recipe %s; declare install_paths to enable cleanup)\n", name, recipeName)
			abandoned++
		}
		for _, name := range art.Builds {
			fmt.Fprintf(logger, "abandoned build: %s (recipe %s; declare install_paths to enable cleanup)\n", name, recipeName)
			abandoned++
		}

		totalRemoved += removed
		totalAbandoned += abandoned
		phase.AddOK(recipeName, fmt.Sprintf("removed %d, abandoned %d", removed, abandoned))
	}

	summary := fmt.Sprintf("%d artifact(s) removed, %d abandoned across %d recipe(s)", totalRemoved, totalAbandoned, len(orphans))
	if dryRun {
		summary = "[DRY RUN] " + summary
	}
	phase.AddOK("cleanup", summary)
}

// resolveBehavior returns the effective delete_behavior for a recipe with
// orphans. If the recipe is still in the next manifest, that wins; otherwise
// the value carried forward from the previous state is used. Empty defaults
// to "delete".
func resolveBehavior(recipeName string, next *state.RecipeState, art state.RecipeArtifacts) string {
	if cur, ok := next.Recipes[recipeName]; ok && cur.DeleteBehavior != "" {
		return cur.DeleteBehavior
	}
	if art.DeleteBehavior != "" {
		return art.DeleteBehavior
	}
	return config.DeleteBehaviorDelete
}

// removeAll runs SafeRemove for each value, returning the number successfully
// removed. Failures are logged and counted as abandoned (incremented via the
// caller's pointer) so the user sees what the rails rejected without the
// command exit code reflecting it.
func removeAll(opts state.SafeRemoveOptions, recipeName string, kind state.ArtifactKind, values []string, abandoned *int, logger io.Writer) int {
	removed := 0
	for _, p := range values {
		err := state.SafeRemove(p, kind, opts)
		if err != nil {
			fmt.Fprintf(logger, "skip %s %s (recipe %s): %v\n", kind, p, recipeName, err)
			*abandoned++
			continue
		}
		removed++
	}
	return removed
}

// abandonAll logs every artifact in `art` as abandoned, used when the recipe
// itself is configured with delete_behavior = "abandon".
func abandonAll(logger io.Writer, recipeName string, art state.RecipeArtifacts) {
	emit := func(kind string, vs []string) {
		for _, v := range vs {
			fmt.Fprintf(logger, "abandoned %s: %s (recipe %s; delete_behavior=abandon)\n", kind, v, recipeName)
		}
	}
	emit("symlink", art.Symlinks)
	emit("dir_symlink", art.DirSymlinks)
	emit("copy", art.Copies)
	emit("directory", art.Directories)
	emit("install_path", art.InstallPaths)
	emit("repo", art.Repos)
	emit("shell_alias", art.ShellAliases)
	emit("shell_function", art.ShellFunctions)
	emit("shell_env", art.ShellEnv)
	emit("package", art.Packages)
	emit("build", art.Builds)
}

func countAll(art state.RecipeArtifacts) int {
	return len(art.Symlinks) +
		len(art.DirSymlinks) +
		len(art.Copies) +
		len(art.Directories) +
		len(art.InstallPaths) +
		len(art.Repos) +
		len(art.ShellAliases) +
		len(art.ShellFunctions) +
		len(art.ShellEnv) +
		len(art.Packages) +
		len(art.Builds)
}

// cleanupBanner is printed when the cleanup phase activates so the user
// knows what's happening (and what flag toggled it).
func cleanupBanner(w io.Writer) {
	color.New(color.FgCyan).Fprintln(w, "\nProcessing recipe cleanup (--enable-cleanup)...")
}
