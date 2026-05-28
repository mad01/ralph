package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateFromLegacy_FreshInstall(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	var buf bytes.Buffer
	err := MigrateFromLegacy(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for fresh install, got: %s", buf.String())
	}
}

func TestMigrateFromLegacy_AlreadyMigrated(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	newDir := filepath.Join(tmpDir, ".config", "ralph")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := MigrateFromLegacy(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output when already migrated, got: %s", buf.String())
	}
}

func TestMigrateFromLegacy_MigratesOldDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	oldDir := filepath.Join(tmpDir, ".config", "dotter")
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "config.toml"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := MigrateFromLegacy(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	newDir := filepath.Join(tmpDir, ".config", "ralph")
	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		t.Error("expected new dir to exist after migration")
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("expected old dir to be gone after migration")
	}
	if !bytes.Contains(buf.Bytes(), []byte("Migrated")) {
		t.Errorf("expected 'Migrated' in output, got: %s", buf.String())
	}
}
