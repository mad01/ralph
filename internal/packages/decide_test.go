package packages

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/testutil"
)

// installed returns an Installed stub that reports reason and counts calls.
func installed(reason string, calls *int) func() InstallationCheck {
	return func() InstallationCheck {
		*calls++
		return InstallationCheck{Reason: reason}
	}
}

func TestDecidePackage(t *testing.T) {
	local := config.Package{Source: config.SourceLocal, WorkingDir: "/src/tool"}
	remote := config.Package{Source: config.SourceRemote, WorkingDir: "/pkg/tool"}
	goInstall := config.Package{Source: config.SourceGoInstall, Version: "v1.2.0"}
	recorded := &buildstate.BuildRecord{GitHash: "tree-a", Version: "v1.2.0"}
	unhashed := &buildstate.BuildRecord{}

	tests := []struct {
		name          string
		in            FreshnessInput
		installReason string
		wantNeeds     bool
		wantCause     FreshnessCause
		wantReason    string
		wantWas       string
		wantNow       string
		wantChecked   bool // the install check ran
	}{
		{
			name:       "remote not cloned builds after sync",
			in:         FreshnessInput{Package: remote},
			wantNeeds:  true,
			wantCause:  CauseNotCloned,
			wantReason: "not cloned",
		},
		{
			name:       "make not cloned builds after sync",
			in:         FreshnessInput{Package: config.Package{Source: config.SourceMake}},
			wantNeeds:  true,
			wantCause:  CauseNotCloned,
			wantReason: "not cloned",
		},
		{
			name:       "local working_dir missing cannot build",
			in:         FreshnessInput{Package: local, Force: true},
			wantCause:  CauseWorkDirMissing,
			wantReason: "working_dir missing",
		},
		{
			name:       "empty source is local",
			in:         FreshnessInput{Package: config.Package{WorkingDir: "/src/x"}},
			wantCause:  CauseWorkDirMissing,
			wantReason: "working_dir missing",
		},
		{
			name: "force rebuilds before any record check",
			in: FreshnessInput{
				Package: local, WorkDirExists: true, Record: recorded,
				TreeHash: "tree-a", Force: true,
			},
			wantNeeds:  true,
			wantCause:  CauseForced,
			wantReason: "forced",
		},
		{
			name:       "missing record builds",
			in:         FreshnessInput{Package: local, WorkDirExists: true, TreeHash: "tree-a"},
			wantNeeds:  true,
			wantCause:  CauseNeverBuilt,
			wantReason: "never built",
		},
		{
			name: "empty recorded hash rebuilds to repair state",
			in: FreshnessInput{
				Package: remote, WorkDirExists: true, Record: unhashed, TreeHash: "tree-a",
			},
			wantNeeds:  true,
			wantCause:  CauseNoSourceHash,
			wantReason: "no recorded source hash",
		},
		{
			name: "empty recorded hash without a current hash falls through",
			in: FreshnessInput{
				Package: local, WorkDirExists: true, Record: unhashed,
			},
			wantCause:   CauseUpToDate,
			wantChecked: true,
		},
		{
			name: "tree hash mismatch rebuilds",
			in: FreshnessInput{
				Package: local, WorkDirExists: true, Record: recorded, TreeHash: "tree-b",
			},
			wantNeeds:  true,
			wantCause:  CauseSourceChanged,
			wantReason: "git hash changed",
			wantWas:    "tree-a",
			wantNow:    "tree-b",
		},
		{
			name: "unknown current hash is no change",
			in: FreshnessInput{
				Package: local, WorkDirExists: true, Record: recorded,
			},
			wantCause:   CauseUpToDate,
			wantChecked: true,
		},
		{
			name: "dirty subtree rebuilds",
			in: FreshnessInput{
				Package: remote, WorkDirExists: true, Record: recorded,
				TreeHash: "tree-a", Dirty: true,
			},
			wantNeeds:  true,
			wantCause:  CauseUncommitted,
			wantReason: "uncommitted changes",
		},
		{
			name: "missing install path rebuilds",
			in: FreshnessInput{
				Package: local, WorkDirExists: true, Record: recorded, TreeHash: "tree-a",
			},
			installReason: "install_path missing (/bin/tool)",
			wantNeeds:     true,
			wantCause:     CauseInstalled,
			wantReason:    "install_path missing (/bin/tool)",
			wantChecked:   true,
		},
		{
			name: "version check mismatch rebuilds",
			in: FreshnessInput{
				Package: remote, WorkDirExists: true, Record: recorded, TreeHash: "tree-a",
			},
			installReason: "installed revision 1111111 does not match source HEAD 2222222",
			wantNeeds:     true,
			wantCause:     CauseInstalled,
			wantReason:    "installed revision 1111111 does not match source HEAD 2222222",
			wantChecked:   true,
		},
		{
			name: "fresh package is up to date",
			in: FreshnessInput{
				Package: local, WorkDirExists: true, Record: recorded, TreeHash: "tree-a",
			},
			wantCause:   CauseUpToDate,
			wantChecked: true,
		},
		{
			name:       "go-install force reinstalls",
			in:         FreshnessInput{Package: goInstall, Record: recorded, Force: true},
			wantNeeds:  true,
			wantCause:  CauseForced,
			wantReason: "forced",
		},
		{
			name:       "go-install without record installs",
			in:         FreshnessInput{Package: goInstall},
			wantNeeds:  true,
			wantCause:  CauseNeverBuilt,
			wantReason: "never installed",
		},
		{
			name: "go-install version change reinstalls",
			in: FreshnessInput{
				Package: config.Package{Source: config.SourceGoInstall, Version: "v1.3.0"},
				Record:  recorded,
			},
			wantNeeds:  true,
			wantCause:  CauseVersionChanged,
			wantReason: "version changed",
			wantWas:    "v1.2.0",
			wantNow:    "v1.3.0",
		},
		{
			name:       "go-install ignores the working dir and source hash",
			in:         FreshnessInput{Package: goInstall, Record: unhashed, Dirty: true},
			wantNeeds:  true,
			wantCause:  CauseVersionChanged,
			wantReason: "version changed",
			wantNow:    "v1.2.0",
		},
		{
			name:          "go-install stale artifact reinstalls",
			in:            FreshnessInput{Package: goInstall, Record: recorded},
			installReason: "no recorded install hash",
			wantNeeds:     true,
			wantCause:     CauseInstalled,
			wantReason:    "no recorded install hash",
			wantChecked:   true,
		},
		{
			name:        "go-install current version is up to date",
			in:          FreshnessInput{Package: goInstall, Record: recorded},
			wantCause:   CauseUpToDate,
			wantChecked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			tt.in.Installed = installed(tt.installReason, &calls)
			got := DecidePackage(tt.in)
			if got.NeedsBuild != tt.wantNeeds || got.Cause != tt.wantCause ||
				got.Reason != tt.wantReason {
				t.Errorf(
					"DecidePackage() = {%v %q %q}, want {%v %q %q}",
					got.NeedsBuild, got.Cause, got.Reason,
					tt.wantNeeds, tt.wantCause, tt.wantReason,
				)
			}
			if got.Was != tt.wantWas || got.Now != tt.wantNow {
				t.Errorf("Was, Now = %q, %q, want %q, %q", got.Was, got.Now, tt.wantWas, tt.wantNow)
			}
			if checked := calls > 0; checked != tt.wantChecked {
				t.Errorf("install check ran = %v, want %v", checked, tt.wantChecked)
			}
		})
	}
}

