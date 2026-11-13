package config

import "github.com/mad01/ralph/internal/execx"

// DefaultExecTimeout is the fallback timeout for build and package commands
// (and git operations) when no explicit timeout is configured.
const DefaultExecTimeout = execx.DefaultTimeout
