package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

// checkStateFiles reports whether ralph's two state files parse. A missing
// file is a fresh install, not a problem. An unreadable file is a failure
// whose message names the file and the way back:
//   - .recipe_state: the next 'ralph up' moves a file that does not parse
//     aside and writes a fresh manifest; a file it cannot read stays as it is.
//   - .builds_state: 'ralph up --reset-builds' deletes it.
func checkStateFiles(rpt *report.Report) {
	phase := rpt.AddPhase("State")
	status, msg, err := recipeStateStatus()
	phase.AddResult(state.FileName, "", status, msg, err)
	status, msg, err = buildStateStatus()
	phase.AddResult(buildStateFileName, "", status, msg, err)
}

// buildStateFileName is the basename of the build state file.
const buildStateFileName = ".builds_state"

// recipeStateStatus checks .recipe_state: it must parse, and its schema
// version must be one this binary knows. A newer version means a newer ralph
// wrote it, and this binary would rewrite it in its older schema.
func recipeStateStatus() (report.Status, string, error) {
	path, err := state.GetStatePath()
	if err != nil {
		return report.StatusFail, err.Error(), err
	}
	if !stateFileExists(path) {
		return report.StatusOK, "not created yet: " + path, nil
	}
	s, err := state.Load()
	if errors.Is(err, state.ErrCorrupt) {
		return report.StatusFail, fmt.Sprintf(
			"%s: %v; the next 'ralph up' moves it aside to %s and records a fresh one "+
				"(cleanup then forgets artifacts recorded before)",
			path,
			err,
			state.CorruptBackupPattern(path),
		), err
	}
	if err != nil {
		return report.StatusFail, fmt.Sprintf(
			"%s: %v; 'ralph up' skips recording recipe state until the file can be read",
			path,
			err,
		), err
	}
	if s.Version > state.CurrentRecipeStateVersion {
		return report.StatusFail, fmt.Sprintf(
			"%s: version %d is newer than this ralph supports (%d); upgrade ralph "+
				"before running 'ralph up'",
			path,
			s.Version,
			state.CurrentRecipeStateVersion,
		), nil
	}
	return report.StatusOK, path, nil
}

// buildStateStatus checks that .builds_state parses. The file carries no
// schema version, so there is no version to compare.
func buildStateStatus() (report.Status, string, error) {
	path, err := buildstate.GetStateFilePath()
	if err != nil {
		return report.StatusFail, err.Error(), err
	}
	if !stateFileExists(path) {
		return report.StatusOK, "not created yet: " + path, nil
	}
	if _, err := buildstate.LoadBuildState(); err != nil {
		return report.StatusFail, fmt.Sprintf(
			"%s: %v; run 'ralph up --reset-builds' to reset it (every package and "+
				"run-once build reruns)",
			path,
			err,
		), err
	}
	return report.StatusOK, path, nil
}

// stateFileExists reports whether path exists. An error other than "not
// found" counts as existing, so the load that follows reports it.
func stateFileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