func TestDecidePackage_NilInstalledSkipsCheck(t *testing.T) {
	in := FreshnessInput{
		Package:       config.Package{Source: config.SourceLocal},
		WorkDirExists: true,
		Record:        &buildstate.BuildRecord{},
	}
	if got := DecidePackage(in); got.NeedsBuild || got.Cause != CauseUpToDate {
		t.Errorf("DecidePackage() = %+v, want up to date", got)
	}
}

// ObservePackage wires the real git facts and a lazy install check, so a
// deleted binary shows up as a rebuild in ralph list as in ralph up.
func TestObservePackage_MissingInstallPath(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "repo")
	testutil.InitGitRepo(t, workDir)
	pkg := config.Package{
		Source:       config.SourceLocal,
		WorkingDir:   workDir,
		InstallPaths: []string{filepath.Join(root, "bin", "tool")},
	}

	in := ObservePackage(pkg, &buildstate.BuildRecord{GitHash: treeHash(t, workDir)})
	if !in.WorkDirExists || in.TreeHash == "" || in.Dirty {
		t.Fatalf("ObservePackage() = %+v, want an existing clean git working dir", in)
	}
	got := DecidePackage(in)
	if got.Cause != CauseInstalled || !strings.Contains(got.Reason, "install_path missing") {
		t.Errorf("DecidePackage() = %+v, want an install_path missing rebuild", got)
	}
}
