package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mad01/ralph/internal/gitutil"
)

// SourcesDir returns the expanded cache directory for remote recipe sources.
func SourcesDir() (string, error) {
	return ExpandPath(DefaultSourcesDir)
}

// SourceCheckoutPath returns the cached checkout path for a recipe source.
func SourceCheckoutPath(sourcesDir string, src RecipeSource) string {
	return filepath.Join(sourcesDir, src.Name)
}

// SourceRecipesDir returns the source's recipe directory name within its
// checkout, defaulting to DefaultRecipesDir.
func SourceRecipesDir(src RecipeSource) string {
	if src.RecipesDir == "" {
		return DefaultRecipesDir
	}
	return src.RecipesDir
}

// EnsureSourceCheckout makes sure the source's cached checkout exists and sits
// at the pinned ref. A missing checkout is cloned; an existing one is moved to
// Ref when HEAD differs (fetching first if the ref is unknown locally).
// Branch refs with update=true are pulled by the `ralph up` sync phase, not
// here — config loading must not hit the network for an already-pinned cache.
func EnsureSourceCheckout(w io.Writer, src RecipeSource, sourcesDir string) (string, error) {
	target := SourceCheckoutPath(sourcesDir, src)

	info, err := os.Stat(target)
	if err == nil && info.IsDir() {
		if src.Ref == "" {
			return target, nil
		}
		want := gitutil.ResolveRef(target, src.Ref)
		if want == "" {
			// Ref unknown locally (e.g. pin moved to a new tag, or to a branch
			// that was never checked out here): fetch first. ResolveRef only
			// sees local refs, so it can still come back empty for a remote
			// branch — git checkout's DWIM below resolves origin/<ref> and
			// creates the tracking branch, and its error is the real answer.
			if err := gitutil.Fetch(w, target); err != nil {
				return "", fmt.Errorf("recipe_source '%s': %w", src.Name, err)
			}
			want = gitutil.ResolveRef(target, src.Ref)
		}
		if want != "" && gitutil.GetGitHash(target) == want {
			return target, nil
		}
		if err := gitutil.Checkout(w, target, src.Ref); err != nil {
			return "", fmt.Errorf(
				"recipe_source '%s': ref '%s': %w",
				src.Name,
				src.Ref,
				err,
			)
		}
		return target, nil
	}

	if err := os.MkdirAll(sourcesDir, 0o755); err != nil {
		return "", fmt.Errorf("recipe_source '%s': creating sources dir: %w", src.Name, err)
	}
	fmt.Fprintf(w, "Cloning recipe source '%s' from %s...\n", src.Name, src.URL)
	if err := gitutil.Clone(w, src.URL, target, ""); err != nil {
		return "", fmt.Errorf("recipe_source '%s': %w", src.Name, err)
	}
	if src.Ref != "" {
		if err := gitutil.Checkout(w, target, src.Ref); err != nil {
			return "", fmt.Errorf("recipe_source '%s': %w", src.Name, err)
		}
	}
	return target, nil
}

// JoinSourcePath resolves a recipe item's source against the dotfiles repo
// path. Items merged from remote recipe sources carry absolute sources
// (rooted in the source's cached checkout) and pass through unchanged.
func JoinSourcePath(repoPath, source string) string {
	if filepath.IsAbs(source) {
		return source
	}
	return filepath.Join(repoPath, source)
}
