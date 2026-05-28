package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

func TestBuildIntendedManifest_TracksOwnedDotfile(t *testing.T) {
	cfg := &config.Config{
		Dotfiles: map[string]config.Dotfile{
			"foo": {
				Source:      "foo.conf",
				Target:      "/tmp/foo.conf",
				OwnerRecipe: "fooer",
			},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{
			{Name: "fooer", DeleteBehavior: "delete"},
		},
	}

	got := buildIntendedManifest(cfg, "anyhost", time.Unix(1700000000, 0))
	rec := got.Recipes["fooer"]
	if len(rec.Symlinks) != 1 || rec.Symlinks[0] != "/tmp/foo.conf" {
		t.Errorf("expected /tmp/foo.conf in symlinks, got %v", rec.Symlinks)
	}
	if rec.DeleteBehavior != "delete" {
		t.Errorf("expected delete_behavior=delete, got %q", rec.DeleteBehavior)
	}
}

func TestBuildIntendedManifest_SkipsItemsWithoutOwner(t *testing.T) {
	cfg := &config.Config{
		Dotfiles: map[string]config.Dotfile{
			"orphan": {Source: "x", Target: "/tmp/x"}, // no OwnerRecipe
		},
	}
	got := buildIntendedManifest(cfg, "anyhost", time.Now())
	if len(got.Recipes) != 0 {
		t.Errorf("expected no tracked recipes, got %v", got.Recipes)
	}
}

func TestBuildIntendedManifest_RespectsHostFilter(t *testing.T) {
	cfg := &config.Config{
		Dotfiles: map[string]config.Dotfile{
			"foo": {
				Source:      "x",
				Target:      "/tmp/x",
				Hosts:       []string{"otherhost"},
				OwnerRecipe: "fooer",
			},
		},
	}
	got := buildIntendedManifest(cfg, "myhost", time.Now())
	if rec, ok := got.Recipes["fooer"]; ok && len(rec.Symlinks) > 0 {
		t.Errorf("expected host-filtered item to be excluded, got %v", rec.Symlinks)
	}
}

func TestBuildIntendedManifest_RespectsEnableFalse(t *testing.T) {
	enabled := false
	cfg := &config.Config{
		Dotfiles: map[string]config.Dotfile{
			"foo": {
				Source:      "x",
				Target:      "/tmp/x",
				OwnerRecipe: "fooer",
				Enable:      &enabled,
			},
		},
	}
	got := buildIntendedManifest(cfg, "anyhost", time.Now())
	if rec, ok := got.Recipes["fooer"]; ok && len(rec.Symlinks) > 0 {
		t.Errorf("expected disabled item to be excluded, got %v", rec.Symlinks)
	}
}

func TestBuildIntendedManifest_TracksInstallPathsFromPackagesAndBuilds(t *testing.T) {
	cfg := &config.Config{
		Packages: map[string]config.Package{
			"brain": {
				Source:       "local",
				Build:        []string{"make"},
				InstallPaths: []string{"/tmp/brain"},
				OwnerRecipe:  "brain",
			},
		},
		Hooks: config.HooksConfig{
			Builds: map[string]config.Build{
				"go_install": {
					Commands:     []string{"go install foo"},
					Run:          "always",
					InstallPaths: []string{"/tmp/foo"},
					OwnerRecipe:  "claude-mcp",
				},
			},
		},
	}
	got := buildIntendedManifest(cfg, "anyhost", time.Now())
	if rec := got.Recipes["brain"]; len(rec.InstallPaths) != 1 || rec.InstallPaths[0] != "/tmp/brain" {
		t.Errorf("expected brain install_paths to include /tmp/brain, got %v", rec.InstallPaths)
	}
	if rec := got.Recipes["claude-mcp"]; len(rec.InstallPaths) != 1 || rec.InstallPaths[0] != "/tmp/foo" {
		t.Errorf("expected claude-mcp install_paths to include /tmp/foo, got %v", rec.InstallPaths)
	}
}

func TestRunCleanup_DeleteRemovesOrphanedSymlinks(t *testing.T) {
	dir, err := os.MkdirTemp("", "ralph-cleanup-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

	link := filepath.Join(dir, "old.link")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("fooer", state.KindSymlink, link)
	prev.SetMetadata("fooer", time.Now(), "delete")
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	runCleanup(prev, next, false, logger, phase)

	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("expected orphan symlink removed, lstat err=%v", err)
	}
	if !strings.Contains(logger.String(), "removed symlink") {
		t.Errorf("expected log line for removal, got: %s", logger.String())
	}
}

func TestRunCleanup_AbandonLeavesArtifactsInPlace(t *testing.T) {
	dir, err := os.MkdirTemp("", "ralph-cleanup-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

	link := filepath.Join(dir, "abandon.link")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("fooer", state.KindSymlink, link)
	prev.SetMetadata("fooer", time.Now(), "abandon")
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	runCleanup(prev, next, false, logger, phase)

	if _, err := os.Lstat(link); err != nil {
		t.Errorf("expected abandoned symlink to remain, got %v", err)
	}
	if !strings.Contains(logger.String(), "abandoned symlink") {
		t.Errorf("expected abandon log line, got: %s", logger.String())
	}
}

func TestRunCleanup_DryRunDoesNotModify(t *testing.T) {
	dir, err := os.MkdirTemp("", "ralph-cleanup-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

	link := filepath.Join(dir, "dryrun.link")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("fooer", state.KindSymlink, link)
	prev.SetMetadata("fooer", time.Now(), "delete")
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	runCleanup(prev, next, true, logger, phase) // dryRun=true

	if _, err := os.Lstat(link); err != nil {
		t.Errorf("expected symlink to remain in dry-run, got %v", err)
	}
	if !strings.Contains(logger.String(), "would remove") {
		t.Errorf("expected 'would remove' log in dry-run, got: %s", logger.String())
	}
}

func TestRunCleanup_RepoOrphanIsAlwaysAbandoned(t *testing.T) {
	dir, err := os.MkdirTemp("", "ralph-cleanup-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

	repoDir := filepath.Join(dir, "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("fooer", state.KindRepo, repoDir)
	prev.SetMetadata("fooer", time.Now(), "delete") // even with delete, repos must abandon
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	runCleanup(prev, next, false, logger, phase)

	if _, err := os.Stat(repoDir); err != nil {
		t.Errorf("expected repo to be untouched, got %v", err)
	}
	if !strings.Contains(logger.String(), "abandoned repo") {
		t.Errorf("expected abandon log for repo, got: %s", logger.String())
	}
}

func TestBuildIntendedManifest_TracksToolConfigFiles(t *testing.T) {
	cfg := &config.Config{
		Tools: []config.Tool{
			{
				Name:         "nvim",
				CheckCommand: "which nvim",
				ConfigFiles: []config.Dotfile{
					{
						Source:      "nvim/init.lua",
						Target:      "/tmp/nvim/init.lua",
						OwnerRecipe: "editors",
					},
					{
						Source:      "nvim/plugins.lua",
						Target:      "/tmp/nvim/plugins.lua",
						Action:      "copy",
						OwnerRecipe: "editors",
					},
				},
			},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{
			{Name: "editors", DeleteBehavior: "delete"},
		},
	}

	got := buildIntendedManifest(cfg, "anyhost", time.Unix(1700000000, 0))
	rec := got.Recipes["editors"]

	// init.lua should be tracked as a symlink (default action)
	foundSymlink := false
	for _, s := range rec.Symlinks {
		if s == "/tmp/nvim/init.lua" {
			foundSymlink = true
		}
	}
	if !foundSymlink {
		t.Errorf("expected /tmp/nvim/init.lua in symlinks, got %v", rec.Symlinks)
	}

	// plugins.lua should be tracked as a copy
	foundCopy := false
	for _, c := range rec.Copies {
		if c == "/tmp/nvim/plugins.lua" {
			foundCopy = true
		}
	}
	if !foundCopy {
		t.Errorf("expected /tmp/nvim/plugins.lua in copies, got %v", rec.Copies)
	}
}

func TestBuildIntendedManifest_ToolConfigFilesSkipNoOwner(t *testing.T) {
	cfg := &config.Config{
		Tools: []config.Tool{
			{
				Name:         "tool",
				CheckCommand: "which tool",
				ConfigFiles: []config.Dotfile{
					{
						Source: "tool.conf",
						Target: "/tmp/tool.conf",
						// No OwnerRecipe set
					},
				},
			},
		},
	}

	got := buildIntendedManifest(cfg, "anyhost", time.Now())
	if len(got.Recipes) != 0 {
		t.Errorf("expected no tracked recipes for tool config without owner, got %v", got.Recipes)
	}
}

func TestRunCleanup_NoOrphans_NoOp(t *testing.T) {
	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("fooer", state.KindSymlink, "/tmp/keep")
	prev.SetMetadata("fooer", time.Now(), "delete")
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	next.AddArtifact("fooer", state.KindSymlink, "/tmp/keep")
	next.SetMetadata("fooer", time.Now(), "delete")

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	runCleanup(prev, next, false, &bytes.Buffer{}, phase)

	ok, _, _, _ := phase.Counts()
	if ok < 1 {
		t.Errorf("expected at least one OK entry for the no-op cleanup, got phase counts: ok=%d", ok)
	}
}
