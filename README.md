# ralph

*"Me fail dotfiles? That's unpossible."*

A dotfiles manager written in Go. Declare your setup in a TOML file -- symlinks, copies, aliases, functions, repos, build hooks, packages -- and `ralph up` makes it happen. Idempotent, repeatable, no surprises.

## Install

```bash
# With Go 1.21+
go install github.com/mad01/ralph/cmd/ralph@latest

# Or build from source
git clone https://github.com/mad01/ralph.git
cd ralph && make build
```

Pre-built binaries are available on the [Releases](https://github.com/mad01/ralph/releases) page.

## Quick start

```bash
# Create a config file at ~/.config/ralph/config.toml
ralph init

# Set up a dotfiles repo
mkdir -p ~/.dotfiles
cp ~/.bashrc ~/.dotfiles/.bashrc
```

Add entries to your config:

```toml
dotfiles_repo_path = "~/.dotfiles"

[dotfiles.bashrc]
source = ".bashrc"
target = "~/.bashrc"

[shell.aliases.ll]
command = "ls -alhF"

[shell.aliases.g]
command = "git"
```

Apply it:

```bash
ralph up
```

Run it again and nothing changes. Run it after editing your config and only the diff gets applied.

For scripting, add `-o json` to get a machine-readable run report on stdout:

```bash
ralph up --no-sync -o json | jq '.summary'
```

See [Commands → JSON output](docs/commands.md#json-output) for the document shape.

## What ralph manages

- **Dotfiles** -- symlink, copy, or template files from a source repo to their targets
- **Directories** -- create directories before other operations
- **Git repositories** -- clone, pull, pin to branch or commit
- **Shell config** -- aliases, functions, and environment variables injected into your rc file
- **Build hooks** -- run commands during apply (always, once, or manual)
- **Packages** -- clone remote tools or track local projects; `ralph up` syncs and rebuilds on changes
- **Tool checks** -- verify tools are installed, show install hints
- **Recipes** -- split config into modular `recipe.toml` files alongside source files, ordered into dependency waves (`ralph graph` renders the layout)

## Documentation

| Guide | Description |
|-------|-------------|
| [Getting Started](docs/getting-started.md) | Zero to working setup |
| [Commands](docs/commands.md) | All commands with flags and examples |
| [Configuration](docs/configuration.md) | Full `config.toml` schema reference |
| [Recipes](docs/recipes.md) | Modular configuration with auto-discovery |
| [Packages](docs/packages.md) | Package management (`ralph up`) |
| [Workflows](docs/workflows.md) | Daily usage patterns |
| [Templating](docs/templating.md) | Go template system for dotfiles |
| [Migration](docs/migration.md) | Symlink migration after reorganization |

## Examples

| Example | Description |
|---------|-------------|
| [minimal](examples/minimal/) | Simplest setup -- 3 dotfiles, 2 aliases |
| [recipe-based](examples/recipe-based/) | Modular config with auto-discovery |
| [with-packages](examples/with-packages/) | Local and remote package builds |
| [dotfiles-repo](examples/dotfiles-repo/) | Full dotfiles repository structure |

## Contributing

Contributions welcome. Open an issue or PR on [GitHub](https://github.com/mad01/ralph).

## License

See [LICENSE](LICENSE) file.
