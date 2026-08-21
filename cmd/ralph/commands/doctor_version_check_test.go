package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledBuildRequiresVersionCheckOptIn(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "probed")
	binary := filepath.Join(dir, "tool")
	script := fmt.Sprintf(
		"#!/bin/sh\ntouch %q\nprintf '%%s\\n' '{\"version\":\"1234567\"}'\n",
		marker,
	)
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, ok := installedBuild([]string{binary}, false); ok {
		t.Error("disabled version check unexpectedly returned build metadata")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("disabled version check executed binary, stat error = %v", err)
	}

	if _, ok := installedBuild([]string{binary}, true); !ok {
		t.Error("enabled version check did not return build metadata")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("enabled version check did not execute binary: %v", err)
	}
}
