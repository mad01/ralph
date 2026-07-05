package config

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/mad01/ralph/internal/gitutil"
)

// makeSourceRepo creates a local git repo shaped like a recipe source: a
// recipes/<recipeName>/recipe.toml declaring one dotfile whose source lives
// next to it. Returns the repo path and the two commit hashes (first has the
// tag "v1.0.0"; second is the branch tip of main).
func makeSourceRepo(t *testing.T, recipeName string) (repoPath, first, second string) {
	t.Helper()
	repoPath = t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	recipeDir := filepath.Join(repoPath, "recipes", recipeName)
	if err := os.MkdirAll(recipeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	recipe := `
[recipe]
name = "` + recipeName + `"

[dotfiles.` + recipeName + `_conf]
source = "files/app.conf"
target = "~/.config/` + recipeName + `/app.conf"
`
	if err := os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(recipeDir, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recipeDir, "files", "app.conf"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	run("init", "-b", "main")
	run("add", ".")
	run("commit", "-m", "first")
	first = gitutil.GetGitHash(repoPath)
	run("tag", "v1.0.0")

	if err := os.WriteFile(filepath.Join(recipeDir, "files", "app.conf"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "second")
	second = gitutil.GetGitHash(repoPath)

	return repoPath, first, second
}

func TestEnsureSourceCheckout_PinTypes(t *testing.T) {
	src, first, second := makeSourceRepo(t, "app")

	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"branch", "main", second},
		{"tag", "v1.0.0", first},
		{"commit", first, first},
		{"default branch", "", second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourcesDir := t.TempDir()
			source := RecipeSource{Name: "app", URL: src, Ref: tt.ref}

			checkout, err := EnsureSourceCheckout(io.Discard, source, sourcesDir)
			if err != nil {
				t.Fatalf("ensure failed: %v", err)
			}
			if checkout != filepath.Join(sourcesDir, "app") {
				t.Errorf("checkout = %q, want %q", checkout, filepath.Join(sourcesDir, "app"))
			}
			if got := gitutil.GetGitHash(checkout); got != tt.want {
				t.Errorf("HEAD = %s, want %s", got, tt.want)
			}

			// Ensure again: idempotent, no error, same state.
			if _, err := EnsureSourceCheckout(io.Discard, source, sourcesDir); err != nil {
				t.Fatalf("re-ensure failed: %v", err)
			}
			if got := gitutil.GetGitHash(checkout); got != tt.want {
				t.Errorf("HEAD after re-ensure = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestEnsureSourceCheckout_RefChangeMovesCheckout(t *testing.T) {
	src, first, second := makeSourceRepo(t, "app")
	sourcesDir := t.TempDir()

	if _, err := EnsureSourceCheckout(io.Discard, RecipeSource{Name: "app", URL: src, Ref: "main"}, sourcesDir); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	checkout := filepath.Join(sourcesDir, "app")
	if got := gitutil.GetGitHash(checkout); got != second {
		t.Fatalf("HEAD = %s, want %s", got, second)
	}

	// Repin to the tag: the existing checkout must move.
	if _, err := EnsureSourceCheckout(io.Discard, RecipeSource{Name: "app", URL: src, Ref: "v1.0.0"}, sourcesDir); err != nil {
		t.Fatalf("re-pin failed: %v", err)
	}
	if got := gitutil.GetGitHash(checkout); got != first {
		t.Errorf("HEAD = %s, want tag commit %s", got, first)
	}
}

func TestProcessRecipes_RemoteSource(t *testing.T) {
	src, _, _ := makeSourceRepo(t, "app")

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg := &Config{
		DotfilesRepoPath: t.TempDir(),
		RecipeSources:    []RecipeSource{{Name: "moon", URL: src, Ref: "main"}},
	}
	if err := ProcessRecipes(cfg, "testhost"); err != nil {
		t.Fatalf("ProcessRecipes failed: %v", err)
	}

	df, ok := cfg.Dotfiles["app_conf"]
	if !ok {
		t.Fatalf("dotfile app_conf not merged; have %v", cfg.Dotfiles)
	}
	wantPrefix := filepath.Join(home, ".config", "ralph", "sources", "moon")
	if !filepath.IsAbs(df.Source) || !strings.HasPrefix(df.Source, wantPrefix) {
		t.Errorf("source = %q, want absolute path under %q", df.Source, wantPrefix)
	}
	if df.OwnerRecipe != "moon/app" {
		t.Errorf("owner = %q, want moon/app", df.OwnerRecipe)
	}

	var loaded *LoadedRecipeInfo
	for i := range cfg.LoadedRecipes {
		if cfg.LoadedRecipes[i].Name == "moon/app" {
			loaded = &cfg.LoadedRecipes[i]
		}
	}
	if loaded == nil {
		t.Fatalf("no LoadedRecipes entry moon/app; have %+v", cfg.LoadedRecipes)
	}
	if !filepath.IsAbs(loaded.Dir) {
		t.Errorf("loaded dir = %q, want absolute", loaded.Dir)
	}

	// The merged source file must actually exist in the checkout.
	if _, err := os.Stat(df.Source); err != nil {
		t.Errorf("merged source does not exist: %v", err)
	}
}

func TestProcessRecipes_RemoteSource_DisabledByOverride(t *testing.T) {
	src, _, _ := makeSourceRepo(t, "app")

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	disabled := false
	cfg := &Config{
		DotfilesRepoPath: t.TempDir(),
		RecipeSources:    []RecipeSource{{Name: "moon", URL: src}},
		RecipesConfig: RecipesConfig{
			Overrides: map[string]RecipeOverride{
				"moon/app": {Enable: &disabled},
			},
		},
	}
	if err := ProcessRecipes(cfg, "testhost"); err != nil {
		t.Fatalf("ProcessRecipes failed: %v", err)
	}
	if _, ok := cfg.Dotfiles["app_conf"]; ok {
		t.Error("dotfile merged despite moon/app disabled override")
	}
}

func TestProcessRecipes_RemoteSource_DisabledSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	disabled := false
	cfg := &Config{
		DotfilesRepoPath: t.TempDir(),
		// URL points nowhere: a disabled source must not be cloned at all.
		RecipeSources: []RecipeSource{
			{Name: "moon", URL: "/nonexistent/repo.git", Enable: &disabled},
		},
	}
	if err := ProcessRecipes(cfg, "testhost"); err != nil {
		t.Fatalf("ProcessRecipes failed: %v", err)
	}
}

func TestJoinSourcePath(t *testing.T) {
	if got := JoinSourcePath("/repo", "recipes/x/file"); got != "/repo/recipes/x/file" {
		t.Errorf("relative join = %q", got)
	}
	if got := JoinSourcePath("/repo", "/abs/checkout/file"); got != "/abs/checkout/file" {
		t.Errorf("absolute passthrough = %q", got)
	}
}

func TestDecodeRecipeSources(t *testing.T) {
	raw := `
dotfiles_repo_path = "~/dots"

[[recipe_sources]]
name = "thismoon"
url  = "git@github.com:mad01/thismoon.git"
ref  = "main"
update = true
recipes_dir = "recipes"
`
	var cfg Config
	if _, err := toml.Decode(raw, &cfg); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(cfg.RecipeSources) != 1 {
		t.Fatalf("expected 1 recipe source, got %d", len(cfg.RecipeSources))
	}
	src := cfg.RecipeSources[0]
	if src.Name != "thismoon" {
		t.Errorf("name = %q, want thismoon", src.Name)
	}
	if src.URL != "git@github.com:mad01/thismoon.git" {
		t.Errorf("url = %q", src.URL)
	}
	if src.Ref != "main" {
		t.Errorf("ref = %q, want main", src.Ref)
	}
	if !src.Update {
		t.Error("update = false, want true")
	}
	if src.RecipesDir != "recipes" {
		t.Errorf("recipes_dir = %q, want recipes", src.RecipesDir)
	}
}

func TestValidateRecipeSources(t *testing.T) {
	valid := RecipeSource{
		Name: "thismoon",
		URL:  "git@github.com:mad01/thismoon.git",
		Ref:  "main",
	}

	tests := []struct {
		name    string
		sources []RecipeSource
		wantErr string
	}{
		{
			name:    "valid single source",
			sources: []RecipeSource{valid},
		},
		{
			name: "valid minimal source",
			sources: []RecipeSource{
				{Name: "x", URL: "https://github.com/mad01/x.git"},
			},
		},
		{
			name: "empty name",
			sources: []RecipeSource{
				{URL: "https://github.com/mad01/x.git"},
			},
			wantErr: "name cannot be empty",
		},
		{
			name: "name with slash",
			sources: []RecipeSource{
				{Name: "a/b", URL: "https://github.com/mad01/x.git"},
			},
			wantErr: "invalid characters",
		},
		{
			name: "name is dot-dot",
			sources: []RecipeSource{
				{Name: "..", URL: "https://github.com/mad01/x.git"},
			},
			wantErr: "invalid",
		},
		{
			name: "name with leading dash",
			sources: []RecipeSource{
				{Name: "-x", URL: "https://github.com/mad01/x.git"},
			},
			wantErr: "invalid",
		},
		{
			name: "duplicate names",
			sources: []RecipeSource{
				valid,
				{Name: "thismoon", URL: "https://github.com/mad01/other.git"},
			},
			wantErr: "duplicate",
		},
		{
			name: "empty url",
			sources: []RecipeSource{
				{Name: "x"},
			},
			wantErr: "url cannot be empty",
		},
		{
			name: "unsafe url",
			sources: []RecipeSource{
				{Name: "x", URL: "ext::sh -c whoami"},
			},
			wantErr: "unsafe url",
		},
		{
			name: "unsafe ref",
			sources: []RecipeSource{
				{Name: "x", URL: "https://github.com/mad01/x.git", Ref: "--upload-pack=evil"},
			},
			wantErr: "unsafe ref",
		},
		{
			name: "recipes_dir with parent traversal",
			sources: []RecipeSource{
				{
					Name:       "x",
					URL:        "https://github.com/mad01/x.git",
					RecipesDir: "../outside",
				},
			},
			wantErr: "must not contain '..'",
		},
		{
			name: "absolute recipes_dir",
			sources: []RecipeSource{
				{Name: "x", URL: "https://github.com/mad01/x.git", RecipesDir: "/etc"},
			},
			wantErr: "must be relative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				DotfilesRepoPath: "~/dots",
				RecipeSources:    tt.sources,
			}
			err := ValidateConfig(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
