---
name: ralph-recipe
description: Create and manage ralph recipes - modular configuration files for organizing dotfiles by tool or purpose. Use when creating new recipes, debugging recipe loading, or restructuring dotfile configurations.
---

# ralph Recipes

Use this skill when creating, editing, or troubleshooting ralph recipes.

## What Are Recipes?

Recipes are modular `recipe.toml` files that break a large config into focused units (e.g., one recipe per tool: nvim, tmux, git). They live alongside source files in the dotfiles repo.

## Recipe Structure

```
~/.dotfiles/
  recipes/
    nvim/
      recipe.toml          # Config for nvim dotfiles
      init.lua             # Source files alongside recipe
      lua/
        ...
    tmux/
      recipe.toml
      tmux.conf
```

## Recipe File Format

A `recipe.toml` has the same sections as the main config, plus optional metadata:

```toml
[recipe]
name = "nvim"
description = "Neovim configuration"

[recipe.legacy_paths]
"old/path/init.lua" = "init.lua"    # For migration support

[dotfiles.nvim_config]
source = "init.lua"                  # Relative to recipe directory
target = "~/.config/nvim/init.lua"
action = "symlink"

[shell.aliases.vi]
command = "nvim"

[hooks.builds.nvim_plugins]
commands = ["nvim --headless +PlugInstall +qa"]
run = "once"
```

## Discovery Modes

### Mode A: Explicit references in config.toml

```toml
[[recipes]]
name = "nvim"              # Looks for recipes/nvim/recipe.toml

[[recipes]]
path = "custom/path/recipe.toml"
enable = false

[[recipes]]
name = "tmux"
hosts = ["workstation"]
```

### Mode B: Auto-discovery

```toml
[recipes_config]
auto_discover = true
dir = "recipes"            # Default directory to scan
exclude = ["*.bak", "experimental"]

[recipes_config.overrides.nvim]
enable = false             # Disable specific auto-discovered recipe

[recipes_config.overrides.tmux]
hosts = ["workstation"]    # Host-filter specific recipe
```

## Path Resolution

- Source paths in recipes are **relative to the recipe directory**
- During loading, they get prefixed with the recipe's directory path
- Example: recipe at `recipes/nvim/recipe.toml` with `source = "init.lua"` resolves to `recipes/nvim/init.lua`

## Merging Rules

- Recipes merge into the main config
- Duplicate keys across recipes cause an error (no silent overwriting)
- Shell aliases/functions/env merge additively
- Tools append to the tools list

## Creating a New Recipe

1. Create directory: `~/.dotfiles/recipes/<name>/`
2. Create `recipe.toml` with metadata and config sections
3. Place source files alongside the recipe
4. Add reference to main config or enable auto-discovery
5. Run `ralph apply --dry-run` to verify
