package hooks

import (
	"io"
	"testing"
)

// --- Tests for expandVariables ---

func TestExpandVariables_AllPlaceholders(t *testing.T) {
	context := &HookContext{
		DotfileName: "bashrc",
		SourcePath:  "/home/user/dotfiles/.bashrc",
		TargetPath:  "/home/user/.bashrc",
	}

	script := "echo {dotfile} from {source} to {target}"
	result := expandVariables(script, context)
	expected := "echo bashrc from /home/user/dotfiles/.bashrc to /home/user/.bashrc"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExpandVariables_SourcePathTargetPath(t *testing.T) {
	context := &HookContext{
		SourcePath: "/src/path",
		TargetPath: "/tgt/path",
	}

	script := "cp {source_path} {target_path}"
	result := expandVariables(script, context)
	expected := "cp /src/path /tgt/path"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExpandVariables_NilContext(t *testing.T) {
	script := "echo {dotfile}"
	result := expandVariables(script, nil)

	// With nil context, should return unchanged
	if result != script {
		t.Errorf("expected unchanged script %q, got %q", script, result)
	}
}

func TestExpandVariables_EmptyString(t *testing.T) {
	context := &HookContext{
		DotfileName: "test",
	}

	result := expandVariables("", context)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestExpandVariables_NoPlaceholders(t *testing.T) {
	context := &HookContext{
		DotfileName: "test",
	}

	script := "echo hello world"
	result := expandVariables(script, context)

	if result != script {
		t.Errorf("expected unchanged %q, got %q", script, result)
	}
}

func TestExpandVariables_MultipleSamePlaceholder(t *testing.T) {
	context := &HookContext{
		DotfileName: "vimrc",
	}

	script := "echo {dotfile} and again {dotfile}"
	result := expandVariables(script, context)
	expected := "echo vimrc and again vimrc"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExpandVariables_EmptyContextValues(t *testing.T) {
	context := &HookContext{
		DotfileName: "",
		SourcePath:  "",
		TargetPath:  "",
	}

	script := "echo {dotfile}"
	result := expandVariables(script, context)
	expected := "echo "

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// --- Tests for Run ---

func TestRun_DryRunDoesNotExecute(t *testing.T) {
	// Dry run should not actually execute the command
	err := Run(io.Discard, "echo test", &HookContext{}, true)
	if err != nil {
		t.Errorf("expected no error in dry run, got: %v", err)
	}
}

func TestRun_EmptyCommand(t *testing.T) {
	err := Run(io.Discard, "", &HookContext{}, false)
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestRun_WhitespaceOnlyCommand(t *testing.T) {
	err := Run(io.Discard, "   ", &HookContext{}, false)
	if err == nil {
		t.Error("expected error for whitespace-only command")
	}
}

func TestRun_SimpleCommand(t *testing.T) {
	// Test that a simple command runs successfully
	err := Run(io.Discard, "true", nil, false)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestRun_FailingCommand(t *testing.T) {
	err := Run(io.Discard, "false", nil, false)
	if err == nil {
		t.Error("expected error for failing command")
	}
}

func TestRun_CommandWithArguments(t *testing.T) {
	err := Run(io.Discard, "test -d /tmp", nil, false)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestRun_VariableExpansion(t *testing.T) {
	context := &HookContext{
		DotfileName: "test_file",
	}
	// Use a command that will succeed if the variable is expanded
	err := Run(io.Discard, "test {dotfile} = test_file", context, false)
	if err != nil {
		t.Errorf("expected variable expansion to work, got: %v", err)
	}
}

// --- Tests for RunHooks ---

func TestRunHooks_EmptyScripts(t *testing.T) {
	err := RunHooks(io.Discard, nil, PreApply, &HookContext{}, false)
	if err != nil {
		t.Errorf("expected no error for nil scripts, got: %v", err)
	}

	err = RunHooks(io.Discard, []string{}, PostApply, &HookContext{}, false)
	if err != nil {
		t.Errorf("expected no error for empty scripts, got: %v", err)
	}
}

func TestRunHooks_SingleScript(t *testing.T) {
	scripts := []string{"true"}
	err := RunHooks(io.Discard, scripts, PreApply, &HookContext{}, false)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestRunHooks_MultipleScripts(t *testing.T) {
	scripts := []string{"true", "true", "true"}
	err := RunHooks(io.Discard, scripts, PostApply, &HookContext{}, false)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestRunHooks_StopsOnFirstFailure(t *testing.T) {
	// Second script fails - should stop there
	scripts := []string{"true", "false", "true"}
	err := RunHooks(io.Discard, scripts, PreLink, &HookContext{}, false)
	if err == nil {
		t.Error("expected error when script fails")
	}
}

func TestRunHooks_DryRun(t *testing.T) {
	// With dry run, even a failing command shouldn't error
	scripts := []string{"false"}
	err := RunHooks(io.Discard, scripts, PostLink, &HookContext{}, true)
	if err != nil {
		t.Errorf("expected no error in dry run mode, got: %v", err)
	}
}

func TestRunHooks_HookTypePreApply(t *testing.T) {
	err := RunHooks(io.Discard, []string{"true"}, PreApply, nil, false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunHooks_HookTypePostApply(t *testing.T) {
	err := RunHooks(io.Discard, []string{"true"}, PostApply, nil, false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunHooks_HookTypePreLink(t *testing.T) {
	err := RunHooks(io.Discard, []string{"true"}, PreLink, nil, false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunHooks_HookTypePostLink(t *testing.T) {
	err := RunHooks(io.Discard, []string{"true"}, PostLink, nil, false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
