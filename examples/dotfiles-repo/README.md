# Dotfiles Repository Example

A comprehensive ralph setup that demonstrates the full range of configuration options in a single `config.toml`. This example represents what a typical dotfiles repository looks like when managed by ralph.

## What is included

### Dotfiles

All dotfiles are symlinked from subdirectories in this repository to their expected locations:

| Name | Source | Target | Templated |
|------|--------|--------|-----------|
| bashrc | `bash/.bashrc` | `~/.bashrc` | Yes |
| zshrc | `zsh/.zshrc` | `~/.zshrc` | Yes |
| nvim_config | `nvim/init.vim` | `~/.config/nvim/init.vim` | No |
| gitconfig | `git/.gitconfig.tmpl` | `~/.gitconfig` | Yes |
| gitignore_global | `git/.gitignore_global` | `~/.gitignore_global` | No |
| tmux_conf | `tmux/.tmux.conf` | `~/.tmux.conf` | Yes |
| starship_config | `starship/starship.toml` | `~/.config/starship.toml` | No |
| alacritty_config | `alacritty/alacritty.yml` | `~/.config/alacritty/alacritty.yml` | Yes |
| kitty_config | `kitty/kitty.conf` | `~/.config/kitty/kitty.conf` | Yes |

### Template variables

The config defines variables used in templated dotfiles:

- `name` -- user's display name (used in `.gitconfig.tmpl`)
- `email` -- user's email address
- `signing_key` -- GPG signing key
- `editor` -- preferred editor (default: `nvim`)
- `shell_theme` -- theme name for tools that support it (used in `kitty.conf`)

### Shell configuration

- **Aliases**: `ls`, `ll`, `la`, `lla` (lsd), `cat` (bat), `vim` (nvim), `g` (git), `k` (kubectl), `dk` (docker), `dc` (docker compose), `tf` (terraform).
- **Functions**: `mkcd` (create and enter directory), `backup` (timestamped file backup).

### Tool checks

Ralph verifies that the following tools are installed and provides install hints if they are missing: Neovim, Starship, lsd, bat, zoxide, fzf.

### Hooks

The config includes commented-out examples for pre-apply, post-apply, pre-link, and post-link hooks. Uncomment and customize these for your own workflow.

## Directory structure

```
dotfiles-repo/
  config.toml                Ralph configuration
  alacritty/                 Alacritty terminal config (templated)
  bash/                      Bash shell config (templated)
  git/                       Git config and global gitignore
  kitty/
    kitty.conf               Kitty terminal config (templated, theme-aware)
  nvim/
    init.vim                 Neovim configuration
  starship/                  Starship prompt config
  tmux/                      Tmux config (templated)
  zsh/                       Zsh shell config (templated)
```

## How to use this example

1. Clone or copy this directory as your dotfiles repo at `~/.dotfiles`.
2. Copy `config.toml` to `~/.config/ralph/config.toml` and update `dotfiles_repo_path` to point to your repo location.
3. Edit the `[template_variables]` section with your real name, email, and preferences.
4. Populate the empty subdirectories (`alacritty/`, `bash/`, `git/`, `starship/`, `tmux/`, `zsh/`) with your actual dotfiles.
5. Run `ralph up`.

This example works well as a starting point. For modular setups, see the [recipe-based example](../recipe-based/). For package management, see the [with-packages example](../with-packages/).
