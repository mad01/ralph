package config

// IsEnabled checks if a config item is enabled.
// nil or true = enabled, false = disabled
func IsEnabled(enable *bool) bool {
	if enable == nil {
		return true
	}
	return *enable
}
