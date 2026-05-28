package commands

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionCmd_JSONShape(t *testing.T) {
	prev := Version
	Version = "abc1234"
	defer func() { Version = prev }()

	var out string
	withOutputFormat("json", func() {
		out = captureStdout(t, func() {
			if err := versionCmd.RunE(versionCmd, nil); err != nil {
				t.Fatalf("version RunE: %v", err)
			}
		})
	})

	var doc map[string]string
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("version -o json did not emit valid JSON: %v\n%s", err, out)
	}
	if doc["version"] != "abc1234" {
		t.Errorf("version field = %q, want %q", doc["version"], "abc1234")
	}
	if len(doc) != 1 {
		t.Errorf("expected exactly one key (version), got %v", doc)
	}
}

func TestVersionCmd_TextIsPlain(t *testing.T) {
	prev := Version
	Version = "abc1234"
	defer func() { Version = prev }()

	var out string
	withOutputFormat("text", func() {
		out = captureStdout(t, func() {
			if err := versionCmd.RunE(versionCmd, nil); err != nil {
				t.Fatalf("version RunE: %v", err)
			}
		})
	})
	if strings.TrimSpace(out) != "abc1234" {
		t.Errorf("text version = %q, want %q", strings.TrimSpace(out), "abc1234")
	}
}
