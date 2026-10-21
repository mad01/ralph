package commands

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
)

// scanDir is one directory the dangling-link check reads. recipe names the
// owning recipe when exactly one recipe links into the directory and is empty
// when several do (a shared parent such as ~ or ~/.config).
type scanDir struct {
	path   string
	recipe string
}

// danglingLink is a symlink whose destination no longer exists.
type danglingLink struct {
	path   string
	target string
}

// checkDanglingLinks reports symlinks left behind in directories recipes link
// into: links whose destination is gone and points into the dotfiles repo or
// the recipe sources cache. Such a link is a ralph artifact no recipe declares
// any more, one the .recipe_state manifest no longer tracks (it predates the
// manifest, or a ralph up without cleanup dropped it), so ralph clean can't
// remove it. Targets of dotfiles that apply on this machine are skipped because
// checkDotfiles already reports a broken one. The check only reports; removal
// stays manual.
func checkDanglingLinks(rpt *report.Report, cfg *config.Config) {
	roots := ralphLinkRoots(cfg)
	dirs := danglingScanDirs(cfg)
	if len(roots) == 0 || len(dirs) == 0 {
		return
	}
	phase := rpt.AddPhase("Dangling links")
	declared := activeDotfileTargets(cfg)

	problems := 0
	for _, dir := range dirs {
		problems += reportDanglingIn(phase, dir, roots, declared)
	}
	if problems == 0 {
		noun := "directories"
		if len(dirs) == 1 {
			noun = "directory"
		}
		phase.AddResult(
			"dangling links",
			"",
			report.StatusOK,
			fmt.Sprintf("none in %d %s", len(dirs), noun),
			nil,
		)
	}
}

// reportDanglingIn adds a warning row per dangling link in dir, or one failure
// row when dir can't be read, and returns how many rows it added.
func reportDanglingIn(
	phase *report.Phase,
	dir scanDir,
	roots []string,
	declared map[string]bool,
) int {
	links, err := danglingLinksIn(dir.path, roots, declared)
	if err != nil {
		phase.AddResult(
			config.ShortenHome(dir.path),
			dir.recipe,
			report.StatusFail,
			fmt.Sprintf("error reading directory: %v", err),
			err,
		)
		return 1
	}
	for _, link := range links {
		phase.AddResult(
			config.ShortenHome(link.path),
			dir.recipe,
			report.StatusWarn,
			fmt.Sprintf(
				"dangling symlink to missing %s; no recipe declares it, remove it by hand",
				config.ShortenHome(link.target),
			),
			nil,
		)
	}
	return len(links)
}

// ralphLinkRoots returns the directories ralph creates symlinks into: the
// dotfiles repo and the recipe sources cache. A dangling link pointing
// anywhere else belongs to some other tool and is not reported.
func ralphLinkRoots(cfg *config.Config) []string {
	var roots []string
	if cfg.DotfilesRepoPath != "" {
		if repo, ok := expandedPath(cfg.DotfilesRepoPath); ok {
			roots = append(roots, repo)
		}
	}
	if sources, err := config.SourcesDir(); err == nil && sources != "" {
		roots = append(roots, filepath.Clean(sources))
	}
	return roots
}

// danglingScanDirs lists, sorted by path, the directories recipes link into:
// dirs_mirror targets, directories targets, and the parent of every dotfile
// target. An item counts whatever its own host, profile, or enable filter, so
// links left by a disabled item are found too. Recipes filtered out as a whole
// never reach cfg, so a directory only such a recipe links into is not read. A
// path that fails to expand is left out; the per-item checks report it.
func danglingScanDirs(cfg *config.Config) []scanDir {
	owners := make(map[string]map[string]bool)
	add := func(path, recipe string) {
		if owners[path] == nil {
			owners[path] = make(map[string]bool)
		}
		owners[path][recipe] = true
	}
	for _, mirror := range cfg.DirsMirror {
		if target, ok := expandedPath(mirror.Target); ok {
			add(target, mirror.OwnerRecipe)
		}
	}
	for _, dir := range cfg.Directories {
		if target, ok := expandedPath(dir.Target); ok {
			add(target, dir.OwnerRecipe)
		}
	}
	for _, df := range cfg.Dotfiles {
		if target, ok := expandedPath(df.Target); ok {
			add(filepath.Dir(target), df.OwnerRecipe)
		}
	}

	dirs := make([]scanDir, 0, len(owners))
	for path, recipes := range owners {
		dir := scanDir{path: path}
		if len(recipes) == 1 {
			for recipe := range recipes {
				dir.recipe = recipe
			}
		}
		dirs = append(dirs, dir)
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].path < dirs[j].path })
	return dirs
}

// activeDotfileTargets returns the expanded target of every dotfile that
// applies on this machine. That is the set checkDotfiles inspects, so a broken
// link there is already reported; a disabled or filtered dotfile's target
// stays in the dangling-link scan.
func activeDotfileTargets(cfg *config.Config) map[string]bool {
	host := config.GetCurrentHost()
	active := make(map[string]bool, len(cfg.Dotfiles))
	for _, df := range cfg.Dotfiles {
		if !config.IsEnabled(df.Enable) ||
			!config.ShouldApplyForHost(df.Hosts, host) ||
			!config.ShouldApplyForProfiles(df.Profiles, cfg.Profiles) {
			continue
		}
		if target, ok := expandedPath(df.Target); ok {
			active[target] = true
		}
	}
	return active
}

// expandedPath expands ~ and environment variables in path and cleans it. ok
// is false when the path is empty or can't be expanded.
func expandedPath(path string) (string, bool) {
	expanded, err := config.ExpandPath(path)
	if err != nil || expanded == "" {
		return "", false
	}
	return filepath.Clean(expanded), true
}

// danglingLinksIn returns the direct children of dir that are symlinks to a
// missing destination under one of roots, skipping paths in declared. A dir
// that is missing or not a directory has nothing to report here; the
// Directories phase reports a declared directory in that state.
func danglingLinksIn(dir string, roots []string, declared map[string]bool) ([]danglingLink, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var links []danglingLink
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink == 0 {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if declared[path] {
			continue
		}
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		dest, err := os.Readlink(path)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(dir, dest)
		}
		dest = filepath.Clean(dest)
		if underAnyRoot(dest, roots) {
			links = append(links, danglingLink{path: path, target: dest})
		}
	}
	return links, nil
}

// underAnyRoot reports whether path is one of roots or lies beneath one.
func underAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if path == root || strings.HasPrefix(path, root+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
