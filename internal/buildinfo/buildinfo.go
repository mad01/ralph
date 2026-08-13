// Package buildinfo reports which build of ralph is running: the version
// string, the commit it was built from, the last release tag, and when it was
// built. The four values are injected at link time (see the Makefile and
// .goreleaser.yml); whatever the linker did not set is recovered from the
// metadata the Go toolchain stamps into every binary.
//
// It backs `ralph version -o json`, the cross-tool convention sibling tools
// follow so a single probe can ask any of them what build it is:
//
//	{
//	  "version": "2917e73",
//	  "commit": "2917e735a634884fa21ff45a833e2067dc2236be",
//	  "tag": "v0.1.0",
//	  "build_time": "2026-08-13T19:40:11Z"
//	}
//
// All four keys are always present. A value the build could not determine is
// the empty string, so consumers parse one fixed shape.
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// devVersion is the version of a binary built without linker flags, and the
// signal that Get should look for something better in the stamped build info.
const devVersion = "dev"

// shortSHALen matches `git rev-parse --short` in this repo, so a linked build
// and a fallback build report the same shape of version string.
const shortSHALen = 7

// Build metadata set at link time with `-ldflags -X`. Version keeps its
// historical meaning: the short commit for local and fleet builds, the release
// tag for goreleaser builds.
var (
	Version   = devVersion
	Commit    string
	Tag       string
	BuildTime string
)

// Info is the build metadata of a ralph-convention binary. The JSON field
// names are the frozen cross-tool contract — every field is emitted, empty
// when unknown.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Tag       string `json:"tag"`
	BuildTime string `json:"build_time"`
}

// Get returns the build metadata of the running binary, filling anything the
// linker left unset from the Go toolchain's stamped build info.
func Get() Info {
	linked := Info{Version: Version, Commit: Commit, Tag: Tag, BuildTime: BuildTime}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return resolve(linked, nil)
	}
	return resolve(linked, bi)
}

// resolve fills the gaps in the link-time metadata from bi, which may be nil
// when the binary carries no stamped build info. It never overwrites a value
// the linker set: ldflags are the authoritative source.
func resolve(linked Info, bi *debug.BuildInfo) Info {
	linked = fromStamps(linked, bi)
	if linked.Version == "" {
		// A builder outside a git checkout passes an empty -X rather than none,
		// and `ralph version` still has to print a token.
		linked.Version = devVersion
	}
	return linked
}

// fromStamps fills the gaps in linked from the toolchain's stamped build info.
func fromStamps(linked Info, bi *debug.BuildInfo) Info {
	if bi == nil {
		return linked
	}

	var revision, vcsTime string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			vcsTime = s.Value
		}
	}

	if linked.Commit == "" {
		linked.Commit = revision
	}
	if linked.BuildTime == "" {
		linked.BuildTime = vcsTime
	}

	// A module version is all a `go install <module>@<ref>` build has to go on:
	// installing from the module proxy stamps no vcs settings, so the commit
	// survives only as the abbreviated sha inside a pseudo-version. Treat it as
	// best effort — that commit is 12 characters, not a full sha.
	moduleVersion := bi.Main.Version
	if moduleVersion == "(devel)" {
		moduleVersion = ""
	}
	if linked.Commit == "" {
		linked.Commit = pseudoVersionCommit(moduleVersion)
	}
	if linked.Tag == "" {
		linked.Tag = moduleTag(moduleVersion)
	}

	if linked.Version == "" || linked.Version == devVersion {
		switch {
		case revision != "":
			linked.Version = shortSHA(revision)
		case moduleVersion != "":
			linked.Version = moduleVersion
		}
	}

	return linked
}

// moduleTag returns the release tag a module version names, or "" when it names
// none. A pseudo-version names a commit between tags rather than a tag, and Go
// appends "+dirty" when the tree had uncommitted changes, which means the build
// matches no tag even when the version string opens with one.
func moduleTag(v string) string {
	if v == "" || strings.ContainsRune(v, '+') {
		return ""
	}
	if pseudoVersionCommit(v) != "" {
		return ""
	}
	return v
}

// pseudoVersionCommit returns the abbreviated commit embedded in a Go module
// pseudo-version such as v0.1.1-0.20260813194011-2917e735a634, or "" when v is
// not a pseudo-version. The timestamp segment carries a prefix when the module
// had an earlier tag ("0." after a release, "<pre>.0." after a pre-release),
// so only the part after the last dot is the timestamp.
func pseudoVersionCommit(v string) string {
	// A dirty tree gets a "+dirty" suffix; the commit is still in there.
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, "-")
	if len(parts) < 3 {
		return ""
	}
	timestamp, commit := parts[len(parts)-2], parts[len(parts)-1]
	if i := strings.LastIndex(timestamp, "."); i >= 0 {
		timestamp = timestamp[i+1:]
	}
	if len(timestamp) != 14 || !isDigits(timestamp) || len(commit) != 12 || !isHex(commit) {
		return ""
	}
	return commit
}

// shortSHA truncates a commit to the length git's --short produces.
func shortSHA(sha string) string {
	if len(sha) > shortSHALen {
		return sha[:shortSHALen]
	}
	return sha
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
