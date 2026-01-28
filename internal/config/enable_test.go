package config

import "testing"

func TestIsEnabled_Nil_ReturnsTrue(t *testing.T) {
	result := IsEnabled(nil)
	if !result {
		t.Error("IsEnabled(nil) should return true (default enabled)")
	}
}

func TestIsEnabled_True_ReturnsTrue(t *testing.T) {
	enabled := true
	result := IsEnabled(&enabled)
	if !result {
		t.Error("IsEnabled(&true) should return true")
	}
}

func TestIsEnabled_False_ReturnsFalse(t *testing.T) {
	enabled := false
	result := IsEnabled(&enabled)
	if result {
		t.Error("IsEnabled(&false) should return false")
	}
}
