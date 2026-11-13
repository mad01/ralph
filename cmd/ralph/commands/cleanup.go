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
	"github.com/mad01/ralph/internal/hooks"
	"github.com/mad01/ralph/internal/packages"
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
// orphaned, because they belong to other hosts. Item-level host- and
// profile-filtered items are excluded here, so their previously applied
// artifacts become orphans and are cleaned up — unlike a recipe-level
// host/profile mismatch, which freezes.
//
// Returns an error if an intended artifact set cannot be enumerated (e.g. a
// dirs_mirror source directory is unreadable). Callers must abort cleanup on
// error rather than diff against an incomplete manifest, which would treat
// live artifacts as orphans and delete them.
//
// The second result is the set of filesystem paths declared by active items
// that have no owner recipe (declared straight in config.toml). They are never
// recorded in state, but cleanup must protect them: see protectedPaths.
func buildIntendedManifest(
	cfg *config.Config,
	currentHost string,
	now time.Time,
) (*state.RecipeState, map[string]bool, error) {
	s := &state.RecipeState{
		Version: state.CurrentRecipeStateVersion,
		Recipes: map[string]state.RecipeArtifacts{},
	}
	unowned := map[string]bool{}
	// track records a filesystem artifact under its owner recipe, or in
	// unowned when the item has none.
	track := func(owner string, kind state.ArtifactKind, path string) {
		if owner == "" {
			unowned[filepath.Clean(path)] = true
			return
		}
		s.AddArtifact(owner, kind, path)
	}

	// Pre-populate per-recipe metadata so abandon/delete behavior is
	// preserved even for recipes whose entire artifact set was emptied
	// this apply (rare, but possible).
	for _, info := range cfg.LoadedRecipes {
		behavior := info.DeleteBehavior
		if behavior == "" {
			behavior = config.DeleteBehaviorDelete
		}
		s.SetMetadata(info.Name, now, behavior)
		s.SetRecipeSource(info.Name, info.RecipeSource)
		// Persist uninstall hooks so cleanup can run them when this recipe
		// later becomes an orphan — even if its recipe.toml is gone by then.
		s.SetUninstallHooks(info.Name, info.PreUninstall, info.PostUninstall)
	}

	for name, df := range cfg.Dotfiles {
		if !config.Applies(df.Enable, df.Hosts, df.Profiles, currentHost, cfg.Profiles) {
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
		track(df.OwnerRecipe, kind, target)
		_ = name
	}

	for _, dm := range cfg.DirsMirror {
		if !config.Applies(dm.Enable, dm.Hosts, dm.Profiles, currentHost, cfg.Profiles) {
			continue
		}
		// Walk the source directory to discover individual symlink targets
		expandedRepo, err := config.ExpandPath(cfg.DotfilesRepoPath)
		if err != nil {
			continue
		}
		absoluteSource := config.JoinSourcePath(expandedRepo, dm.Source)
		entries, err := os.ReadDir(absoluteSource)
		if err != nil {
			if dm.OwnerRecipe == "" {
				// Nothing of an ownerless mirror is recorded, so an unreadable
				// source cannot orphan anything; it only goes unprotected.
				continue
			}
			return nil, nil, fmt.Errorf(
				"cleanup: cannot read dirs_mirror source %q for recipe %q: %w",
				absoluteSource,
				dm.OwnerRecipe,
				err,
			)
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
			track(dm.OwnerRecipe, kind, entryTarget)
		}
	}

	for name, dir := range cfg.Directories {
		if !config.Applies(dir.Enable, dir.Hosts, dir.Profiles, currentHost, cfg.Profiles) {
			continue
		}
		target, err := config.ExpandPath(dir.Target)
		if err != nil {
			continue
		}
		track(dir.OwnerRecipe, state.KindDirectory, target)
		_ = name
	}

	for name, r := range cfg.Repos {
		if r.OwnerRecipe == "" ||
			!config.Applies(r.Enable, r.Hosts, r.Profiles, currentHost, cfg.Profiles) {
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
		if !config.Applies(t.Enable, t.Hosts, t.Profiles, currentHost, cfg.Profiles) {
			continue
		}
		for _, df := range t.ConfigFiles {
			if !config.Applies(df.Enable, df.Hosts, df.Profiles, currentHost, cfg.Profiles) {
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
			track(df.OwnerRecipe, kind, target)
		}
	}

	for name, alias := range cfg.Shell.Aliases {
		if alias.OwnerRecipe == "" ||
			!config.Applies(alias.Enable, alias.Hosts, alias.Profiles, currentHost, cfg.Profiles) {
			continue
		}
		s.AddArtifact(alias.OwnerRecipe, state.KindShellAlias, name)
	}

	for name, fn := range cfg.Shell.Functions {
		if fn.OwnerRecipe == "" ||
			!config.Applies(fn.Enable, fn.Hosts, fn.Profiles, currentHost, cfg.Profiles) {
			continue
		}
		s.AddArtifact(fn.OwnerRecipe, state.KindShellFunc, name)
	}

	// shell.env is a flat map[string]string so there's no per-entry owner
	// today. Skipping for v1 — env vars are rarely orphaned in practice
	// and the right fix is to upgrade ShellConfig.Env to a typed struct
	// later.

	for name, pkg := range cfg.Packages {
		if !config.Applies(pkg.Enable, pkg.Hosts, pkg.Profiles, currentHost, cfg.Profiles) {
			continue
		}
		for _, ip := range pkg.InstallPaths {
			expanded, err := config.ExpandPath(ip)
			if err != nil {
				continue
			}
			track(pkg.OwnerRecipe, state.KindInstallPath, expanded)
		}
		if pkg.OwnerRecipe == "" {
			continue
		}
		s.AddArtifact(pkg.OwnerRecipe, state.KindPackage, name)
		// Remote/make packages clone into packages_dir. Track that clone dir so
		// removing the package surfaces it as an orphan (abandon-with-log) — it
		// is otherwise invisible and leaks a full checkout. ResolvePackagePaths
		// is the single source of truth for where the clone lands.
		if pkg.Source == "remote" || pkg.Source == "make" {
			resolved := packages.ResolvePackagePaths(name, pkg, cfg.PackagesDir)
			if resolved.Target != "" {
				s.AddArtifact(pkg.OwnerRecipe, state.KindPackageClone, resolved.Target)
			}
		}
	}

	for name, b := range cfg.Hooks.Builds {
		if !config.Applies(b.Enable, b.Hosts, b.Profiles, currentHost, cfg.Profiles) {
			continue
		}
		for _, ip := range b.InstallPaths {
			expanded, err := config.ExpandPath(ip)
			if err != nil {
				continue
			}
			track(b.OwnerRecipe, state.KindInstallPath, expanded)
		}
		if b.OwnerRecipe != "" {
			s.AddArtifact(b.OwnerRecipe, state.KindBuild, name)
		}
	}

	return s, unowned, nil
}

// protectedPaths returns every filesystem path the active config still
// declares: the intended manifest's recipe-owned paths plus the paths of items
// with no owner recipe. Cleanup never removes a protected path, whichever
// recipe the previous state attributed it to. Take it before any orphan is
// carried into next: a carried path is not "still declared".
func protectedPaths(next *state.RecipeState, unowned map[string]bool) map[string]bool {
	protected := next.AllPaths()
	for p := range unowned {
		protected[p] = true
	}
	return protected
}

// frozenRecipeSet returns exact recipe names that are filtered on this host.
// Their artifacts must not be treated as orphans during cleanup.
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

// frozenRecipeSourceSet returns the recipe sources whose previously recorded
// recipes freeze this run: remote sources filtered by this machine's
// profiles, and the origin of every recipes directory discovery found
// missing. The empty name is the local dotfiles repo, matching the empty
// source local recipes record. Persisted recipe provenance determines which
// prior recipes freeze.
func frozenRecipeSourceSet(cfg *config.Config) map[string]bool {
	if len(cfg.ProfileFilteredRecipeSources) == 0 && len(cfg.MissingRecipeDirs) == 0 {
		return nil
	}
	frozen := make(
		map[string]bool,
		len(cfg.ProfileFilteredRecipeSources)+len(cfg.MissingRecipeDirs),
	)
	for _, name := range cfg.ProfileFilteredRecipeSources {
		frozen[name] = true
	}
	for _, dir := range cfg.MissingRecipeDirs {
		frozen[dir.Source] = true
	}
	return frozen
}

// keepFrozenRecipes carries every recipe cfg freezes from prev into next (see
// carryForwardFrozenRecipes) and warns on errW once per missing recipes
// directory whose recorded recipes it kept, so a moved or unmounted dotfiles
// repo is visible instead of silently loading as zero recipes.
func keepFrozenRecipes(cfg *config.Config, prev, next *state.RecipeState, errW io.Writer) {
	tracked := make(map[string]bool, len(next.Recipes))
	for name := range next.Recipes {
		tracked[name] = true
	}
	carryForwardFrozenRecipes(prev, next, frozenRecipeSet(cfg), frozenRecipeSourceSet(cfg))

	for _, dir := range cfg.MissingRecipeDirs {
		kept := 0
		for name, art := range next.Recipes {
			if !tracked[name] && art.RecipeSource == dir.Source {
				kept++
			}
		}
		if kept == 0 {
			continue
		}
		fmt.Fprintln(errW, color.YellowString(
			"Warning: recipes directory %s is missing; kept %d recipe(s) recorded from it, nothing removed. "+
				"If you removed it on purpose, an empty directory there (or auto_discover = false) lets cleanup proceed.",
			dir.Path,
			kept,
		))
	}
}

// carryForwardFrozenRecipes copies each frozen recipe's previously-recorded
// artifacts from prev into next when next doesn't already track them. Recipes
// freeze by exact name or by their persisted source provenance (the empty
// source is the local dotfiles repo). This keeps filtered recipes out of the
// orphan diff and preserves their entries in the saved state for future runs.
func carryForwardFrozenRecipes(
	prev, next *state.RecipeState,
	frozenRecipes, frozenSources map[string]bool,
) {
	if prev == nil || next == nil || (len(frozenRecipes) == 0 && len(frozenSources) == 0) {
		return
	}
	if next.Recipes == nil {
		next.Recipes = map[string]state.RecipeArtifacts{}
	}
	for name, art := range prev.Recipes {
		if _, alreadyTracked := next.Recipes[name]; alreadyTracked {
			continue
		}
		frozen := frozenRecipes[name] || frozenSources[art.RecipeSource]
		if !frozen && prev.Version < state.CurrentRecipeStateVersion &&
			art.RecipeSource == "" {
			// Version 0 predates explicit source provenance. Remote recipe
			// identities were already namespaced as <source>/<recipe>, so infer
			// provenance once and persist it in the upgraded state. Restrict the
			// fallback to legacy state so a current local recipe whose name
			// contains a slash never freezes by prefix.
			for source := range frozenSources {
				if strings.HasPrefix(name, source+"/") {
					art.RecipeSource = source
					frozen = true
					break
				}
			}
		}
		if !frozen {
			continue
		}
		// Frozen means cleanup will not touch it, whatever an earlier run
		// marked.
		art.PendingRemoval = false
		next.Recipes[name] = art
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
//
// `protected` is the cross-recipe set of paths still declared by an active
// recipe in the intended manifest (next.AllPaths()) — any such path is kept,
// never removed, even when prev attributed it to a now-orphaned recipe name.
// The caller passes it explicitly so `ralph clean --recipe X` can protect
// against the FULL manifest, not just the single recipe it scoped the diff to.
//
// Returns the artifacts that failed removal, keyed by recipe, so the caller can
// re-track them (state.MergeRetry) and retry on the next run instead of dropping
// them from the manifest as permanent garbage after one transient failure.
func runCleanup(
	prev, next *state.RecipeState,
	protected map[string]bool,
	dryRun bool,
	logger io.Writer,
	phase *report.Phase,
) map[string]state.RecipeArtifacts {
	orphans := state.Diff(prev, next)
	if len(orphans) == 0 {
		phase.AddOK("cleanup", "no orphans")
		return nil
	}

	// Stable iteration order for deterministic reports/logs.
	names := make([]string, 0, len(orphans))
	for n := range orphans {
		names = append(names, n)
	}
	sort.Strings(names)

	failed := map[string]state.RecipeArtifacts{}
	totalRemoved := 0
	totalAbandoned := 0
	for _, recipeName := range names {
		art := orphans[recipeName]
		behavior := resolveBehavior(recipeName, next, art)

		// pre_uninstall runs before any artifact removal, regardless of
		// delete_behavior — these hooks clean up external state (registered
		// MCP servers, t-man agents, git hooks) that ralph doesn't track. Diff
		// only attaches uninstall hooks when the recipe is gone entirely, so a
		// recipe that merely lost one artifact won't reach here with hooks.
		runUninstallHooks(logger, recipeName, art.PreUninstall, "pre_uninstall", dryRun)

		if behavior == config.DeleteBehaviorAbandon {
			abandonAll(logger, recipeName, art)
			totalAbandoned += countAll(art)
			runUninstallHooks(logger, recipeName, art.PostUninstall, "post_uninstall", dryRun)
			// Configured intent, not an error condition — keep as OK so
			// the apply exit code stays clean.
			phase.AddOK(
				recipeName,
				fmt.Sprintf("abandoned %d artifact(s) (delete_behavior=abandon)", countAll(art)),
			)
			continue
		}

		removed := 0
		abandoned := 0
		opts := state.SafeRemoveOptions{DryRun: dryRun, Logger: logger}
		var fail state.RecipeArtifacts
		fail.DeleteBehavior = behavior

		removed += removePaths(
			opts,
			recipeName,
			state.KindSymlink,
			art.Symlinks,
			protected,
			&abandoned,
			&fail.Symlinks,
			logger,
		)
		removed += removePaths(
			opts,
			recipeName,
			state.KindDirSymlink,
			art.DirSymlinks,
			protected,
			&abandoned,
			&fail.DirSymlinks,
			logger,
		)
		removed += removePaths(
			opts,
			recipeName,
			state.KindCopy,
			art.Copies,
			protected,
			&abandoned,
			&fail.Copies,
			logger,
		)
		removed += removePaths(
			opts,
			recipeName,
			state.KindInstallPath,
			art.InstallPaths,
			protected,
			&abandoned,
			&fail.InstallPaths,
			logger,
		)
		// Directories last: any nested artifacts above must be removed
		// first so the directory is empty when SafeRemove inspects it.
		removed += removePaths(
			opts,
			recipeName,
			state.KindDirectory,
			art.Directories,
			protected,
			&abandoned,
			&fail.Directories,
			logger,
		)

		// These kinds are tracked but never auto-removed — log them so
		// the user knows ralph noticed.
		for _, p := range art.PackageClones {
			fmt.Fprintf(
				logger,
				"abandoned package clone: %s (recipe %s; run 'rm -rf %s' to remove)\n",
				p,
				recipeName,
				p,
			)
			abandoned++
		}
		for _, p := range art.Repos {
			fmt.Fprintf(
				logger,
				"abandoned repo: %s (recipe %s; auto-removal disabled in v1)\n",
				p,
				recipeName,
			)
			abandoned++
		}
		for _, name := range art.ShellAliases {
			fmt.Fprintf(
				logger,
				"abandoned shell alias: %s (recipe %s; cleared on next shell config regeneration)\n",
				name,
				recipeName,
			)
			abandoned++
		}
		for _, name := range art.ShellFunctions {
			fmt.Fprintf(
				logger,
				"abandoned shell function: %s (recipe %s; cleared on next shell config regeneration)\n",
				name,
				recipeName,
			)
			abandoned++
		}
		for _, name := range art.ShellEnv {
			fmt.Fprintf(
				logger,
				"abandoned shell env var: %s (recipe %s)\n",
				name,
				recipeName,
			)
			abandoned++
		}
		for _, name := range art.Packages {
			fmt.Fprintf(
				logger,
				"abandoned package: %s (recipe %s; declare install_paths to enable cleanup)\n",
				name,
				recipeName,
			)
			abandoned++
		}
		for _, name := range art.Builds {
			fmt.Fprintf(
				logger,
				"abandoned build: %s (recipe %s; declare install_paths to enable cleanup)\n",
				name,
				recipeName,
			)
			abandoned++
		}

		runUninstallHooks(logger, recipeName, art.PostUninstall, "post_uninstall", dryRun)

		if fail.HasAny() {
			failed[recipeName] = fail
		}
		totalRemoved += removed
		totalAbandoned += abandoned
		phase.AddOK(recipeName, fmt.Sprintf("removed %d, abandoned %d", removed, abandoned))
	}

	summary := fmt.Sprintf(
		"%d artifact(s) removed, %d abandoned across %d recipe(s)",
		totalRemoved,
		totalAbandoned,
		len(orphans),
	)
	if dryRun {
		summary = "[DRY RUN] " + summary
	}
	phase.AddOK("cleanup", summary)
	return failed
}

// runUninstallHooks runs a recipe's persisted pre/post uninstall hook commands
// during cleanup. Failures are warn-level: a hook that fails must not abort
// cleanup or flip the apply's exit code — removing the artifacts is the
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
func resolveBehavior(recipeName string, next *state.RecipeState, art state.RecipeArtifacts) string {
	if cur, ok := next.Recipes[recipeName]; ok && cur.DeleteBehavior != "" {
		return cur.DeleteBehavior
	}
	if art.DeleteBehavior != "" {
		return art.DeleteBehavior
	}
	return config.DeleteBehaviorDelete
}

// removePaths runs SafeRemove for each path of the given kind, returning the
// number successfully removed. It enforces two rules:
//
//   - A path still declared by an active recipe in the intended manifest
//     (`protected`) is kept untouched and logged. This is the cross-recipe
//     guard: it protects a still-needed binary or dotfile (notably the running
//     ralph binary, and any artifact a renamed recipe just re-created) when prev
//     attributed it to a recipe name that no longer matches.
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

// cleanupBanner is printed when the cleanup phase activates so the user
// knows what's happening (and what flag toggled it).
func cleanupBanner(w io.Writer) {
	_, _ = color.New(color.FgCyan).Fprintln(w, "\nProcessing recipe cleanup (--enable-cleanup)...")
}

// manifestOptions controls one recordManifestAndCleanup call.
type manifestOptions struct {
	cleanup    bool // remove orphans: --enable-cleanup or auto_cleanup
	dryRun     bool
	syncFailed bool // a sync step failed earlier in this run
}

// recordManifestAndCleanup builds the intended recipe-state manifest and ALWAYS
// persists it after a non-dry-run apply, so the cleanup baseline never goes
// stale even on runs without cleanup enabled. Removal of orphans is gated by
// opts.cleanup; recording ownership is not. A run that does not remove its
// orphans keeps them in the saved state (see settleOrphans), so a recipe
// disabled or dropped between two cleanup runs is still cleaned by the next.
//
// Recipes cfg freezes (host or profile filtered, or from a recipes directory
// discovery found missing) are carried forward untouched. The caller prints
// the cleanup banner (it knows the --enable-cleanup vs auto_cleanup
// distinction) before calling when opts.cleanup is true.
func recordManifestAndCleanup(
	cfg *config.Config,
	currentHost string,
	opts manifestOptions,
	w io.Writer,
	rpt *report.Report,
) {
	next, unowned, manifestErr := buildIntendedManifest(cfg, currentHost, time.Now())
	if manifestErr != nil {
		// An incomplete manifest must never be diffed (live artifacts would
		// look like orphans) nor saved (it would clobber a good baseline).
		fmt.Fprintln(
			os.Stderr,
			color.YellowString(
				"Warning: skipping recipe-state update, could not build manifest: %v",
				manifestErr,
			),
		)
		if opts.cleanup {
			rpt.AddPhase("Cleanup").AddWarn("manifest", manifestErr.Error())
		}
		return
	}

	prev, loadErr := state.Load()
	if loadErr != nil {
		fmt.Fprintln(
			os.Stderr,
			color.YellowString("Warning: could not load recipe state: %v", loadErr),
		)
		if opts.cleanup {
			rpt.AddPhase("Cleanup").AddWarn("load", loadErr.Error())
		}
		return
	}
	keepFrozenRecipes(cfg, prev, next, os.Stderr)

	cleanupPhase := settleOrphans(cfg, prev, next, protectedPaths(next, unowned), opts, w, rpt)

	if !opts.dryRun {
		if err := state.Save(next); err != nil {
			fmt.Fprintln(
				os.Stderr,
				color.YellowString("Warning: could not save recipe state: %v", err),
			)
			if cleanupPhase != nil {
				cleanupPhase.AddWarn("save", err.Error())
			}
		}
	}
}

// settleOrphans decides what happens to the orphans state.Diff(prev, next)
// reports. A cleanup run removes them (honoring delete_behavior) and re-tracks
// failed removals so they retry next run. A run without cleanup, or one whose
// sync failed (the config on disk may be stale or partial), removes nothing
// and carries the orphans into next, uninstall hooks included, so the next
// cleanup still finds them. protected (see protectedPaths) must be taken
// before this call, since it adds orphans to next. It returns the cleanup
// phase, or nil when cleanup is off.
func settleOrphans(
	cfg *config.Config,
	prev, next *state.RecipeState,
	protected map[string]bool,
	opts manifestOptions,
	w io.Writer,
	rpt *report.Report,
) *report.Phase {
	if !opts.cleanup {
		next.CarryOrphans(state.Diff(prev, next))
		return nil
	}

	phase := rpt.AddPhase("Cleanup")
	switch {
	case opts.syncFailed:
		phase.AddWarn(
			"sync",
			"orphan removal skipped: a sync step failed earlier in this run; orphans stay recorded for the next cleanup (`ralph clean` removes them without syncing)",
		)
		next.CarryOrphans(state.Diff(prev, next))
	case len(prev.Recipes) == 0 && cfg.RecipesConfig.AutoCleanup:
		fmt.Fprintln(
			w,
			color.CyanString(
				"First run with auto_cleanup: seeding state baseline (no artifacts will be removed).",
			),
		)
		phase.AddOK("baseline", "initial state recorded")
	default:
		failed := runCleanup(prev, next, protected, opts.dryRun, w, phase)
		if !opts.dryRun {
			for name, art := range failed {
				next.MergeRetry(name, art)
			}
		}
	}
	return phase
}
