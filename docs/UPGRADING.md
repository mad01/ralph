# Upgrading

Steps to run after you update ralph.

Read the section for every release between your old version and your new one, oldest first. A release with no section here needs no steps.

`ralph version -o json` prints the `tag` of the build you run. If you build from `main`, read [Unreleased](#unreleased) after every update.

Every entry has the same three lines:

- `Change:` what changed, with the pull request number.
- `Run:` the commands to run after the update.
- `Verify:` a command, and the output that shows the step worked.

## Unreleased

### State files follow the config file

Change: [#69](https://github.com/mad01/ralph/pull/69). ralph keeps its state (`.recipe_state`, `.builds_state`, `generated/`, `pkg/`, `sources/`) in the directory that holds the config file. Before, the state was always in `~/.config/ralph`.

Run: only when your config file is not `~/.config/ralph/config.toml`, which is the case when you set `RALPH_CONFIG` or `XDG_CONFIG_HOME`. Without this step the first run starts from empty state: builds rerun, and cleanup finds nothing to remove. Do one of these before the first `ralph up`:

- Keep the old directory: export `RALPH_STATE_DIR=~/.config/ralph` in your shell rc file.
- Move the state: move `.recipe_state` and `.builds_state` from `~/.config/ralph` into the directory of your config file. Move `pkg/` and `sources/` too, or ralph clones them again.

Verify: `ralph config` prints the directory you chose on its `state dir:` line, and prints no warning that starts with `Warning: ralph keeps its state next to the config file`.

## Adding an entry

A pull request that changes what a user must do after an update adds an entry under [Unreleased](#unreleased), in the three-line form. Examples: a state file that moves, a config key that is renamed, a command that is removed, a command to run once.

Give the entry a `###` heading that names the change. Write `Run` as commands a user can paste, and say who needs the step when it is not everyone. Write `Verify` so that it also passes for a user who already did the step.

`go test ./internal/upgradenotes/` checks the form of this page, and CI runs it. The `ralph-upgrade-note` skill (`ralph install-skills`) walks an agent through writing an entry.

When a release is cut, rename `## Unreleased` to the new tag (for example `## v0.4.0`) and add an empty `## Unreleased` above it, before the tag is pushed. The release notes link to this page at the tag.
