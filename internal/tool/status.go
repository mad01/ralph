package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mad01/ralph/internal/execx"
)

// checkTimeout bounds a tool's check_command. Checks are cheap probes such as
// `command -v` or `--version`, so this is short; it matches the doctor's
// verify timeout. A var so tests can shorten it.
var checkTimeout = 30 * time.Second

// CheckStatus runs the tool's check_command and reports whether it exits 0.
// An empty command reports false. The error is non-nil only when the check
// did not finish within its timeout, so callers can tell a hung check from a
// missing tool.
func CheckStatus(checkCommand string) (bool, error) {
	if strings.TrimSpace(checkCommand) == "" {
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	// "sh -c" allows shell constructs in the check_command (pipes, command -v).
	// The command comes from the user's own config.
	err := execx.Command(ctx, "sh", "-c", checkCommand).Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return false, fmt.Errorf("check timed out after %s: %s", checkTimeout, checkCommand)
	}
	return err == nil, nil
}
