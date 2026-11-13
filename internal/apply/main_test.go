package apply

import (
	"os"
	"testing"

	"github.com/mad01/ralph/internal/testutil"
)

// TestMain keeps the developer's RALPH_CONFIG, RALPH_STATE_DIR and
// XDG_CONFIG_HOME out of the tests: with them cleared, a test that points
// HOME at a temp dir keeps every state path inside it.
func TestMain(m *testing.M) {
	testutil.ClearRalphEnv()
	os.Exit(m.Run())
}
