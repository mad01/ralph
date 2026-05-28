package commands

import (
	"testing"

	"github.com/mad01/ralph/internal/config"
)

func TestConfigWithoutRecipe_FiltersAliases(t *testing.T) {
	cfg := &config.Config{
		Shell: config.ShellConfig{
			Aliases: map[string]config.ShellAlias{
				"keep":   {Command: "echo keep", OwnerRecipe: "alpha"},
				"remove": {Command: "echo remove", OwnerRecipe: "beta"},
				"also":   {Command: "echo also", OwnerRecipe: "alpha"},
			},
		},
	}

	got := configWithoutRecipe(cfg, "beta")

	if len(got.Shell.Aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(got.Shell.Aliases))
	}
	if _, ok := got.Shell.Aliases["keep"]; !ok {
		t.Error("expected alias 'keep' to remain")
	}
	if _, ok := got.Shell.Aliases["also"]; !ok {
		t.Error("expected alias 'also' to remain")
	}
	if _, ok := got.Shell.Aliases["remove"]; ok {
		t.Error("expected alias 'remove' to be filtered out")
	}
}

func TestConfigWithoutRecipe_FiltersFunctions(t *testing.T) {
	cfg := &config.Config{
		Shell: config.ShellConfig{
			Functions: map[string]config.ShellFunction{
				"fn_keep":   {Body: "echo keep", OwnerRecipe: "alpha"},
				"fn_remove": {Body: "echo remove", OwnerRecipe: "beta"},
			},
		},
	}

	got := configWithoutRecipe(cfg, "beta")

	if len(got.Shell.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(got.Shell.Functions))
	}
	if _, ok := got.Shell.Functions["fn_keep"]; !ok {
		t.Error("expected function 'fn_keep' to remain")
	}
	if _, ok := got.Shell.Functions["fn_remove"]; ok {
		t.Error("expected function 'fn_remove' to be filtered out")
	}
}

func TestConfigWithoutRecipe_FiltersEnvVars(t *testing.T) {
	cfg := &config.Config{
		Shell: config.ShellConfig{
			Env: map[string]string{
				"PATH_EXT": "/usr/local/bin",
				"EDITOR":   "nvim",
				"MY_VAR":   "recipe_val",
			},
			EnvOwners: map[string]string{
				"EDITOR": "alpha",
				"MY_VAR": "beta",
			},
		},
	}

	got := configWithoutRecipe(cfg, "beta")

	if len(got.Shell.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(got.Shell.Env))
	}
	if got.Shell.Env["PATH_EXT"] != "/usr/local/bin" {
		t.Error("expected PATH_EXT to remain (no owner)")
	}
	if got.Shell.Env["EDITOR"] != "nvim" {
		t.Error("expected EDITOR to remain (owned by alpha)")
	}
	if _, ok := got.Shell.Env["MY_VAR"]; ok {
		t.Error("expected MY_VAR to be filtered out (owned by beta)")
	}
}

func TestConfigWithoutRecipe_PreservesEnvWithoutOwners(t *testing.T) {
	cfg := &config.Config{
		Shell: config.ShellConfig{
			Env: map[string]string{
				"PATH_EXT": "/usr/local/bin",
				"EDITOR":   "nvim",
			},
		},
	}

	got := configWithoutRecipe(cfg, "anything")

	if len(got.Shell.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(got.Shell.Env))
	}
}

func TestIsOwnedByRecipe(t *testing.T) {
	cfg := &config.Config{
		Hooks: config.HooksConfig{
			Builds: map[string]config.Build{
				"compile": {OwnerRecipe: "alpha"},
				"lint":    {OwnerRecipe: "beta"},
			},
		},
		Packages: map[string]config.Package{
			"tool-a": {OwnerRecipe: "alpha"},
			"tool-b": {OwnerRecipe: "gamma"},
		},
	}

	tests := []struct {
		dep    string
		recipe string
		want   bool
	}{
		{"builds.compile", "alpha", true},
		{"builds.compile", "beta", false},
		{"builds.lint", "beta", true},
		{"builds.nonexistent", "alpha", false},
		{"packages.tool-a", "alpha", true},
		{"packages.tool-b", "alpha", false},
		{"packages.tool-b", "gamma", true},
		{"packages.nonexistent", "gamma", false},
		// malformed inputs
		{"nodot", "alpha", false},
		{"", "alpha", false},
		{"unknown.compile", "alpha", false},
		{"builds.", "alpha", false},
		{".builds", "alpha", false},
	}

	for _, tt := range tests {
		t.Run(tt.dep+"_"+tt.recipe, func(t *testing.T) {
			got := isOwnedByRecipe(tt.dep, cfg, tt.recipe)
			if got != tt.want {
				t.Errorf("isOwnedByRecipe(%q, cfg, %q) = %v, want %v", tt.dep, tt.recipe, got, tt.want)
			}
		})
	}
}

func TestConfigWithoutRecipe_EmptyRecipeName_NoOp(t *testing.T) {
	cfg := &config.Config{
		Shell: config.ShellConfig{
			Aliases: map[string]config.ShellAlias{
				"a1": {Command: "echo a1", OwnerRecipe: "alpha"},
				"a2": {Command: "echo a2", OwnerRecipe: "beta"},
			},
			Functions: map[string]config.ShellFunction{
				"f1": {Body: "echo f1", OwnerRecipe: "alpha"},
			},
		},
	}

	got := configWithoutRecipe(cfg, "")

	// Empty recipe name matches nothing (no OwnerRecipe is ""), so all items stay.
	if len(got.Shell.Aliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(got.Shell.Aliases))
	}
	if len(got.Shell.Functions) != 1 {
		t.Errorf("expected 1 function, got %d", len(got.Shell.Functions))
	}
}
