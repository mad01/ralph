---
name: ralph-debug
description: Troubleshoot ralph issues - broken symlinks, config validation errors, build failures, recipe conflicts, and host filtering problems. Use when ralph up fails or dotfiles are not being applied correctly.
---

# ralph Troubleshooting

Use this skill when diagnosing ralph issues.

## Diagnostic Commands

```bash
ralph doctor              # Full health check
ralph list                # Show all managed items and status
ralph up --dry-run -v     # Verbose dry run - shows what would happen
ralph up --verbose        # Verbose output during apply
```

## Common Issues

### Config won't load

```bash
# Validate TOML syntax
ralph doctor

# Check config location
ls -la ~/.config/ralph/config.toml
echo $XDG_CONFIG_HOME
```

Common causes:
- Invalid TOML syntax (missing quotes, bad escapes)
- Missing `dotfiles_repo_path`
- Dotfile missing `source` or `target`
- Recipe conflict (duplicate dotfile keys across recipes)

### Symlinks not created

Check:
1. Source file exists: `ls -la ~/.dotfiles/<source>`
2. Host filter: item may have `hosts = ["other-host"]`
3. Enable flag: item may have `enable = false`
4. Parent directory exists for target path

```bash
ralph list                    # Shows status of all items
ralph up --dry-run -v         # Shows what would be linked
hostname                      # Check current hostname
```

### Build hooks not running

Build hooks track state in `~/.config/ralph/.builds_state`:

```bash
cat ~/.config/ralph/.builds_state    # Check stored git hashes
ralph up --build <name>              # Run specific build
ralph up --force                     # Force all builds
ralph up --reset-builds              # Clear all build state
```

A deleted binary does **not** need `--reset-builds`: when a package's declared `install_path` is missing from disk, a plain `ralph up` rebuilds it even though the source is unchanged. Reach for `--reset-builds` only when you want to force a full rebuild of everything.

Run modes:
- `always` - runs every time
- `once` - runs on first apply, then only on git changes
- `manual` - only runs with `--build <name>`

### Recipe not loading

```bash
# Check recipe file exists
ls ~/.dotfiles/recipes/<name>/recipe.toml

# Check if excluded
grep -A5 'recipes_config' ~/.config/ralph/config.toml

# Verbose apply shows recipe loading
ralph up --dry-run -v
```

Common causes:
- Recipe not referenced in `[[recipes]]` and auto_discover not enabled
- Recipe excluded by glob pattern
- Recipe disabled via override
- Host filter on recipe reference

### Packages not building

```bash
ralph up -v                   # Pull, sync, and apply with verbose output
ralph up --force              # Force rebuild all packages
```

Check:
- Remote package cloned: `ls ~/.config/ralph/pkg/<name>`
- Working directory exists
- Build commands are correct
- Package state: `cat ~/.config/ralph/.builds_state`

### Shell config not applied

Generated files live in `~/.config/ralph/generated/`:

```bash
ls ~/.config/ralph/generated/
cat ~/.config/ralph/generated/aliases.*
cat ~/.config/ralph/generated/functions.*
```

The RC file (~/.bashrc, ~/.zshrc, etc.) should contain a managed block:

```bash
grep -A5 "RALPH MANAGED" ~/.zshrc
```

If the block is missing, run `ralph up` to inject it.

## File Locations

| File | Purpose |
|------|---------|
| `~/.config/ralph/config.toml` | Main configuration |
| `~/.config/ralph/.builds_state` | Build hook state (JSON) |
| `~/.config/ralph/generated/` | Generated shell scripts |
| `~/.config/ralph/pkg/` | Cloned remote packages |
| `~/.dotfiles/recipes/` | Recipe files (default) |
