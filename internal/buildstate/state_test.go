package buildstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestComputeBuildHash_Stable(t *testing.T) {
	a := ComputeBuildHash("foo", []string{"echo a"}, "", "/tmp")
	b := ComputeBuildHash("foo", []string{"echo a"}, "", "/tmp")
	if a != b {
		t.Fatalf("expected stable hash, got %s vs %s", a, b)
	}
	if len(a) != 64 {
		t.Errorf("expected 64-char hex sha256, got %d", len(a))
	}
}

func TestComputeBuildHash_DiffersOnInput(t *testing.T) {
	tests := []struct {
		name string
		a, b string
	}{
		{
			"different commands",
			ComputeBuildHash("foo", []string{"echo a"}, "", "/tmp"),
			ComputeBuildHash("foo", []string{"echo b"}, "", "/tmp"),
		},
		{
			"different names",
			ComputeBuildHash("foo", []string{"echo a"}, "", "/tmp"),
			ComputeBuildHash("bar", []string{"echo a"}, "", "/tmp"),
		},
		{
			"different dirs",
			ComputeBuildHash("foo", []string{"echo a"}, "", "/tmp"),
			ComputeBuildHash("foo", []string{"echo a"}, "", "/var"),
		},
		{
			"command boundary",
			ComputeBuildHash("n", []string{"ab", "c"}, "", "/"),
			ComputeBuildHash("n", []string{"a", "bc"}, "", "/"),
		},
	}
	for _, tt := range tests {
		if tt.a == tt.b {
			t.Errorf("%s: expected different hashes", tt.name)
		}
	}
}

func TestComputeInstallHash(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("a", "v1")
	b := write("b", "other")

	// Empty list → empty hash, no error.
	if h, err := ComputeInstallHash(nil); err != nil || h != "" {
		t.Fatalf("empty: got (%q, %v), want (\"\", nil)", h, err)
	}

	// Stable + order-independent (sorted internally).
	h1, err := ComputeInstallHash([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	h2, err := ComputeInstallHash([]string{b, a})
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("expected order-independent hash, got %s vs %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char sha256, got %d", len(h1))
	}

	// Changing content changes the hash.
	write("a", "v2")
	h3, err := ComputeInstallHash([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if h3 == h1 {
		t.Error("expected hash to change when a file's content changes")
	}

	// A missing path is an error (must not masquerade as unchanged).
	if _, err := ComputeInstallHash([]string{filepath.Join(dir, "nope")}); err == nil {
		t.Error("expected error for missing install path")
	}
}

func TestLoadSaveBuildState_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	orig := GetStateFilePath
	GetStateFilePath = func() (string, error) { return statePath, nil }
	t.Cleanup(func() { GetStateFilePath = orig })

	state := &BuildState{
		Builds: map[string]BuildRecord{
			"test": {
				CompletedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				GitHash:     "abc123",
				ContentHash: "def456",
				Version:     "v1.0.0",
			},
		},
	}

	if err := SaveBuildState(state); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadBuildState()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	rec := loaded.Builds["test"]
	if rec.GitHash != "abc123" {
		t.Errorf("GitHash = %q, want abc123", rec.GitHash)
	}
	if rec.ContentHash != "def456" {
		t.Errorf("ContentHash = %q, want def456", rec.ContentHash)
	}
	if rec.Version != "v1.0.0" {
		t.Errorf("Version = %q, want v1.0.0", rec.Version)
	}
}

func TestLoadBuildState_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "nonexistent.json")

	orig := GetStateFilePath
	GetStateFilePath = func() (string, error) { return statePath, nil }
	t.Cleanup(func() { GetStateFilePath = orig })

	state, err := LoadBuildState()
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(state.Builds) != 0 {
		t.Errorf("expected empty builds, got %d", len(state.Builds))
	}
}
