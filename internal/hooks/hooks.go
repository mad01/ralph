package hooks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/mad01/ralph/internal/execx"
)

// hookTimeout bounds one lifecycle hook command (pre/post apply, pre/post
// link, pre/post uninstall). A var so tests can shorten it.
var hookTimeout = execx.DefaultTimeout

// HookType represents the different types of hooks that can be triggered.
type HookType string

const (
	PreApply  HookType = "pre_apply"
	PostApply HookType = "post_apply"
	PreLink   HookType = "pre_link"
	PostLink  HookType = "post_link"
)

// HookContext contains context information for hook execution
type HookContext struct {
	// DotfileName is the name of the dotfile (only for pre/post link hooks)
	DotfileName string
	// SourcePath is the source path of the dotfile (only for pre/post link hooks)
	SourcePath string
	// TargetPath is the target path of the dotfile (only for pre/post link hooks)
	TargetPath string
	// DryRun indicates whether this is a dry run
	DryRun bool
}

// Run executes a hook script with the given context
func Run(w io.Writer, script string, hookContext *HookContext, dryRun bool) error {
	// Expand the script command with context variables
	expandedScript := expandVariables(script, hookContext)

	if dryRun {
		fmt.Fprintf(w, "[DRY RUN] Would run hook: %s\n", expandedScript)
		return nil
	}

	if strings.TrimSpace(expandedScript) == "" {
		return fmt.Errorf("empty hook command")
	}

	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()

	cmd := execx.Command(ctx, "sh", "-c", expandedScript)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("hook timed out after %s", hookTimeout)
	}
	return err
}

// RunHooks executes all hooks of a specific type with the given context
func RunHooks(
	w io.Writer,
	scripts []string,
	hookType HookType,
	context *HookContext,
	dryRun bool,
) error {
	if len(scripts) == 0 {
		return nil
	}

	fmt.Fprintf(w, "Running %s hooks...\n", hookType)
	for _, script := range scripts {
		if err := Run(w, script, context, dryRun); err != nil {
			return fmt.Errorf("hook %s failed: %w", script, err)
		}
	}
	return nil
}

// placeholderPattern matches a link hook placeholder such as {target}; the
// submatch is the placeholder's name.
var placeholderPattern = regexp.MustCompile(`\{(dotfile|source|target|source_path|target_path)\}`)

// hasLinkContext reports whether c describes a dotfile being linked. Apply
// and uninstall hooks run with a context that sets none of the link fields.
func (c *HookContext) hasLinkContext() bool {
	return c != nil && (c.DotfileName != "" || c.SourcePath != "" || c.TargetPath != "")
}

// expandVariables replaces the placeholders in a link hook's script with the
// context's values. A script without link context is returned unchanged, and
// a placeholder directly preceded by '$' is left alone, so a shell variable
// such as ${target} keeps working.
func expandVariables(script string, context *HookContext) string {
	if !context.hasLinkContext() {
		return script
	}

	values := map[string]string{
		"dotfile":     context.DotfileName,
		"source":      context.SourcePath,
		"target":      context.TargetPath,
		"source_path": context.SourcePath,
		"target_path": context.TargetPath,
	}

	var b strings.Builder
	last := 0
	for _, m := range placeholderPattern.FindAllStringSubmatchIndex(script, -1) {
		start, end := m[0], m[1]
		if start > 0 && script[start-1] == '$' {
			continue
		}
		b.WriteString(script[last:start])
		b.WriteString(values[script[m[2]:m[3]]])
		last = end
	}
	b.WriteString(script[last:])
	return b.String()
}
