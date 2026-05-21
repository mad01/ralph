package shell

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
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

	// Point GetRalphGeneratedDir to our tempDir for this test
	originalGetRalphGeneratedDir := GetRalphGeneratedDir
	GetRalphGeneratedDir = func() (string, error) {
		return filepath.Join(tempDir, "ralph_generated_dry_run"), nil
	}
	defer func() { GetRalphGeneratedDir = originalGetRalphGeneratedDir }()

	var buf bytes.Buffer
	aliasPath, funcPath, err := GenerateShellConfigs(&buf, cfg, Bash, true)
	output := buf.String()

	if err != nil {
		t.Fatalf("GenerateShellConfigs (dry run) failed: %v", err)
	}

	generatedDirForTest, _ := GetRalphGeneratedDir() // Get the overridden path
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

	originalGetRalphGeneratedDir := GetRalphGeneratedDir
	generatedDirForTest := filepath.Join(tempDir, "ralph_generated_actual_bash")
	GetRalphGeneratedDir = func() (string, error) { return generatedDirForTest, nil }
	defer func() { GetRalphGeneratedDir = originalGetRalphGeneratedDir }()

	aliasPath, funcPath, err := GenerateShellConfigs(io.Discard, cfg, Bash, false)
	if err != nil {
		t.Fatalf("GenerateShellConfigs (Bash) failed: %v", err)
	}

	// Verify alias file content
	aliasContent, _ := os.ReadFile(aliasPath)
	// Expected aliases sorted alphabetically: gcm, ll
	expectedAliasContentBash := `#!/bin/sh
# Ralph generated aliases - DO NOT EDIT MANUALLY

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
# Ralph generated functions - DO NOT EDIT MANUALLY

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

	originalGetRalphGeneratedDir := GetRalphGeneratedDir
	generatedDirForTest := filepath.Join(tempDir, "ralph_generated_actual_fish")
	GetRalphGeneratedDir = func() (string, error) { return generatedDirForTest, nil }
	defer func() { GetRalphGeneratedDir = originalGetRalphGeneratedDir }()

	aliasPath, funcPath, err := GenerateShellConfigs(io.Discard, cfg, Fish, false)
	if err != nil {
		t.Fatalf("GenerateShellConfigs (Fish) failed: %v", err)
	}

	// Alias content should be the same for Fish as it's sourced by sh-compatible `alias`
	aliasContent, _ := os.ReadFile(aliasPath)
	// Expected aliases sorted alphabetically: gcm, ll
	expectedAliasContentFish := `#!/bin/sh
# Ralph generated aliases - DO NOT EDIT MANUALLY

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
# Ralph generated functions - DO NOT EDIT MANUALLY

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

	originalGetRalphGeneratedDir := GetRalphGeneratedDir
	generatedDirForTest := filepath.Join(tempDir, "ralph_generated_empty")
	GetRalphGeneratedDir = func() (string, error) { return generatedDirForTest, nil }
	defer func() { GetRalphGeneratedDir = originalGetRalphGeneratedDir }()

	aliasPath, funcPath, err := GenerateShellConfigs(io.Discard, cfg, Bash, false)
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

func TestGenerateEnvFile_DryRun(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, GeneratedEnvFilename)

	envVars := map[string]string{
		"FOO": "bar",
		"BAZ": "qux quux",
	}

	var buf bytes.Buffer
	err := GenerateEnvFile(&buf, envVars, outputPath, true)
	if err != nil {
		t.Fatalf("GenerateEnvFile (dry run) failed: %v", err)
	}

	// File must not be created in dry run.
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Errorf("Dry run created env file %s", outputPath)
	}

	output := buf.String()
	if !strings.Contains(output, "[DRY RUN] Would write generated env vars to:") {
		t.Errorf("Expected dry run output for env vars, got: %s", output)
	}
}

func TestGenerateEnvFile_ActualWrite(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, GeneratedEnvFilename)

	envVars := map[string]string{
		"PATH_EXTRA": "/usr/local/bin",
		"EDITOR":     "vim",
		"MY_VAR":     `has "quotes" inside`,
	}

	var buf bytes.Buffer
	err := GenerateEnvFile(&buf, envVars, outputPath, false)
	if err != nil {
		t.Fatalf("GenerateEnvFile failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Could not read generated env file: %v", err)
	}

	// Keys must be sorted: EDITOR, MY_VAR, PATH_EXTRA
	expected := "#!/bin/sh\n" +
		"# Ralph generated environment variables - DO NOT EDIT MANUALLY\n\n" +
		`export EDITOR="vim"` + "\n" +
		`export MY_VAR="has \"quotes\" inside"` + "\n" +
		`export PATH_EXTRA="/usr/local/bin"` + "\n"

	if string(content) != expected {
		t.Errorf("Env file content mismatch.\nGot:\n%s\nWant:\n%s", string(content), expected)
	}

	if !strings.Contains(buf.String(), "Generated env vars at:") {
		t.Errorf("Expected success message in output, got: %s", buf.String())
	}
}

func TestGenerateEnvFile_EmptyMap_RemovesExistingFile(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, GeneratedEnvFilename)

	// Pre-create the file to simulate a previous run.
	if err := os.WriteFile(outputPath, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}

	err := GenerateEnvFile(io.Discard, map[string]string{}, outputPath, false)
	if err != nil {
		t.Fatalf("GenerateEnvFile (empty) failed: %v", err)
	}

	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Errorf("Expected env file to be removed when envVars is empty, but file still exists at %s", outputPath)
	}
}

func TestGenerateEnvFile_EmptyMap_DryRun_NoRemoval(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, GeneratedEnvFilename)

	// Pre-create the file.
	if err := os.WriteFile(outputPath, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}

	err := GenerateEnvFile(io.Discard, map[string]string{}, outputPath, true)
	if err != nil {
		t.Fatalf("GenerateEnvFile (empty, dry run) failed: %v", err)
	}

	// In dry run, the existing file must remain untouched.
	if _, statErr := os.Stat(outputPath); os.IsNotExist(statErr) {
		t.Errorf("Dry run removed existing env file %s; it should not have been removed", outputPath)
	}
}

func TestGetEnvFilePath(t *testing.T) {
	tempDir := t.TempDir()

	originalGetRalphGeneratedDir := GetRalphGeneratedDir
	GetRalphGeneratedDir = func() (string, error) { return filepath.Join(tempDir, "generated"), nil }
	defer func() { GetRalphGeneratedDir = originalGetRalphGeneratedDir }()

	path, err := GetEnvFilePath()
	if err != nil {
		t.Fatalf("GetEnvFilePath failed: %v", err)
	}

	expected := filepath.Join(tempDir, "generated", GeneratedEnvFilename)
	if path != expected {
		t.Errorf("GetEnvFilePath returned %s, want %s", path, expected)
	}
}
