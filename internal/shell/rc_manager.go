package shell

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mad01/ralph/internal/paths"
)

const (
	RalphBlockBeginMarker = "# BEGIN RALPH MANAGED BLOCK"
	RalphBlockEndMarker   = "# END RALPH MANAGED BLOCK"

	// Legacy markers for backward compatibility detection
	legacyBlockBeginMarker = "# BEGIN DOTTER MANAGED BLOCK"
	legacyBlockEndMarker   = "# END DOTTER MANAGED BLOCK"
)

// SupportedShell represents a shell type that ralph can manage.
type SupportedShell string

const (
	Bash SupportedShell = "bash"
	Zsh  SupportedShell = "zsh"
	Fish SupportedShell = "fish"
	// Add other shells as needed (e.g., Powershell)
)

// GetRCFilePath returns the typical path for the RC file of a given shell.
func GetRCFilePath(shell SupportedShell) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}

	switch shell {
	case Bash:
		return filepath.Join(homeDir, ".bashrc"), nil
	case Zsh:
		// Zsh can also use .zprofile for login shells or .zshenv for all invocations.
		// .zshrc is typically for interactive shells. This is usually what we want.
		// Check if ZDOTDIR is set, otherwise default to ~/.zshrc
		if zdotdir := os.Getenv("ZDOTDIR"); zdotdir != "" {
			return filepath.Join(zdotdir, ".zshrc"), nil
		}
		return filepath.Join(homeDir, ".zshrc"), nil
	case Fish:
		// Fish typically uses ~/.config/fish/config.fish
		configDir := filepath.Join(homeDir, ".config", "fish")
		return filepath.Join(configDir, "config.fish"), nil
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}

const (
	// rcBackupSuffix names the one backup InjectSourceLines keeps next to an
	// rc file: ~/.zshrc is copied to ~/.zshrc.ralph.bak before each changing
	// write, overwriting the previous backup.
	rcBackupSuffix = ".ralph.bak"

	// newRCFilePerm is the mode of an rc file InjectSourceLines creates.
	newRCFilePerm os.FileMode = 0o644
)

// ErrMalformedBlock reports an rc file whose RALPH MANAGED BLOCK markers do
// not pair up into exactly one block. ralph refuses to edit such a file,
// because it cannot tell which lines are its own.
var ErrMalformedBlock = errors.New("malformed ralph managed block")

// MalformedBlockFix tells the user how to repair an rc file that fails with
// ErrMalformedBlock.
const MalformedBlockFix = "restore the missing marker or delete the stray one"

// ManagedBlockBody returns the lines between the markers of the ralph managed
// block in content, reading the markers as InjectSourceLines does: legacy
// DOTTER markers count as ralph ones. found is false when content has no
// block. Markers that do not pair up are the ErrMalformedBlock that
// InjectSourceLines refuses the file with.
func ManagedBlockBody(content []byte) (body []string, found bool, err error) {
	lines := strings.Split(string(content), "\n")
	migrateLegacyMarkers(lines)
	begin, end, err := findRalphBlock(lines)
	if err != nil || begin == -1 {
		return nil, false, err
	}
	return lines[begin+1 : end], true, nil
}

// InjectSourceLines makes additionalLines the body of the ralph managed block
// in the given shell's rc file, appending the block when the file has none.
// A file whose block markers do not pair up is left as it is, and the error
// wraps ErrMalformedBlock.
//
// A changing write first copies the current file to the backup next to it,
// then replaces the file through a temp file and a rename, keeping its mode.
// A symlinked rc file is written at the link's destination, so the link stays.
// If dryRun is true, it prints what it would do instead of writing anything.
func InjectSourceLines(
	w io.Writer,
	shell SupportedShell,
	additionalLines []string,
	dryRun bool,
) error {
	rcFilePath, err := GetRCFilePath(shell)
	if err != nil {
		return fmt.Errorf("cannot get RC file path for %s: %w", shell, err)
	}

	rcDir := filepath.Dir(rcFilePath)
	if !dryRun {
		if err := os.MkdirAll(rcDir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory for rc file %s: %w", rcFilePath, err)
		}
	} else {
		// Check if dir exists in dry run for more accurate messaging
		if _, statErr := os.Stat(rcDir); os.IsNotExist(statErr) {
			fmt.Fprintf(w, "[DRY RUN] Would create directory for rc file %s\n", rcDir)
		}
	}

	rc, err := loadRCFile(rcFilePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(rc.content), "\n")
	newLines, modified, err := ensureRalphBlock(lines, additionalLines)
	if err != nil {
		return fmt.Errorf(
			"rc file %s: %w; %s, then rerun ralph (file left unchanged)",
			rcFilePath,
			err,
			MalformedBlockFix,
		)
	}
	if !modified {
		fmt.Fprintf(w, "RC file %s is already up to date.\n", rcFilePath)
		return nil
	}

	output := strings.Join(newLines, "\n")
	if output != "" && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	if dryRun {
		fmt.Fprintf(w, "[DRY RUN] Would update rc file: %s\n", rcFilePath)
		if rc.exists {
			fmt.Fprintf(w, "[DRY RUN] Would back up the current content to %s\n", rc.backupPath())
		}
		fmt.Fprintln(w, "[DRY RUN] New content would be:")
		fmt.Fprintln(w, output) // Potentially long, consider summarizing or showing diff
		return nil
	}
	fmt.Fprintf(w, "Updating rc file: %s\n", rcFilePath)
	return rc.replace([]byte(output))
}

