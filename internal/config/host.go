package config

import (
	"os"
	"slices"
	"strings"
)

// GetCurrentHost returns the normalized short hostname of the current machine
func GetCurrentHost() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	return normalizeHost(hostname)
}

// normalizeHost lowercases a hostname and strips everything after the first
// dot. macOS can report "name.local" or a DHCP/VPN-assigned FQDN from
// os.Hostname(); the canonical form for hosts filters is the short name.
func normalizeHost(host string) string {
	host = strings.ToLower(host)
	if i := strings.IndexByte(host, '.'); i >= 0 {
		host = host[:i]
	}
	return host
}

// ShouldApplyForHost checks if an action should apply based on hosts list.
// Empty/nil hosts list means apply to all hosts. Both sides are normalized
// to lowercase short names before comparison.
func ShouldApplyForHost(hosts []string, currentHost string) bool {
	if len(hosts) == 0 {
		return true
	}
	currentHost = normalizeHost(currentHost)
	for _, h := range hosts {
		if normalizeHost(h) == currentHost {
			return true
		}
	}
	return false
}

// ShouldApplyForProfiles checks if a recipe or item should apply based on its
// profile labels and the machine's profiles. Empty/nil recipeProfiles means
// apply everywhere. Otherwise it applies when the two sets intersect: at least
// one recipe profile is present in machineProfiles. Comparison is exact
// (profiles are freeform strings, not normalized like hostnames).
func ShouldApplyForProfiles(recipeProfiles, machineProfiles []string) bool {
	if len(recipeProfiles) == 0 {
		return true
	}
	for _, rp := range recipeProfiles {
		if slices.Contains(machineProfiles, rp) {
			return true
		}
	}
	return false
}
