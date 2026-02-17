package dotfile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/mad01/ralph/internal/config"
)

// ProcessTemplate takes a source file path, processes it as a Go template,
// and returns the processed content as a byte slice.
// It uses data from the ralphConfig and environment variables for templating.
func ProcessTemplate(sourcePath string, ralphConfig *config.Config, templateData map[string]interface{}) ([]byte, error) {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file '%s': %w", sourcePath, err)
	}

	tmpl, err := template.New(filepath.Base(sourcePath)).
		Funcs(template.FuncMap{ // Add any custom template functions here if needed
			"env": os.Getenv,
		}).
		Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template '%s': %w", sourcePath, err)
	}

	// Prepare data for the template
	data := make(map[string]interface{})

	// Add ralph config if available - provides access to global config like DotfilesRepoPath
	if ralphConfig != nil {
		data["RalphConfig"] = ralphConfig
		data["DotterConfig"] = ralphConfig // backward compatibility
		// Merge variables from config.TemplateVariables
		// These will override any same-named keys from other sources if any (none yet)
		for k, v := range ralphConfig.TemplateVariables {
			data[k] = v
		}
	}

	// Add custom data passed in templateData (e.g. from command line flags in future, or per-dotfile variables)
	// These could override general TemplateVariables if names clash.
	for k, v := range templateData {
		data[k] = v
	}

	var processedContent bytes.Buffer
	if err := tmpl.Execute(&processedContent, data); err != nil {
		return nil, fmt.Errorf("failed to execute template '%s': %w", sourcePath, err)
	}

	return processedContent.Bytes(), nil
}

// WriteProcessedTemplateToFile handles processing a template and writing it to a temporary file.
// This temp file can then be symlinked.
// Returns the path to the temporary processed file.
// If dryRun is true, it processes the template (to catch errors) but does not write the file,
// and returns a placeholder path.
func WriteProcessedTemplateToFile(sourcePath string, ralphConfig *config.Config, templateData map[string]interface{}, dryRun bool) (string, error) {
	processedBytes, err := ProcessTemplate(sourcePath, ralphConfig, templateData)
	if err != nil {
		return "", err // Error in processing is an error regardless of dryRun
	}

	if dryRun {
		fmt.Printf("[DRY RUN] Would write processed template for '%s' to a temporary file.\n", sourcePath)
		// Return a fake path for dry run symlinking to use
		return filepath.Join(os.TempDir(), "ralph_dry_run_processed_template", filepath.Base(sourcePath)+".processed"), nil
	}

	// Create a temporary file to store the processed template
	// It's good practice to put these in a ralph-specific temp location
	tempDir := filepath.Join(os.TempDir(), "ralph", "processed_templates")
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create temp directory for processed templates: %w", err)
	}

	tempFile, err := os.CreateTemp(tempDir, filepath.Base(sourcePath)+".*.processed")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file for processed template: %w", err)
	}
	defer tempFile.Close()

	_, err = tempFile.Write(processedBytes)
	if err != nil {
		return "", fmt.Errorf("failed to write processed template to temporary file: %w", err)
	}

	return tempFile.Name(), nil
}
