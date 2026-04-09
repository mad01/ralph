package skills

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestListBundled(t *testing.T) {
	names, err := ListBundled()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(names) < 3 {
		t.Fatalf("expected at least 3 bundled skills, got %d: %v", len(names), names)
	}

	expected := map[string]bool{"ralph": false, "ralph-recipe": false, "ralph-debug": false}
	for _, name := range names {
		if _, ok := expected[name]; ok {
			expected[name] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("expected bundled skill %q not found", name)
		}
	}
}

func TestInstallSkill_NewDirectory(t *testing.T) {
	targetDir := t.TempDir()

	result := installSkill(io.Discard, "ralph", targetDir, InstallOptions{})

	if result.Action != "installed" {
		t.Errorf("expected action 'installed', got %q: %s", result.Action, result.Message)
	}

	// Verify SKILL.md was written
	skillMD := filepath.Join(targetDir, "ralph", "SKILL.md")
	if _, err := os.Stat(skillMD); os.IsNotExist(err) {
		t.Error("expected SKILL.md to be written")
	}

	// Verify marker was written
	marker := filepath.Join(targetDir, "ralph", ".ralph-managed")
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		t.Error("expected .ralph-managed marker to be written")
	}
}

func TestInstallSkill_UpdatesRalphManaged(t *testing.T) {
	targetDir := t.TempDir()

	// First install
	result1 := installSkill(io.Discard, "ralph", targetDir, InstallOptions{})
	if result1.Action != "installed" {
		t.Fatalf("first install: expected 'installed', got %q", result1.Action)
	}

	// Second install — should update (has marker)
	result2 := installSkill(io.Discard, "ralph", targetDir, InstallOptions{})
	if result2.Action != "installed" {
		t.Errorf("second install: expected 'installed' (update), got %q: %s", result2.Action, result2.Message)
	}
}

func TestInstallSkill_SkipsExternalSymlink(t *testing.T) {
	targetDir := t.TempDir()
	otherDir := t.TempDir()

	// Pre-create a symlink (simulating external management)
	os.Symlink(otherDir, filepath.Join(targetDir, "ralph"))

	result := installSkill(io.Discard, "ralph", targetDir, InstallOptions{})

	if result.Action != "skipped" {
		t.Errorf("expected 'skipped', got %q: %s", result.Action, result.Message)
	}
}

func TestInstallSkill_ForceOverwritesSymlink(t *testing.T) {
	targetDir := t.TempDir()
	otherDir := t.TempDir()

	os.Symlink(otherDir, filepath.Join(targetDir, "ralph"))

	result := installSkill(io.Discard, "ralph", targetDir, InstallOptions{Force: true})

	if result.Action != "installed" {
		t.Errorf("expected 'installed', got %q: %s", result.Action, result.Message)
	}

	// Should be a real directory now, not a symlink
	info, err := os.Lstat(filepath.Join(targetDir, "ralph"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected real directory, got symlink")
	}
}

func TestInstallSkill_SkipsUnmanagedDir(t *testing.T) {
	targetDir := t.TempDir()

	// Create a directory without our marker
	os.MkdirAll(filepath.Join(targetDir, "ralph"), 0755)
	os.WriteFile(filepath.Join(targetDir, "ralph", "SKILL.md"), []byte("custom"), 0644)

	result := installSkill(io.Discard, "ralph", targetDir, InstallOptions{})

	if result.Action != "skipped" {
		t.Errorf("expected 'skipped', got %q: %s", result.Action, result.Message)
	}
}

func TestInstallSkill_DryRun(t *testing.T) {
	targetDir := t.TempDir()

	result := installSkill(io.Discard, "ralph", targetDir, InstallOptions{DryRun: true})

	if result.Action != "installed" {
		t.Errorf("expected 'installed', got %q", result.Action)
	}

	// Directory should NOT exist
	if _, err := os.Stat(filepath.Join(targetDir, "ralph")); !os.IsNotExist(err) {
		t.Error("expected directory to not be created in dry run")
	}
}

func TestInstall_WritesAllBundledSkills(t *testing.T) {
	// Override the target dir by testing Install end-to-end
	// Install writes to DefaultClaudeSkillsDir, so we test via installSkill for each bundled skill
	targetDir := t.TempDir()

	names, err := ListBundled()
	if err != nil {
		t.Fatalf("ListBundled: %v", err)
	}

	for _, name := range names {
		result := installSkill(io.Discard, name, targetDir, InstallOptions{})
		if result.Action != "installed" {
			t.Errorf("skill %s: expected 'installed', got %q: %s", name, result.Action, result.Message)
		}

		// Verify SKILL.md exists and is non-empty
		data, err := os.ReadFile(filepath.Join(targetDir, name, "SKILL.md"))
		if err != nil {
			t.Errorf("skill %s: failed to read SKILL.md: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("skill %s: SKILL.md is empty", name)
		}

		// Verify marker exists
		if !hasMarker(filepath.Join(targetDir, name)) {
			t.Errorf("skill %s: missing .ralph-managed marker", name)
		}
	}
}

func TestInstall_EmbeddedContentMatchesBundled(t *testing.T) {
	// Verify that embedded SKILL.md files contain expected frontmatter
	names, err := ListBundled()
	if err != nil {
		t.Fatalf("ListBundled: %v", err)
	}

	for _, name := range names {
		data, err := bundledSkills.ReadFile(filepath.Join("bundled", name, "SKILL.md"))
		if err != nil {
			t.Errorf("skill %s: failed to read from embed: %v", name, err)
			continue
		}

		content := string(data)
		if len(content) < 50 {
			t.Errorf("skill %s: embedded SKILL.md suspiciously short (%d bytes)", name, len(content))
		}
		// Verify YAML frontmatter exists
		if content[:4] != "---\n" {
			t.Errorf("skill %s: SKILL.md missing YAML frontmatter", name)
		}
	}
}

func TestInstall_FullFlow(t *testing.T) {
	// Temporarily override DefaultClaudeSkillsDir is not feasible,
	// so test the Install function by calling it and checking results
	results := Install(io.Discard, InstallOptions{DryRun: true})

	if len(results) < 3 {
		t.Fatalf("expected at least 3 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Action == "error" {
			t.Errorf("skill %s: unexpected error: %s: %v", r.Name, r.Message, r.Err)
		}
	}
}
