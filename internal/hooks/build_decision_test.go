package hooks

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/testutil"
)

// fakeFacts is a BuildFacts that records which facts DecideBuild read.
type fakeFacts struct {
	record    *BuildRecord
	err       error
	treeHash  string
	dirty     bool
	readState bool
	readGit   bool
}

func (f *fakeFacts) Record() (BuildRecord, bool, error) {
	f.readState = true
	if f.err != nil {
		return BuildRecord{}, false, f.err
	}
	if f.record == nil {
		return BuildRecord{}, false, nil
	}
	return *f.record, true, nil
}

func (f *fakeFacts) TreeHash() string {
	f.readGit = true
	return f.treeHash
}

func (f *fakeFacts) Dirty() bool {
	f.readGit = true
	return f.dirty
}

func TestDecideBuild(t *testing.T) {
	const name, dir = "tool", "/src/tool"
	cmds := []string{"make"}
	contentHash := computeBuildHash(name, cmds, "", dir)
	once := config.Build{Commands: cmds, Run: config.RunOnce}
	idempotent := config.Build{Commands: cmds, Run: config.RunAlways, Idempotent: true}

	tests := []struct {
		name       string
		build      config.Build
		workingDir string
		force      bool
		specific   string
		facts      fakeFacts
		wantRun    bool
		wantReason BuildReason
		wantWas    string
		wantNow    string
		wantState  bool // the decision read the state file
		wantGit    bool // the decision ran git
	}{
		{
			name:       "always runs without reading state or git",
			build:      config.Build{Commands: cmds, Run: config.RunAlways},
			workingDir: dir,
			wantRun:    true,
			wantReason: ReasonAlways,
		},
		{
			name:       "once without a record runs",
			build:      once,
			workingDir: dir,
			wantRun:    true,
			wantReason: ReasonNotYetRun,
			wantState:  true,
		},
		{
			name:       "once under force runs without reading state",
			build:      once,
			workingDir: dir,
			force:      true,
			facts:      fakeFacts{record: &BuildRecord{GitHash: "tree-a"}},
			wantRun:    true,
			wantReason: ReasonForced,
		},
		{
			name:       "once without a working dir stays completed",
			build:      once,
			facts:      fakeFacts{record: &BuildRecord{GitHash: "tree-a"}, treeHash: "tree-b"},
			wantReason: ReasonCompleted,
			wantState:  true,
		},
		{
			name:       "once with an empty recorded hash stays completed",
			build:      once,
			workingDir: dir,
			facts:      fakeFacts{record: &BuildRecord{}, treeHash: "tree-b", dirty: true},
			wantReason: ReasonCompleted,
			wantState:  true,
		},
		{
			name:       "once with a moved subtree reruns",
			build:      once,
			workingDir: dir,
			facts:      fakeFacts{record: &BuildRecord{GitHash: "tree-a"}, treeHash: "tree-b"},
			wantRun:    true,
			wantReason: ReasonSourceChanged,
			wantWas:    "tree-a",
			wantNow:    "tree-b",
			wantState:  true,
			wantGit:    true,
		},
		{
			name:       "once with uncommitted changes reruns",
			build:      once,
			workingDir: dir,
			facts: fakeFacts{
				record: &BuildRecord{GitHash: "tree-a"}, treeHash: "tree-a", dirty: true,
			},
			wantRun:    true,
			wantReason: ReasonUncommitted,
			wantState:  true,
			wantGit:    true,
		},
		{
			name:       "once with an unknown current hash checks for changes",
			build:      once,
			workingDir: dir,
			facts:      fakeFacts{record: &BuildRecord{GitHash: "tree-a"}},
			wantReason: ReasonCompleted,
			wantState:  true,
			wantGit:    true,
		},
		{
			name:       "once with an unchanged clean subtree skips",
			build:      once,
			workingDir: dir,
			facts:      fakeFacts{record: &BuildRecord{GitHash: "tree-a"}, treeHash: "tree-a"},
			wantReason: ReasonCompleted,
			wantState:  true,
			wantGit:    true,
		},
		{
			name:       "manual without --build skips",
			build:      config.Build{Commands: cmds, Run: config.RunManual},
			specific:   "other",
			wantReason: ReasonManual,
		},
		{
			name:       "manual named with --build runs",
			build:      config.Build{Commands: cmds, Run: config.RunManual},
			specific:   name,
			wantRun:    true,
			wantReason: ReasonRequested,
		},
		{
			name:       "idempotent with a matching content hash skips",
			build:      idempotent,
			workingDir: dir,
			facts:      fakeFacts{record: &BuildRecord{ContentHash: contentHash}},
			wantReason: ReasonContentUnchanged,
			wantState:  true,
		},
		{
			name:       "idempotent with a stale content hash falls to its run mode",
			build:      idempotent,
			workingDir: dir,
			facts:      fakeFacts{record: &BuildRecord{ContentHash: "stale"}},
			wantRun:    true,
			wantReason: ReasonAlways,
			wantState:  true,
		},
		{
			name:       "idempotent without a record falls to its run mode",
			build:      idempotent,
			workingDir: dir,
			wantRun:    true,
			wantReason: ReasonAlways,
			wantState:  true,
		},
		{
			name:       "idempotent match skips a manual build named with --build",
			build:      config.Build{Commands: cmds, Run: config.RunManual, Idempotent: true},
			workingDir: dir,
			specific:   name,
			facts:      fakeFacts{record: &BuildRecord{ContentHash: contentHash}},
			wantReason: ReasonContentUnchanged,
			wantState:  true,
		},
		{
			name:       "force skips the idempotent check",
			build:      idempotent,
			workingDir: dir,
			force:      true,
			facts:      fakeFacts{record: &BuildRecord{ContentHash: contentHash}},
			wantRun:    true,
			wantReason: ReasonAlways,
		},
		{
			name: "idempotent once with changed content but done source stays completed",
			build: config.Build{
				Commands: []string{"make", "make install"}, Run: config.RunOnce, Idempotent: true,
			},
			workingDir: dir,
			facts: fakeFacts{
				record: &BuildRecord{
					GitHash:     "tree-a",
					ContentHash: contentHash,
				}, treeHash: "tree-a",
			},
			wantReason: ReasonCompleted,
			wantState:  true,
			wantGit:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := tt.facts
			got, err := DecideBuild(BuildInput{
				Name:          name,
				Build:         tt.build,
				WorkingDir:    tt.workingDir,
				Force:         tt.force,
				SpecificBuild: tt.specific,
				Facts:         &facts,
			})
			if err != nil {
				t.Fatalf("DecideBuild() error = %v", err)
			}
			if got.Run != tt.wantRun || got.Reason != tt.wantReason {
				t.Errorf("DecideBuild() = {%v %q}, want {%v %q}",
					got.Run, got.Reason, tt.wantRun, tt.wantReason)
			}
			if got.WasHash != tt.wantWas || got.NowHash != tt.wantNow {
				t.Errorf("hashes = %q, %q, want %q, %q",
					got.WasHash, got.NowHash, tt.wantWas, tt.wantNow)
			}
			if facts.readState != tt.wantState || facts.readGit != tt.wantGit {
				t.Errorf("read state, git = %v, %v, want %v, %v",
					facts.readState, facts.readGit, tt.wantState, tt.wantGit)
			}
		})
	}
}

