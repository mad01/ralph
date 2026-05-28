# Recipe-Based Ralph Example

A modular ralph setup that uses recipes with auto-discovery. Instead of putting everything into a single `config.toml`, each tool or concern gets its own recipe directory with a `recipe.toml` file.

## What is included

- **Main config**: Sets up auto-discovery so ralph automatically finds all `recipe.toml` files under `recipes/`.
- **editors recipe**: Neovim configuration and a `vim` alias.
- **git recipe**: Templated `.gitconfig`, a global gitignore, and common git aliases.
- **terminals recipe**: Kitty terminal configuration.

## Directory structure

```
recipe-based/
  config.toml                      Main config with auto-discovery enabled
  recipes/
    editors/
      recipe.toml                  Editor recipe definition
      nvim/
        init.vim                   Neovim configuration
    git/
      recipe.toml                  Git recipe definition
      .gitconfig.tmpl              Templated git config
      .gitignore_global            Global gitignore patterns
    terminals/
      recipe.toml                  Terminal recipe definition
      kitty/
        kitty.conf                 Kitty terminal configuration
```

## How auto-discovery works

When `auto_discover = true` is set in `[recipes_config]`, ralph walks the `recipes/` directory tree looking for `recipe.toml` files. Each recipe found is loaded and merged into the main configuration automatically. The `exclude` list uses glob patterns to skip directories you do not want.

## Getting started

1. Copy `config.toml` to `~/.config/ralph/config.toml`.
2. Copy the `recipes/` directory into your dotfiles repository.
3. Update template variables in `config.toml` (name, email, editor).
4. Run `ralph up`.

For details on how recipes work, see the [Recipes guide](../../docs/recipes.md).
