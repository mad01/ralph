package skills

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mad01/ralph/internal/config"
)

//go:embed all:bundled
var bundledSkills embed.FS

// InstallOptions holds options for skill installation.
type InstallOptions struct {
	DryRun bool
	Force  bool
}

// InstallResult holds the result of installing a single skill.
type InstallResult struct {
	Name    string
	Action  string // "installed", "updated", "skipped", "error"
	Message string
	Err     error
}

// ListBundled returns the names of all bundled skills.
func ListBundled() ([]string, error) {
	entries, err := fs.ReadDir(bundledSkills, "bundled")
	if err != nil {
		return nil, fmt.Errorf("reading bundled skills: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Verify SKILL.md exists
		if _, err := fs.Stat(bundledSkills, filepath.Join("bundled", entry.Name(), "SKILL.md")); err == nil {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// Install writes bundled skills to the Claude skills directory (~/.claude/skills/).
func Install(w io.Writer, opts InstallOptions) []InstallResult {
	var results []InstallResult

	targetDir, err := config.ExpandPath(config.DefaultClaudeSkillsDir)
	if err != nil {
		targetDir = config.DefaultClaudeSkillsDir
	}

	if !opts.DryRun {
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			results = append(results, InstallResult{
				Name:    "skills-dir",
				Action:  "error",
				Message: fmt.Sprintf("failed to create %s", targetDir),
				Err:     err,
			})
			return results
		}
	}

	names, err := ListBundled()
	if err != nil {
		results = append(results, InstallResult{
			Name:    "bundled",
			Action:  "error",
			Message: "failed to list bundled skills",
			Err:     err,
		})
		return results
	}

	for _, name := range names {
		result := installSkill(w, name, targetDir, opts)
		results = append(results, result)
	}

	return results
}

func installSkill(w io.Writer, name, targetDir string, opts InstallOptions) InstallResult {
	skillDir := filepath.Join(targetDir, name)

	// Check if already exists
	if info, err := os.Lstat(skillDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			// It's a symlink — someone manages this externally
			if !opts.Force {
				return InstallResult{
					Name:    name,
					Action:  "skipped",
					Message: "exists (symlink, managed externally), use --force to overwrite",
				}
			}
			if !opts.DryRun {
				os.Remove(skillDir)
			}
		} else if info.IsDir() {
			// Directory exists — check if it has our marker
			if !opts.Force {
				if hasMarker(skillDir) {
					// We installed this before, update it
					if !opts.DryRun {
						os.RemoveAll(skillDir)
					}
				} else {
					return InstallResult{
						Name:    name,
						Action:  "skipped",
						Message: "exists (not managed by ralph), use --force to overwrite",
					}
				}
			} else {
				if !opts.DryRun {
					os.RemoveAll(skillDir)
				}
			}
		}
	}

	if opts.DryRun {
		fmt.Fprintf(w, "  [DRY RUN] Would install skill: %s → %s\n", name, skillDir)
		return InstallResult{
			Name:    name,
			Action:  "installed",
			Message: "[DRY RUN] would install",
		}
	}

	// Write all files for this skill
	srcDir := filepath.Join("bundled", name)
	if err := writeEmbeddedDir(bundledSkills, srcDir, skillDir); err != nil {
		return InstallResult{
			Name:    name,
			Action:  "error",
			Message: "failed to write skill files",
			Err:     err,
		}
	}

	// Write marker so we know we installed this
	writeMarker(skillDir)

	fmt.Fprintf(w, "  Installed: %s → %s\n", name, skillDir)
	return InstallResult{
		Name:    name,
		Action:  "installed",
		Message: "installed",
	}
}

func writeEmbeddedDir(fsys embed.FS, srcDir, destDir string) error {
	return fs.WalkDir(fsys, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Compute relative path from srcDir
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(destDir, rel)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}

		data, err := fsys.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", path, err)
		}

		return os.WriteFile(destPath, data, 0o644)
	})
}

const markerFile = ".ralph-managed"

func hasMarker(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, markerFile))
	return err == nil
}

func writeMarker(dir string) {
	os.WriteFile(
		filepath.Join(dir, markerFile),
		[]byte("installed by ralph install-skills\n"),
		0o644,
	)
}
