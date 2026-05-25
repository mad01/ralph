# Workflows

Common workflows for managing your dotfiles with ralph.

## Core loop

The typical ralph workflow is: edit your config or dotfiles, run `ralph apply`, and verify with `ralph doctor`.

```
edit config/dotfiles  -->  ralph apply  -->  ralph doctor
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

# Apply all configurations
ralph apply

# Sync and build managed packages (if any are configured)
ralph sync
ralph apply
```

## Editing dotfiles

Files managed via symlink update automatically when you edit the source in your dotfiles repo. Templates and copies require a re-apply.

```bash
# Edit a symlinked file -- changes take effect immediately
vim ~/.dotfiles/.bashrc

# Edit a template -- re-apply to regenerate
vim ~/.dotfiles/.gitconfig.tmpl
ralph apply
```

## Adding a new dotfile

1. Copy (or move) the file into your dotfiles repo.
2. Add an entry to `config.toml` (or a [recipe](recipes.md) file).
3. Run `ralph apply`.

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
ralph apply
```

## Updating packages

Package management is a two-step process: `ralph sync` pulls remote repos (including `make` packages), and `ralph apply` rebuilds packages that have changed. Go-install packages skip the sync step -- change the `version` field and run `ralph apply` to update them. See [packages](packages.md) for full details.

```bash
# Sync remote packages then build
ralph sync && ralph apply

# Sync a single package
ralph sync --package=my-tool

# Force rebuild all packages
ralph apply --force

# Preview without making changes
ralph sync --dry-run
ralph apply --dry-run

# Check which packages have updates available
ralph outdated

# Check then sync and apply
ralph outdated && ralph sync && ralph apply
```

## Health checks

`ralph doctor` verifies the full state of your setup: config validity, symlink integrity, directory existence, repository status, build state, package health, tool availability, and RC file sourcing.

```bash
ralph doctor
```

Address any issues it reports, then run `ralph apply` to fix what can be fixed automatically.

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
ralph sync && ralph apply
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
ralph apply
```

## Command cheat sheet

| Task | Command |
|------|---------|
| Apply configuration | `ralph apply` |
| Preview changes | `ralph apply --dry-run` |
| Sync remote packages | `ralph sync` |
| Sync one package | `ralph sync --package=NAME` |
| Sync and build | `ralph sync && ralph apply` |
| Check for updates | `ralph outdated` |
| Check for updates (JSON) | `ralph outdated --json` |
| Force rebuild all packages | `ralph apply --force` |
| Check health | `ralph doctor` |
| List managed items | `ralph list` |
| List packages by source | `ralph list --source=remote` |
| Migrate symlinks | `ralph migrate` |
| Initialize config | `ralph init` |
| Show version | `ralph version` |

## Global flags

These flags work on all commands:

| Flag | Short | Description |
|------|-------|-------------|
| `--dry-run` | `-n` | Show what would happen without making changes. Implies --verbose. |
| `--verbose` | `-v` | Show per-item detail in summary output. Default shows count-only phase lines. |
| `--quiet` | `-q` | Show only failures in summary output |
