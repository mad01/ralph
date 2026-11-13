package apply

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/dotfile"
	"github.com/mad01/ralph/internal/hooks"
	"github.com/mad01/ralph/internal/progress"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

// dotfileTally counts dotfile outcomes for the phase's closing line.
type dotfileTally struct {
	applied, skipped, failed int
}

// applyDotfiles processes dotfile symlinks, copies, and templates with pre/post link hooks.
func applyDotfiles(s *sequence) {
	fmt.Fprintln(s.out, "\nProcessing dotfiles...")
	var tally dotfileTally
	dfPhase := s.rpt.AddPhase("Dotfiles")

	dfNames := make([]string, 0, len(s.cfg.Dotfiles))
	for name := range s.cfg.Dotfiles {
		dfNames = append(dfNames, name)
	}
	sort.Strings(dfNames)
	dfProg := progress.New("Dotfiles", len(dfNames))
	if s.opts.Verbose || s.opts.DryRun {
		dfProg = progress.NewQuiet()
	}
	for _, name := range dfNames {
		dfProg.TickWith(name)
		applyDotfile(s, name, s.cfg.Dotfiles[name], dfPhase, &tally)
	}
	dfProg.Done()
	if s.opts.DryRun {
		fmt.Fprintf(
			s.out,
			"  Dotfiles (dry run): would apply %s, %s skipped, %s failed.\n",
			color.GreenString(
				"%d",
				tally.applied,
			),
			color.CyanString("%d", tally.skipped),
			color.YellowString("%d", tally.failed),
		)
	} else {
		fmt.Fprintf(s.out, "  Dotfiles processed: %s applied, %s skipped, %s failed.\n",
			color.GreenString("%d", tally.applied), color.CyanString("%d", tally.skipped), color.YellowString("%d", tally.failed))
	}
}

// applyDotfile links, copies or templates one dotfile between its pre- and
// post-link hooks, recording the outcome in dfPhase and tally.
func applyDotfile(
	s *sequence,
	name string,
	df config.Dotfile,
	dfPhase *report.Phase,
	tally *dotfileTally,
) {
	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	if reason := s.skipReason(df.Enable, df.Hosts, df.Profiles); reason != "" {
		fmt.Fprintf(s.out, "  %s %s\n", color.CyanString("skip"), dim(name+" ("+reason+")"))
		dfPhase.AddSkip(name, reason)
		return
	}
	fmt.Fprintf(s.out, "  %s\n", bold(name))
	fmt.Fprintf(s.out, "    %s → %s\n", dim(df.Target), dim(df.Source))

	if !runPreLinkHooks(s, name, df, dfPhase) {
		tally.failed++
		return
	}

	symlinkErr, ok := placeDotfile(s, name, df, dfPhase)
	if !ok {
		tally.failed++
		return
	}

	if errors.Is(symlinkErr, dotfile.ErrSkipped) {
		tally.skipped++
		dfPhase.AddSkip(name, "target exists")
	} else if symlinkErr != nil {
		fmt.Fprintln(s.errOut, color.RedString("    error: %s: %v", name, symlinkErr))
		tally.failed++
		dfPhase.AddFail(name, symlinkErr.Error(), symlinkErr)
	} else {
		tally.applied++
		// Post-link hooks run only once the dotfile is in place.
		if runPostLinkHooks(s, name, df, dfPhase) {
			dfPhase.AddOK(name, "")
		}
	}
}

// runPreLinkHooks runs one dotfile's pre-link hooks and reports whether they
// all succeeded. A failure is recorded in dfPhase and stops the dotfile.
func runPreLinkHooks(s *sequence, name string, df config.Dotfile, dfPhase *report.Phase) bool {
	preHooks, exists := s.cfg.Hooks.PreLink[name]
	if !exists || len(preHooks) == 0 {
		return true
	}
	err := hooks.RunHooks(
		s.out, preHooks, hooks.PreLink, linkHookContext(s, name, df), s.opts.DryRun,
	)
	if err != nil {
		fmt.Fprintln(
			s.errOut,
			color.RedString("Error executing pre-link hooks for %s: %v", name, err),
		)
		dfPhase.AddFail(name, fmt.Sprintf("pre-link hook: %v", err), err)
		return false
	}
	return true
}

// runPostLinkHooks runs one dotfile's post-link hooks and reports whether
// they all succeeded. A failure is recorded in dfPhase as a warning.
func runPostLinkHooks(s *sequence, name string, df config.Dotfile, dfPhase *report.Phase) bool {
	postHooks, exists := s.cfg.Hooks.PostLink[name]
	if !exists || len(postHooks) == 0 {
		return true
	}
	err := hooks.RunHooks(
		s.out, postHooks, hooks.PostLink, linkHookContext(s, name, df), s.opts.DryRun,
	)
	if err != nil {
		fmt.Fprintln(
			s.errOut,
			color.YellowString("Warning: post-link hook for %s failed: %v", name, err),
		)
		dfPhase.AddWarn(name+"/post-hook", err.Error())
		return false
	}
	return true
}

// linkHookContext is the context a pre- or post-link hook of one dotfile runs with.
func linkHookContext(s *sequence, name string, df config.Dotfile) *hooks.HookContext {
	return &hooks.HookContext{
		DotfileName: name,
		SourcePath:  config.JoinSourcePath(s.cfg.DotfilesRepoPath, df.Source),
		TargetPath:  df.Target,
		DryRun:      s.opts.DryRun,
	}
}

