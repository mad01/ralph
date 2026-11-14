package upgradenotes

import (
	"os"
	"strings"
	"testing"
)

// repoDoc is the page this repository ships. Every test starts from it, so
// the checks run against the real form, not a hand-written sample.
const repoDoc = "../../docs/UPGRADING.md"

func readRepoDoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(repoDoc)
	if err != nil {
		t.Fatalf("reading %s: %v", repoDoc, err)
	}
	return string(b)
}

// TestRepoDocFollowsTheForm is the gate: a pull request that adds a malformed
// entry to docs/UPGRADING.md fails here.
func TestRepoDocFollowsTheForm(t *testing.T) {
	doc := Parse(readRepoDoc(t))
	for _, p := range Check(doc) {
		t.Errorf("%s: %s", repoDoc, p)
	}
	if len(doc.Releases) == 0 {
		t.Fatalf("%s: no release section found", repoDoc)
	}
}

// sample is one well-formed page. The cases below break it one way each.
const sample = `# Upgrading

## Unreleased

### State files follow the config file

Change: #69. State lives next to the config file.

Run: only with RALPH_CONFIG set. Do one of these:

- export RALPH_STATE_DIR=~/.config/ralph

Verify: ` + "`ralph config`" + ` prints the directory.

## v0.3.0

### An older change

Change: #35. Something moved.

Run: ` + "`ralph up`" + `.

Verify: ` + "`ralph doctor`" + ` is green.

## Adding an entry

### Not an entry

Prose under a section that is no release.
`

func TestParseReadsEntries(t *testing.T) {
	doc := Parse(sample)

	if got := len(doc.Entries); got != 2 {
		t.Fatalf("entries = %d, want 2 (a heading under a prose section is no entry)", got)
	}
	first := doc.Entries[0]
	if first.Release != Unreleased || first.Title != "State files follow the config file" {
		t.Errorf("first entry = %q in %q", first.Title, first.Release)
	}
	if !strings.Contains(first.Run, "export RALPH_STATE_DIR") {
		t.Errorf("Run lost its continuation lines: %q", first.Run)
	}
	if strings.Contains(first.Run, "Verify") || !strings.HasPrefix(first.Verify, "`ralph config`") {
		t.Errorf("Verify not split from Run: run=%q verify=%q", first.Run, first.Verify)
	}
	if doc.Entries[1].Release != "v0.3.0" {
		t.Errorf("second entry release = %q, want v0.3.0", doc.Entries[1].Release)
	}
	if problems := Check(doc); len(problems) != 0 {
		t.Errorf("well-formed sample has problems: %v", problems)
	}
}

func TestCheckReportsProblems(t *testing.T) {
	tests := []struct {
		name    string
		old     string
		new     string
		wantMsg string
	}{
		{
			name:    "missing verify",
			old:     "Verify: `ralph config` prints the directory.\n",
			new:     "",
			wantMsg: "once each, in that order",
		},
		{
			name:    "labels out of order",
			old:     "Change: #69. State lives next to the config file.\n\nRun: only",
			new:     "Run: first.\n\nChange: #69. State lives next to the config file.\n\nRun: only",
			wantMsg: "once each, in that order",
		},
		{
			name:    "empty run",
			old:     "Run: `ralph up`.",
			new:     "Run:",
			wantMsg: "empty Run:",
		},
		{
			name:    "no pull request number",
			old:     "Change: #35. Something moved.",
			new:     "Change: Something moved.",
			wantMsg: "must name the pull request",
		},
		{
			name:    "unreleased is not first",
			old:     "## Unreleased",
			new:     "## v0.4.0",
			wantMsg: "first release section",
		},
		{
			name:    "release section twice",
			old:     "## v0.3.0",
			new:     "## Unreleased",
			wantMsg: "appears twice",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(sample, tt.old) {
				t.Fatalf("sample does not contain %q", tt.old)
			}
			problems := Check(Parse(strings.Replace(sample, tt.old, tt.new, 1)))
			for _, p := range problems {
				if strings.Contains(p.Message, tt.wantMsg) {
					return
				}
			}
			t.Errorf("want a problem containing %q, got %v", tt.wantMsg, problems)
		})
	}
}
