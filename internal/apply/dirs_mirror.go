package apply

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/dotfile"
	"github.com/mad01/ralph/internal/progress"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

// applyDirsMirror processes dirs_mirror entries, creating symlinks for each
// entry (file or subdirectory) found in the source directory.
func applyDirsMirror(s *sequence) {
	if len(s.cfg.DirsMirror) == 0 {
		return
	}

	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	dmPhase := s.rpt.AddPhase("Dirs mirror")
	dmNames := make([]string, 0, len(s.cfg.DirsMirror))
	for name := range s.cfg.DirsMirror {
		dmNames = append(dmNames, name)
	}
	sort.Strings(dmNames)

	prog := progress.New("Dirs mirror", len(dmNames))
	if s.opts.Verbose || s.opts.DryRun {
		prog = progress.NewQuiet()
	}

	for _, name := range dmNames {
		prog.TickWith(name)
		dm := s.cfg.DirsMirror[name]

		if reason := s.skipReason(dm.Enable, dm.Hosts, dm.Profiles); reason != "" {
			fmt.Fprintf(s.out, "  %s %s\n", color.CyanString("skip"), dim(name+" ("+reason+")"))
			dmPhase.AddSkip(name, reason)
			continue
		}

		fmt.Fprintf(s.out, "  %s\n", bold(name))
		entries, failure := openMirror(s, dm)
		if failure != nil {
			failure.report(s, name, dmPhase)
			continue
		}

		linked, failed := linkMirrorEntries(s, name, dm, entries)
		if failed > 0 {
			dmPhase.AddFail(
				name,
				fmt.Sprintf("%d linked, %d failed", linked, failed),
				fmt.Errorf("%d entries failed", failed),
			)
		} else {
			dmPhase.AddOK(name, fmt.Sprintf("%d entries linked", linked))
		}
	}
	prog.Done()
}

// mirrorFailure is what stops one dirs_mirror: line is printed after the
// "    error: <name>: " prefix, and msg and err are recorded in the report.
type mirrorFailure struct {
	line, msg string
	err       error
}

// mirrorFail returns a mirrorFailure whose printed line is format applied to args.
func mirrorFail(err error, msg, format string, args ...any) *mirrorFailure {
	return &mirrorFailure{line: fmt.Sprintf(format, args...), msg: msg, err: err}
}

// report prints f and records it as the failure of the mirror name in phase.
func (f *mirrorFailure) report(s *sequence, name string, phase *report.Phase) {
	fmt.Fprintln(s.errOut, color.RedString("    error: %s: %s", name, f.line))
	phase.AddFail(name, f.msg, f.err)
}

// openMirror reads a dirs_mirror's source directory and makes sure its target
// directory exists (a dry run creates nothing). It returns the failure that
// stops the mirror, or nil.
func openMirror(s *sequence, dm config.DirMirror) ([]os.DirEntry, *mirrorFailure) {
	absoluteSource, err := mirrorSource(s.cfg.DotfilesRepoPath, dm)
	if err != nil {
		return nil, mirrorFail(err, fmt.Sprintf("expand source: %v", err),
			"failed to expand source path: %v", err)
	}

	// Ensure source directory exists
	srcInfo, err := os.Stat(absoluteSource)
	if err != nil {
		return nil, mirrorFail(err, fmt.Sprintf("source not found: %v", err),
			"source directory '%s' does not exist: %v", absoluteSource, err)
	}
	if !srcInfo.IsDir() {
		err := fmt.Errorf("source '%s' is not a directory", absoluteSource)
		return nil, mirrorFail(err, err.Error(), "%v", err)
	}

	// Read entries from source directory
	entries, err := os.ReadDir(absoluteSource)
	if err != nil {
		return nil, mirrorFail(err, fmt.Sprintf("read source dir: %v", err),
			"failed to read source directory: %v", err)
	}

	expandedTarget, err := config.ExpandPath(dm.Target)
	if err != nil {
		return nil, mirrorFail(err, fmt.Sprintf("expand target: %v", err),
			"failed to expand target path: %v", err)
	}

	// Ensure target directory exists
	if !s.opts.DryRun {
		if err := os.MkdirAll(expandedTarget, 0o755); err != nil {
			return nil, mirrorFail(err, fmt.Sprintf("create target dir: %v", err),
				"failed to create target directory: %v", err)
		}
	}
	return entries, nil
}

