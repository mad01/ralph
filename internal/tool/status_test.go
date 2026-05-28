package tool

import "testing"

func TestCheckStatus(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		want    bool
	}{
		{"true succeeds", "true", true},
		{"false fails", "false", false},
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"echo succeeds", "echo hello", true},
		{"nonexistent command", "nonexistent_command_abc123", false},
		{"command -v existing", "command -v sh", true},
		{"command -v missing", "command -v nonexistent_cmd_xyz", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckStatus(tt.cmd)
			if got != tt.want {
				t.Errorf("CheckStatus(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}
