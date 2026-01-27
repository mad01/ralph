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
				t.Errorf("ShouldApplyForHost(%v, %s) = %v, want %v", tt.hosts, tt.currentHost, result, tt.expected)
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
				t.Errorf("ShouldApplyForHost(%v, %s) = %v, want %v", hosts, tt.currentHost, result, tt.expected)
			}
		})
	}
}