// placeDotfile renders a template dotfile first, then creates the dotfile the
// way its kind says (see placerFor) and returns the creator's error. ok is
// false when the template failed; that is recorded in dfPhase as a warning.
func placeDotfile(
	s *sequence,
	name string,
	df config.Dotfile,
	dfPhase *report.Phase,
) (symlinkErr error, ok bool) {
	dotfileToSymlink := df
	repoPathForSymlink := s.cfg.DotfilesRepoPath

	if df.IsTemplate {
		processedPath, rendered := renderTemplate(s, name, df, dfPhase)
		if !rendered {
			return nil, false
		}
		dotfileToSymlink.Source = processedPath
		repoPathForSymlink = "" // Processed template is an absolute path
	}

	place := placerFor(dotfileKind(df.Action))
	symlinkErr = place(s.out, dotfileToSymlink, repoPathForSymlink, s.link, s.opts.DryRun)

	// Cleanup for templated files: the processed template was copied to the
	// target, so remove the temp file. Dry-run wrote nothing.
	if df.IsTemplate && repoPathForSymlink == "" && !s.opts.DryRun {
		removeProcessedTemplate(s, dotfileToSymlink.Source)
	}
	return symlinkErr, true
}

// renderTemplate renders a template dotfile's source and returns the path of
// the rendered file. A failure is printed and recorded in dfPhase as a
// warning, and ok is false.
func renderTemplate(
	s *sequence,
	name string,
	df config.Dotfile,
	dfPhase *report.Phase,
) (processedPath string, ok bool) {
	dim := color.New(color.Faint).SprintFunc()
	templateData := make(map[string]any)
	currentSourcePath := config.JoinSourcePath(s.cfg.DotfilesRepoPath, df.Source)

	fmt.Fprintf(s.out, "    %s\n", dim("template: "+df.Source))
	// WriteProcessedTemplateToFile returns a real path in both modes (a
	// placeholder under os.TempDir() on dry-run), so no sentinel is
	// needed.
	processedPath, templateErr := dotfile.WriteProcessedTemplateToFile(
		s.out,
		currentSourcePath,
		s.cfg,
		templateData,
		s.opts.DryRun,
	)
	if templateErr != nil {
		fmt.Fprintln(
			s.errOut,
			color.YellowString(
				"    - Warning: Error processing template for %s: %v",
				name,
				templateErr,
			),
		)
		dfPhase.AddWarn(name, fmt.Sprintf("template error: %v", templateErr))
		return "", false
	}
	return processedPath, true
}

// placeFunc creates one dotfile at its target, the shape dotfile.CreateSymlink,
// CreateDirSymlink and CopyFile share.
type placeFunc func(
	w io.Writer,
	df config.Dotfile,
	repoPath string,
	action dotfile.SymlinkAction,
	dryRun bool,
) error

// placerFor returns the function that creates an artifact of kind: a copy, a
// directory symlink, or (any other kind) a plain symlink. The dotfiles and
// dirs_mirror run steps both pick their creator here.
func placerFor(kind state.ArtifactKind) placeFunc {
	switch kind {
	case state.KindCopy:
		return dotfile.CopyFile
	case state.KindDirSymlink:
		return dotfile.CreateDirSymlink
	}
	return dotfile.CreateSymlink
}

// removeProcessedTemplate removes the temporary file a template rendered to,
// warning when that fails.
func removeProcessedTemplate(s *sequence, path string) {
	if strings.HasPrefix(path, os.TempDir()) ||
		strings.Contains(path, "ralph-temp-") {
		if removeErr := os.Remove(path); removeErr != nil {
			fmt.Fprintln(
				s.errOut,
				color.YellowString(
					"    - Warning: failed to remove temporary processed file %s: %v",
					path,
					removeErr,
				),
			)
		}
	}
}

// declareDotfiles records the target of every applying dotfile under the kind
// applyDotfile creates.
func declareDotfiles(m *manifest) {
	for _, df := range m.cfg.Dotfiles {
		m.declareDotfile(df)
	}
}

// declareDotfile records one dotfile's target under its kind when it applies.
func (m *manifest) declareDotfile(df config.Dotfile) {
	if !m.applies(df.Enable, df.Hosts, df.Profiles) {
		return
	}
	target, err := linkTarget(df)
	if err != nil {
		return
	}
	m.track(df.OwnerRecipe, dotfileKind(df.Action), target)
}

// dotfileKind is the artifact kind a dotfile's action creates: "copy" copies
// the file, "symlink_dir" links a directory, and anything else ("symlink", the
// default) is a plain symlink.
func dotfileKind(action config.LinkAction) state.ArtifactKind {
	switch action {
	case config.ActionCopy:
		return state.KindCopy
	case config.ActionSymlinkDir:
		return state.KindDirSymlink
	}
	return state.KindSymlink
}

// linkTarget is the path on disk a linked or copied dotfile occupies: its
// target, expanded the way dotfile.CreateSymlink, CreateDirSymlink and
// CopyFile expand it.
func linkTarget(df config.Dotfile) (string, error) {
	return config.ExpandPath(df.Target)
}
