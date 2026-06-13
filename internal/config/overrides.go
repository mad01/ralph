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

	// Backup before modifying. Refuse if a stale .bak exists from a prior
	// failed run — overwriting it could destroy the last known-good config.
	bakPath := configPath + ".bak"
	if _, err := os.Stat(bakPath); err == nil {
		return fmt.Errorf(
			"backup file %s exists from a prior run; if %s looks correct, remove the .bak; otherwise restore with: mv %s %s",
			bakPath,
			configPath,
			bakPath,
			configPath,
		)
	}
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

// bareTOMLKeyPattern matches TOML bare keys (letters, digits, '_' and '-').
var bareTOMLKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// tomlHeaderKey renders a recipe name as a TOML key segment, quoting it when it
// isn't a valid bare key. Without this, a name containing a dot (e.g.
// "web.tools") would be written as nested tables and the override would be
// unreadable.
func tomlHeaderKey(name string) string {
	if bareTOMLKeyPattern.MatchString(name) {
		return name
	}
	return `"` + strings.ReplaceAll(name, `"`, `\"`) + `"`
}

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

// RemoveRecipeOverride removes the enable line (or entire section) for a recipe
// override in config.toml. If the section contains only an enable line, the
// entire section (header + contents) is removed. If other fields exist, only
// the enable line is removed. If no section exists, it's a no-op.
func RemoveRecipeOverride(configPath, recipeName string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}
	original := string(data)

	// Backup before modifying. Refuse if a stale .bak exists from a prior
	// failed run — overwriting it could destroy the last known-good config.
	bakPath := configPath + ".bak"
	if _, err := os.Stat(bakPath); err == nil {
		return fmt.Errorf(
			"backup file %s exists from a prior run; if %s looks correct, remove the .bak; otherwise restore with: mv %s %s",
			bakPath,
			configPath,
			bakPath,
			configPath,
		)
	}
	if err := os.WriteFile(bakPath, data, 0o644); err != nil {
		return fmt.Errorf("creating backup: %w", err)
	}

	modified := removeOverride(original, recipeName)

	if modified == original {
		// Nothing changed — clean up backup and return.
		_ = os.Remove(bakPath)
		return nil
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

// removeOverride removes the enable line or entire override section from raw
// TOML text. Returns the original content unchanged if no section is found.
func removeOverride(content, recipeName string) string {
	bareHeader := fmt.Sprintf(`[recipes_config.overrides.%s]`, recipeName)
	quotedHeader := fmt.Sprintf(`[recipes_config.overrides."%s"]`, recipeName)

	lines := strings.Split(content, "\n")

	// Find the section.
	sectionStart := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == bareHeader || trimmed == quotedHeader {
			sectionStart = i
			break
		}
	}

	if sectionStart < 0 {
		return content
	}

	// Find the end of this section (next section header or EOF).
	sectionEnd := len(lines)
	for i := sectionStart + 1; i < len(lines); i++ {
		if sectionHeaderPattern.MatchString(lines[i]) {
			sectionEnd = i
			break
		}
	}

	// Collect non-blank, non-comment content lines within the section body.
	enablePattern := regexp.MustCompile(`^\s*enable\s*=`)
	enableLineIdx := -1
	hasOtherFields := false

	for i := sectionStart + 1; i < sectionEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if enablePattern.MatchString(lines[i]) {
			enableLineIdx = i
		} else {
			hasOtherFields = true
		}
	}

	if enableLineIdx < 0 {
		// No enable line in the section — nothing to remove.
		return content
	}

	if hasOtherFields {
		// Other fields exist — only remove the enable line.
		result := make([]string, 0, len(lines)-1)
		result = append(result, lines[:enableLineIdx]...)
		result = append(result, lines[enableLineIdx+1:]...)
		return strings.Join(result, "\n")
	}

	// Section contains only enable (plus blanks/comments) — remove the entire
	// section. Also consume any leading blank lines above the header that
	// separate it from prior content.
	removeStart := sectionStart
	for removeStart > 0 && strings.TrimSpace(lines[removeStart-1]) == "" {
		removeStart--
	}

	result := make([]string, 0, len(lines))
	result = append(result, lines[:removeStart]...)
	result = append(result, lines[sectionEnd:]...)
	return strings.Join(result, "\n")
}

// appendNewSection appends a new [recipes_config.overrides.<name>] section
// at the end of the config file.
func appendNewSection(content, recipeName, enableVal string) string {
	// Ensure file ends with a newline before appending.
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	section := fmt.Sprintf(
		"\n[recipes_config.overrides.%s]\nenable = %s\n",
		tomlHeaderKey(recipeName),
		enableVal,
	)
	return content + section
}