// rcFile is a shell rc file as InjectSourceLines found it.
type rcFile struct {
	path    string      // the path the shell reads, e.g. ~/.zshrc
	target  string      // the file to write: path, or the link's destination
	content []byte      // current content; empty when the file does not exist
	perm    os.FileMode // permission bits a rewrite keeps
	exists  bool
}

// loadRCFile reads the rc file at path, following a symlink to its
// destination. A missing file loads as empty.
func loadRCFile(path string) (rcFile, error) {
	target, err := rcWriteTarget(path)
	if err != nil {
		return rcFile{}, err
	}
	rc := rcFile{path: path, target: target, perm: newRCFilePerm}
	info, err := os.Stat(target)
	if os.IsNotExist(err) {
		return rc, nil
	}
	if err != nil {
		return rcFile{}, fmt.Errorf("failed to stat rc file %s: %w", path, err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return rcFile{}, fmt.Errorf("failed to read rc file %s: %w", path, err)
	}
	rc.content, rc.perm, rc.exists = content, info.Mode().Perm(), true
	return rc, nil
}

// rcWriteTarget returns the file to write for the rc file at path: path
// itself, or the final destination when path is a symlink, so a rewrite
// keeps the link. A dangling link is an error: writing through it would
// create a file wherever it points, and replacing it would drop the link.
func rcWriteTarget(path string) (string, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to stat rc file %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf(
			"rc file %s is a symlink whose destination cannot be resolved; fix or remove the link: %w",
			path,
			err,
		)
	}
	return target, nil
}

// backupPath is where replace keeps the previous content. It sits next to
// the path the shell reads, so the backup of a symlinked rc file stays out
// of the repo the link points into.
func (rc rcFile) backupPath() string {
	return rc.path + rcBackupSuffix
}

// replace writes data in place of the rc file. When a file exists, its
// current content goes to the backup first, and a failed backup leaves the
// rc file untouched.
func (rc rcFile) replace(data []byte) error {
	if rc.exists {
		if err := writeFileAtomic(rc.backupPath(), rc.content, rc.perm); err != nil {
			return fmt.Errorf(
				"failed to back up rc file %s to %s: %w",
				rc.path,
				rc.backupPath(),
				err,
			)
		}
	}
	if err := writeFileAtomic(rc.target, data, rc.perm); err != nil {
		return fmt.Errorf("failed to write updated rc file %s: %w", rc.path, err)
	}
	return nil
}

// writeFileAtomic replaces path with data through a temp file in the same
// directory and a rename, so a crash or a full disk never leaves path half
// written. The rename replaces whatever is at path, a symlink included.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".ralph-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := writeAndClose(tmp, data, perm); err != nil {
		_ = os.Remove(tmpPath) // best effort; the write error is what matters
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath) // best effort; the rename error is what matters
		return err
	}
	return nil
}

// writeAndClose writes data to f, sets perm, flushes it to disk and closes it.
func writeAndClose(f *os.File, data []byte, perm os.FileMode) error {
	_, err := f.Write(data)
	if err == nil {
		err = f.Chmod(perm)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// ensureRalphBlock returns lines with a single well-formed RALPH managed block
// whose body is exactly contentLines, plus whether anything changed. It is
// idempotent: re-running with the same contentLines (comments and blank lines
// included) reports modified=false and returns the input unchanged. Markers
// that do not pair up into one block are an ErrMalformedBlock.
func ensureRalphBlock(lines []string, contentLines []string) ([]string, bool, error) {
	// If any legacy marker is migrated, the file must be rewritten even when
	// the block body already matches.
	legacyMigrated := migrateLegacyMarkers(lines)

	startIndex, endIndex, err := findRalphBlock(lines)
	if err != nil {
		return nil, false, err
	}
	if startIndex == -1 {
		return appendRalphBlock(lines, contentLines), true, nil
	}

	existing := lines[startIndex+1 : endIndex]
	if !legacyMigrated && slices.Equal(existing, contentLines) {
		return lines, false, nil
	}
	finalLines := append([]string{}, lines[:startIndex]...)
	finalLines = append(finalLines, RalphBlockBeginMarker)
	finalLines = append(finalLines, contentLines...)
	finalLines = append(finalLines, RalphBlockEndMarker)
	finalLines = append(finalLines, lines[endIndex+1:]...)
	return finalLines, true, nil
}

// migrateLegacyMarkers rewrites DOTTER block markers in lines to RALPH
// markers in place and reports whether it changed any.
func migrateLegacyMarkers(lines []string) bool {
	migrated := false
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case legacyBlockBeginMarker:
			lines[i] = RalphBlockBeginMarker
			migrated = true
		case legacyBlockEndMarker:
			lines[i] = RalphBlockEndMarker
			migrated = true
		}
	}
	return migrated
}

