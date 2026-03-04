# Migration

ralph can update broken symlinks after you reorganize your dotfiles repository, and it can automatically migrate legacy configuration from the older "dotter" tool name.

## Types of migration

ralph supports two distinct migration scenarios:

1. **Legacy migration (dotter to ralph)** -- automatic, runs at the start of every `ralph apply`. This handles the rename from the older "dotter" tool to ralph, updating config paths and references transparently.

2. **Symlink migration after repo reorganization** -- manual, using the `ralph migrate` command. This is what you need when you restructure your dotfiles repo and existing symlinks break because files moved to new locations.

The rest of this guide focuses on symlink migration.

## When you need symlink migration

When you reorganize files in your dotfiles repository, any existing symlinks on your system will point to the old paths that no longer exist. For example, if you move from a flat structure:

```
~/.dotfiles/
  ralph_files/
    nvim/init.lua
    ideavimrc
```

to a recipe-based structure:

```
~/.dotfiles/
  editors/
    recipe.toml
    nvim/init.lua
    ideavimrc
```

then symlinks like `~/.config/nvim/init.lua -> ~/.dotfiles/ralph_files/nvim/init.lua` will be broken because the source file no longer exists at that path.

## Defining legacy paths

To tell ralph about the old-to-new path mapping, add a `[recipe.legacy_paths]` section to your recipe files. Keys are old paths (relative to `dotfiles_repo_path`), and values are new paths (relative to the recipe directory):

```toml
# editors/recipe.toml
[recipe]
name = "editors"
description = "Editor configurations"

[recipe.legacy_paths]
"ralph_files/nvim" = "nvim"
"ralph_files/ideavimrc" = "ideavimrc"

[dotfiles.nvim_config]
source = "nvim"
target = "~/.config/nvim"
action = "symlink_dir"

[dotfiles.ideavimrc]
source = "ideavimrc"
target = "~/.ideavimrc"
```

In this example:
- `"ralph_files/nvim" = "nvim"` means that files previously at `~/.dotfiles/ralph_files/nvim` are now at `~/.dotfiles/editors/nvim` (ralph resolves the new path relative to the recipe directory automatically).
- `"ralph_files/ideavimrc" = "ideavimrc"` means the file moved from `~/.dotfiles/ralph_files/ideavimrc` to `~/.dotfiles/editors/ideavimrc`.

## Workflow

Follow these steps to migrate symlinks after reorganizing your dotfiles:

### 1. Reorganize files in your dotfiles repo

Move files to their new locations. For example, move files from a flat `ralph_files/` directory into topic-based recipe directories.

### 2. Create recipe.toml files with legacy_paths

In each recipe directory, create a `recipe.toml` that includes `[recipe.legacy_paths]` mapping old paths to new paths.

### 3. Update config.toml to reference recipes

Add recipe references to your main config:

```toml
dotfiles_repo_path = "~/.dotfiles"

[[recipes]]
path = "editors/recipe.toml"

[[recipes]]
path = "terminals/recipe.toml"
```

Or use auto-discovery:

```toml
[recipes_config]
auto_discover = true
```

### 4. Preview the migration

Always preview first with `--dry-run`:

```bash
ralph migrate --dry-run
```

This shows a summary of what ralph found:

```
Checking for symlinks that need migration...
Found 4 legacy path mapping(s) in recipes.

Migration Plan Summary
======================
Already correct:  3
Needs update:     2
Broken symlinks:  0
Not symlinks:     0
Not yet created:  1
Errors:           0

Symlinks to update:
  /home/user/.config/nvim
    Current: /home/user/.dotfiles/ralph_files/nvim (BROKEN)
    New:     /home/user/.dotfiles/editors/nvim
  /home/user/.ideavimrc
    Current: /home/user/.dotfiles/ralph_files/ideavimrc (BROKEN)
    New:     /home/user/.dotfiles/editors/ideavimrc
```

### 5. Run the migration

```bash
ralph migrate
```

This removes old symlinks and creates new ones pointing to the correct locations.

### 6. Verify with apply

```bash
ralph apply
```

This brings everything into sync, creating any new symlinks and running the full apply cycle.

## Migration plan statuses

The migration plan reports each symlink with one of these statuses:

| Status | Meaning |
|--------|---------|
| CORRECT | Symlink already points to the expected location |
| UPDATE | Symlink points to a legacy path and will be updated |
| BROKEN | Symlink is broken but no legacy mapping was found |
| NOT_SYMLINK | Target exists but is a regular file, not a symlink |
| NOT_EXIST | Target does not exist yet (will be created by `ralph apply`) |
| ERROR | An error occurred checking the symlink |

## Tips

- **Always preview with `--dry-run` first.** This shows exactly what will change without touching any files.
- **After migration, legacy_paths entries can be removed.** Once all machines have been migrated, the `[recipe.legacy_paths]` mappings are no longer needed. Keeping them around does no harm, but removing them keeps your recipe files clean.
- **Run `ralph doctor` to verify no broken symlinks remain.** This checks your full setup for problems after migration.
- **Broken symlinks without legacy mappings need manual attention.** If the migration plan shows BROKEN symlinks that have no matching legacy path, you need to either add a mapping or fix those symlinks manually.
- **The migration is idempotent.** Running `ralph migrate` multiple times is safe. Symlinks that are already correct are left alone.

## Further reading

- [Getting started](getting-started.md) -- initial setup walkthrough
- [Recipes](recipes.md) -- modular configuration with recipe.toml files
- [Configuration reference](configuration.md) -- full config.toml documentation
