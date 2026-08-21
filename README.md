# ralph

*"Me fail dotfiles? That's unpossible."*

A dotfiles manager written in Go. Declare your setup in a TOML file -- symlinks, copies, aliases, functions, repos, build hooks, packages -- and `ralph up` makes it happen. Idempotent, repeatable, no surprises.

## Install

```bash
# With Go 1.25+
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
- **Remote recipe sources** -- pull recipes from other git repos, so a project can ship its code and its install recipe together
- **Machine profiles** -- tag machines and recipes with roles (`personal`, `work`, ...) and hostnames; only matching recipes apply

## Remote recipe sources

A repo can ship recipes next to its code and any machine can consume them.
Point a `[[recipe_sources]]` stanza at the repo:

```toml
[[recipe_sources]]
name = "thismoon"
url = "git@github.com:mad01/thismoon.git"
ref = "main"
update = true
profiles = ["personal"]
```

When the source profiles match the machine, ralph clones it into
`~/.config/ralph/sources/<name>`, discovers
`recipe.toml` files under its `recipes/` directory, and merges them under the
namespaced identity `<source>/<recipe>` (here `thismoon/reminder`,
`thismoon/csl`, ...). A branch ref with `update = true` follows the branch on
every `ralph up`; a tag or commit ref pins. Machine-private wiring (secrets,
host config, overlays) stays in your own config repo as companion recipes
layered on top. A source with non-matching profiles is never checked out or
synced on that machine.

See [Recipes → Remote sources](docs/recipes.md) for the full reference, and
[thismoon](https://github.com/mad01/thismoon) for a repo built around this
pattern.

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

## Releases

Releases are tag-driven: pushing a `v*` tag runs GoReleaser in CI, which
builds a darwin/arm64 binary (ralph targets macOS), embeds the build metadata
via ldflags, and publishes a tar.gz archive with checksums and a changelog to
the [Releases](https://github.com/mad01/ralph/releases) page.

`ralph version` prints which build you are running, and `ralph version -o json`
prints its version, commit, tag, and build time. See
[commands](docs/commands.md#ralph-version) for the field-by-field contract that
sibling tools follow.

## Contributing

Contributions welcome. Open an issue or PR on [GitHub](https://github.com/mad01/ralph).

## License

See [LICENSE](LICENSE) file.
