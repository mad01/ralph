package dotfile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

func TestCreateDirectory(t *testing.T) {
	tests := []struct {
		name     string
		dir      config.Directory
		dryRun   bool
		setup    func(t *testing.T, target string)
		wantErr  bool
		checkDir bool
	}{
		{
			name:     "creates new directory",
			dir:      config.Directory{Mode: "0755"},
			dryRun:   false,
			checkDir: true,
		},
		{
			name:     "dry run does not create",
			dir:      config.Directory{Mode: "0755"},
			dryRun:   true,
			checkDir: false,
		},
		{
			name: "already exists is ok",
			dir:  config.Directory{},
			setup: func(t *testing.T, target string) {
				_ = os.MkdirAll(target, 0o755)
			},
			dryRun:   false,
			checkDir: true,
		},
		{
			name: "file exists at target is error",
			dir:  config.Directory{},
			setup: func(t *testing.T, target string) {
				_ = os.MkdirAll(filepath.Dir(target), 0o755)
				_ = os.WriteFile(target, []byte("file"), 0o644)
			},
			dryRun:  false,
			wantErr: true,
		},
		{
			name:    "invalid mode",
			dir:     config.Directory{Mode: "invalid"},
			dryRun:  false,
			wantErr: true,
		},
		{
			name:     "custom mode",
			dir:      config.Directory{Mode: "0700"},
			dryRun:   false,
			checkDir: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			target := filepath.Join(tmpDir, "testdir")

			tt.dir.Target = target
			if tt.setup != nil {
				tt.setup(t, target)
			}

			var buf bytes.Buffer
			err := CreateDirectory(&buf, tt.dir, tt.dryRun)

			if (err != nil) != tt.wantErr {
				t.Fatalf("CreateDirectory() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.checkDir {
				info, err := os.Stat(target)
				if err != nil {
					t.Fatalf("expected directory to exist: %v", err)
				}
				if !info.IsDir() {
					t.Error("expected target to be a directory")
				}
			}

			if tt.dryRun && !tt.wantErr {
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Error("dry run should not create directory")
				}
			}
		})
	}
}
