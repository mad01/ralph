package config

import (
	"os"
	"strings"
)

// GetCurrentHost returns the lowercase hostname of the current machine
func GetCurrentHost() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	return strings.ToLower(hostname)
}

// ShouldApplyForHost checks if an action should apply based on hosts list.
// Empty/nil hosts list means apply to all hosts.
func ShouldApplyForHost(hosts []string, currentHost string) bool {
	if len(hosts) == 0 {
		return true
	}
	for _, h := range hosts {
		if strings.ToLower(h) == currentHost {
			return true
		}
	}
	return false
}
