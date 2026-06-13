package shell

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mad01/ralph/internal/config"
)

const (
	GeneratedAliasesFilename   = "generated_aliases.sh"
	GeneratedFunctionsFilename = "generated_functions.sh"
	GeneratedEnvFilename       = "generated_env.sh"
)

// GenerateShellConfigs generates script files for aliases and functions
// and returns the paths to the generated files and any errors.
// If dryRun is true, it prints what it would do and returns the prospective paths,
// but does not write any files.
func GenerateShellConfigs(
	w io.Writer,
	cfg *config.Config,
	shellType SupportedShell,
	dryRun bool,
) (aliasFilePath string, funcFilePath string, err error) {
	currentHost := config.GetCurrentHost()
	generatedDir, err := GetRalphGeneratedDir()
	if err != nil {
		return "", "", fmt.Errorf("failed to get ralph generated scripts directory: %w", err)
	}
	if !dryRun {
		if err := os.MkdirAll(generatedDir, 0o755); err != nil {
			return "", "", fmt.Errorf(
				"failed to create directory for generated shell scripts '%s': %w",
				generatedDir,
				err,
			)
		}
	} else {
		if _, statErr := os.Stat(generatedDir); os.IsNotExist(statErr) {
			fmt.Fprintf(w, "[DRY RUN] Would create directory for generated shell scripts: %s\n", generatedDir)
		}
	}

	aliasFilePath = filepath.Join(generatedDir, GeneratedAliasesFilename)
	funcFilePath = filepath.Join(generatedDir, GeneratedFunctionsFilename)

	// Generate Aliases - filter by enable and host
	filteredAliases := make(map[string]config.ShellAlias)
	for name, alias := range cfg.Shell.Aliases {
		if config.IsEnabled(alias.Enable) && config.ShouldApplyForHost(alias.Hosts, currentHost) {
			filteredAliases[name] = alias
		}
	}

	if len(filteredAliases) > 0 {
		var aliasContent strings.Builder
		aliasContent.WriteString("#!/bin/sh\n")
		aliasContent.WriteString("# Ralph generated aliases - DO NOT EDIT MANUALLY\n\n")

		// Sort alias names for consistent output
		aliasNames := make([]string, 0, len(filteredAliases))
		for name := range filteredAliases {
			aliasNames = append(aliasNames, name)
		}
		sort.Strings(aliasNames)

		for _, name := range aliasNames { // Iterate over sorted names
			alias := filteredAliases[name]
			// Basic sanitization for alias name and command could be added here if necessary
			fmt.Fprintf(
				&aliasContent,
				"alias %s='%s'\n",
				name,
				strings.ReplaceAll(alias.Command, "'", "'\\''"),
			)
		}

		if dryRun {
			fmt.Fprintf(w, "[DRY RUN] Would write generated aliases to: %s\n", aliasFilePath)
		} else {
			if err := os.WriteFile(aliasFilePath, []byte(aliasContent.String()), 0o644); err != nil {
				return aliasFilePath, "", fmt.Errorf("failed to write generated aliases file '%s': %w", aliasFilePath, err)
			}
			fmt.Fprintf(w, "Generated aliases at: %s\n", aliasFilePath)
		}
	} else {
		if !dryRun { // Only attempt removal if not in dry run
			if _, err := os.Stat(aliasFilePath); err == nil { // Check if file exists before removing
				if err := os.Remove(aliasFilePath); err != nil {
					log.Printf("Warning: could not remove existing empty alias file %s: %v\n", aliasFilePath, err)
				}
			}
		}
		aliasFilePath = "" // Indicate no file generated
	}

	// Generate Functions - filter by enable and host
	filteredFunctions := make(map[string]config.ShellFunction)
	for name, function := range cfg.Shell.Functions {
		if config.IsEnabled(function.Enable) &&
			config.ShouldApplyForHost(function.Hosts, currentHost) {
			filteredFunctions[name] = function
		}
	}

	if len(filteredFunctions) > 0 {
		var funcContent strings.Builder
		funcContent.WriteString(
			"#!/bin/sh\n",
		) // Or make this dependent on shellType for more complex functions
		funcContent.WriteString("# Ralph generated functions - DO NOT EDIT MANUALLY\n\n")

		// Sort function names for consistent output
		funcNames := make([]string, 0, len(filteredFunctions))
		for name := range filteredFunctions {
			funcNames = append(funcNames, name)
		}
		sort.Strings(funcNames)

		for _, name := range funcNames { // Iterate over sorted names
			function := filteredFunctions[name]
			// For POSIX shells, function syntax is: func_name() { body }
			// Fish shell syntax is different: function func_name; body; end;
			// For now, sticking to POSIX sh compatible.
			if shellType == Fish {
				fmt.Fprintf(
					&funcContent,
					"function %s\n  %s\nend\n\n",
					name,
					strings.TrimSpace(function.Body),
				)
			} else {
				fmt.Fprintf(&funcContent, "%s() {\n%s\n}\n\n", name, strings.TrimSpace(function.Body))
			}
		}

		if dryRun {
			fmt.Fprintf(w, "[DRY RUN] Would write generated functions to: %s\n", funcFilePath)
		} else {
			if err := os.WriteFile(funcFilePath, []byte(funcContent.String()), 0o644); err != nil {
				return aliasFilePath, funcFilePath, fmt.Errorf("failed to write generated functions file '%s': %w", funcFilePath, err)
			}
			fmt.Fprintf(w, "Generated functions at: %s\n", funcFilePath)
		}
	} else {
		if !dryRun { // Only attempt removal if not in dry run
			if _, err := os.Stat(funcFilePath); err == nil { // Check if file exists before removing
				if err := os.Remove(funcFilePath); err != nil {
					log.Printf("Warning: could not remove existing empty function file %s: %v\n", funcFilePath, err)
				}
			}
		}
		funcFilePath = "" // Indicate no file generated
	}

	return aliasFilePath, funcFilePath, nil
}

