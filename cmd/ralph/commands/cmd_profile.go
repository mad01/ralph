package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/BurntSushi/toml"
	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/spf13/cobra"
)

// profileCmd groups the machine-profile subcommands. Profiles gate which
// recipes apply: a recipe with `profiles = [...]` runs only when its profiles
// intersect this machine's. A machine declares its own profiles in the
// git-ignored config.local.toml overlay, which these commands own.
var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Show or set this machine's ralph profiles",
	Long: `Read and write the machine profiles in the git-ignored config.local.toml
overlay next to your config.toml.

Profiles gate which recipes apply: a recipe with profiles = [...] runs only when
its profiles intersect this machine's. A machine with no profiles runs only the
unlabelled recipes.`,
}

var profileShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print this machine's profiles",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		localPath, err := localConfigPath()
		if err != nil {
			return err
		}
		profiles, err := readProfiles(localPath)
		if err != nil {
			return err
		}
		if len(profiles) == 0 {
			color.Yellow("No profiles set (%s).", localPath)
			fmt.Println("Set them with: ralph profile set <name>...")
			return nil
		}
		fmt.Println(strings.Join(profiles, " "))
		return nil
	},
}

var profileSetCmd = &cobra.Command{
	Use:   "set [profile...]",
	Short: "Set this machine's profiles in config.local.toml",
	Long: `Write the given profiles to the git-ignored config.local.toml overlay,
preserving any other keys already in the file. Profiles may be space- or
comma-separated. With no arguments on a terminal, prompts for the list
(default: personal).

Examples:
  ralph profile set personal
  ralph profile set work
  ralph profile set          # prompts`,
	RunE: func(_ *cobra.Command, args []string) error {
		localPath, err := localConfigPath()
		if err != nil {
			return err
		}

		// Split each argument too, so a single quoted "personal work" (as a
		// provisioning script may pass) parses the same as separate arguments.
		profiles := normalizeProfiles(splitProfiles(strings.Join(args, " ")))
		if len(profiles) == 0 {
			input := ""
			prompt := &survey.Input{
				Message: "Machine profile(s) (space- or comma-separated):",
				Default: "personal",
				Help:    "Recipes gate on these. Common: personal, work.",
			}
			if err := survey.AskOne(prompt, &input); err != nil {
				return fmt.Errorf("reading profiles from prompt (pass them as arguments instead): %w", err)
			}
			profiles = normalizeProfiles(splitProfiles(input))
		}
		if len(profiles) == 0 {
			return fmt.Errorf("no profiles given")
		}

		if err := writeProfiles(localPath, profiles); err != nil {
			return err
		}
		color.Green("Set profiles in %s: %s", localPath, strings.Join(profiles, ", "))
		return nil
	},
}

// localConfigPath resolves the config.local.toml overlay path from the default
// config location.
func localConfigPath() (string, error) {
	mainPath, err := config.GetDefaultConfigPath()
	if err != nil {
		return "", fmt.Errorf("resolving config path: %w", err)
	}
	return config.LocalConfigPath(mainPath), nil
}

// readProfiles returns the profiles declared in the overlay, or nil when the
// file does not exist. Other keys are ignored.
func readProfiles(localPath string) ([]string, error) {
	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("checking %s: %w", localPath, err)
	}
	var parsed struct {
		Profiles []string `toml:"profiles"`
	}
	if _, err := toml.DecodeFile(localPath, &parsed); err != nil {
		return nil, fmt.Errorf("reading %s: %w", localPath, err)
	}
	return parsed.Profiles, nil
}

// writeProfiles sets the profiles key in the overlay, preserving any other keys
// already present, and creates the file (and its directory) if missing. Inline
// comments in an existing file are not preserved.
func writeProfiles(localPath string, profiles []string) error {
	overlay := map[string]any{}
	if _, err := os.Stat(localPath); err == nil {
		if _, err := toml.DecodeFile(localPath, &overlay); err != nil {
			return fmt.Errorf("reading %s: %w", localPath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", localPath, err)
	}
	overlay["profiles"] = profiles

	var buf bytes.Buffer
	buf.WriteString("# Machine-local ralph overrides — git-ignored, not committed.\n")
	buf.WriteString("# Written by `ralph profile set`. Recipes whose profiles intersect these apply here.\n\n")
	if err := toml.NewEncoder(&buf).Encode(overlay); err != nil {
		return fmt.Errorf("encoding overlay: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	if err := os.WriteFile(localPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", localPath, err)
	}
	return nil
}

// normalizeProfiles trims each entry, drops empties, and removes duplicates
// while preserving first-seen order.
func normalizeProfiles(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// splitProfiles splits a free-form list on commas and whitespace, so both
// "personal, work" and "personal work" parse.
func splitProfiles(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
}

func init() {
	profileCmd.AddCommand(profileShowCmd)
	profileCmd.AddCommand(profileSetCmd)
	rootCmd.AddCommand(profileCmd)
}
