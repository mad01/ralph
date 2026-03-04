# Minimal Ralph Example

The simplest possible ralph setup. This example manages three dotfiles and defines two shell aliases -- enough to get started with ralph without any advanced features.

## What is included

- **Dotfiles**: `.bashrc`, `.gitconfig`, `.tmux.conf` -- symlinked from your dotfiles repo to your home directory.
- **Aliases**: `ll` (detailed file listing) and `g` (short for `git`).

## Directory structure

```
minimal/
  config.toml          Ralph configuration
  dotfiles/
    .bashrc            Bash shell configuration
    .gitconfig         Git configuration
    .tmux.conf         Tmux configuration
```

## Getting started

1. Copy `config.toml` to `~/.config/ralph/config.toml`.
2. Place the dotfiles directory contents into your dotfiles repository (default `~/.dotfiles`).
3. Run `ralph apply`.

For a step-by-step walkthrough, see the [Getting Started guide](../../docs/getting-started.md).
