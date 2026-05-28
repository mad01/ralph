package hooks

import (
	"bytes"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/testutil"
)

func TestResetBuildState_WritesToWriter(t *testing.T) {
	tmpDir := testutil.WithHome(t)
	testutil.EnsureRalphDir(t, tmpDir)

	state := &BuildState{
		Builds: map[string]BuildRecord{
			"b": {CompletedAt: time.Now()},
		},
	}
	if err := SaveBuildState(state); err != nil {
		t.Fatalf("save: %v", err)
	}

	var buf bytes.Buffer
	if err := ResetBuildState(&buf); err != nil {
		t.Fatalf("reset: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("reset")) {
		t.Errorf("expected 'reset' in output, got: %s", buf.String())
	}
}

func TestResetBuildStateForName_WritesToWriter(t *testing.T) {
	_ = testutil.WithHome(t)

	state := &BuildState{
		Builds: map[string]BuildRecord{
			"keep":   {CompletedAt: time.Now()},
			"delete": {CompletedAt: time.Now()},
		},
	}
	if err := SaveBuildState(state); err != nil {
		t.Fatalf("save: %v", err)
	}

	var buf bytes.Buffer
	if err := ResetBuildStateForName(&buf, "delete"); err != nil {
		t.Fatalf("reset: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("delete")) {
		t.Errorf("expected build name in output, got: %s", buf.String())
	}
}
