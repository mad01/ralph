package apply

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/config"
)

func TestIntendedManifest_TracksOwnedDotfile(t *testing.T) {
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

	got, _, _ := IntendedManifest(cfg, "anyhost", time.Unix(1700000000, 0))
	rec := got.Recipes["fooer"]
	if len(rec.Symlinks) != 1 || rec.Symlinks[0] != "/tmp/foo.conf" {
		t.Errorf("expected /tmp/foo.conf in symlinks, got %v", rec.Symlinks)
	}
	if rec.DeleteBehavior != "delete" {
		t.Errorf("expected delete_behavior=delete, got %q", rec.DeleteBehavior)
	}
}

func TestIntendedManifest_PersistsUninstallHooks(t *testing.T) {
	cfg := &config.Config{
		LoadedRecipes: []config.LoadedRecipeInfo{
			{
				Name:           "svc",
				DeleteBehavior: "delete",
				PreUninstall:   []string{"t-man remove svc"},
				PostUninstall:  []string{"echo bye"},
			},
		},
	}
	got, _, _ := IntendedManifest(cfg, "anyhost", time.Now())
	rec := got.Recipes["svc"]
	if len(rec.PreUninstall) != 1 || rec.PreUninstall[0] != "t-man remove svc" {
		t.Errorf("expected pre_uninstall persisted, got %v", rec.PreUninstall)
	}
	if len(rec.PostUninstall) != 1 || rec.PostUninstall[0] != "echo bye" {
		t.Errorf("expected post_uninstall persisted, got %v", rec.PostUninstall)
	}
}

func TestIntendedManifest_SkipsItemsWithoutOwner(t *testing.T) {
	cfg := &config.Config{
		Dotfiles: map[string]config.Dotfile{
			"orphan": {Source: "x", Target: "/tmp/x"}, // no OwnerRecipe
		},
	}
	got, _, _ := IntendedManifest(cfg, "anyhost", time.Now())
	if len(got.Recipes) != 0 {
		t.Errorf("expected no tracked recipes, got %v", got.Recipes)
	}
}

func TestIntendedManifest_RespectsHostFilter(t *testing.T) {
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
	got, _, _ := IntendedManifest(cfg, "myhost", time.Now())
	if rec, ok := got.Recipes["fooer"]; ok && len(rec.Symlinks) > 0 {
		t.Errorf("expected host-filtered item to be excluded, got %v", rec.Symlinks)
	}
}

func TestIntendedManifest_RespectsProfileFilter(t *testing.T) {
	cfg := &config.Config{
		Profiles: []string{"personal"},
		Dotfiles: map[string]config.Dotfile{
			"foo": {
				Source:      "x",
				Target:      "/tmp/x",
				Profiles:    []string{"work"},
				OwnerRecipe: "fooer",
			},
		},
	}
	got, _, _ := IntendedManifest(cfg, "anyhost", time.Now())
	if rec, ok := got.Recipes["fooer"]; ok && len(rec.Symlinks) > 0 {
		t.Errorf("expected profile-filtered item to be excluded, got %v", rec.Symlinks)
	}

	cfg.Profiles = []string{"work"}
	got, _, _ = IntendedManifest(cfg, "anyhost", time.Now())
	if rec := got.Recipes["fooer"]; len(rec.Symlinks) != 1 || rec.Symlinks[0] != "/tmp/x" {
		t.Errorf("expected profile-matching item to be tracked, got %v", rec.Symlinks)
	}
}

func TestIntendedManifest_RespectsEnableFalse(t *testing.T) {
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
	got, _, _ := IntendedManifest(cfg, "anyhost", time.Now())
	if rec, ok := got.Recipes["fooer"]; ok && len(rec.Symlinks) > 0 {
		t.Errorf("expected disabled item to be excluded, got %v", rec.Symlinks)
	}
}

func TestIntendedManifest_TracksInstallPathsFromPackagesAndBuilds(t *testing.T) {
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
	got, _, _ := IntendedManifest(cfg, "anyhost", time.Now())
	if rec := got.Recipes["brain"]; len(rec.InstallPaths) != 1 ||
		rec.InstallPaths[0] != "/tmp/brain" {
		t.Errorf("expected brain install_paths to include /tmp/brain, got %v", rec.InstallPaths)
	}
	if rec := got.Recipes["claude-mcp"]; len(rec.InstallPaths) != 1 ||
		rec.InstallPaths[0] != "/tmp/foo" {
		t.Errorf("expected claude-mcp install_paths to include /tmp/foo, got %v", rec.InstallPaths)
	}
}

func TestIntendedManifest_PersistsRecipeSource(t *testing.T) {
	cfg := &config.Config{
		LoadedRecipes: []config.LoadedRecipeInfo{
			{Name: "moon/app", RecipeSource: "moon"},
		},
	}

	got, _, err := IntendedManifest(cfg, "anyhost", time.Now())
	if err != nil {
		t.Fatalf("IntendedManifest() error = %v", err)
	}
	if source := got.Recipes["moon/app"].RecipeSource; source != "moon" {
		t.Errorf("RecipeSource = %q, want moon", source)
	}
}

func TestIntendedManifest_ReadDirErrorAborts(t *testing.T) {
	cfg := &config.Config{
		DotfilesRepoPath: t.TempDir(),
		DirsMirror: map[string]config.DirMirror{
			"m": {
				Source:      "does-not-exist",
				Target:      "/tmp/mirror-target",
				OwnerRecipe: "mirrorer",
			},
		},
	}
	// A transient/unreadable mirror source must abort manifest construction
	// rather than silently dropping the recipe's artifacts (which would make
	// live symlinks look like orphans and get deleted).
	_, _, err := IntendedManifest(cfg, "anyhost", time.Now())
	if err == nil {
		t.Fatal("expected error when dirs_mirror source cannot be read, got nil")
	}
}

// C005 — remote/make package clone dirs are tracked, and abandoned-with-log on
// removal (never auto-deleted).
func TestIntendedManifest_TracksPackageClone(t *testing.T) {
	cfg := &config.Config{
		PackagesDir: "/tmp/ralph-pkgs",
		Packages: map[string]config.Package{
			"csl": {
				Source:      "remote",
				Repo:        "git@github.com:mad01/csl.git",
				OwnerRecipe: "packages",
			},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{{Name: "packages", DeleteBehavior: "delete"}},
	}
	got, _, err := IntendedManifest(cfg, "anyhost", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rec := got.Recipes["packages"]
	want := filepath.Join("/tmp/ralph-pkgs", "csl")
	if len(rec.PackageClones) != 1 || rec.PackageClones[0] != want {
		t.Errorf("expected package clone %q tracked, got %v", want, rec.PackageClones)
	}
}
