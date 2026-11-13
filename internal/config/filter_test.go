package config

import "testing"

func TestSkipReason(t *testing.T) {
	on, off := true, false
	tests := []struct {
		name     string
		enable   *bool
		hosts    []string
		profiles []string
		want     string
	}{
		{name: "no filters applies", want: ""},
		{name: "enabled explicitly applies", enable: &on, want: ""},
		{
			name:     "matching host and profile applies",
			hosts:    []string{"Laptop"},
			profiles: []string{"work"},
		},
		{name: "disabled", enable: &off, want: SkipDisabled},
		{name: "other host", hosts: []string{"desktop"}, want: SkipHost},
		{name: "other profile", profiles: []string{"personal"}, want: SkipProfile},
		{
			name:     "disabled wins over host and profile",
			enable:   &off,
			hosts:    []string{"desktop"},
			profiles: []string{"personal"},
			want:     SkipDisabled,
		},
		{
			name:     "host wins over profile",
			hosts:    []string{"desktop"},
			profiles: []string{"personal"},
			want:     SkipHost,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SkipReason(tt.enable, tt.hosts, tt.profiles, "laptop", []string{"work"})
			if got != tt.want {
				t.Errorf("SkipReason() = %q, want %q", got, tt.want)
			}
			if applies := Applies(tt.enable, tt.hosts, tt.profiles, "laptop", []string{"work"}); applies != (tt.want == "") {
				t.Errorf("Applies() = %v, want %v", applies, tt.want == "")
			}
		})
	}
}
