package state

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SafeRemoveOptions controls SafeRemove behavior.
type SafeRemoveOptions struct {
	// DryRun, when true, logs what WOULD be removed without touching disk.
	DryRun bool
	// AllowedPrefixes is the absolute set of root paths under which removal
	// is permitted. Any path outside these prefixes is rejected. If empty,
	// SafeRemove uses DefaultAllowedPrefixes($HOME).
	AllowedPrefixes []string
	// Logger receives every action line; nil discards them.
	Logger io.Writer
	// Self, when set, is treated as the path of the currently-running binary;
	// SafeRemove refuses to delete it (or a symlink that resolves to it). When
	// empty, SafeRemove resolves os.Executable() itself. This is primarily a
	// test injection point — production callers leave it empty.
	Self string
}

// DefaultAllowedPrefixes returns the conservative set of roots ralph will
// touch when removing an artifact: the user's home, $HOME/.config,
// $HOME/.local, $HOME/code/bin, and $HOME/.cache. Anything outside this
// set requires explicit opt-in via SafeRemoveOptions.AllowedPrefixes.
//
// Note: $HOME alone is included so legitimate paths like $HOME/.zshrc
// (touched indirectly via the RALPH MANAGED block — though removal there
// is "stop emitting" rather than file deletion) are within scope. The
// kind-specific verifications below still apply.
func DefaultAllowedPrefixes(home string) []string {
	return []string{
		home,
	}
}

// Sentinel errors. Wrap them with fmt.Errorf for context; use errors.Is in
// callers (and tests) to assert the rail that fired.
var (
	ErrGlobInPath  = errors.New("safe_delete: glob characters not allowed in path")
	ErrOutsideHome = errors.New("safe_delete: path is outside the allowed prefix set")
	ErrEmptyPath   = errors.New("safe_delete: empty path")
	ErrNotAbsolute = errors.New("safe_delete: path must be absolute")
	ErrWrongKind   = errors.New(
		"safe_delete: actual filesystem entry does not match the expected artifact kind",
	)
	ErrDirNotEmpty     = errors.New("safe_delete: directory is not empty")
	ErrUnknownKind     = errors.New("safe_delete: unknown artifact kind")
	ErrUnsupportedKind = errors.New(
		"safe_delete: artifact kind is intentionally not auto-removed in v1",
	)
	ErrSelfDelete = errors.New(
		"safe_delete: refusing to delete the currently-running executable",
	)
)

// glob characters that we reject outright. We never expand globs in paths
// we're about to delete — a typo in install_paths must not turn into
// `rm -rf /etc/*`.
const globChars = "*?[]{}"

// SafeRemove deletes the given path if (and only if) every safety rail
// passes. The caller supplies the artifact kind so SafeRemove can verify
// the on-disk entry matches what the manifest claimed (e.g. a "symlink"
// must be a symlink at deletion time; a "directory" must be empty).
//
// On success returns nil. On rejection returns a wrapped sentinel error
// suitable for errors.Is checks. On a dry-run match, logs and returns nil
// without touching disk.
func SafeRemove(path string, kind ArtifactKind, opts SafeRemoveOptions) error {
	if path == "" {
		return ErrEmptyPath
	}
	if containsGlob(path) {
		return fmt.Errorf("%w: %s", ErrGlobInPath, path)
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return fmt.Errorf("%w: %s", ErrNotAbsolute, path)
	}

	prefixes := opts.AllowedPrefixes
	if len(prefixes) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("safe_delete: home dir: %w", err)
		}
		prefixes = DefaultAllowedPrefixes(home)
	}
	if !underAnyPrefix(clean, prefixes) {
		return fmt.Errorf(
			"%w: %s (allowed: %s)",
			ErrOutsideHome,
			clean,
			strings.Join(prefixes, ", "),
		)
	}

	// Never delete the binary we're running from. A self-delete mid-cleanup
	// bricks ralph (can't run ralph to fix ralph), so this guard sits at the
	// lowest level and covers every kind/caller as defense in depth.
	if isRunningExecutable(clean, selfExecutable(opts.Self)) {
		return fmt.Errorf("%w: %s", ErrSelfDelete, clean)
	}

	switch kind {
	case KindSymlink, KindDirSymlink:
		return removeSymlink(clean, kind, opts)
	case KindCopy, KindInstallPath:
		return removeRegularFile(clean, kind, opts)
	case KindDirectory:
		return removeEmptyDirectory(clean, opts)
	case KindRepo, KindPackageClone:
		// Repos and package clones are always abandoned in v1 — a git checkout
		// may hold uncommitted work. Cleanup logs the abandonment and never
		// calls SafeRemove for these; if a caller does, refuse explicitly.
		logf(opts.Logger, "abandon %s (auto-removal disabled in v1): %s", kind, clean)
		return fmt.Errorf("%w: %s (kind=%s)", ErrUnsupportedKind, clean, kind)
	case KindShellAlias, KindShellFunc, KindShellEnv, KindPackage, KindBuild:
		// These don't have a single removable filesystem path at the
		// generic layer. Cleanup for them is "stop emitting" (shell
		// aliases/functions/env regenerate; packages/builds simply stop
		// running). The caller should not invoke SafeRemove for these.
		return fmt.Errorf("%w: %s (kind=%s)", ErrUnsupportedKind, clean, kind)
	default:
		return fmt.Errorf("%w: %s (kind=%s)", ErrUnknownKind, clean, kind)
	}
}

