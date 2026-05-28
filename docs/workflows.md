# Workflows

Common workflows for managing your dotfiles with ralph.

## Core loop

The typical ralph workflow is: edit your config or dotfiles, run `ralph up`, and verify with `ralph doctor`.

```
edit config/dotfiles  -->  ralph up  -->  ralph doctor
```

## New machine setup

Install ralph, clone your dotfiles repo, point ralph at it, and apply.

```bash
# Install ralph
go install github.com/mad01/ralph/cmd/ralph@latest

# Clone your dotfiles repo
git clone https://github.com/you/dotfiles.git ~/.dotfiles

# Symlink the config (if your config.toml lives in the dotfiles repo)
mkdir -p ~/.config/ralph
ln -s ~/.dotfiles/config.toml ~/.config/ralph/config.toml

# Or run the interactive setup instead
ralph init

# Apply all configurations (sync + apply in one step)
ralph up
```

## Editing dotfiles

Files managed via symlink update automatically when you edit the source in your dotfiles repo. Templates and copies require a re-apply.

```bash
# Edit a symlinked file -- changes take effect immediately
vim ~/.dotfiles/.bashrc

# Edit a template -- re-apply to regenerate
vim ~/.dotfiles/.gitconfig.tmpl
ralph up
```

## Adding a new dotfile

1. Copy (or move) the file into your dotfiles repo.
2. Add an entry to `config.toml` (or a [recipe](recipes.md) file).
3. Run `ralph up`.

```bash
# Move the file into your dotfiles repo
cp ~/.config/starship.toml ~/.dotfiles/starship.toml

# Add the entry to config.toml
cat >> ~/.config/ralph/config.toml << 'EOF'

[dotfiles.starship]
source = "starship.toml"
target = "~/.config/starship.toml"
EOF

# Apply
ralph up
```

## Updating packages

`ralph up` handles the full package lifecycle: it pulls remote repos (including `make` packages) and rebuilds packages that have changed, all in one command. Go-install packages update when you change the `version` field. Use `ralph up --no-sync` to skip the pull step and only build. See [packages](packages.md) for full details.

```bash
# Sync remote packages and build
ralph up

# Build only (skip sync)
ralph up --no-sync

# Force rebuild all packages
ralph up --force

# Preview without making changes
ralph up --dry-run

# Check which packages have updates available
ralph outdated

# Check then sync and apply
ralph outdated && ralph up
```

## Health checks

`ralph doctor` verifies the full state of your setup: config validity, symlink integrity, directory existence, repository status, build state, package health, tool availability, and RC file sourcing.

```bash
ralph doctor
```

Address any issues it reports, then run `ralph up` to fix what can be fixed automatically.

## Multi-machine sync

Use the `hosts` field to target items to specific machines. The same config works everywhere -- items without a `hosts` field apply on all hosts, and items with a `hosts` list apply only on matching hostnames.

```toml
[dotfiles.work-vpn]
source = "work/vpn.conf"
target = "~/.config/vpn.conf"
hosts = ["work-laptop"]

[dotfiles.bashrc]
source = ".bashrc"
target = "~/.bashrc"
# no hosts field = applies everywhere
```

To sync across machines, push your dotfiles repo and pull on the other side:

```bash
# On machine A
cd ~/.dotfiles && git add -A && git commit -m "update" && git push

# On machine B
cd ~/.dotfiles && git pull
ralph up
```

Recipes also support host filtering at the recipe level, which applies to all items in the recipe. See [recipes](recipes.md) for details.

## Reorganizing dotfiles into recipes

When your config outgrows a single file, extract sections into [recipe](recipes.md) directories:

1. Create recipe directories and move source files into them.
2. Write `recipe.toml` files with the relevant entries.
3. Add `[recipe.legacy_paths]` so `ralph migrate` can fix existing symlinks.
4. Remove the migrated entries from `config.toml` and add recipe references.
5. Preview and execute the migration.

```bash
ralph migrate --dry-run
ralph migrate
ralph up
```

## Command cheat sheet

| Task | Command |
|------|---------|
| Sync and apply | `ralph up` |
| Apply only (skip sync) | `ralph up --no-sync` |
| Preview changes | `ralph up --dry-run` |
| Clean orphaned artifacts | `ralph up --enable-cleanup` |
| Uninstall a recipe | `ralph down <recipe>` |
| Scaffold a new recipe | `ralph add <recipe>` |
| Enable a recipe | `ralph enable <recipe>` |
| Disable a recipe | `ralph disable <recipe>` |
| List recipes and status | `ralph list recipes` |
| Check health | `ralph doctor` |
| Check for package updates | `ralph outdated` |
| List managed items | `ralph list` |

## Global flags

These flags work on all commands:

| Flag | Short | Description |
|------|-------|-------------|
| `--dry-run` | `-n` | Show what would happen without making changes. Implies --verbose. |
| `--verbose` | `-v` | Show per-item detail in summary output. Default shows count-only phase lines. |
| `--quiet` | `-q` | Show only failures in summary output |
