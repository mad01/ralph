# Recipes

Recipes let you split your configuration into modular `recipe.toml` files that live next to your source files.

## Why recipes

A single `config.toml` works fine when you manage a handful of dotfiles. As your configuration grows to cover editors, git, shell customizations, Kubernetes tooling, and more, a flat config becomes hard to navigate. Recipes solve this by grouping related dotfiles, aliases, tools, and hooks into self-contained units. Each recipe lives in its own directory alongside the source files it manages.

## Recipe file format

A recipe file is named `recipe.toml` and supports the same sections as the main config, plus a `[recipe]` metadata block.

### Metadata

```toml
[recipe]
name = "editors"
description = "Editor configurations for neovim and IntelliJ"

[recipe.legacy_paths]
"ralph_files/nvim/init.lua" = "nvim/init.lua"
```

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Human-readable name for the recipe |
| `description` | string | Description of what the recipe provides |
| `legacy_paths` | map | Old-to-new path mappings for [migration](migration.md) after reorganizing files |

### Available sections

A recipe can define any of these sections, identical in format to the main config:

- `[dotfiles.*]` -- dotfile entries
- `[directories.*]` -- directories to create
- `[repos.*]` -- git repositories to clone
- `[[tools]]` -- tools to check
- `[shell.aliases.*]`, `[shell.functions.*]`, `[shell.env]` -- shell configuration
- `[hooks]` -- pre/post apply hooks and builds
- `[packages.*]` -- managed packages
- `[template_variables]` -- template variables

### Path resolution

Paths in recipes are relative to the recipe directory, not to `dotfiles_repo_path`. Ralph resolves these automatically. For example, if a recipe at `recipes/editors/recipe.toml` defines a dotfile with `source = "nvim/init.vim"`, it resolves to `recipes/editors/nvim/init.vim` within the dotfiles repo.

## Loading recipes

There are two modes for loading recipes. Choose one -- they cannot be combined.

### Mode 1: Explicit list

List each recipe explicitly in your main `config.toml`. This is recommended for most users because it gives you full control over which recipes are loaded and in what order.

```toml
[[recipes]]
path = "editors/recipe.toml"

[[recipes]]
path = "git/recipe.toml"

[[recipes]]
path = "kubernetes/recipe.toml"
hosts = ["work-laptop"]
```

You can also use the short name form, which looks for `recipes/<name>/recipe.toml`:

```toml
[[recipes]]
name = "editors"

[[recipes]]
name = "git"

[[recipes]]
name = "kubernetes"
hosts = ["work-laptop"]
```

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Short name -- resolves to `recipes/<name>/recipe.toml` |
| `path` | string | Explicit path to `recipe.toml` relative to `dotfiles_repo_path` |
| `enable` | bool | `nil`/`true` = enabled, `false` = disabled |
| `hosts` | list | Hostnames this recipe applies to (empty = all hosts) |

### Mode 2: Auto-discovery

Let ralph find all `recipe.toml` files in a directory tree. Useful when you have many recipes and do not want to maintain an explicit list.

```toml
[recipes_config]
auto_discover = true
dir = "recipes"
exclude = ["experimental/*"]

[recipes_config.overrides.kubernetes]
hosts = ["work-laptop"]

[recipes_config.overrides.work-tools]
enable = false
```

| Field | Type | Description |
|-------|------|-------------|
| `auto_discover` | bool | Enable auto-discovery |
| `dir` | string | Directory to search (default: `"recipes"`) |
| `exclude` | list | Glob patterns to exclude from discovery |
| `overrides` | map | Per-recipe overrides keyed by directory name, supporting `enable` and `hosts` |

## Host filtering

Setting `hosts` on a recipe reference applies that filter to every item in the recipe that does not already define its own `hosts` field. Items with their own `hosts` field keep their own filter.

```toml
# This recipe only applies on work-laptop
[[recipes]]
name = "kubernetes"
hosts = ["work-laptop"]
```

Inside the recipe, an item can override the recipe-level filter:

```toml
# This tool applies on all hosts, despite the recipe-level filter
[[tools]]
name = "kubectl"
check_command = "command -v kubectl"
install_hint = "https://kubernetes.io/docs/tasks/tools/"
hosts = []  # explicit empty = all hosts is NOT supported; omit the field instead
```

In practice, items without a `hosts` field inherit the recipe-level filter, and items with a `hosts` field use their own.

## Name conflict detection

Ralph raises an error if the same item name appears in multiple recipes or in both a recipe and the main config. This applies to dotfiles, directories, repos, shell aliases, shell functions, shell env vars, pre/post link hooks, builds, packages, and template variables.

Tools are the exception -- they are appended without conflict checks since they are a list rather than a map.

## Example directory layout

```
~/.dotfiles/
  config.toml
  recipes/
    editors/
      recipe.toml
      nvim/
        init.vim
        lua/
          settings.lua
      ideavimrc
    git/
      recipe.toml
      .gitconfig.tmpl
      .gitignore_global
    shell/
      recipe.toml
    kubernetes/
      recipe.toml
```

The editors `recipe.toml` might look like:

```toml
[recipe]
name = "editors"
description = "Neovim and IntelliJ IDEA configurations"

[dotfiles.nvim-config]
source = "nvim/init.vim"
target = "~/.config/nvim/init.vim"

[dotfiles.ideavimrc]
source = "ideavimrc"
target = "~/.ideavimrc"

[shell.aliases.vim]
command = "nvim"
```

## Migrating to recipes

If you have a working flat config and want to reorganize into recipes:

1. Create recipe directories and move source files into them.
2. Create `recipe.toml` files with the relevant dotfile, alias, and hook entries.
3. Add `[recipe.legacy_paths]` mappings from old source paths to new paths.
4. Update `config.toml` to reference the recipes (and remove the entries you moved).
5. Run `ralph migrate --dry-run` to preview symlink updates.
6. Run `ralph migrate` to fix the symlinks.
7. Run `ralph apply` to verify everything is in sync.

See [migration](migration.md) for more details on the migrate command and legacy path mappings.
