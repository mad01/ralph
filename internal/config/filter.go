package config

// Skip reasons SkipReason returns, in the order it checks them.
const (
	SkipDisabled = "disabled"
	SkipHost     = "host filter"
	SkipProfile  = "profile filter"
)

// SkipReason reports why an item with these enable, hosts and profiles fields
// does not apply on a machine with this host and these profiles: SkipDisabled,
// SkipHost or SkipProfile, checked in that order. It returns "" when the item
// applies.
func SkipReason(
	enable *bool,
	hosts, profiles []string,
	host string,
	machineProfiles []string,
) string {
	switch {
	case !IsEnabled(enable):
		return SkipDisabled
	case !ShouldApplyForHost(hosts, host):
		return SkipHost
	case !ShouldApplyForProfiles(profiles, machineProfiles):
		return SkipProfile
	}
	return ""
}

// Applies reports whether an item is enabled and passes its host and profile
// filters on this machine; it is SkipReason without the reason.
func Applies(enable *bool, hosts, profiles []string, host string, machineProfiles []string) bool {
	return SkipReason(enable, hosts, profiles, host, machineProfiles) == ""
}
