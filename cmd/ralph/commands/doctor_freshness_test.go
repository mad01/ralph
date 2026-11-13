package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/gitutil"
	"github.com/mad01/ralph/internal/hooks"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/testutil"
)

// gitWorkDir makes a committed git repo and returns it with its tree hash.
func gitWorkDir(t *testing.T) (dir, tree string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "src")
	testutil.InitGitRepo(t, dir)
	tree = gitutil.GetTreeHash(dir)
	if tree == "" {
		t.Skip("git unavailable")
	}
	return dir, tree
}

func makeDirty(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Doctor reports a build the way ralph up would treat it: a run = "once"
// build whose source moved or is dirty is drift, not "completed".
func TestBuildStateStep(t *testing.T) {
	dir, tree := gitWorkDir(t)
	done := time.Date(2026, 8, 13, 19, 32, 0, 0, time.UTC)
	once := config.Build{Commands: []string{"make"}, Run: config.RunOnce, WorkingDir: dir}

	tests := []struct {
		name       string
		build      config.Build
		record     *hooks.BuildRecord
		dirty      bool
		wantStatus report.Status
		wantMsg    string
	}{
		{
			name:       "once with a moved subtree",
			build:      once,
			record:     &hooks.BuildRecord{CompletedAt: done, GitHash: "0000000"},
			wantStatus: report.StatusWarn,
			wantMsg:    msgSourceChanged,
		},
		{
			name:       "once with uncommitted changes",
			build:      once,
			record:     &hooks.BuildRecord{CompletedAt: done, GitHash: tree},
			dirty:      true,
			wantStatus: report.StatusWarn,
			wantMsg:    msgUncommitted,
		},
		{
			name:       "once completed and unchanged",
			build:      once,
			record:     &hooks.BuildRecord{CompletedAt: done, GitHash: tree},
			wantStatus: report.StatusOK,
			wantMsg:    "completed at 2026-08-13 19:32:00",
		},
		{
			name:       "once not yet run",
			build:      once,
			wantStatus: report.StatusWarn,
			wantMsg:    "not yet run",
		},
		{
			name:       "always",
			build:      config.Build{Commands: []string{"make"}, Run: config.RunAlways},
			wantStatus: report.StatusOK,
			wantMsg:    "runs every apply",
		},
		{
			name:       "manual",
			build:      config.Build{Commands: []string{"make"}, Run: config.RunManual},
			wantStatus: report.StatusSkip,
			wantMsg:    "manual",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.dirty {
				makeDirty(t, dir)
				t.Cleanup(func() { _ = os.Remove(filepath.Join(dir, "new.txt")) })
			}
			state := &hooks.BuildState{Builds: map[string]hooks.BuildRecord{}}
			if tt.record != nil {
				state.Builds["tool"] = *tt.record
			}
			step := buildStateStep("tool", tt.build, tt.build.WorkingDir, state)
			if step.Status != tt.wantStatus || step.Message != tt.wantMsg {
				t.Errorf("buildStateStep() = %v %q, want %v %q",
					step.Status, step.Message, tt.wantStatus, tt.wantMsg)
			}
		})
	}
}

// Doctor reports a package the way ralph up would treat it.
func TestPackageStateStep(t *testing.T) {
	dir, tree := gitWorkDir(t)
	done := time.Date(2026, 8, 13, 19, 32, 0, 0, time.UTC)
	local := config.Package{Source: config.SourceLocal, WorkingDir: dir, Build: []string{"make"}}
	goInstall := config.Package{
		Source:  config.SourceGoInstall,
		Module:  "example.com/tool",
		Version: "v1.3.0",
	}

	tests := []struct {
		name       string
		pkg        config.Package
		workDir    string
		record     *hooks.BuildRecord
		dirty      bool
		wantStatus report.Status
		wantMsg    string
	}{
		{
			name:       "never built",
			pkg:        local,
			workDir:    dir,
			wantStatus: report.StatusWarn,
			wantMsg:    "never built",
		},
		{
			name:       "dirty source",
			pkg:        local,
			workDir:    dir,
			record:     &hooks.BuildRecord{CompletedAt: done, GitHash: tree},
			dirty:      true,
			wantStatus: report.StatusWarn,
			wantMsg:    msgUncommitted,
		},
		{
			name:       "moved source",
			pkg:        local,
			workDir:    dir,
			record:     &hooks.BuildRecord{CompletedAt: done, GitHash: "0000000"},
			wantStatus: report.StatusWarn,
			wantMsg:    msgSourceChanged,
		},
		{
			name:       "record without a source hash",
			pkg:        local,
			workDir:    dir,
			record:     &hooks.BuildRecord{CompletedAt: done},
			wantStatus: report.StatusWarn,
			wantMsg:    msgNoSourceHash,
		},
		{
			name:       "up to date",
			pkg:        local,
			workDir:    dir,
			record:     &hooks.BuildRecord{CompletedAt: done, GitHash: tree},
			wantStatus: report.StatusOK,
			wantMsg:    "last built at 2026-08-13 19:32:00",
		},
		{
			name:       "go-install version changed",
			pkg:        goInstall,
			record:     &hooks.BuildRecord{CompletedAt: done, Version: "v1.2.0"},
			wantStatus: report.StatusWarn,
			wantMsg:    "version changed (v1.2.0 → v1.3.0)" + doctorRunUp,
		},
		{
			name:       "go-install current",
			pkg:        goInstall,
			record:     &hooks.BuildRecord{CompletedAt: done, Version: "v1.3.0"},
			wantStatus: report.StatusOK,
			wantMsg:    "last built at 2026-08-13 19:32:00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.dirty {
				makeDirty(t, dir)
				t.Cleanup(func() { _ = os.Remove(filepath.Join(dir, "new.txt")) })
			}
			state := &hooks.BuildState{Builds: map[string]hooks.BuildRecord{}}
			if tt.record != nil {
				state.Builds["pkg:tool"] = *tt.record
			}
			step := packageStateStep("tool", tt.pkg, tt.workDir, state)
			if step.Status != tt.wantStatus || !strings.HasPrefix(step.Message, tt.wantMsg) {
				t.Errorf("packageStateStep() = %v %q, want %v %q",
					step.Status, step.Message, tt.wantStatus, tt.wantMsg)
			}
		})
	}
}
