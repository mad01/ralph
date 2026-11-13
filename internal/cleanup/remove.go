package cleanup

import (
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/hooks"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

// RunOptions controls one Run call.
type RunOptions struct {
	// Protected holds the paths the active config still declares (see
	// ProtectedPaths). Run never removes one; nil protects nothing.
	Protected map[string]bool
	DryRun    bool          // say what would be removed; remove nothing
	Out       io.Writer     // per-artifact log lines; nil discards them
	Phase     *report.Phase // receives a step per orphaned recipe and a summary
}

// out returns the writer for per-artifact log lines.
func (o RunOptions) out() io.Writer {
	if o.Out == nil {
		return io.Discard
	}
	return o.Out
}

// Run applies the diff between prev and next manifests, honoring each
// recipe's delete_behavior. Filesystem-removable kinds (symlinks, copies,
// directories, install_paths) go through state.SafeRemove. Repos are always
// abandoned (auto-removal disabled in v1). Shell aliases/functions/env,
// packages, and builds are tracked but their "removal" is "stop emitting":
// handled implicitly by the corresponding generators
// (shell.GenerateShellConfigs, etc.) when the recipe drops out of the merged
// config. Run logs them as abandoned-by-design so the user can see they were
// noticed.
//
// Reports counts via opts.Phase. opts.Out receives per-action lines (one per
// file removed/abandoned).
//
// opts.Protected is the cross-recipe set of paths still declared by the
// active config (see ProtectedPaths); any such path is kept, never removed,
// even when prev attributed it to a now-orphaned recipe name. The caller
// passes it explicitly so `ralph clean --recipe X` can protect against the
// FULL manifest, not just the single recipe it scoped the diff to.
//
// Returns the artifacts that failed removal, keyed by recipe, so the caller can
// re-track them (state.MergeRetry) and retry on the next run instead of dropping
// them from the manifest as permanent garbage after one transient failure.
func Run(prev, next *state.RecipeState, opts RunOptions) map[string]state.RecipeArtifacts {
	orphans := state.Diff(prev, next)
	if len(orphans) == 0 {
		opts.Phase.AddOK("cleanup", "no orphans")
		return nil
	}

	// Stable iteration order for deterministic reports/logs.
	names := slices.Sorted(maps.Keys(orphans))

	failed := map[string]state.RecipeArtifacts{}
	totalRemoved := 0
	totalAbandoned := 0
	for _, recipeName := range names {
		art := orphans[recipeName]
		behavior := resolveBehavior(recipeName, next, art)

		// pre_uninstall runs before any artifact removal, regardless of
		// delete_behavior: these hooks clean up external state (registered
		// MCP servers, t-man agents, git hooks) that ralph doesn't track. Diff
		// only attaches uninstall hooks when the recipe is gone entirely, so a
		// recipe that merely lost one artifact won't reach here with hooks.
		runUninstallHooks(opts.out(), recipeName, art.PreUninstall, "pre_uninstall", opts.DryRun)

		if behavior == config.DeleteBehaviorAbandon {
			totalAbandoned += abandonRecipe(opts, recipeName, art)
			continue
		}

		removed, abandoned, fail := deleteRecipe(opts, recipeName, art)
		fail.DeleteBehavior = string(behavior)
		if fail.HasAny() {
			failed[recipeName] = fail
		}
		totalRemoved += removed
		totalAbandoned += abandoned
	}

	summary := fmt.Sprintf(
		"%d artifact(s) removed, %d abandoned across %d recipe(s)",
		totalRemoved,
		totalAbandoned,
		len(orphans),
	)
	if opts.DryRun {
		summary = "[DRY RUN] " + summary
	}
	opts.Phase.AddOK("cleanup", summary)
	return failed
}

// abandonRecipe leaves every artifact of a delete_behavior = "abandon" recipe
// in place, logs each one, runs the recipe's post_uninstall hooks, and returns
// how many artifacts it abandoned.
func abandonRecipe(opts RunOptions, recipeName string, art state.RecipeArtifacts) int {
	abandonAll(opts.out(), recipeName, art)
	runUninstallHooks(opts.out(), recipeName, art.PostUninstall, "post_uninstall", opts.DryRun)
	// Configured intent, not an error condition: keep as OK so
	// the apply exit code stays clean.
	opts.Phase.AddOK(
		recipeName,
		fmt.Sprintf("abandoned %d artifact(s) (delete_behavior=abandon)", countAll(art)),
	)
	return countAll(art)
}

// deleteRecipe removes the filesystem artifacts of a delete_behavior =
// "delete" recipe, logs the ones never auto-removed, runs the recipe's
// post_uninstall hooks, and returns how many artifacts it removed and kept,
// and the paths whose removal failed.
func deleteRecipe(
	opts RunOptions,
	recipeName string,
	art state.RecipeArtifacts,
) (removed, abandoned int, fail state.RecipeArtifacts) {
	logger := opts.out()
	removeOpts := state.SafeRemoveOptions{DryRun: opts.DryRun, Logger: logger}
	removed, abandoned, fail = removeArtifacts(removeOpts, recipeName, art, opts.Protected, logger)
	abandoned += logAbandoned(logger, recipeName, art)

	runUninstallHooks(logger, recipeName, art.PostUninstall, "post_uninstall", opts.DryRun)

	opts.Phase.AddOK(recipeName, fmt.Sprintf("removed %d, abandoned %d", removed, abandoned))
	return removed, abandoned, fail
}

// removeArtifacts removes each filesystem artifact of one orphaned recipe
// through removePaths and returns how many it removed, how many it kept, and
// the paths whose removal failed.
func removeArtifacts(
	opts state.SafeRemoveOptions,
	recipeName string,
	art state.RecipeArtifacts,
	protected map[string]bool,
	logger io.Writer,
) (removed, abandoned int, fail state.RecipeArtifacts) {
	// Directories last: any nested artifacts above must be removed
	// first so the directory is empty when SafeRemove inspects it.
	kinds := []struct {
		kind   state.ArtifactKind
		paths  []string
		failed *[]string
	}{
		{state.KindSymlink, art.Symlinks, &fail.Symlinks},
		{state.KindDirSymlink, art.DirSymlinks, &fail.DirSymlinks},
		{state.KindCopy, art.Copies, &fail.Copies},
		{state.KindInstallPath, art.InstallPaths, &fail.InstallPaths},
		{state.KindDirectory, art.Directories, &fail.Directories},
	}
	for _, k := range kinds {
		removed += removePaths(
			opts, recipeName, k.kind, k.paths, protected, &abandoned, k.failed, logger,
		)
	}
	return removed, abandoned, fail
}

// logAbandoned logs the artifacts of one orphaned recipe that are tracked but
// never auto-removed, so the user knows ralph noticed them, and returns how
// many it logged.
func logAbandoned(logger io.Writer, recipeName string, art state.RecipeArtifacts) int {
	const regen = "cleared on next shell config regeneration"
	const declare = "declare install_paths to enable cleanup"
	// Each format takes the artifact as %[1]s and the recipe as %[2]s.
	groups := []struct {
		items  []string
		format string
	}{
		{
			art.PackageClones,
			"abandoned package clone: %[1]s (recipe %[2]s; run 'rm -rf %[1]s' to remove)\n",
		},
		{art.Repos, "abandoned repo: %[1]s (recipe %[2]s; auto-removal disabled in v1)\n"},
		{art.ShellAliases, "abandoned shell alias: %[1]s (recipe %[2]s; " + regen + ")\n"},
		{art.ShellFunctions, "abandoned shell function: %[1]s (recipe %[2]s; " + regen + ")\n"},
		{art.ShellEnv, "abandoned shell env var: %[1]s (recipe %[2]s)\n"},
		{art.Packages, "abandoned package: %[1]s (recipe %[2]s; " + declare + ")\n"},
		{art.Builds, "abandoned build: %[1]s (recipe %[2]s; " + declare + ")\n"},
	}
	abandoned := 0
	for _, g := range groups {
		for _, item := range g.items {
			fmt.Fprintf(logger, g.format, item, recipeName)
			abandoned++
		}
	}
	return abandoned
}

// runUninstallHooks runs a recipe's persisted pre/post uninstall hook commands
// during cleanup. Failures are warn-level: a hook that fails must not abort
// cleanup or flip the apply's exit code. Removing the artifacts is the
// primary goal, and the external state these hooks touch is best-effort.
func runUninstallHooks(
	logger io.Writer,
	recipeName string,
	scripts []string,
	label string,
	dryRun bool,
) {
	if len(scripts) == 0 {
		return
	}
	fmt.Fprintf(logger, "Running %s hooks for recipe '%s'...\n", label, recipeName)
	for _, script := range scripts {
		if err := hooks.Run(logger, script, &hooks.HookContext{DryRun: dryRun}, dryRun); err != nil {
			fmt.Fprintf(
				logger,
				"warning: %s hook failed for recipe '%s': %v\n",
				label,
				recipeName,
				err,
			)
		}
	}
}

// resolveBehavior returns the effective delete_behavior for a recipe with
// orphans. If the recipe is still in the next manifest, that wins; otherwise
// the value carried forward from the previous state is used. Empty defaults
// to "delete".
func resolveBehavior(
	recipeName string,
	next *state.RecipeState,
	art state.RecipeArtifacts,
) config.DeleteBehavior {
	if cur, ok := next.Recipes[recipeName]; ok && cur.DeleteBehavior != "" {
		return config.DeleteBehavior(cur.DeleteBehavior)
	}
	return config.DeleteBehavior(art.DeleteBehavior).OrDelete()
}

// removePaths runs SafeRemove for each path of the given kind, returning the
// number successfully removed. It enforces two rules:
//
//   - A path still declared by the active config (`protected`) is kept
//     untouched and logged. This is the cross-recipe guard: it protects a
//     still-needed binary or dotfile (notably the running ralph binary, and
//     any artifact a renamed recipe just re-created) when prev attributed it
//     to a recipe name that no longer matches.
//   - A genuine SafeRemove failure is logged, counted via *abandoned, AND
//     appended to *failed so the caller can re-track it for a retry next run.
//     An already-gone path returns nil from SafeRemove and counts as removed.
func removePaths(
	opts state.SafeRemoveOptions,
	recipeName string,
	kind state.ArtifactKind,
	paths []string,
	protected map[string]bool,
	abandoned *int,
	failed *[]string,
	logger io.Writer,
) int {
	removed := 0
	for _, p := range paths {
		if protected[filepath.Clean(p)] {
			fmt.Fprintf(
				logger,
				"protected %s: %s (recipe %s; still declared by an active recipe)\n",
				kind,
				p,
				recipeName,
			)
			continue
		}
		if err := state.SafeRemove(p, kind, opts); err != nil {
			fmt.Fprintf(logger, "skip %s %s (recipe %s): %v\n", kind, p, recipeName, err)
			*abandoned++
			*failed = append(*failed, p)
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
			fmt.Fprintf(
				logger,
				"abandoned %s: %s (recipe %s; delete_behavior=abandon)\n",
				kind,
				v,
				recipeName,
			)
		}
	}
	emit("symlink", art.Symlinks)
	emit("dir_symlink", art.DirSymlinks)
	emit("copy", art.Copies)
	emit("directory", art.Directories)
	emit("install_path", art.InstallPaths)
	emit("package_clone", art.PackageClones)
	emit("repo", art.Repos)
	emit("shell_alias", art.ShellAliases)
	emit("shell_function", art.ShellFunctions)
	emit("shell_env", art.ShellEnv)
	emit("package", art.Packages)
	emit("build", art.Builds)
}

// countAll returns the number of artifacts of every kind in art.
func countAll(art state.RecipeArtifacts) int {
	return len(art.Symlinks) +
		len(art.DirSymlinks) +
		len(art.Copies) +
		len(art.Directories) +
		len(art.InstallPaths) +
		len(art.PackageClones) +
		len(art.Repos) +
		len(art.ShellAliases) +
		len(art.ShellFunctions) +
		len(art.ShellEnv) +
		len(art.Packages) +
		len(art.Builds)
}
