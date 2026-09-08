package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// SourceOverridesFileName is the optional file at the root of a recipe
// source's checkout that carries [recipes_config.overrides] entries. A source
// uses it to enable, disable, host-scope, or set vars on recipes it does not
// own — typically recipes from another source — on every machine where the
// source itself is active.
const SourceOverridesFileName = "overrides.toml"

// sourceOverridesFile is the on-disk shape of overrides.toml: the same
// [recipes_config.overrides.<key>] table config.toml uses, and nothing else,
// so a block can move between the two files unchanged.
type sourceOverridesFile struct {
	RecipesConfig struct {
		Overrides map[string]RecipeOverride `toml:"overrides"`
	} `toml:"recipes_config"`
}

// LoadSourceOverrides reads the overrides.toml at the root of a source
// checkout. A missing file yields an empty map. Unlike recipe files, unknown
// keys are an error rather than a warning: the file exists for one purpose,
// and a mistyped table would otherwise silently apply nothing.
func LoadSourceOverrides(checkout string) (map[string]RecipeOverride, error) {
	path := filepath.Join(checkout, SourceOverridesFileName)

	var file sourceOverridesFile
	md, err := toml.DecodeFile(path, &file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to decode %s: %w", path, err)
	}
	if summary := undecodedSummary(md.Undecoded()); summary != "" {
		return nil, fmt.Errorf(
			"%s: unknown keys (only [recipes_config.overrides.<key>] is allowed): %s",
			path,
			summary,
		)
	}
	return file.RecipesConfig.Overrides, nil
}

// applySourceOverrides layers one source's overrides onto
// cfg.RecipesConfig.Overrides, replacing whatever config.toml set for the same
// keys. Two active sources overriding the same key is an error: the sources
// have no defined order between them, so the outcome would depend on config
// declaration order, which is exactly the kind of silent coupling the
// namespaced identities exist to prevent.
func applySourceOverrides(
	cfg *Config,
	srcName string,
	overrides map[string]RecipeOverride,
) error {
	if len(overrides) == 0 {
		return nil
	}
	if cfg.RecipesConfig.Overrides == nil {
		cfg.RecipesConfig.Overrides = make(map[string]RecipeOverride, len(overrides))
	}
	if cfg.SourceOverrideOrigins == nil {
		cfg.SourceOverrideOrigins = make(map[string]string, len(overrides))
	}
	for key, override := range overrides {
		if prior, ok := cfg.SourceOverrideOrigins[key]; ok && prior != srcName {
			return fmt.Errorf(
				"recipe override '%s' defined by two recipe sources: '%s' and '%s'",
				key,
				prior,
				srcName,
			)
		}
		cfg.RecipesConfig.Overrides[key] = override
		cfg.SourceOverrideOrigins[key] = srcName
	}
	return nil
}

// reapplyLocalOverrides puts the machine-local overlay's overrides back on
// top after sources have layered theirs in, so config.local.toml stays the
// last word on any override key regardless of which layer set it first.
func reapplyLocalOverrides(cfg *Config) {
	if len(cfg.LocalOverrides) == 0 {
		return
	}
	if cfg.RecipesConfig.Overrides == nil {
		cfg.RecipesConfig.Overrides = make(map[string]RecipeOverride, len(cfg.LocalOverrides))
	}
	maps.Copy(cfg.RecipesConfig.Overrides, cfg.LocalOverrides)
}
