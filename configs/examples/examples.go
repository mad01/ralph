// Package examples holds ralph's shipped example configs and embeds the
// starter config `ralph init` writes, so an installed binary carries it.
package examples

import _ "embed"

// DefaultConfig is the starter config.toml that `ralph init` writes.
//
//go:embed default.config.toml
var DefaultConfig string

// DefaultRepoPathLine is the dotfiles_repo_path assignment in DefaultConfig
// that `ralph init` replaces with the path the user enters.
const DefaultRepoPathLine = `dotfiles_repo_path = "~/.dotfiles"`
