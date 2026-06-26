package shell

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// InjectSourceLines ensures that the specified sourceLine (e.g., "source ~/.config/ralph/generated.sh")
// is present in the ralph managed block of the given shell rc file.
// If the block doesn't exist, it's created.
// If the line already exists in the block, it's not added again.
// additionalLines are other lines to ensure are within the block.
// If dryRun is true, it prints what it would do instead of modifying the file.
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

	fileContent, err := os.ReadFile(rcFilePath)
	if os.IsNotExist(err) {
		fileContent = []byte{}
	} else if err != nil {
		return fmt.Errorf("failed to read rc file %s: %w", rcFilePath, err)
	}

	lines := strings.Split(string(fileContent), "\n")
	newLines, modified := ensureRalphBlock(lines, additionalLines)

	if modified {
		output := strings.Join(newLines, "\n")
		if output != "" && !strings.HasSuffix(output, "\n") {
			output += "\n"
		}
		if dryRun {
			fmt.Fprintf(w, "[DRY RUN] Would update rc file: %s\n", rcFilePath)
			fmt.Fprintln(w, "[DRY RUN] New content would be:")
			fmt.Fprintln(w, output) // Potentially long, consider summarizing or showing diff
		} else {
			fmt.Fprintf(w, "Updating rc file: %s\n", rcFilePath)
			if err := os.WriteFile(rcFilePath, []byte(output), 0o644); err != nil {
				return fmt.Errorf("failed to write updated rc file %s: %w", rcFilePath, err)
			}
		}
	} else {
		fmt.Fprintf(w, "RC file %s is already up to date.\n", rcFilePath)
	}
	return nil
}

// ensureRalphBlock returns lines with a single well-formed RALPH managed block
// whose body is exactly contentLines, plus whether anything changed. It is
// idempotent: re-running with the same contentLines (comments and blank lines
// included) reports modified=false and returns the input unchanged.
func ensureRalphBlock(lines []string, contentLines []string) ([]string, bool) {
	// Migrate any legacy DOTTER markers to RALPH markers. If we change any,
	// the file must be rewritten even when the block body already matches.
	legacyMigrated := false
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case legacyBlockBeginMarker:
			lines[i] = RalphBlockBeginMarker
			legacyMigrated = true
		case legacyBlockEndMarker:
			lines[i] = RalphBlockEndMarker
			legacyMigrated = true
		}
	}

	// Locate a well-formed block: a begin marker followed by a later end marker.
	startIndex, endIndex := -1, -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == RalphBlockBeginMarker && startIndex == -1 {
			startIndex = i
		} else if trimmed == RalphBlockEndMarker && startIndex != -1 {
			endIndex = i
			break
		}
	}

	if startIndex != -1 && endIndex != -1 {
		// Well-formed block: rewrite only if the body differs (or we migrated
		// legacy markers above).
		existing := lines[startIndex+1 : endIndex]
		if !legacyMigrated && slices.Equal(existing, contentLines) {
			return lines, false
		}
		finalLines := append([]string{}, lines[:startIndex]...)
		finalLines = append(finalLines, RalphBlockBeginMarker)
		finalLines = append(finalLines, contentLines...)
		finalLines = append(finalLines, RalphBlockEndMarker)
		finalLines = append(finalLines, lines[endIndex+1:]...)
		return finalLines, true
	}

	// No well-formed block (absent or malformed, e.g. missing end marker).
	// Strip any partial block markers and their stale body, then append a
	// fresh block after the surviving content.
	cleaned := []string{}
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == RalphBlockBeginMarker {
			inBlock = true
			continue
		}
		if trimmed == RalphBlockEndMarker && inBlock {
			inBlock = false
			continue
		}
		if !inBlock {
			cleaned = append(cleaned, line)
		}
	}
	for len(cleaned) > 0 && strings.TrimSpace(cleaned[len(cleaned)-1]) == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	if len(cleaned) > 0 {
		cleaned = append(cleaned, "") // blank line before our block if file not empty
	}
	cleaned = append(cleaned, RalphBlockBeginMarker)
	cleaned = append(cleaned, contentLines...)
	cleaned = append(cleaned, RalphBlockEndMarker)
	return cleaned, true
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

// getRalphGeneratedDirInternal returns the directory path where ralph stores its generated scripts.
// e.g. ~/.config/ralph/generated or $XDG_CONFIG_HOME/ralph/generated
func getRalphGeneratedDirInternal() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %w", err)
		}
		configHome = filepath.Join(homeDir, ".config")
	}
	return filepath.Join(configHome, "ralph", "generated"), nil
}
