// Package state tracks per-recipe artifact ownership so ralph can clean up
// orphans when a recipe is removed or disabled. The on-disk layout is a
// single JSON file at ~/.config/ralph/.recipe_state, separate from the
// existing .builds_state file (which tracks build run-state).
//
// State is written at the end of every successful apply and read at the
// start of the next apply to compute orphans (artifacts present last time
// but absent now). Removal is gated by each recipe's delete_behavior:
// "delete" (default) cleans up; "abandon" leaves orphans in place with a
// log line.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// FileName is the basename of the state file under ~/.config/ralph/.
const FileName = ".recipe_state"

// ArtifactKind enumerates the kinds of artifacts a recipe can produce that
// ralph knows how to track and (where safe) clean up.
type ArtifactKind string

const (
	KindSymlink     ArtifactKind = "symlink"
	KindCopy        ArtifactKind = "copy"
	KindDirectory   ArtifactKind = "directory"
	KindDirSymlink  ArtifactKind = "dir_symlink"
	KindRepo        ArtifactKind = "repo"
	KindShellAlias  ArtifactKind = "shell_alias"
	KindShellFunc   ArtifactKind = "shell_function"
	KindShellEnv    ArtifactKind = "shell_env"
	KindPackage     ArtifactKind = "package"
	KindBuild       ArtifactKind = "build"
	KindInstallPath ArtifactKind = "install_path"
	// KindPackageClone is the on-disk clone directory of a remote/make package
	// (under packages_dir). Like repos, it is tracked but never auto-removed —
	// a git clone may hold uncommitted work — so cleanup abandons it with a log
	// line telling the user the exact rm command to run.
	KindPackageClone ArtifactKind = "package_clone"
)

// RecipeState is the root of the persisted manifest.
type RecipeState struct {
	Recipes map[string]RecipeArtifacts `json:"recipes"`
}

// RecipeArtifacts records every artifact owned by a single recipe along
// with the recipe's delete_behavior at the time of the last apply. Names
// (shell_aliases, shell_functions, shell_env, packages, builds) are stored
// rather than paths because cleanup for those kinds is "stop emitting"
// rather than "remove from disk".
type RecipeArtifacts struct {
	AppliedAt      time.Time `json:"applied_at"`
	DeleteBehavior string    `json:"delete_behavior"`
	Symlinks       []string  `json:"symlinks,omitempty"`
	Copies         []string  `json:"copies,omitempty"`
	Directories    []string  `json:"directories,omitempty"`
	DirSymlinks    []string  `json:"dir_symlinks,omitempty"`
	Repos          []string  `json:"repos,omitempty"`
	ShellAliases   []string  `json:"shell_aliases,omitempty"`
	ShellFunctions []string  `json:"shell_functions,omitempty"`
	ShellEnv       []string  `json:"shell_env,omitempty"`
	Packages       []string  `json:"packages,omitempty"`
	Builds         []string  `json:"builds,omitempty"`
	InstallPaths   []string  `json:"install_paths,omitempty"`
	PackageClones  []string  `json:"package_clones,omitempty"`
	// Uninstall hooks are recipe-level shell commands persisted at apply
	// time so cleanup can run them when the recipe becomes an orphan (even
	// if its recipe.toml is gone from disk). Order matters, so unlike the
	// artifact lists these are never sorted or deduped.
	PreUninstall  []string `json:"pre_uninstall,omitempty"`
	PostUninstall []string `json:"post_uninstall,omitempty"`
}

// GetStatePath returns the absolute path of the recipe-state file.
// It is a variable to allow tests to override the path without mutating $HOME.
var GetStatePath = getStatePathInternal

func getStatePathInternal() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}
	return filepath.Join(home, ".config", "ralph", FileName), nil
}

