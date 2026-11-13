package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mad01/ralph/internal/gitutil"
	"github.com/mad01/ralph/internal/paths"
)

// SourcesDir returns the cache directory for remote recipe sources: sources/
// in the state directory (paths.StateDir).
func SourcesDir() (string, error) {
	dir, err := paths.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sources"), nil
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

// SourceCheckoutState says how a source's cached checkout compares with the
// one EnsureSourceCheckout leaves behind.
type SourceCheckoutState int

const (
	// SourceCheckoutReady means the checkout exists and, when the source pins
	// a ref, HEAD is at it.
	SourceCheckoutReady SourceCheckoutState = iota
	// SourceCheckoutMissing means the source has not been cloned.
	SourceCheckoutMissing
	// SourceCheckoutOffPin means the checkout exists but HEAD is not at the
	// pinned ref, or the ref is not known locally.
	SourceCheckoutOffPin
)

// InspectSourceCheckout reports the state of the source's cached checkout
// without changing it: it runs local git queries only, never a fetch.
func InspectSourceCheckout(src RecipeSource, sourcesDir string) SourceCheckoutState {
	target := SourceCheckoutPath(sourcesDir, src)
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		return SourceCheckoutMissing
	}
	if src.Ref == "" {
		return SourceCheckoutReady
	}
	want := gitutil.ResolveRef(target, src.Ref)
	if want == "" || gitutil.GetGitHash(target) != want {
		return SourceCheckoutOffPin
	}
	return SourceCheckoutReady
}

// readOnlySourceCheckout is EnsureSourceCheckout for a load that writes
// nothing (a dry run or a check): it never clones, fetches or checks out. It
// returns the checkout path and whether the checkout exists, and warns on w
// when recipes load differently from a real run: the source is not cloned,
// or HEAD is not at the pinned ref.
func readOnlySourceCheckout(w io.Writer, src RecipeSource, sourcesDir string) (string, bool) {
	target := SourceCheckoutPath(sourcesDir, src)
	switch InspectSourceCheckout(src, sourcesDir) {
	case SourceCheckoutMissing:
		fmt.Fprintf(w,
			"Warning: recipe source '%s' is not cloned, and a dry run or check does not clone it; "+
				"its recipes are not loaded, so this preview may be incomplete\n",
			src.Name)
		return target, false
	case SourceCheckoutOffPin:
		fmt.Fprintf(
			w,
			"Warning: recipe source '%s' is not at ref '%s', and a dry run or check does not move it; "+
				"its recipes load from the current checkout, so this preview may be incomplete\n",
			src.Name,
			src.Ref,
		)
	}
	return target, true
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
