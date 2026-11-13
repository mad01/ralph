package commands

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// commandsDocPath is the command reference, relative to this package directory.
const commandsDocPath = "../../../docs/commands.md"

// userCommandPaths returns the command path of every command below cmd that a
// user can type, skipping hidden commands and cobra's built-in help and
// completion commands.
func userCommandPaths(cmd *cobra.Command) []string {
	var paths []string
	for _, c := range cmd.Commands() {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		paths = append(paths, c.CommandPath())
		paths = append(paths, userCommandPaths(c)...)
	}
	return paths
}

// markdownHeadings returns the heading lines of the markdown file at path,
// leaving out lines inside fenced code blocks (where "#" starts a comment).
func markdownHeadings(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var headings []string
	inFence := false
	for line := range strings.SplitSeq(string(body), "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(line, "#") {
			headings = append(headings, line)
		}
	}
	return headings
}

// Every command a user can type has a heading in docs/commands.md that names
// it in backticks, as in "## `ralph up`" or "### `ralph state show`", so a new
// command cannot ship without a section in the reference.
func TestCommandsDoc_EveryCommandHasHeading(t *testing.T) {
	headings := markdownHeadings(t, commandsDocPath)
	paths := userCommandPaths(rootCmd)
	if len(paths) == 0 {
		t.Fatal("walk found no commands; the tree did not load")
	}

	for _, path := range paths {
		want := "`" + path + "`"
		names := func(heading string) bool { return strings.Contains(heading, want) }
		if !slices.ContainsFunc(headings, names) {
			t.Errorf("docs/commands.md has no heading naming %s", want)
		}
	}
}
