package commands

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/buildinfo"
)

// withBuildInfo pins every link-time build variable for the duration of fn, so
// the assertions never depend on how the test binary itself was built.
func withBuildInfo(t *testing.T, info buildinfo.Info, fn func()) {
	t.Helper()
	prev := buildinfo.Info{
		Version:   buildinfo.Version,
		Commit:    buildinfo.Commit,
		Tag:       buildinfo.Tag,
		BuildTime: buildinfo.BuildTime,
	}
	defer func() {
		buildinfo.Version, buildinfo.Commit = prev.Version, prev.Commit
		buildinfo.Tag, buildinfo.BuildTime = prev.Tag, prev.BuildTime
	}()
	buildinfo.Version, buildinfo.Commit = info.Version, info.Commit
	buildinfo.Tag, buildinfo.BuildTime = info.Tag, info.BuildTime
	fn()
}

// The four-key object is the cross-tool contract other tools probe, so the key
// set is pinned exactly.
func TestVersionCmd_JSONShape(t *testing.T) {
	info := buildinfo.Info{
		Version:   "abc1234",
		Commit:    "abc1234def5678901234567890123456789abcde",
		Tag:       "v0.1.0",
		BuildTime: "2026-08-13T19:40:11Z",
	}

	var out string
	withBuildInfo(t, info, func() {
		withOutputFormat("json", func() {
			out = captureStdout(t, func() {
				if err := versionCmd.RunE(versionCmd, nil); err != nil {
					t.Fatalf("version RunE: %v", err)
				}
			})
		})
	})

	var doc map[string]string
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("version -o json did not emit valid JSON: %v\n%s", err, out)
	}
	want := map[string]string{
		"version":    info.Version,
		"commit":     info.Commit,
		"tag":        info.Tag,
		"build_time": info.BuildTime,
	}
	for key, wantValue := range want {
		if doc[key] != wantValue {
			t.Errorf("%s field = %q, want %q", key, doc[key], wantValue)
		}
	}
	if len(doc) != len(want) {
		t.Errorf("expected exactly %d keys, got %v", len(want), doc)
	}
	if !strings.Contains(out, "\n  \"version\"") {
		t.Errorf("expected 2-space indented JSON, got:\n%s", out)
	}
}

// Unknown fields stay in the output as empty strings — consumers parse one
// fixed shape whatever the build knew.
func TestVersionCmd_JSONKeepsUnknownFieldsEmpty(t *testing.T) {
	var out string
	withBuildInfo(t, buildinfo.Info{Version: "abc1234"}, func() {
		withOutputFormat("json", func() {
			out = captureStdout(t, func() {
				if err := versionCmd.RunE(versionCmd, nil); err != nil {
					t.Fatalf("version RunE: %v", err)
				}
			})
		})
	})

	var doc map[string]string
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("version -o json did not emit valid JSON: %v\n%s", err, out)
	}
	for _, key := range []string{"version", "commit", "tag", "build_time"} {
		if _, present := doc[key]; !present {
			t.Errorf("key %q missing from %s", key, out)
		}
	}
	if doc["tag"] != "" {
		t.Errorf("tag = %q, want empty (nothing set it)", doc["tag"])
	}
}

func TestVersionCmd_TextIsPlain(t *testing.T) {
	var out string
	withBuildInfo(t, buildinfo.Info{Version: "abc1234", Tag: "v0.1.0"}, func() {
		withOutputFormat("text", func() {
			out = captureStdout(t, func() {
				if err := versionCmd.RunE(versionCmd, nil); err != nil {
					t.Fatalf("version RunE: %v", err)
				}
			})
		})
	})
	if strings.TrimSpace(out) != "abc1234" {
		t.Errorf("text version = %q, want %q", strings.TrimSpace(out), "abc1234")
	}
}
