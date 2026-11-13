package commands

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

func TestNormalizeProfiles(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", nil, []string{}},
		{"trims", []string{"  personal  "}, []string{"personal"}},
		{"drops empties", []string{"personal", "", "  "}, []string{"personal"}},
		{
			"dedupes preserving order",
			[]string{"work", "personal", "work"},
			[]string{"work", "personal"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeProfiles(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("normalizeProfiles(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitProfiles(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"spaces", "personal work", []string{"personal", "work"}},
		{"commas", "personal,work", []string{"personal", "work"}},
		{"mixed", "personal, work", []string{"personal", "work"}},
		{"tabs", "personal\twork", []string{"personal", "work"}},
		{"empty", "", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitProfiles(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitProfiles(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestReadProfilesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.local.toml")
	got, err := readProfiles(path)
	if err != nil {
		t.Fatalf("readProfiles() on missing file error = %v", err)
	}
	if got != nil {
		t.Errorf("readProfiles() on missing file = %v, want nil", got)
	}
}

func TestWriteThenReadProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.local.toml")
	want := []string{"personal", "work"}
	if err := writeProfiles(path, want); err != nil {
		t.Fatalf("writeProfiles() error = %v", err)
	}
	got, err := readProfiles(path)
	if err != nil {
		t.Fatalf("readProfiles() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip profiles = %v, want %v", got, want)
	}
}

func TestWriteProfilesPreservesOtherKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.local.toml")
	writeFile(t, path, "packages_dir = \"/custom/pkg\"\nprofiles = [\"old\"]\n")

	if err := writeProfiles(path, []string{"personal"}); err != nil {
		t.Fatalf("writeProfiles() error = %v", err)
	}

	body := readFile(t, path)
	if !strings.Contains(body, `packages_dir = "/custom/pkg"`) {
		t.Errorf("writeProfiles dropped an unrelated key:\n%s", body)
	}
	got, err := readProfiles(path)
	if err != nil {
		t.Fatalf("readProfiles() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"personal"}) {
		t.Errorf("profiles = %v, want [personal]", got)
	}
}

// withProfileFixture points GetDefaultConfigPath at a config.toml in a temp dir
// and returns the path of the config.local.toml overlay next to it.
func withProfileFixture(t *testing.T) string {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.toml")
	orig := config.GetDefaultConfigPath
	config.GetDefaultConfigPath = func() (string, error) { return configPath, nil }
	t.Cleanup(func() { config.GetDefaultConfigPath = orig })
	return config.LocalConfigPath(configPath)
}

// `ralph profile set --dry-run` must not create the overlay, and must say what
// it would write.
func TestProfileSet_DryRunCreatesNothing(t *testing.T) {
	localPath := withProfileFixture(t)
	setDryRun(t, true)

	var runErr error
	out := captureStdout(t, func() {
		runErr = profileSetCmd.RunE(profileSetCmd, []string{"work"})
	})
	if runErr != nil {
		t.Fatalf("profile set: %v", runErr)
	}

	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run profile set created %s (stat err = %v)", localPath, err)
	}
	if !strings.Contains(out, "Would write") || !strings.Contains(out, localPath) {
		t.Errorf("dry-run output does not name the file it would write:\n%s", out)
	}
	if !strings.Contains(out, `profiles = ["work"]`) {
		t.Errorf("dry-run output does not show the profiles it would write:\n%s", out)
	}
}

// `ralph profile set --dry-run` must leave an existing overlay as it was.
func TestProfileSet_DryRunLeavesOverlayUntouched(t *testing.T) {
	localPath := withProfileFixture(t)
	before := "packages_dir = \"/custom/pkg\"\nprofiles = [\"old\"]\n"
	writeFile(t, localPath, before)
	setDryRun(t, true)

	captureStdout(t, func() {
		if err := profileSetCmd.RunE(profileSetCmd, []string{"work"}); err != nil {
			t.Errorf("profile set: %v", err)
		}
	})

	if after := readFile(t, localPath); after != before {
		t.Fatalf("dry-run profile set modified %s:\n%s", localPath, after)
	}
}

// Without --dry-run, profile set writes the overlay.
func TestProfileSet_WritesOverlay(t *testing.T) {
	localPath := withProfileFixture(t)
	setDryRun(t, false)

	captureStdout(t, func() {
		if err := profileSetCmd.RunE(profileSetCmd, []string{"work"}); err != nil {
			t.Errorf("profile set: %v", err)
		}
	})

	got, err := readProfiles(localPath)
	if err != nil {
		t.Fatalf("readProfiles() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"work"}) {
		t.Errorf("profiles = %v, want [work]", got)
	}
}

func TestWriteProfilesCreatesMissingDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.local.toml")
	if err := writeProfiles(path, []string{"personal"}); err != nil {
		t.Fatalf("writeProfiles() into missing dir error = %v", err)
	}
	got, err := readProfiles(path)
	if err != nil {
		t.Fatalf("readProfiles() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"personal"}) {
		t.Errorf("profiles = %v, want [personal]", got)
	}
}
