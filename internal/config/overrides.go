package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// SetRecipeOverride modifies config.toml to set enable = true/false in
// [recipes_config.overrides.<recipeName>]. It preserves comments and formatting
// by operating on the raw text rather than marshaling/unmarshaling.
func SetRecipeOverride(configPath, recipeName string, enable bool) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}
	original := string(data)

	// Backup before modifying.
	bakPath := configPath + ".bak"
	if err := os.WriteFile(bakPath, data, 0o644); err != nil {
		return fmt.Errorf("creating backup: %w", err)
	}

	modified, err := applyOverride(original, recipeName, enable)
	if err != nil {
		return fmt.Errorf("applying override: %w", err)
	}

	if err := os.WriteFile(configPath, []byte(modified), 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Validate the modified file parses as valid TOML.
	var cfg Config
	if _, err := toml.Decode(modified, &cfg); err != nil {
		// Rollback: restore original content from backup.
		if rbErr := os.Rename(bakPath, configPath); rbErr != nil {
			return fmt.Errorf("TOML validation failed (%w) and rollback failed: %v", err, rbErr)
		}
		return fmt.Errorf("TOML validation failed, rolled back: %w", err)
	}

	// Clean up backup on success.
	_ = os.Remove(bakPath)
	return nil
}

// applyOverride modifies the raw TOML text to set the enable value for a
// recipe override section. It handles both bare and quoted key forms.
func applyOverride(content, recipeName string, enable bool) (string, error) {
	enableVal := fmt.Sprintf("%t", enable)

	// Build patterns for the section header. Match both quoted and bare keys:
	//   [recipes_config.overrides.foo]
	//   [recipes_config.overrides."foo"]
	bareHeader := fmt.Sprintf(`[recipes_config.overrides.%s]`, recipeName)
	quotedHeader := fmt.Sprintf(`[recipes_config.overrides."%s"]`, recipeName)

	// Find the section in the content.
	sectionStart := -1
	headerLine := ""
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == bareHeader || trimmed == quotedHeader {
			sectionStart = i
			headerLine = line
			break
		}
	}

	if sectionStart >= 0 {
		// Section exists — find and update or insert the enable line.
		return updateExistingSection(lines, sectionStart, headerLine, enableVal)
	}

	// Section doesn't exist — append it.
	return appendNewSection(content, recipeName, enableVal), nil
}

// sectionHeaderPattern matches any TOML table header: [something] or [[something]]
var sectionHeaderPattern = regexp.MustCompile(`^\s*\[{1,2}[^\[\]]+\]{1,2}\s*$`)

// updateExistingSection finds the enable line within an existing override
// section and updates it. If no enable line exists, one is inserted after
// the section header.
func updateExistingSection(lines []string, sectionStart int, _, enableVal string) (string, error) {
	// Determine the end of this section: next section header or EOF.
	sectionEnd := len(lines)
	for i := sectionStart + 1; i < len(lines); i++ {
		if sectionHeaderPattern.MatchString(lines[i]) {
			sectionEnd = i
			break
		}
	}

	// Look for an existing enable line within the section.
	enablePattern := regexp.MustCompile(`^(\s*)enable\s*=`)
	for i := sectionStart + 1; i < sectionEnd; i++ {
		if enablePattern.MatchString(lines[i]) {
			// Preserve leading whitespace.
			loc := enablePattern.FindStringSubmatchIndex(lines[i])
			indent := ""
			if loc != nil && loc[2] >= 0 {
				indent = lines[i][loc[2]:loc[3]]
			}
			lines[i] = fmt.Sprintf("%senable = %s", indent, enableVal)
			return strings.Join(lines, "\n"), nil
		}
	}

	// No enable line found in the section — insert one after the header.
	newLine := fmt.Sprintf("enable = %s", enableVal)
	updated := make([]string, 0, len(lines)+1)
	updated = append(updated, lines[:sectionStart+1]...)
	updated = append(updated, newLine)
	updated = append(updated, lines[sectionStart+1:]...)
	return strings.Join(updated, "\n"), nil
}

// appendNewSection appends a new [recipes_config.overrides.<name>] section
// at the end of the config file.
func appendNewSection(content, recipeName, enableVal string) string {
	// Ensure file ends with a newline before appending.
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	section := fmt.Sprintf("\n[recipes_config.overrides.%s]\nenable = %s\n", recipeName, enableVal)
	return content + section
}