// Load reads the recipe state from disk. A missing file returns an empty
// state without error so first-apply on a fresh machine works seamlessly.
func Load() (*RecipeState, error) {
	p, err := GetStatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &RecipeState{Recipes: map[string]RecipeArtifacts{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read recipe state: %w", err)
	}
	var s RecipeState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse recipe state: %w", err)
	}
	if s.Recipes == nil {
		s.Recipes = map[string]RecipeArtifacts{}
	}
	return &s, nil
}

// Save writes the recipe state to disk, creating the parent directory if
// needed. Each list inside a RecipeArtifacts is sorted+deduped before write
// so the on-disk file is stable across applies (helps diffing in git when
// the user inspects state, and keeps tests deterministic).
func Save(s *RecipeState) error {
	p, err := GetStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	normalize(s)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal recipe state: %w", err)
	}
	tmpPath := p + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("writing temp state file: %w", err)
	}
	if err := os.Rename(tmpPath, p); err != nil {
		os.Remove(tmpPath) // clean up on rename failure
		return fmt.Errorf("renaming state file: %w", err)
	}
	return nil
}

// AddArtifact records ownership of an artifact under a given recipe. Safe
// to call on the same (recipe, kind, value) tuple multiple times — Save
// dedupes before writing.
func (s *RecipeState) AddArtifact(recipe string, kind ArtifactKind, value string) {
	if recipe == "" || value == "" {
		return
	}
	if s.Recipes == nil {
		s.Recipes = map[string]RecipeArtifacts{}
	}
	a := s.Recipes[recipe]
	switch kind {
	case KindSymlink:
		a.Symlinks = append(a.Symlinks, value)
	case KindCopy:
		a.Copies = append(a.Copies, value)
	case KindDirectory:
		a.Directories = append(a.Directories, value)
	case KindDirSymlink:
		a.DirSymlinks = append(a.DirSymlinks, value)
	case KindRepo:
		a.Repos = append(a.Repos, value)
	case KindShellAlias:
		a.ShellAliases = append(a.ShellAliases, value)
	case KindShellFunc:
		a.ShellFunctions = append(a.ShellFunctions, value)
	case KindShellEnv:
		a.ShellEnv = append(a.ShellEnv, value)
	case KindPackage:
		a.Packages = append(a.Packages, value)
	case KindBuild:
		a.Builds = append(a.Builds, value)
	case KindInstallPath:
		a.InstallPaths = append(a.InstallPaths, value)
	case KindPackageClone:
		a.PackageClones = append(a.PackageClones, value)
	}
	s.Recipes[recipe] = a
}

// AllInstallPaths returns the set (filepath.Clean'd) of every install_path
// tracked across all recipes in the state. Cleanup uses this against the
// intended manifest to protect a still-declared binary from removal even when
// the previous state attributed it to a different recipe name (renamed,
// re-attributed across ralph versions, or moved between recipes). Without this
// cross-recipe view, Diff — which keys orphans by recipe name — would treat a
// live, still-declared binary as an orphan and delete it.
func (s *RecipeState) AllInstallPaths() map[string]bool {
	out := map[string]bool{}
	if s == nil {
		return out
	}
	for _, art := range s.Recipes {
		for _, p := range art.InstallPaths {
			out[filepath.Clean(p)] = true
		}
	}
	return out
}

// AllPaths returns the set (filepath.Clean'd) of every filesystem-removable
// path tracked across all recipes — symlinks, dir-symlinks, copies, directories,
// and install_paths. Cleanup uses this against the intended manifest to protect
// ANY path still declared by an active recipe from removal, regardless of which
// recipe name the previous state attributed it to. This is the cross-recipe
// guard that makes a recipe rename safe: the artifacts the new name re-created
// this apply are not deleted as orphans of the old name. Package clones and
// repos are excluded because they are never auto-removed anyway.
func (s *RecipeState) AllPaths() map[string]bool {
	out := map[string]bool{}
	if s == nil {
		return out
	}
	add := func(paths []string) {
		for _, p := range paths {
			out[filepath.Clean(p)] = true
		}
	}
	for _, art := range s.Recipes {
		add(art.Symlinks)
		add(art.DirSymlinks)
		add(art.Copies)
		add(art.Directories)
		add(art.InstallPaths)
	}
	return out
}

