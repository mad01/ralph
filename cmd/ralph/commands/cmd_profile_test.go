package commands

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
		{"dedupes preserving order", []string{"work", "personal", "work"}, []string{"work", "personal"}},
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
