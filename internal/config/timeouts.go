package config

import (
	"time"

	"github.com/mad01/ralph/internal/execx"
)

// DefaultExecTimeout is the fallback timeout for build and package commands
// (and git operations) when no explicit timeout is configured.
const DefaultExecTimeout = execx.DefaultTimeout

// ExecTimeout turns a build or package timeout field (seconds) into a
// duration. Zero means DefaultExecTimeout.
func ExecTimeout(seconds int) time.Duration {
	if seconds == 0 {
		return DefaultExecTimeout
	}
	return time.Duration(seconds) * time.Second
}