// GetEnvFilePath returns the canonical path for the generated env file.
// The env file should be sourced BEFORE generated_aliases.sh and generated_functions.sh
// because aliases and functions may reference env vars defined in it.
func GetEnvFilePath() (string, error) {
	generatedDir, err := GetRalphGeneratedDir()
	if err != nil {
		return "", fmt.Errorf("failed to get ralph generated scripts directory: %w", err)
	}
	return filepath.Join(generatedDir, GeneratedEnvFilename), nil
}

// GenerateEnvFile writes a shell script exporting the given environment variables
// to outputPath. Keys are sorted alphabetically for deterministic output.
// Values are single-quoted so shell metacharacters ($(...), backticks, $VAR, \)
// are taken literally rather than executed/expanded when the file is sourced.
// If envVars is empty, any existing file at outputPath is removed and no file is written.
// If dryRun is true, the function prints what it would do without modifying any files.
func GenerateEnvFile(w io.Writer, envVars map[string]string, outputPath string, dryRun bool) error {
	if len(envVars) == 0 {
		if !dryRun {
			if _, err := os.Stat(outputPath); err == nil {
				if err := os.Remove(outputPath); err != nil {
					log.Printf(
						"Warning: could not remove existing empty env file %s: %v\n",
						outputPath,
						err,
					)
				}
			}
		}
		return nil
	}

	// Sort keys alphabetically for consistent output.
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var content strings.Builder
	content.WriteString("#!/bin/sh\n")
	content.WriteString("# Ralph generated environment variables - DO NOT EDIT MANUALLY\n\n")
	for _, k := range keys {
		// Single-quote the value so $(...), backticks, $VAR, and \ are taken
		// literally and never executed/expanded when the file is sourced.
		// Embedded single quotes are escaped via the '\'' idiom.
		escaped := strings.ReplaceAll(envVars[k], "'", `'\''`)
		fmt.Fprintf(&content, "export %s='%s'\n", k, escaped)
	}

	if dryRun {
		fmt.Fprintf(w, "[DRY RUN] Would write generated env vars to: %s\n", outputPath)
		return nil
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for env file '%s': %w", outputPath, err)
	}

	if err := os.WriteFile(outputPath, []byte(content.String()), 0o644); err != nil {
		return fmt.Errorf("failed to write generated env file '%s': %w", outputPath, err)
	}
	fmt.Fprintf(w, "Generated env vars at: %s\n", outputPath)
	return nil
}
