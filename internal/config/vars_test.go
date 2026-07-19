package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func recipeWithVars(
	vars map[string]string,
	functions map[string]ShellFunction,
	aliases map[string]ShellAlias,
) *Recipe {
	return &Recipe{
		Recipe: RecipeMetadata{Name: "test", Vars: vars},
		Shell:  ShellConfig{Functions: functions, Aliases: aliases},
	}
}

func TestExpandRecipeVars(t *testing.T) {
	tests := []struct {
		name      string
		recipe    *Recipe
		overrides map[string]string
		wantErr   string
		check     func(t *testing.T, r *Recipe)
	}{
		{
			name: "default value expands in function name and body",
			recipe: recipeWithVars(
				map[string]string{"alias_name": "rm"},
				map[string]ShellFunction{"{{vars.alias_name}}": {Body: "echo {{vars.alias_name}}"}},
				nil,
			),
			check: func(t *testing.T, r *Recipe) {
				fn, ok := r.Shell.Functions["rm"]
				if !ok {
					t.Fatalf("Functions keys = %v, want key 'rm'", keysOf(r.Shell.Functions))
				}
				if fn.Body != "echo rm" {
					t.Errorf("Body = %q, want %q", fn.Body, "echo rm")
				}
			},
		},
		{
			name: "override replaces default",
			recipe: recipeWithVars(
				map[string]string{"alias_name": "rm"},
				map[string]ShellFunction{"{{vars.alias_name}}": {Body: "toss-bin \"$@\""}},
				nil,
			),
			overrides: map[string]string{"alias_name": "del"},
			check: func(t *testing.T, r *Recipe) {
				if _, ok := r.Shell.Functions["del"]; !ok {
					t.Fatalf("Functions keys = %v, want key 'del'", keysOf(r.Shell.Functions))
				}
				if _, ok := r.Shell.Functions["rm"]; ok {
					t.Error("Functions still holds key 'rm', want it renamed to 'del'")
				}
			},
		},
		{
			name: "alias name and command expand",
			recipe: recipeWithVars(
				map[string]string{"pager": "less"},
				nil,
				map[string]ShellAlias{"{{vars.pager}}-follow": {Command: "{{vars.pager}} +F"}},
			),
			check: func(t *testing.T, r *Recipe) {
				alias, ok := r.Shell.Aliases["less-follow"]
				if !ok {
					t.Fatalf("Aliases keys = %v, want key 'less-follow'", keysOf(r.Shell.Aliases))
				}
				if alias.Command != "less +F" {
					t.Errorf("Command = %q, want %q", alias.Command, "less +F")
				}
			},
		},
		{
			name: "no vars declared leaves recipe untouched",
			recipe: recipeWithVars(
				nil,
				map[string]ShellFunction{"repo": {Body: "csl repo \"$@\""}},
				nil,
			),
			check: func(t *testing.T, r *Recipe) {
				if _, ok := r.Shell.Functions["repo"]; !ok {
					t.Errorf("Functions keys = %v, want key 'repo'", keysOf(r.Shell.Functions))
				}
			},
		},
		{
			name:      "override of undeclared variable errors",
			recipe:    recipeWithVars(map[string]string{"alias_name": "rm"}, nil, nil),
			overrides: map[string]string{"alias_nmae": "del"},
			wantErr:   "vars override 'alias_nmae' is not declared",
		},
		{
			name:      "override without any declared vars errors",
			recipe:    recipeWithVars(nil, nil, nil),
			overrides: map[string]string{"alias_name": "del"},
			wantErr:   "not declared in [recipe.vars] (declared: none)",
		},
		{
			name: "undeclared placeholder errors",
			recipe: recipeWithVars(
				map[string]string{"alias_name": "rm"},
				map[string]ShellFunction{"{{vars.alais_name}}": {Body: "x"}},
				nil,
			),
			wantErr: "references {{vars.alais_name}}, which is not declared",
		},
		{
			name: "expansion collision between function names errors",
			recipe: recipeWithVars(
				map[string]string{"alias_name": "rm"},
				map[string]ShellFunction{
					"{{vars.alias_name}}": {Body: "a"},
					"rm":                  {Body: "b"},
				},
				nil,
			),
			wantErr: "collides with another function",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExpandRecipeVars(tt.recipe, tt.overrides)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ExpandRecipeVars() = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExpandRecipeVars() returned error: %v", err)
			}
			tt.check(t, tt.recipe)
		})
	}
}

func keysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestProcessRecipes_VarsOverrideReachesRecipe(t *testing.T) {
	tempDir := t.TempDir()

	recipeDir := filepath.Join(tempDir, "recipes", "toss")
	_ = os.MkdirAll(recipeDir, 0o755)
	_ = os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(`
[recipe]
name = "toss"

[recipe.vars]
alias_name = "rm"

[shell.functions."{{vars.alias_name}}"]
body = 'toss-bin --safe-mode "$@"'
`), 0o644)

	cfg := &Config{
		DotfilesRepoPath: tempDir,
		RecipesConfig: RecipesConfig{
			AutoDiscover: true,
			Overrides: map[string]RecipeOverride{
				"toss": {Vars: map[string]string{"alias_name": "del"}},
			},
		},
	}

	if err := ProcessRecipes(cfg, "test-host"); err != nil {
		t.Fatalf("ProcessRecipes() returned error: %v", err)
	}

	fn, ok := cfg.Shell.Functions["del"]
	if !ok {
		t.Fatalf("Shell.Functions keys = %v, want key 'del'", keysOf(cfg.Shell.Functions))
	}
	if fn.Body != `toss-bin --safe-mode "$@"` {
		t.Errorf("Body = %q, want the recipe body unchanged", fn.Body)
	}
	if _, ok := cfg.Shell.Functions["rm"]; ok {
		t.Error("Shell.Functions holds 'rm', want only the overridden 'del'")
	}
}

func TestProcessRecipes_VarsDefaultWithoutOverride(t *testing.T) {
	tempDir := t.TempDir()

	recipeDir := filepath.Join(tempDir, "recipes", "toss")
	_ = os.MkdirAll(recipeDir, 0o755)
	_ = os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(`
[recipe]
name = "toss"

[recipe.vars]
alias_name = "rm"

[shell.functions."{{vars.alias_name}}"]
body = 'toss-bin --safe-mode "$@"'
`), 0o644)

	cfg := &Config{
		DotfilesRepoPath: tempDir,
		RecipesConfig:    RecipesConfig{AutoDiscover: true},
	}

	if err := ProcessRecipes(cfg, "test-host"); err != nil {
		t.Fatalf("ProcessRecipes() returned error: %v", err)
	}

	if _, ok := cfg.Shell.Functions["rm"]; !ok {
		t.Fatalf("Shell.Functions keys = %v, want default key 'rm'", keysOf(cfg.Shell.Functions))
	}
}
