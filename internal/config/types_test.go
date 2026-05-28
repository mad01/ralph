package config

import (
	"testing"

	"github.com/BurntSushi/toml"
)

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
