package config

import "time"

// DefaultExecTimeout is the fallback timeout for build and package commands
// (and git operations) when no explicit timeout is configured.
const DefaultExecTimeout = 10 * time.Minute
