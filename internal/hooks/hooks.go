package hooks

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// HookType represents the different types of hooks that can be triggered.
type HookType string

const (
	// Pre-apply hooks run before any dotfiles are processed
	PreApply HookType = "pre_apply"
	// Post-apply hooks run after all dotfiles are processed
	PostApply HookType = "post_apply"
	// Pre-link hooks run before a specific dotfile is symlinked
	PreLink HookType = "pre_link"
	// Post-link hooks run after a specific dotfile is symlinked
	PostLink HookType = "post_link"
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
func Run(w io.Writer, script string, context *HookContext, dryRun bool) error {
	// Expand the script command with context variables
	expandedScript := expandVariables(script, context)

	if dryRun {
		fmt.Fprintf(w, "[DRY RUN] Would run hook: %s\n", expandedScript)
		return nil
	}

	// Split the command and arguments
	parts := strings.Fields(expandedScript)
	if len(parts) == 0 {
		return fmt.Errorf("empty hook command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// RunHooks executes all hooks of a specific type with the given context
func RunHooks(w io.Writer, scripts []string, hookType HookType, context *HookContext, dryRun bool) error {
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

// expandVariables replaces placeholder variables in the script with context values
func expandVariables(script string, context *HookContext) string {
	if context == nil {
		return script
	}

	replacements := map[string]string{
		"{dotfile}":     context.DotfileName,
		"{source}":      context.SourcePath,
		"{target}":      context.TargetPath,
		"{source_path}": context.SourcePath,
		"{target_path}": context.TargetPath,
	}

	result := script
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}
