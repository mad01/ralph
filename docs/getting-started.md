# Getting Started

ralph is a dotfiles manager that reads a TOML config file and sets up symlinks, copies, aliases, functions, environment variables, repos, build hooks, and packages on your machine.

## Prerequisites

You need one of the following:

- **Go 1.25+** (for `go install` or building from source)
- **Make** (for building from source with the Makefile)

## Installation

### Option 1: go install

```bash
go install github.com/mad01/ralph/cmd/ralph@latest
```

This places the `ralph` binary in your `$GOPATH/bin` (or `$HOME/go/bin` by default). Make sure that directory is in your `PATH`.

### Option 2: Build from source

```bash
git clone https://github.com/mad01/ralph.git
cd ralph
make build
```

This produces a `ralph` binary in the current directory. Move it somewhere in your `PATH`:

```bash
mv ralph /usr/local/bin/
```

### Option 3: Pre-built binaries

Download a binary for your platform from the [Releases](https://github.com/mad01/ralph/releases) page.

## First-time setup

### 1. Initialize ralph

Run `ralph init` to create a configuration file:

```bash
ralph init
```

This walks you through an interactive setup. It creates a config file at `~/.config/ralph/config.toml` and asks for the path to your dotfiles repository (default: `~/.dotfiles`).

### 2. Create a dotfiles repo (if you do not have one)

If you do not already have a dotfiles repository, create one and move some config files into it:

```bash
mkdir ~/.dotfiles
cp ~/.bashrc ~/.dotfiles/.bashrc
cp ~/.gitconfig ~/.dotfiles/.gitconfig
```

### 3. Add your first dotfile to the config

Open `~/.config/ralph/config.toml` and add a dotfile entry:

```toml
dotfiles_repo_path = "~/.dotfiles"

[dotfiles.bashrc]
source = ".bashrc"
target = "~/.bashrc"
```

The `source` path is relative to your `dotfiles_repo_path`. The `target` is the absolute path where the symlink will be created.

### 4. Add an alias

While you have the config open, add a shell alias:

```toml
[shell.aliases.ll]
command = "ls -alhF"
```

ralph generates shell scripts for your aliases and injects source lines into your shell's rc file so they load automatically.

### 5. Apply your configuration

```bash
ralph up
```

This pulls your dotfiles repo, syncs remote packages, creates symlinks, generates shell configuration files, and sets everything up according to your config. Run with `--dry-run` first if you want to preview changes without making them:

```bash
ralph up --dry-run
```

Dry-run automatically enables verbose output, so you see every item that would be processed.

### 6. Verify your setup

```bash
ralph doctor
```

This checks for common problems like broken symlinks, missing tools, and configuration issues.

## Version controlling your config

It is a good practice to keep `config.toml` in your dotfiles repository and symlink it to the location ralph expects:

```bash
# Copy (or move) config.toml into your dotfiles repo
cp ~/.config/ralph/config.toml ~/.dotfiles/config.toml

# Replace the original with a symlink
ln -sf ~/.dotfiles/config.toml ~/.config/ralph/config.toml
```

This way your ralph configuration is version-controlled alongside the files it manages.

## Global flags

These flags are available on all ralph commands:

| Flag | Short | Description |
|------|-------|-------------|
| `--dry-run` | `-n` | Show what changes would be made without making them |
| `--verbose` | `-v` | Show per-item detail in the apply summary. Without this flag, only a count line is printed per phase. |
| `--quiet` | `-q` | Show only failures in the summary |

## Next steps

- [Commands reference](commands.md) -- all commands including `ralph up`, `ralph add`, `ralph enable`/`disable`, and more
- [Configuration reference](configuration.md) -- full details on every config option
- [Recipes](recipes.md) -- split your config into modular recipe files
- [Templating](templating.md) -- use Go templates in your dotfiles
- [Migration](migration.md) -- update symlinks after reorganizing your dotfiles repo
