package config

import (
	"strings"
	"testing"
)

func TestGetCurrentHost_ReturnsLowercase(t *testing.T) {
	host := GetCurrentHost()
	if host != strings.ToLower(host) {
		t.Errorf("GetCurrentHost() returned non-lowercase hostname: %s", host)
	}
}

func TestShouldApplyForHost_EmptyHosts_ReturnsTrue(t *testing.T) {
	if !ShouldApplyForHost([]string{}, "anyhost") {
		t.Error("ShouldApplyForHost() with empty hosts should return true")
	}
}

func TestShouldApplyForHost_NilHosts_ReturnsTrue(t *testing.T) {
	if !ShouldApplyForHost(nil, "anyhost") {
		t.Error("ShouldApplyForHost() with nil hosts should return true")
	}
}

func TestShouldApplyForHost_MatchingHost_ReturnsTrue(t *testing.T) {
	if !ShouldApplyForHost([]string{"myhost"}, "myhost") {
		t.Error("ShouldApplyForHost() with matching host should return true")
	}
}

func TestShouldApplyForHost_NonMatchingHost_ReturnsFalse(t *testing.T) {
	if ShouldApplyForHost([]string{"otherhost"}, "myhost") {
		t.Error("ShouldApplyForHost() with non-matching host should return false")
	}
}

func TestShouldApplyForHost_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name        string
		hosts       []string
		currentHost string
		expected    bool
	}{
		{
			name:        "uppercase in hosts list",
			hosts:       []string{"MYHOST"},
			currentHost: "myhost",
			expected:    true,
		},
		{
			name:        "mixed case in hosts list",
			hosts:       []string{"MyHost"},
			currentHost: "myhost",
			expected:    true,
		},
		{
			name:        "uppercase current host",
			hosts:       []string{"myhost"},
			currentHost: "myhost", // GetCurrentHost always returns lowercase
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldApplyForHost(tt.hosts, tt.currentHost)
			if result != tt.expected {
				t.Errorf(
					"ShouldApplyForHost(%v, %s) = %v, want %v",
					tt.hosts,
					tt.currentHost,
					result,
					tt.expected,
				)
			}
		})
	}
}

func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "short name unchanged", input: "myhost", expected: "myhost"},
		{name: "strips .local suffix", input: "myhost.local", expected: "myhost"},
		{name: "strips FQDN domain", input: "myhost.example.com", expected: "myhost"},
		{name: "lowercases", input: "MyHost", expected: "myhost"},
		{name: "lowercases and strips", input: "MyHost.Local", expected: "myhost"},
		{name: "empty string", input: "", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeHost(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeHost(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestShouldApplyForHost_NormalizesHostnames(t *testing.T) {
	tests := []struct {
		name        string
		hosts       []string
		currentHost string
		expected    bool
	}{
		{
			name:        ".local current host matches short name in hosts",
			hosts:       []string{"myhost"},
			currentHost: "myhost.local",
			expected:    true,
		},
		{
			name:        "FQDN current host matches short name in hosts",
			hosts:       []string{"myhost"},
			currentHost: "myhost.example.com",
			expected:    true,
		},
		{
			name:        ".local entry in hosts matches short current host",
			hosts:       []string{"myhost.local"},
			currentHost: "myhost",
			expected:    true,
		},
		{
			name:        "different short names do not match",
			hosts:       []string{"otherhost"},
			currentHost: "myhost.local",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldApplyForHost(tt.hosts, tt.currentHost)
			if result != tt.expected {
				t.Errorf(
					"ShouldApplyForHost(%v, %s) = %v, want %v",
					tt.hosts,
					tt.currentHost,
					result,
					tt.expected,
				)
			}
		})
	}
}

func TestShouldApplyForHost_MultipleHosts(t *testing.T) {
	hosts := []string{"host1", "host2", "host3"}

	tests := []struct {
		name        string
		currentHost string
		expected    bool
	}{
		{
			name:        "first host matches",
			currentHost: "host1",
			expected:    true,
		},
		{
			name:        "middle host matches",
			currentHost: "host2",
			expected:    true,
		},
		{
			name:        "last host matches",
			currentHost: "host3",
			expected:    true,
		},
		{
			name:        "no host matches",
			currentHost: "host4",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldApplyForHost(hosts, tt.currentHost)
			if result != tt.expected {
				t.Errorf(
					"ShouldApplyForHost(%v, %s) = %v, want %v",
					hosts,
					tt.currentHost,
					result,
					tt.expected,
				)
			}
		})
	}
}

func TestShouldApplyForProfiles(t *testing.T) {
	tests := []struct {
		name            string
		recipeProfiles  []string
		machineProfiles []string
		expected        bool
	}{
		{
			name:            "empty recipe profiles applies everywhere",
			recipeProfiles:  nil,
			machineProfiles: []string{"personal"},
			expected:        true,
		},
		{
			name:            "empty recipe profiles applies with no machine profiles",
			recipeProfiles:  []string{},
			machineProfiles: nil,
			expected:        true,
		},
		{
			name:            "single intersection",
			recipeProfiles:  []string{"personal"},
			machineProfiles: []string{"personal"},
			expected:        true,
		},
		{
			name:            "disjoint sets",
			recipeProfiles:  []string{"work"},
			machineProfiles: []string{"personal"},
			expected:        false,
		},
		{
			name:            "non-empty recipe profiles, empty machine profiles",
			recipeProfiles:  []string{"personal"},
			machineProfiles: nil,
			expected:        false,
		},
		{
			name:            "multi-profile overlap",
			recipeProfiles:  []string{"work", "homelab"},
			machineProfiles: []string{"personal", "homelab"},
			expected:        true,
		},
		{
			name:            "multi-profile no overlap",
			recipeProfiles:  []string{"work", "homelab"},
			machineProfiles: []string{"personal", "home"},
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldApplyForProfiles(tt.recipeProfiles, tt.machineProfiles)
			if result != tt.expected {
				t.Errorf(
					"ShouldApplyForProfiles(%v, %v) = %v, want %v",
					tt.recipeProfiles,
					tt.machineProfiles,
					result,
					tt.expected,
				)
			}
		})
	}
}
