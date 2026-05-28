package packages

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/testutil"
)

// On a branch with no upstream tracking, GitPull should detect the "no
// tracking information" error and attempt the origin/<branch> fallback. This
// must work in verbose mode too — previously stderr went only to os.Stderr in
// verbose mode, so the fallback never fired (#22).
func TestGitPull_NoTrackingFallbackFiresInVerboseMode(t *testing.T) {
	origin := t.TempDir()
	testutil.InitGitRepo(t, origin)

	clone := t.TempDir()
	testutil.RunGitCmd(t, t.TempDir(), "clone", origin, clone)
	// Create a local branch with no upstream.
	testutil.RunGitCmd(t, clone, "checkout", "-b", "orphanbranch")

	var buf bytes.Buffer
	// verbose=true is the regression case.
	_ = GitPull(context.Background(), &buf, clone, false, true)

	if !strings.Contains(buf.String(), "No tracking info, pulling origin/orphanbranch") {
		t.Errorf("expected no-tracking fallback to fire in verbose mode, output:\n%s", buf.String())
	}
}
