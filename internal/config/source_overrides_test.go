package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// commitSourceFile writes rel inside a source repo made by makeSourceRepo and
// commits it, so a checkout of the default branch sees it.
func commitSourceFile(t *testing.T, repoPath, rel, content string) {
	t.Helper()
	path := filepath.Join(repoPath, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "add " + rel}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// isolateHome points HOME at a fresh temp dir so source checkouts land under
// a throwaway ~/.config/ralph/sources.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

func TestLoadSourceOverrides(t *testing.T) {
	disabled := false
	tests := []struct {
		name    string
		content *string // nil = no file
		want    map[string]RecipeOverride
		wantErr string
	}{
		{
			name: "missing file is empty",
		},
		{
			name: "valid overrides",
			content: ptr(`
[recipes_config.overrides."moon/app"]
enable = false

[recipes_config.overrides."moon/tool"]
hosts = ["work-laptop"]
vars = { alias_name = "del" }
`),
			want: map[string]RecipeOverride{
				"moon/app": {Enable: &disabled},
				"moon/tool": {
					Hosts: []string{"work-laptop"},
					Vars:  map[string]string{"alias_name": "del"},
				},
			},
		},
		{
			name: "unknown top-level table is an error",
			content: ptr(`
[recipes_config.overrides."moon/app"]
enable = false

[dotfiles.stray]
source = "x"
target = "~/.x"
`),
			wantErr: "unknown keys",
		},
		{
			name: "unknown field inside an entry is an error",
			content: ptr(`
[recipes_config.overrides."moon/app"]
enabled = false
`),
			wantErr: "unknown keys",
		},
		{
			name:    "malformed toml is an error",
			content: ptr(`[recipes_config.overrides."moon/app"`),
			wantErr: "failed to decode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkout := t.TempDir()
			if tt.content != nil {
				path := filepath.Join(checkout, SourceOverridesFileName)
				if err := os.WriteFile(path, []byte(*tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got, err := LoadSourceOverrides(checkout)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadSourceOverrides() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d overrides, want %d: %v", len(got), len(tt.want), got)
			}
			for key, want := range tt.want {
				o, ok := got[key]
				if !ok {
					t.Errorf("missing override %q", key)
					continue
				}
				if !sameEnable(o.Enable, want.Enable) {
					t.Errorf("%s: Enable = %v, want %v", key, o.Enable, want.Enable)
				}
				if strings.Join(o.Hosts, ",") != strings.Join(want.Hosts, ",") {
					t.Errorf("%s: Hosts = %v, want %v", key, o.Hosts, want.Hosts)
				}
				for k, v := range want.Vars {
					if o.Vars[k] != v {
						t.Errorf("%s: Vars[%s] = %q, want %q", key, k, o.Vars[k], v)
					}
				}
			}
		})
	}
}

func ptr(s string) *string { return &s }

func sameEnable(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func TestProcessRecipes_SourceOverrides_DisablesOtherSourceRecipe(t *testing.T) {
	isolateHome(t)
	moon, _, _ := makeSourceRepo(t, "app")
	work, _, _ := makeSourceRepo(t, "tool")
	commitSourceFile(t, work, SourceOverridesFileName, `
[recipes_config.overrides."moon/app"]
enable = false
`)

	cfg := &Config{
		DotfilesRepoPath: t.TempDir(),
		// moon is declared (and processed) first; work's overrides must still
		// reach it, which is why overrides are collected before discovery.
		RecipeSources: []RecipeSource{
			{Name: "moon", URL: moon},
			{Name: "work", URL: work},
		},
	}
	if err := ProcessRecipes(cfg, "testhost"); err != nil {
		t.Fatalf("ProcessRecipes() error = %v", err)
	}
	if _, ok := cfg.Dotfiles["app_conf"]; ok {
		t.Error("moon/app merged despite the work source's overrides.toml disabling it")
	}
	if _, ok := cfg.Dotfiles["tool_conf"]; !ok {
		t.Error("work/tool not merged; the overriding source's own recipes must still apply")
	}
	if got := cfg.SourceOverrideOrigins["moon/app"]; got != "work" {
		t.Errorf("SourceOverrideOrigins[moon/app] = %q, want %q", got, "work")
	}
}

func TestProcessRecipes_SourceOverrides_ConflictBetweenSources(t *testing.T) {
	isolateHome(t)
	moon, _, _ := makeSourceRepo(t, "app")
	work, _, _ := makeSourceRepo(t, "tool")
	other, _, _ := makeSourceRepo(t, "extra")
	commitSourceFile(t, work, SourceOverridesFileName, `
[recipes_config.overrides."moon/app"]
enable = false
`)
	commitSourceFile(t, other, SourceOverridesFileName, `
[recipes_config.overrides."moon/app"]
hosts = ["somewhere"]
`)

	cfg := &Config{
		DotfilesRepoPath: t.TempDir(),
		RecipeSources: []RecipeSource{
			{Name: "moon", URL: moon},
			{Name: "work", URL: work},
			{Name: "other", URL: other},
		},
	}
	err := ProcessRecipes(cfg, "testhost")
	if err == nil {
		t.Fatal("ProcessRecipes() succeeded, want conflict error")
	}
	for _, want := range []string{"moon/app", "'work'", "'other'"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestProcessRecipes_SourceOverrides_BeatConfigToml(t *testing.T) {
	isolateHome(t)
	moon, _, _ := makeSourceRepo(t, "app")
	work, _, _ := makeSourceRepo(t, "tool")
	commitSourceFile(t, work, SourceOverridesFileName, `
[recipes_config.overrides."moon/app"]
enable = true
`)

	disabled := false
	cfg := &Config{
		DotfilesRepoPath: t.TempDir(),
		RecipeSources: []RecipeSource{
			{Name: "moon", URL: moon},
			{Name: "work", URL: work},
		},
		RecipesConfig: RecipesConfig{
			Overrides: map[string]RecipeOverride{"moon/app": {Enable: &disabled}},
		},
	}
	if err := ProcessRecipes(cfg, "testhost"); err != nil {
		t.Fatalf("ProcessRecipes() error = %v", err)
	}
	if _, ok := cfg.Dotfiles["app_conf"]; !ok {
		t.Error("moon/app not merged; a source override must win over config.toml")
	}
}

func TestProcessRecipes_SourceOverrides_LocalOverlayWins(t *testing.T) {
	home := isolateHome(t)
	moon, _, _ := makeSourceRepo(t, "app")
	work, _, _ := makeSourceRepo(t, "tool")
	commitSourceFile(t, work, SourceOverridesFileName, `
[recipes_config.overrides."moon/app"]
enable = false
`)

	// Go through the real loader so the overlay stash is exercised end to end.
	configDir := filepath.Join(home, "cfg")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(configDir, "config.toml")
	mainToml := `
dotfiles_repo_path = "` + t.TempDir() + `"

[[recipe_sources]]
name = "moon"
url = "` + moon + `"

[[recipe_sources]]
name = "work"
url = "` + work + `"
`
	if err := os.WriteFile(mainPath, []byte(mainToml), 0o644); err != nil {
		t.Fatal(err)
	}
	localToml := `
[recipes_config.overrides."moon/app"]
enable = true
`
	if err := os.WriteFile(LocalConfigPath(mainPath), []byte(localToml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, localPresent, err := LoadUserConfig(mainPath)
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if !localPresent {
		t.Fatal("local overlay not detected")
	}
	if err := ProcessRecipes(cfg, "testhost"); err != nil {
		t.Fatalf("ProcessRecipes() error = %v", err)
	}
	if _, ok := cfg.Dotfiles["app_conf"]; !ok {
		t.Error("moon/app not merged; config.local.toml must win over a source override")
	}
	// Provenance still names the source that set the key, even though the
	// machine-local overlay had the last word on its value.
	if got := cfg.SourceOverrideOrigins["moon/app"]; got != "work" {
		t.Errorf("SourceOverrideOrigins[moon/app] = %q, want %q", got, "work")
	}
}

func TestProcessRecipes_SourceOverrides_DisablesLocalRecipe(t *testing.T) {
	isolateHome(t)
	work, _, _ := makeSourceRepo(t, "tool")
	commitSourceFile(t, work, SourceOverridesFileName, `
[recipes_config.overrides.localthing]
enable = false
`)

	repo := t.TempDir()
	recipeDir := filepath.Join(repo, "recipes", "localthing")
	if err := os.MkdirAll(recipeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	recipe := `
[recipe]
name = "localthing"

[dotfiles.local_conf]
source = "local.conf"
target = "~/.config/localthing/local.conf"
`
	if err := os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		DotfilesRepoPath: repo,
		RecipesConfig:    RecipesConfig{AutoDiscover: true},
		RecipeSources:    []RecipeSource{{Name: "work", URL: work}},
	}
	if err := ProcessRecipes(cfg, "testhost"); err != nil {
		t.Fatalf("ProcessRecipes() error = %v", err)
	}
	if _, ok := cfg.Dotfiles["local_conf"]; ok {
		t.Error("local recipe merged despite a source override disabling it")
	}
	if _, ok := cfg.Dotfiles["tool_conf"]; !ok {
		t.Error("work/tool not merged")
	}
}

func TestProcessRecipes_SourceOverrides_ProfileFilteredSourceIgnored(t *testing.T) {
	isolateHome(t)
	moon, _, _ := makeSourceRepo(t, "app")
	work, _, _ := makeSourceRepo(t, "tool")
	commitSourceFile(t, work, SourceOverridesFileName, `
[recipes_config.overrides."moon/app"]
enable = false
`)

	cfg := &Config{
		DotfilesRepoPath: t.TempDir(),
		Profiles:         []string{"personal"},
		RecipeSources: []RecipeSource{
			{Name: "moon", URL: moon},
			{Name: "work", URL: work, Profiles: []string{"work"}},
		},
	}
	if err := ProcessRecipes(cfg, "testhost"); err != nil {
		t.Fatalf("ProcessRecipes() error = %v", err)
	}
	if _, ok := cfg.Dotfiles["app_conf"]; !ok {
		t.Error("moon/app not merged; a profile-filtered source's overrides must not apply")
	}
	if len(cfg.SourceOverrideOrigins) != 0 {
		t.Errorf("SourceOverrideOrigins = %v, want empty", cfg.SourceOverrideOrigins)
	}
}