// MergeRetry re-tracks artifacts that failed removal during cleanup so the next
// run retries them, instead of dropping them from the manifest as permanent
// garbage after a single transient failure. Only filesystem paths are merged;
// uninstall hooks and names (packages/builds/shell) are deliberately excluded —
// the hooks already ran this cleanup and must not re-fire on the retry pass.
// Save dedupes, so re-adding an already-tracked path is harmless.
func (s *RecipeState) MergeRetry(recipe string, art RecipeArtifacts) {
	if recipe == "" || !art.HasAny() {
		return
	}
	if s.Recipes == nil {
		s.Recipes = map[string]RecipeArtifacts{}
	}
	a := s.Recipes[recipe]
	if art.DeleteBehavior != "" {
		a.DeleteBehavior = art.DeleteBehavior
	}
	a.Symlinks = append(a.Symlinks, art.Symlinks...)
	a.DirSymlinks = append(a.DirSymlinks, art.DirSymlinks...)
	a.Copies = append(a.Copies, art.Copies...)
	a.Directories = append(a.Directories, art.Directories...)
	a.InstallPaths = append(a.InstallPaths, art.InstallPaths...)
	s.Recipes[recipe] = a
}

// SetMetadata marks the recipe as applied at the given time and records its
// delete_behavior. Called once per recipe per apply, after all that
// recipe's artifacts have been added.
func (s *RecipeState) SetMetadata(recipe string, appliedAt time.Time, deleteBehavior string) {
	if recipe == "" {
		return
	}
	if s.Recipes == nil {
		s.Recipes = map[string]RecipeArtifacts{}
	}
	a := s.Recipes[recipe]
	a.AppliedAt = appliedAt
	a.DeleteBehavior = deleteBehavior
	s.Recipes[recipe] = a
}

// SetUninstallHooks records the recipe's pre/post uninstall hook commands so a
// later cleanup can run them even after the recipe's config has left disk.
// Order is preserved; empty slices are stored as nil.
func (s *RecipeState) SetUninstallHooks(recipe string, pre, post []string) {
	if recipe == "" {
		return
	}
	if s.Recipes == nil {
		s.Recipes = map[string]RecipeArtifacts{}
	}
	a := s.Recipes[recipe]
	a.PreUninstall = pre
	a.PostUninstall = post
	s.Recipes[recipe] = a
}

// DeleteRecipe removes a recipe's entry from the state, e.g. after its
// artifacts have been cleaned up.
func (s *RecipeState) DeleteRecipe(name string) {
	delete(s.Recipes, name)
}

