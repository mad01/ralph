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

func TestConfigWithoutRecipe_PreservesEnv(t *testing.T) {
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
	if got.Shell.Env["PATH_EXT"] != "/usr/local/bin" {
		t.Errorf("PATH_EXT = %q, want %q", got.Shell.Env["PATH_EXT"], "/usr/local/bin")
	}
	if got.Shell.Env["EDITOR"] != "nvim" {
		t.Errorf("EDITOR = %q, want %q", got.Shell.Env["EDITOR"], "nvim")
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