// linkMirrorEntries links every visible entry of a dirs_mirror's source
// directory into its target and returns how many it linked and how many
// failed. A link already in place counts as linked.
func linkMirrorEntries(
	s *sequence,
	name string,
	dm config.DirMirror,
	entries []os.DirEntry,
) (linked, failed int) {
	dim := color.New(color.Faint).SprintFunc()
	place := placerFor(mirrorKind(dm.Action))
	for _, entry := range mirrorEntries(dm, entries) {
		fmt.Fprintf(s.out, "    %s → %s\n", dim(entry.link.Target), dim(entry.link.Source))

		symlinkErr := place(s.out, entry.link, s.cfg.DotfilesRepoPath, s.link, s.opts.DryRun)
		if errors.Is(symlinkErr, dotfile.ErrSkipped) {
			linked++ // already in place, count as success
		} else if symlinkErr != nil {
			fmt.Fprintln(
				s.errOut,
				color.RedString("    error: %s/%s: %v", name, entry.name, symlinkErr),
			)
			failed++
		} else {
			linked++
		}
	}
	return linked, failed
}

// declareDirsMirror records the link each visible source entry of every
// applying dirs_mirror makes. An unreadable source is an error for a mirror a
// recipe owns, since dropping its entries would orphan live links.
func declareDirsMirror(m *manifest) error {
	for _, dm := range m.cfg.DirsMirror {
		if !m.applies(dm.Enable, dm.Hosts, dm.Profiles) {
			continue
		}
		absoluteSource, err := mirrorSource(m.cfg.DotfilesRepoPath, dm)
		if err != nil {
			continue
		}
		entries, err := os.ReadDir(absoluteSource)
		if err != nil {
			if dm.OwnerRecipe == "" {
				// Nothing of an ownerless mirror is recorded, so an unreadable
				// source cannot orphan anything; it only goes unprotected.
				continue
			}
			return fmt.Errorf(
				"cleanup: cannot read dirs_mirror source %q for recipe %q: %w",
				absoluteSource,
				dm.OwnerRecipe,
				err,
			)
		}
		kind := mirrorKind(dm.Action)
		for _, entry := range mirrorEntries(dm, entries) {
			target, err := linkTarget(entry.link)
			if err != nil {
				continue
			}
			m.track(dm.OwnerRecipe, kind, target)
		}
	}
	return nil
}

// mirrorSource returns the absolute source directory of a dirs_mirror, the
// way dotfile.CreateSymlink resolves each entry's source: joined onto the
// dotfiles repo unless absolute, then expanded.
func mirrorSource(repoPath string, dm config.DirMirror) (string, error) {
	return config.ExpandPath(config.JoinSourcePath(repoPath, dm.Source))
}

// mirrorEntry is one link a dirs_mirror makes for an entry of its source
// directory.
type mirrorEntry struct {
	name string         // the entry's name in the source directory
	link config.Dotfile // the mirror's source and target, each joined with name
}

// mirrorEntries returns the link for every entry of a dirs_mirror's source
// directory, skipping hidden files and directories.
func mirrorEntries(dm config.DirMirror, entries []os.DirEntry) []mirrorEntry {
	links := make([]mirrorEntry, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		links = append(links, mirrorEntry{
			name: entry.Name(),
			link: config.Dotfile{
				Source: filepath.Join(dm.Source, entry.Name()),
				Target: filepath.Join(dm.Target, entry.Name()),
			},
		})
	}
	return links
}

// mirrorKind is the artifact kind of a dirs_mirror's links: "symlink_dir"
// links directories, and anything else ("symlink", the default) links each
// entry as a plain symlink. A mirror never copies, so placerFor maps this kind
// to CreateDirSymlink or CreateSymlink.
func mirrorKind(action config.LinkAction) state.ArtifactKind {
	if action == config.ActionSymlinkDir {
		return state.KindDirSymlink
	}
	return state.KindSymlink
}