// removeSymlink unlinks the path only if it is currently a symlink.
// Refuses to remove regular files, directories (non-symlink), or paths
// that have been replaced by something else since apply.
func removeSymlink(path string, kind ArtifactKind, opts SafeRemoveOptions) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		logf(opts.Logger, "already gone (%s): %s", kind, path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("safe_delete: lstat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%w: %s is not a symlink (mode=%s)", ErrWrongKind, path, info.Mode())
	}
	if opts.DryRun {
		logf(opts.Logger, "would remove %s: %s", kind, path)
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("safe_delete: remove %s: %w", path, err)
	}
	logf(opts.Logger, "removed %s: %s", kind, path)
	return nil
}

// removeRegularFile removes a file that must be a regular file or symlink
// to a file. Refuses directories outright.
func removeRegularFile(path string, kind ArtifactKind, opts SafeRemoveOptions) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		logf(opts.Logger, "already gone (%s): %s", kind, path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("safe_delete: lstat %s: %w", path, err)
	}
	mode := info.Mode()
	switch {
	case mode.IsRegular():
		// fall through
	case mode&os.ModeSymlink != 0:
		// fall through; symlinks to files are okay (e.g. a binary
		// installed via cp -P or ln -s).
	default:
		return fmt.Errorf(
			"%w: %s is not a regular file or symlink (mode=%s)",
			ErrWrongKind,
			path,
			mode,
		)
	}
	if opts.DryRun {
		logf(opts.Logger, "would remove %s: %s", kind, path)
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("safe_delete: remove %s: %w", path, err)
	}
	logf(opts.Logger, "removed %s: %s", kind, path)
	return nil
}

// removeEmptyDirectory removes a directory only if it has no entries.
// Refuses non-empty dirs outright (no recursive delete in v1).
func removeEmptyDirectory(path string, opts SafeRemoveOptions) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		logf(opts.Logger, "already gone (directory): %s", path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("safe_delete: lstat %s: %w", path, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s is not a directory (mode=%s)", ErrWrongKind, path, info.Mode())
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("safe_delete: read dir %s: %w", path, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("%w: %s has %d entries", ErrDirNotEmpty, path, len(entries))
	}
	if opts.DryRun {
		logf(opts.Logger, "would remove directory: %s", path)
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("safe_delete: rmdir %s: %w", path, err)
	}
	logf(opts.Logger, "removed directory: %s", path)
	return nil
}

// containsGlob reports whether s contains any of *, ?, [, ], {, }.
func containsGlob(s string) bool {
	return strings.ContainsAny(s, globChars)
}

// underAnyPrefix returns true when path is exactly one of the prefixes or
// is a child of one. Comparison is on cleaned paths to defeat trailing-slash
// or dot-segment shenanigans.
func underAnyPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		clean := filepath.Clean(p)
		if path == clean {
			return true
		}
		// Append the OS separator so /home/u/code does not match /home/u/coding.
		if strings.HasPrefix(path, clean+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func logf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintln(w, fmt.Sprintf(format, args...))
}

// selfExecutable returns the path of the currently-running binary. An explicit
// override (used by tests) wins; otherwise it resolves os.Executable(). Returns
// "" when the path can't be determined — the guard then simply doesn't fire,
// since a best-effort self-check must never block legitimate removals.
func selfExecutable(override string) string {
	if override != "" {
		return override
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

// isRunningExecutable reports whether path refers to the same file as the
// running binary (self). It matches on cleaned paths and, when both resolve,
// on symlink-evaluated paths — so deleting a PATH symlink that points at the
// running binary is also refused.
func isRunningExecutable(path, self string) bool {
	if self == "" {
		return false
	}
	if filepath.Clean(path) == filepath.Clean(self) {
		return true
	}
	rp, err1 := filepath.EvalSymlinks(path)
	rs, err2 := filepath.EvalSymlinks(self)
	return err1 == nil && err2 == nil && rp == rs
}
