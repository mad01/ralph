package config

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

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
