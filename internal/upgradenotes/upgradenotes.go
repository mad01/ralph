// Package upgradenotes parses docs/UPGRADING.md into entries and checks them
// against the form that page describes: release sections holding entries, each
// entry a title followed by Change, Run and Verify, in that order.
//
// The package reads text and returns values. It opens no file and runs no
// command, so a test, a doctor check or a later command can all use it.
package upgradenotes

import (
	"fmt"
	"regexp"
	"strings"
)

// Unreleased is the heading of the section that holds entries no tag carries
// yet. It is always the first release section.
const Unreleased = "Unreleased"

// The three labels of an entry, in the order an entry carries them.
const (
	labelChange = "Change"
	labelRun    = "Run"
	labelVerify = "Verify"
)

var (
	entryLabels = []string{labelChange, labelRun, labelVerify}
	releaseTag  = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	pullRequest = regexp.MustCompile(`#\d+`)
)

// Entry is one upgrade note: what changed, what the user runs, and how they
// see that it worked.
type Entry struct {
	Release string // Unreleased, or the tag the entry shipped in, such as "v0.4.0"
	Title   string
	Change  string
	Run     string
	Verify  string
	Line    int // 1-based line of the entry heading

	labels []string // labels in the order the entry carries them
}

// Doc is a parsed upgrade page.
type Doc struct {
	Releases []Release
	Entries  []Entry
}

// Release is one release section heading.
type Release struct {
	Name string
	Line int // 1-based line of the heading
}

// Problem is one place where a page departs from the form.
type Problem struct {
	Line    int
	Message string
}

// String formats the problem as "line N: message".
func (p Problem) String() string {
	return fmt.Sprintf("line %d: %s", p.Line, p.Message)
}

// Parse reads an upgrade page. A "## " heading that is Unreleased or a tag
// opens a release section; any other "## " heading closes it, so prose
// sections hold no entries. A "### " heading inside a release section opens
// an entry, and a line that starts with a label and a colon opens that field.
func Parse(doc string) Doc {
	var (
		out       Doc
		inRelease bool
		release   string
		entry     *Entry
		field     *string
	)
	for i, line := range strings.Split(doc, "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			name := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			entry, field = nil, nil
			inRelease = name == Unreleased || releaseTag.MatchString(name)
			if inRelease {
				release = name
				out.Releases = append(out.Releases, Release{Name: name, Line: i + 1})
			}
		case strings.HasPrefix(line, "### ") && inRelease:
			out.Entries = append(out.Entries, Entry{
				Release: release,
				Title:   strings.TrimSpace(strings.TrimPrefix(line, "### ")),
				Line:    i + 1,
			})
			entry, field = &out.Entries[len(out.Entries)-1], nil
		case entry != nil:
			field = appendLine(entry, field, line)
		}
	}
	for i := range out.Entries {
		trimFields(&out.Entries[i])
	}
	return out
}

// appendLine adds one body line to the entry. A line that opens a label
// switches the field; any other line continues the field in progress. It
// returns the field later lines continue.
func appendLine(entry *Entry, field *string, line string) *string {
	for _, label := range entryLabels {
		rest, ok := strings.CutPrefix(line, label+":")
		if !ok {
			continue
		}
		entry.labels = append(entry.labels, label)
		field = entry.field(label)
		*field += strings.TrimSpace(rest)
		return field
	}
	if field != nil {
		*field += "\n" + line
	}
	return field
}

// field returns the entry field a label fills.
func (e *Entry) field(label string) *string {
	switch label {
	case labelChange:
		return &e.Change
	case labelRun:
		return &e.Run
	default:
		return &e.Verify
	}
}

func trimFields(e *Entry) {
	e.Change = strings.TrimSpace(e.Change)
	e.Run = strings.TrimSpace(e.Run)
	e.Verify = strings.TrimSpace(e.Verify)
}

// Check returns every problem in a parsed page, in line order. No problems
// means the page follows the form.
func Check(doc Doc) []Problem {
	problems := checkReleases(doc.Releases)
	for _, entry := range doc.Entries {
		problems = append(problems, checkEntry(entry)...)
	}
	return problems
}

// checkReleases checks that Unreleased opens the page and no section repeats.
func checkReleases(releases []Release) []Problem {
	if len(releases) == 0 || releases[0].Name != Unreleased {
		return []Problem{{Line: 1, Message: "the first release section must be \"## Unreleased\""}}
	}
	var problems []Problem
	seen := map[string]bool{}
	for _, release := range releases {
		if seen[release.Name] {
			problems = append(problems, Problem{
				Line:    release.Line,
				Message: fmt.Sprintf("release section %q appears twice", release.Name),
			})
		}
		seen[release.Name] = true
	}
	return problems
}

// checkEntry checks one entry: a title, the three labels once each and in
// order, text behind each, and a pull request number in Change.
func checkEntry(entry Entry) []Problem {
	var problems []Problem
	add := func(format string, args ...any) {
		problems = append(
			problems,
			Problem{Line: entry.Line, Message: fmt.Sprintf(format, args...)},
		)
	}
	if entry.Title == "" {
		add("entry has no title")
	}
	if strings.Join(entry.labels, ",") != strings.Join(entryLabels, ",") {
		add("entry %q must carry Change:, Run: and Verify: once each, in that order; found %v",
			entry.Title, entry.labels)
		return problems
	}
	for _, label := range entryLabels {
		if *entry.field(label) == "" {
			add("entry %q has an empty %s:", entry.Title, label)
		}
	}
	if !pullRequest.MatchString(entry.Change) {
		add("entry %q: Change: must name the pull request, such as #69", entry.Title)
	}
	return problems
}