func TestDecideBuild_Errors(t *testing.T) {
	stateErr := errors.New("corrupt")
	tests := []struct {
		name    string
		build   config.Build
		wantErr string
	}{
		{
			name:    "unknown run mode",
			build:   config.Build{Run: "sometimes"},
			wantErr: "unknown run mode 'sometimes' for build 'tool'",
		},
		{
			name:    "unreadable state for a once build",
			build:   config.Build{Run: config.RunOnce},
			wantErr: "failed to load build state: corrupt",
		},
		{
			name:    "unreadable state for an idempotent build",
			build:   config.Build{Run: config.RunAlways, Idempotent: true},
			wantErr: "failed to load build state: corrupt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecideBuild(BuildInput{
				Name:  "tool",
				Build: tt.build,
				Facts: &fakeFacts{err: stateErr},
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("DecideBuild() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// A build whose state file does not parse still runs when its run mode needs
// no state: an always build never reads it.
func TestDecideBuild_AlwaysIgnoresUnreadableState(t *testing.T) {
	got, err := DecideBuild(BuildInput{
		Name:  "tool",
		Build: config.Build{Run: config.RunAlways},
		Facts: &fakeFacts{err: errors.New("corrupt")},
	})
	if err != nil || !got.Run {
		t.Fatalf("DecideBuild() = %+v, %v, want a run with no error", got, err)
	}
}

// Config loading rejects a build with both script and commands; RunBuild
// checks again before deciding, so such a build fails even when it would
// skip.
func TestRunBuild_ConflictingStepsFailBeforeDecision(t *testing.T) {
	_ = testutil.WithHome(t)
	build := config.Build{Script: "build.sh", Commands: []string{"make"}, Run: config.RunManual}
	err := RunBuild(context.Background(), io.Discard, "tool", build, "host", nil, BuildOptions{})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("RunBuild() error = %v, want the script/commands conflict", err)
	}
}
