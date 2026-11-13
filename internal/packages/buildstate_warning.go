package packages

import (
	"fmt"
	"os"
	"sync"

	"github.com/mad01/ralph/internal/buildstate"
)

// buildStateWarning makes the unreadable-build-state warning print at most
// once per process, however many packages a run builds. A pointer so tests
// can reset it.
var buildStateWarning = new(sync.Once)

// loadBuildState is buildstate.LoadBuildState that also reports a load failure
// on stderr, once per process. An unreadable .builds_state makes every package
// rebuild on every run, and the per-package notes reach only the verbose
// writer, so without this a normal run rebuilds everything with no reason
// given.
func loadBuildState() (*buildstate.BuildState, error) {
	state, err := buildstate.LoadBuildState()
	if err != nil {
		buildStateWarning.Do(func() { warnUnreadableBuildState(err) })
	}
	return state, err
}

func warnUnreadableBuildState(err error) {
	path, pathErr := buildstate.GetStateFilePath()
	if pathErr != nil {
		path = ".builds_state"
	}
	fmt.Fprintf(
		os.Stderr,
		"Warning: build state %s is unreadable (%v); every package rebuilds on each run "+
			"until 'ralph up --reset-builds' resets it\n",
		path,
		err,
	)
}
