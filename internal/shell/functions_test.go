package shell

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/dotter/internal/config"
)

// Helper to create a temporary config for function/alias generation tests
func createTestConfigForShellGen() *config.Config {
	return &config.Config{
		Shell: config.ShellConfig{
			Aliases: map[string]config.ShellAlias{
				"ll":  {Command: "ls -alh"},
				"gcm": {Command: "git checkout master"},
			},
			Functions: map[string]config.ShellFunction{
				"myfunc": {
					Body: "echo \"Hello from myfunc $1\"",
				},
				"another": {
					Body: "echo \"Another one bites the $DUST\"\nls",
				},
			},
		},
	}
}

func TestGenerateShellConfigs_DryRun(t *testing.T) {
	cfg := createTestConfigForShellGen()
	tempDir := t.TempDir()

	// Point GetDotterGeneratedDir to our tempDir for this test
	originalGetDotterGeneratedDir := GetDotterGeneratedDir
	GetDotterGeneratedDir = func() (string, error) {
		return filepath.Join(tempDir, "dotter_generated_dry_run"), nil
	}
	defer func() { GetDotterGeneratedDir = originalGetDotterGeneratedDir }()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	aliasPath, funcPath, err := GenerateShellConfigs(cfg, Bash, true)

	w.Close()
	buf := new(strings.Builder)
	_, _ = io.Copy(buf, r) // Use io.Copy
	os.Stdout = oldStdout
	output := buf.String()

	if err != nil {
		t.Fatalf("GenerateShellConfigs (dry run) failed: %v", err)
	}

	generatedDirForTest, _ := GetDotterGeneratedDir() // Get the overridden path
	expectedAliasPath := filepath.Join(generatedDirForTest, GeneratedAliasesFilename)
	expectedFuncPath := filepath.Join(generatedDirForTest, GeneratedFunctionsFilename)

	if aliasPath != expectedAliasPath {
		t.Errorf("Dry run alias path mismatch. Got %s, want %s", aliasPath, expectedAliasPath)
	}
	if funcPath != expectedFuncPath {
		t.Errorf("Dry run func path mismatch. Got %s, want %s", funcPath, expectedFuncPath)
	}

	// Check that files were NOT created
	if _, statErr := os.Stat(aliasPath); !os.IsNotExist(statErr) {
		t.Errorf("Dry run created alias file %s", aliasPath)
	}
	if _, statErr := os.Stat(funcPath); !os.IsNotExist(statErr) {
		t.Errorf("Dry run created function file %s", funcPath)
	}

	if !strings.Contains(output, "[DRY RUN] Would write generated aliases") {
		t.Errorf("Expected dry run output for aliases, got: %s", output)
	}
	if !strings.Contains(output, "[DRY RUN] Would write generated functions") {
		t.Errorf("Expected dry run output for functions, got: %s", output)
	}
}

func TestGenerateShellConfigs_ActualWrite_Bash(t *testing.T) {
	cfg := createTestConfigForShellGen()
	tempDir := t.TempDir()

	originalGetDotterGeneratedDir := GetDotterGeneratedDir
	generatedDirForTest := filepath.Join(tempDir, "dotter_generated_actual_bash")
	GetDotterGeneratedDir = func() (string, error) { return generatedDirForTest, nil }
	defer func() { GetDotterGeneratedDir = originalGetDotterGeneratedDir }()

	aliasPath, funcPath, err := GenerateShellConfigs(cfg, Bash, false)
	if err != nil {
		t.Fatalf("GenerateShellConfigs (Bash) failed: %v", err)
	}

	// Verify alias file content
	aliasContent, _ := os.ReadFile(aliasPath)
	// Expected aliases sorted alphabetically: gcm, ll
	expectedAliasContentBash := `#!/bin/sh
# Dotter generated aliases - DO NOT EDIT MANUALLY

alias gcm='git checkout master'
alias ll='ls -alh'
`
	if string(aliasContent) != expectedAliasContentBash {
		t.Errorf("Bash alias file content mismatch.\nGot:\n%s\nWant:\n%s", string(aliasContent), expectedAliasContentBash)
	}

	// Verify function file content (Bash/POSIX)
	funcContent, _ := os.ReadFile(funcPath)
	// Expected functions sorted alphabetically: another, myfunc
	expectedFuncContentBash := `#!/bin/sh
# Dotter generated functions - DO NOT EDIT MANUALLY

another() {
echo "Another one bites the $DUST"
ls
}

myfunc() {
echo "Hello from myfunc $1"
}

`
	if string(funcContent) != expectedFuncContentBash {
		t.Errorf("Bash function file content mismatch.\nGot:\n%s\nWant:\n%s", string(funcContent), expectedFuncContentBash)
	}
}