// findRalphBlock returns the indexes of the BEGIN and END marker lines of the
// one RALPH managed block in lines, or -1, -1 when lines have no markers.
// Markers that do not pair up into exactly one block (a BEGIN without an END
// after it, an END without a BEGIN before it, a nested or a second block) are
// an ErrMalformedBlock naming the first offending line.
func findRalphBlock(lines []string) (begin, end int, err error) {
	begin, end = -1, -1
	open := -1 // index of a BEGIN still waiting for its END
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case RalphBlockBeginMarker:
			if open != -1 {
				return -1, -1, malformedBlock(
					i,
					"%q opens a block while the one on line %d is still open",
					RalphBlockBeginMarker,
					open+1,
				)
			}
			if begin != -1 {
				return -1, -1, malformedBlock(
					i,
					"%q starts a second block after the one on lines %d-%d",
					RalphBlockBeginMarker,
					begin+1,
					end+1,
				)
			}
			open = i
		case RalphBlockEndMarker:
			if open == -1 {
				return -1, -1, malformedBlock(i, "%q has no %q before it",
					RalphBlockEndMarker, RalphBlockBeginMarker)
			}
			begin, end, open = open, i, -1
		}
	}
	if open != -1 {
		return -1, -1, malformedBlock(open, "%q has no %q after it",
			RalphBlockBeginMarker, RalphBlockEndMarker)
	}
	return begin, end, nil
}

// malformedBlock returns an ErrMalformedBlock for the line at index.
func malformedBlock(index int, format string, args ...any) error {
	return fmt.Errorf("%w: line %d: %s", ErrMalformedBlock, index+1, fmt.Sprintf(format, args...))
}

// appendRalphBlock returns lines with a block holding contentLines appended,
// after one blank line when lines have content. lines must have no markers.
func appendRalphBlock(lines []string, contentLines []string) []string {
	kept := slices.Clone(lines)
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	if len(kept) > 0 {
		kept = append(kept, "")
	}
	kept = append(kept, RalphBlockBeginMarker)
	kept = append(kept, contentLines...)
	return append(kept, RalphBlockEndMarker)
}

// GetSupportedShells returns a slice of shells ralph explicitly supports for RC file management.
func GetSupportedShells() []SupportedShell {
	return []SupportedShell{Bash, Zsh, Fish}
}

// isSupported returns true if the given shell is in the supported set.
func isSupported(s SupportedShell) bool {
	for _, supported := range GetSupportedShells() {
		if s == supported {
			return true
		}
	}
	return false
}

// ResolveShell determines which shell(s) to target using the following precedence:
// 1. Explicit config value (shell.name in config.toml)
// 2. Auto-detect from $SHELL environment variable
// 3. Fallback to all supported shells
func ResolveShell(configShellName string) []SupportedShell {
	if configShellName != "" {
		s := SupportedShell(configShellName)
		if isSupported(s) {
			return []SupportedShell{s}
		}
	}
	if detected := AutoDetectShell(); detected != "" {
		return []SupportedShell{detected}
	}
	return GetSupportedShells()
}

// AutoDetectShell attempts to determine the current shell from environment variables.
// This is a basic detection and might not be exhaustive.
func AutoDetectShell() SupportedShell {
	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		// On Windows, SHELL might not be set. ComProc might be cmd.exe.
		// For now, we focus on Unix-like shells.
		return "" // Cannot determine
	}

	shellName := filepath.Base(shellPath)
	switch shellName {
	case "bash":
		return Bash
	case "zsh":
		return Zsh
	case "fish":
		return Fish
	default:
		fmt.Fprintf(
			os.Stderr,
			"Warning: Unrecognized shell %s, cannot auto-configure rc file.\n",
			shellName,
		)
		return "" // Or a generic/unknown type
	}
}

// GetRalphGeneratedDir defines the function to get the ralph generated scripts directory.
// This is a variable to allow for easier testing.
var GetRalphGeneratedDir = getRalphGeneratedDirInternal

// getRalphGeneratedDirInternal returns the directory path where ralph stores its generated scripts:
// generated/ in the state directory (paths.StateDir), e.g. ~/.config/ralph/generated.
func getRalphGeneratedDirInternal() (string, error) {
	dir, err := paths.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "generated"), nil
}