// Diff computes the artifacts that exist in `prev` but not in `next` for
// each recipe. Recipes present in `prev` but absent from `next.Recipes`
// are returned with all their artifacts as orphans. This is the input to
// the cleanup phase.
func Diff(prev, next *RecipeState) map[string]RecipeArtifacts {
	out := map[string]RecipeArtifacts{}
	if prev == nil || prev.Recipes == nil {
		return out
	}
	for name, prevArt := range prev.Recipes {
		nextArt, stillPresent := next.Recipes[name]
		orphans := RecipeArtifacts{
			AppliedAt:      prevArt.AppliedAt,
			DeleteBehavior: prevArt.DeleteBehavior,
			Symlinks:       missing(prevArt.Symlinks, nextArt.Symlinks),
			Copies:         missing(prevArt.Copies, nextArt.Copies),
			Directories:    missing(prevArt.Directories, nextArt.Directories),
			DirSymlinks:    missing(prevArt.DirSymlinks, nextArt.DirSymlinks),
			Repos:          missing(prevArt.Repos, nextArt.Repos),
			ShellAliases:   missing(prevArt.ShellAliases, nextArt.ShellAliases),
			ShellFunctions: missing(prevArt.ShellFunctions, nextArt.ShellFunctions),
			ShellEnv:       missing(prevArt.ShellEnv, nextArt.ShellEnv),
			Packages:       missing(prevArt.Packages, nextArt.Packages),
			Builds:         missing(prevArt.Builds, nextArt.Builds),
			InstallPaths:   missing(prevArt.InstallPaths, nextArt.InstallPaths),
			PackageClones:  missing(prevArt.PackageClones, nextArt.PackageClones),
		}
		// Uninstall hooks tear down the whole service (t-man remove, rm -rf of a
		// data dir), so they run ONLY when the recipe is gone entirely. A recipe
		// that merely dropped one artifact (still present in next) keeps its
		// service intact — attaching its hooks here would deregister a live
		// service on an unrelated edit.
		if !stillPresent {
			orphans.PreUninstall = prevArt.PreUninstall
			orphans.PostUninstall = prevArt.PostUninstall
			// Recipe gone entirely: every prev artifact is an orphan.
			orphans.Symlinks = prevArt.Symlinks
			orphans.Copies = prevArt.Copies
			orphans.Directories = prevArt.Directories
			orphans.DirSymlinks = prevArt.DirSymlinks
			orphans.Repos = prevArt.Repos
			orphans.ShellAliases = prevArt.ShellAliases
			orphans.ShellFunctions = prevArt.ShellFunctions
			orphans.ShellEnv = prevArt.ShellEnv
			orphans.Packages = prevArt.Packages
			orphans.Builds = prevArt.Builds
			orphans.InstallPaths = prevArt.InstallPaths
			orphans.PackageClones = prevArt.PackageClones
		}
		if orphans.HasAny() {
			out[name] = orphans
		}
	}
	return out
}

// HasAny reports whether the artifacts list contains any orphaned items.
func (a RecipeArtifacts) HasAny() bool {
	return len(a.Symlinks) > 0 ||
		len(a.Copies) > 0 ||
		len(a.Directories) > 0 ||
		len(a.DirSymlinks) > 0 ||
		len(a.Repos) > 0 ||
		len(a.ShellAliases) > 0 ||
		len(a.ShellFunctions) > 0 ||
		len(a.ShellEnv) > 0 ||
		len(a.Packages) > 0 ||
		len(a.Builds) > 0 ||
		len(a.InstallPaths) > 0 ||
		len(a.PackageClones) > 0
}

// missing returns elements of a that are not present in b.
func missing(a, b []string) []string {
	if len(a) == 0 {
		return nil
	}
	bset := make(map[string]struct{}, len(b))
	for _, x := range b {
		bset[x] = struct{}{}
	}
	var out []string
	for _, x := range a {
		if _, ok := bset[x]; !ok {
			out = append(out, x)
		}
	}
	return out
}

// normalize sorts and dedupes every artifact list in place. Stable on-disk
// order makes `git diff` of the state file readable.
func normalize(s *RecipeState) {
	for name, a := range s.Recipes {
		a.Symlinks = sortDedup(a.Symlinks)
		a.Copies = sortDedup(a.Copies)
		a.Directories = sortDedup(a.Directories)
		a.DirSymlinks = sortDedup(a.DirSymlinks)
		a.Repos = sortDedup(a.Repos)
		a.ShellAliases = sortDedup(a.ShellAliases)
		a.ShellFunctions = sortDedup(a.ShellFunctions)
		a.ShellEnv = sortDedup(a.ShellEnv)
		a.Packages = sortDedup(a.Packages)
		a.Builds = sortDedup(a.Builds)
		a.InstallPaths = sortDedup(a.InstallPaths)
		a.PackageClones = sortDedup(a.PackageClones)
		s.Recipes[name] = a
	}
}

func sortDedup(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
