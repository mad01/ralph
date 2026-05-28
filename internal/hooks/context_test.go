package hooks

import (
	"context"
	"io"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/testutil"
)

func TestRunBuild_CancelledContext(t *testing.T) {
	_ = testutil.WithHome(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	build := config.Build{
		Commands: []string{"sleep 10"},
		Run:      "always",
	}

	err := RunBuild(ctx, io.Discard, "ctx_test", build, "testhost", BuildOptions{})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
