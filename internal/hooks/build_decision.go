package hooks

import (
	"fmt"

	"github.com/mad01/ralph/internal/config"
)

// BuildReason names the rule that decided whether a build runs.
type BuildReason string

const (
	// ReasonAlways runs a run = "always" build.
	ReasonAlways BuildReason = "runs every apply"
	// ReasonForced runs a run = "once" build under --force.
	ReasonForced BuildReason = "forced"
	// ReasonNotYetRun runs a run = "once" build with no recorded success.
	ReasonNotYetRun BuildReason = "not yet run"
	// ReasonSourceChanged reruns a run = "once" build whose working_dir
	// subtree moved since its recorded success.
	ReasonSourceChanged BuildReason = "source changed"
	// ReasonUncommitted reruns a run = "once" build whose working_dir subtree
	// has uncommitted changes.
	ReasonUncommitted BuildReason = "uncommitted changes"
	// ReasonCompleted skips a run = "once" build that has succeeded.
	ReasonCompleted BuildReason = "already completed"
	// ReasonContentUnchanged skips an idempotent build whose content hash
	// matches its record.
	ReasonContentUnchanged BuildReason = "content unchanged"
	// ReasonRequested runs a run = "manual" build named with --build.
	ReasonRequested BuildReason = "requested with --build"
	// ReasonManual skips a run = "manual" build that --build did not name.
	ReasonManual BuildReason = "manual"
)

// BuildDecision is whether a build runs, and the rule that decided.
type BuildDecision struct {
	Run    bool
	Reason BuildReason
	// WasHash and NowHash are the recorded and current tree hashes of the
	// working_dir subtree when Reason is ReasonSourceChanged.
	WasHash, NowHash string
}

// BuildFacts supplies the state and git facts DecideBuild reads. DecideBuild
// asks only for what its rule needs, so an always build that is not
// idempotent reads neither the state file nor git.
type BuildFacts interface {
	// Record returns the build's last successful run; ok is false when
	// there is none.
	Record() (rec BuildRecord, ok bool, err error)
	// TreeHash returns the working_dir subtree's tree hash, "" when unknown.
	TreeHash() string
	// Dirty reports uncommitted changes in the working_dir subtree.
	Dirty() bool
}

// BuildInput is one build as DecideBuild sees it.
type BuildInput struct {
	Name          string
	Build         config.Build
	WorkingDir    string // expanded working_dir; "" when unset
	Force         bool   // rerun once builds and skip the idempotent check
	SpecificBuild string // the --build name; the only way a manual build runs
	Facts         BuildFacts
}

// DecideBuild decides whether a build that passed its enable, host and
// profile filters runs. An idempotent build whose content hash matches its
// record skips whatever its run mode; otherwise the run mode decides.
func DecideBuild(in BuildInput) (BuildDecision, error) {
	if in.Build.Idempotent && !in.Force {
		unchanged, err := contentUnchanged(in)
		if err != nil {
			return BuildDecision{}, err
		}
		if unchanged {
			return BuildDecision{Reason: ReasonContentUnchanged}, nil
		}
	}
	switch in.Build.Run {
	case config.RunAlways:
		return BuildDecision{Run: true, Reason: ReasonAlways}, nil
	case config.RunOnce:
		return decideOnce(in)
	case config.RunManual:
		if in.SpecificBuild == in.Name {
			return BuildDecision{Run: true, Reason: ReasonRequested}, nil
		}
		return BuildDecision{Reason: ReasonManual}, nil
	default:
		return BuildDecision{}, fmt.Errorf(
			"unknown run mode '%s' for build '%s'",
			in.Build.Run,
			in.Name,
		)
	}
}

// contentUnchanged reports whether an idempotent build's content hash
// (name, commands or script, working_dir) matches the one recorded on its
// last success.
func contentUnchanged(in BuildInput) (bool, error) {
	rec, ok, err := in.Facts.Record()
	if err != nil {
		return false, fmt.Errorf("failed to load build state: %w", err)
	}
	hash := computeBuildHash(in.Name, in.Build.Commands, in.Build.Script, in.WorkingDir)
	return ok && rec.ContentHash == hash, nil
}

// decideOnce applies run = "once": run until a success is recorded, then
// again only when the working_dir subtree moved or has uncommitted changes.
// Freshness is keyed on the subtree's tree hash, not the repo-wide commit, so
// commits elsewhere in the repo don't force a rerun. A build without a
// working_dir, or a record without a hash, stays completed. Records from
// older versions hold a commit hash; the first run after upgrading reruns
// once, then stores the tree hash.
func decideOnce(in BuildInput) (BuildDecision, error) {
	if in.Force {
		return BuildDecision{Run: true, Reason: ReasonForced}, nil
	}
	rec, ok, err := in.Facts.Record()
	if err != nil {
		return BuildDecision{}, fmt.Errorf("failed to load build state: %w", err)
	}
	if !ok {
		return BuildDecision{Run: true, Reason: ReasonNotYetRun}, nil
	}
	if in.WorkingDir == "" || rec.GitHash == "" {
		return BuildDecision{Reason: ReasonCompleted}, nil
	}
	if now := in.Facts.TreeHash(); now != "" && now != rec.GitHash {
		return BuildDecision{
			Run:     true,
			Reason:  ReasonSourceChanged,
			WasHash: rec.GitHash,
			NowHash: now,
		}, nil
	}
	if in.Facts.Dirty() {
		return BuildDecision{Run: true, Reason: ReasonUncommitted}, nil
	}
	return BuildDecision{Reason: ReasonCompleted}, nil
}

// StateFacts are the BuildFacts of one build: its record in State and the git
// state of WorkingDir on disk.
type StateFacts struct {
	// State is a loaded build state. Nil reads the state file on each
	// Record call.
	State      *BuildState
	Name       string
	WorkingDir string
}

// Record returns the build's record from State, or from the state file.
func (f StateFacts) Record() (BuildRecord, bool, error) {
	state := f.State
	if state == nil {
		loaded, err := LoadBuildState()
		if err != nil {
			return BuildRecord{}, false, err
		}
		state = loaded
	}
	rec, ok := state.Builds[f.Name]
	return rec, ok, nil
}

// TreeHash returns the tree hash of WorkingDir's subtree.
func (f StateFacts) TreeHash() string { return GetTreeHash(f.WorkingDir) }

// Dirty reports uncommitted changes in WorkingDir's subtree.
func (f StateFacts) Dirty() bool { return HasGitChangesInPath(f.WorkingDir) }
