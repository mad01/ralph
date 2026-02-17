package dotfile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

// createTempTemplateFile is a helper for tests
func createTempTemplateFile(t *testing.T, name string, content string) string {
	t.Helper()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, name)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp template file %s: %v", filePath, err)
	}
	return filePath
}

func TestProcessTemplate_Basic(t *testing.T) {
	os.Setenv("TEST_ENV_VAR", "env_value_for_template")
	defer os.Unsetenv("TEST_ENV_VAR")

	cfg := &config.Config{
		DotfilesRepoPath: "/fake/repo",
		TemplateVariables: map[string]interface{}{
			"ConfigVar": "config_value",
		},
	}

	customData := map[string]interface{}{
		"CustomVar": "custom_data_value",
	}

	templateContent := "Env: {{ env \"TEST_ENV_VAR\" }} | Config: {{ .ConfigVar }} | Custom: {{ .CustomVar }} | Repo: {{ .RalphConfig.DotfilesRepoPath }}"
	templatePath := createTempTemplateFile(t, "test.tmpl", templateContent)

	processed, err := ProcessTemplate(templatePath, cfg, customData)
	if err != nil {
		t.Fatalf("ProcessTemplate failed: %v", err)
	}

	expected := "Env: env_value_for_template | Config: config_value | Custom: custom_data_value | Repo: /fake/repo"
	if string(processed) != expected {
		t.Errorf("ProcessTemplate output mismatch:\nGot:  %s\nWant: %s", string(processed), expected)
	}
}

func TestProcessTemplate_SourceFileDoesNotExist(t *testing.T) {
	_, err := ProcessTemplate("non_existent_template.tmpl", nil, nil)
	if err == nil {
		t.Errorf("ProcessTemplate did not return error for non-existent source file")
	} else {
		if !strings.Contains(err.Error(), "failed to read template file") {
			t.Errorf("Error message '%s' did not contain expected phrase", err.Error())
		}
	}
}

func TestProcessTemplate_MalformedTemplate(t *testing.T) {
	templatePath := createTempTemplateFile(t, "malformed.tmpl", "Hello {{ .UndefinedVar }") // Missing closing curlies
	_, err := ProcessTemplate(templatePath, &config.Config{}, nil)
	if err == nil {
		t.Errorf("ProcessTemplate did not return error for malformed template")
	} else {
		if !strings.Contains(err.Error(), "failed to parse template") {
			t.Errorf("Error message '%s' did not contain expected phrase for parse error", err.Error())
		}
	}
}

func TestWriteProcessedTemplateToFile_DryRun(t *testing.T) {
	templateContent := "Dry run test: {{ env \"USER\" }}"
	templatePath := createTempTemplateFile(t, "dryrun.tmpl", templateContent)

	processedFilePath, err := WriteProcessedTemplateToFile(io.Discard, templatePath, &config.Config{}, nil, true)

	if err != nil {
		t.Fatalf("WriteProcessedTemplateToFile (dry run) failed: %v", err)
	}

	// Check if a placeholder path is returned
	if !strings.Contains(processedFilePath, "ralph_dry_run_processed_template") {
		t.Errorf("Expected dry run to return a placeholder path, got '%s'", processedFilePath)
	}

	// Check that the file was NOT actually created
	if _, statErr := os.Stat(processedFilePath); !os.IsNotExist(statErr) {
		t.Errorf("Dry run created a file at placeholder path '%s' when it should not have", processedFilePath)
	}
}

func TestWriteProcessedTemplateToFile_ActualWrite(t *testing.T) {
	userName := os.Getenv("USER")
	if userName == "" {
		userName = "testuser" // Fallback if USER env var isn't set in test environment
		os.Setenv("USER", userName)
		defer os.Unsetenv("USER")
	}
	templateContent := fmt.Sprintf("Actual write test. User: {{ env \"USER\" }}")
	templatePath := createTempTemplateFile(t, "actual.tmpl", templateContent)

	cfg := &config.Config{TemplateVariables: map[string]interface{}{"TestVar": "Hello"}}

	processedFilePath, err := WriteProcessedTemplateToFile(io.Discard, templatePath, cfg, nil, false)
	if err != nil {
		t.Fatalf("WriteProcessedTemplateToFile failed: %v", err)
	}
	defer os.Remove(processedFilePath)                  // Clean up the created temp file
	defer os.RemoveAll(filepath.Dir(processedFilePath)) // Clean up the ralph/processed_templates dir

	// Check if file exists
	if _, statErr := os.Stat(processedFilePath); os.IsNotExist(statErr) {
		t.Fatalf("WriteProcessedTemplateToFile did not create the processed file at %s", processedFilePath)
	}

	// Check content
	contentBytes, readErr := os.ReadFile(processedFilePath)
	if readErr != nil {
		t.Fatalf("Failed to read processed file %s: %v", processedFilePath, readErr)
	}

	expectedContent := fmt.Sprintf("Actual write test. User: %s", userName)
	if string(contentBytes) != expectedContent {
		t.Errorf("Processed file content mismatch:\nGot:  %s\nWant: %s", string(contentBytes), expectedContent)
	}

	// Check if it's in the expected temp subdirectory
	if !strings.Contains(processedFilePath, filepath.Join("ralph", "processed_templates")) {
		t.Errorf("Processed file '%s' not in expected temp subdirectory structure", processedFilePath)
	}
}