func TestGenerateShellConfigs_ActualWrite_Fish(t *testing.T) {
	cfg := createTestConfigForShellGen()
	tempDir := t.TempDir()

	originalGetDotterGeneratedDir := GetDotterGeneratedDir
	generatedDirForTest := filepath.Join(tempDir, "dotter_generated_actual_fish")
	GetDotterGeneratedDir = func() (string, error) { return generatedDirForTest, nil }
	defer func() { GetDotterGeneratedDir = originalGetDotterGeneratedDir }()

	aliasPath, funcPath, err := GenerateShellConfigs(cfg, Fish, false)
	if err != nil {
		t.Fatalf("GenerateShellConfigs (Fish) failed: %v", err)
	}

	// Alias content should be the same for Fish as it's sourced by sh-compatible `alias`
	aliasContent, _ := os.ReadFile(aliasPath)
	// Expected aliases sorted alphabetically: gcm, ll
	expectedAliasContentFish := `#!/bin/sh
# Dotter generated aliases - DO NOT EDIT MANUALLY

alias gcm='git checkout master'
alias ll='ls -alh'
`
	if string(aliasContent) != expectedAliasContentFish {
		t.Errorf("Fish alias file content mismatch.\nGot:\n%s\nWant:\n%s", string(aliasContent), expectedAliasContentFish)
	}

	// Verify function file content (Fish)
	funcContent, _ := os.ReadFile(funcPath)
	// Expected functions sorted alphabetically: another, myfunc
	expectedFuncContentFish := `#!/bin/sh
# Dotter generated functions - DO NOT EDIT MANUALLY

function another
  echo "Another one bites the $DUST"
ls
end

function myfunc
  echo "Hello from myfunc $1"
end

`
	if string(funcContent) != expectedFuncContentFish {
		t.Errorf("Fish function file content mismatch.\nGot:\n%s\nWant:\n%s", string(funcContent), expectedFuncContentFish)
	}
}

func TestGenerateShellConfigs_NoAliasesOrFunctions(t *testing.T) {
	cfg := &config.Config{} // Empty config
	tempDir := t.TempDir()

	originalGetDotterGeneratedDir := GetDotterGeneratedDir
	generatedDirForTest := filepath.Join(tempDir, "dotter_generated_empty")
	GetDotterGeneratedDir = func() (string, error) { return generatedDirForTest, nil }
	defer func() { GetDotterGeneratedDir = originalGetDotterGeneratedDir }()

	aliasPath, funcPath, err := GenerateShellConfigs(cfg, Bash, false)
	if err != nil {
		t.Fatalf("GenerateShellConfigs (empty) failed: %v", err)
	}

	if aliasPath != "" {
		t.Errorf("Expected empty aliasPath when no aliases, got %s", aliasPath)
	}
	if funcPath != "" {
		t.Errorf("Expected empty funcPath when no functions, got %s", funcPath)
	}

	// Check that files were NOT created (or were removed if they existed from a previous run)
	// The implementation removes them, so we check for IsNotExist
	aliasDiskPath := filepath.Join(generatedDirForTest, GeneratedAliasesFilename)
	funcDiskPath := filepath.Join(generatedDirForTest, GeneratedFunctionsFilename)

	if _, statErr := os.Stat(aliasDiskPath); !os.IsNotExist(statErr) {
		t.Errorf("Alias file %s exists when it should not (no aliases configured)", aliasDiskPath)
	}
	if _, statErr := os.Stat(funcDiskPath); !os.IsNotExist(statErr) {
		t.Errorf("Function file %s exists when it should not (no functions configured)", funcDiskPath)
	}
}
