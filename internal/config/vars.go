package config

import (
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"
)

// varPlaceholder matches {{vars.<name>}} references in recipe strings.
var varPlaceholder = regexp.MustCompile(`\{\{vars\.([A-Za-z_][A-Za-z0-9_]*)\}\}`)

// ExpandRecipeVars resolves {{vars.<name>}} placeholders in the recipe's
// shell aliases and functions — names, commands, and bodies — using the
// recipe's declared [recipe.vars] defaults overlaid with per-machine
// override values. It runs before the recipe merges into the config, so
// expanded names take part in duplicate detection and name validation.
// Overriding a variable the recipe does not declare is an error, as is a
// placeholder no declared variable resolves.
func ExpandRecipeVars(recipe *Recipe, overrides map[string]string) error {
	declared := recipe.Recipe.Vars
	for name := range overrides {
		if _, ok := declared[name]; !ok {
			return fmt.Errorf(
				"vars override '%s' is not declared in [recipe.vars] (declared: %s)",
				name,
				declaredNames(declared),
			)
		}
	}

	values := make(map[string]string, len(declared))
	maps.Copy(values, declared)
	maps.Copy(values, overrides)

	expand := func(field, s string) (string, error) {
		out := varPlaceholder.ReplaceAllStringFunc(s, func(match string) string {
			name := varPlaceholder.FindStringSubmatch(match)[1]
			if value, ok := values[name]; ok {
				return value
			}
			return match
		})
		if match := varPlaceholder.FindString(out); match != "" {
			return "", fmt.Errorf(
				"%s references %s, which is not declared in [recipe.vars] (declared: %s)",
				field,
				match,
				declaredNames(declared),
			)
		}
		return out, nil
	}

	aliases := make(map[string]ShellAlias, len(recipe.Shell.Aliases))
	for name, alias := range recipe.Shell.Aliases {
		newName, err := expand(fmt.Sprintf("shell alias '%s'", name), name)
		if err != nil {
			return err
		}
		alias.Command, err = expand(fmt.Sprintf("shell alias '%s' command", name), alias.Command)
		if err != nil {
			return err
		}
		if _, exists := aliases[newName]; exists {
			return fmt.Errorf(
				"shell alias '%s': expanded name '%s' collides with another alias",
				name,
				newName,
			)
		}
		aliases[newName] = alias
	}
	recipe.Shell.Aliases = aliases

	functions := make(map[string]ShellFunction, len(recipe.Shell.Functions))
	for name, function := range recipe.Shell.Functions {
		newName, err := expand(fmt.Sprintf("shell function '%s'", name), name)
		if err != nil {
			return err
		}
		function.Body, err = expand(fmt.Sprintf("shell function '%s' body", name), function.Body)
		if err != nil {
			return err
		}
		if _, exists := functions[newName]; exists {
			return fmt.Errorf(
				"shell function '%s': expanded name '%s' collides with another function",
				name,
				newName,
			)
		}
		functions[newName] = function
	}
	recipe.Shell.Functions = functions

	return nil
}

// declaredNames renders the declared variable names sorted, for stable
// error messages; "none" when the recipe declares no variables.
func declaredNames(declared map[string]string) string {
	if len(declared) == 0 {
		return "none"
	}
	names := make([]string, 0, len(declared))
	for name := range declared {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
