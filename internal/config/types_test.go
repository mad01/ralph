package config

import (
	"testing"

	"github.com/BurntSushi/toml"
)

func TestHooksConfig_UninstallHooksParseFromTOML(t *testing.T) {
	input := `
[hooks]
pre_uninstall = ["echo cleaning up", "rm -rf /tmp/cache"]
post_uninstall = ["echo done"]
`
	var cfg struct {
		Hooks HooksConfig `toml:"hooks"`
	}
	if _, err := toml.Decode(input, &cfg); err != nil {
		t.Fatalf("TOML decode error: %v", err)
	}

	if len(cfg.Hooks.PreUninstall) != 2 {
		t.Fatalf("expected 2 pre_uninstall hooks, got %d", len(cfg.Hooks.PreUninstall))
	}
	if cfg.Hooks.PreUninstall[0] != "echo cleaning up" {
		t.Errorf("pre_uninstall[0] = %q, want %q", cfg.Hooks.PreUninstall[0], "echo cleaning up")
	}
	if cfg.Hooks.PreUninstall[1] != "rm -rf /tmp/cache" {
		t.Errorf("pre_uninstall[1] = %q, want %q", cfg.Hooks.PreUninstall[1], "rm -rf /tmp/cache")
	}

	if len(cfg.Hooks.PostUninstall) != 1 {
		t.Fatalf("expected 1 post_uninstall hook, got %d", len(cfg.Hooks.PostUninstall))
	}
	if cfg.Hooks.PostUninstall[0] != "echo done" {
		t.Errorf("post_uninstall[0] = %q, want %q", cfg.Hooks.PostUninstall[0], "echo done")
	}
}

func TestRecipesConfig_AutoCleanupParseFromTOML(t *testing.T) {
	input := `
[recipes_config]
auto_discover = true
auto_cleanup = true
`
	var cfg struct {
		RecipesConfig RecipesConfig `toml:"recipes_config"`
	}
	if _, err := toml.Decode(input, &cfg); err != nil {
		t.Fatalf("TOML decode error: %v", err)
	}

	if !cfg.RecipesConfig.AutoCleanup {
		t.Error("expected auto_cleanup = true")
	}
	if !cfg.RecipesConfig.AutoDiscover {
		t.Error("expected auto_discover = true")
	}
}

func TestRecipesConfig_AutoCleanupDefaultsFalse(t *testing.T) {
	input := `
[recipes_config]
auto_discover = true
`
	var cfg struct {
		RecipesConfig RecipesConfig `toml:"recipes_config"`
	}
	if _, err := toml.Decode(input, &cfg); err != nil {
		t.Fatalf("TOML decode error: %v", err)
	}

	if cfg.RecipesConfig.AutoCleanup {
		t.Error("expected auto_cleanup to default to false")
	}
}
