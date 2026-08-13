package buildinfo

import (
	"encoding/json"
	"runtime/debug"
	"testing"
)

// stampedInfo builds the toolchain metadata a binary carries: a module version
// plus optional vcs settings.
func stampedInfo(moduleVersion, revision, vcsTime string) *debug.BuildInfo {
	bi := &debug.BuildInfo{}
	bi.Main.Version = moduleVersion
	if revision != "" {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: "vcs.revision", Value: revision})
	}
	if vcsTime != "" {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: "vcs.time", Value: vcsTime})
	}
	return bi
}

func TestResolve(t *testing.T) {
	const (
		fullSHA = "2917e735a634884fa21ff45a833e2067dc2236be"
		stamped = "2026-08-13T19:40:11Z"
	)

	tests := []struct {
		name   string
		linked Info
		bi     *debug.BuildInfo
		want   Info
	}{
		{
			name:   "ldflags win over stamped metadata",
			linked: Info{Version: "v1.2.3", Commit: "abc", Tag: "v1.2.3", BuildTime: "then"},
			bi:     stampedInfo("(devel)", fullSHA, stamped),
			want:   Info{Version: "v1.2.3", Commit: "abc", Tag: "v1.2.3", BuildTime: "then"},
		},
		{
			name:   "plain go build recovers commit, time and version from vcs stamps",
			linked: Info{Version: devVersion},
			bi:     stampedInfo("(devel)", fullSHA, stamped),
			want:   Info{Version: "2917e73", Commit: fullSHA, Tag: "", BuildTime: stamped},
		},
		{
			name:   "go install @main recovers the commit from the pseudo-version",
			linked: Info{Version: devVersion},
			bi:     stampedInfo("v0.1.1-0.20260813194011-2917e735a634", "", ""),
			want: Info{
				Version: "v0.1.1-0.20260813194011-2917e735a634",
				Commit:  "2917e735a634",
			},
		},
		{
			name:   "go install @v1.2.3 reports the module version as the tag",
			linked: Info{Version: devVersion},
			bi:     stampedInfo("v1.2.3", "", ""),
			want:   Info{Version: "v1.2.3", Tag: "v1.2.3"},
		},
		{
			// Go 1.24+ stamps a pseudo-version for an untagged build instead of
			// "(devel)". It names a commit, not a tag, so tag stays empty.
			name:   "an untagged build reports no tag",
			linked: Info{Version: devVersion},
			bi:     stampedInfo("v0.0.0-20260813194027-de316957922e", fullSHA, stamped),
			want:   Info{Version: "2917e73", Commit: fullSHA, BuildTime: stamped},
		},
		{
			name:   "an untagged dirty build reports no tag",
			linked: Info{Version: devVersion},
			bi:     stampedInfo("v0.0.0-20260813194027-de316957922e+dirty", fullSHA, stamped),
			want:   Info{Version: "2917e73", Commit: fullSHA, BuildTime: stamped},
		},
		{
			// The tree did not match v1.2.3, so claiming that tag would lie.
			name:   "a dirty build off a tag reports no tag",
			linked: Info{Version: devVersion},
			bi:     stampedInfo("v1.2.3+dirty", fullSHA, stamped),
			want:   Info{Version: "2917e73", Commit: fullSHA, BuildTime: stamped},
		},
		{
			name:   "no stamped metadata leaves the linked values untouched",
			linked: Info{Version: devVersion},
			bi:     nil,
			want:   Info{Version: devVersion},
		},
		{
			// The Makefile passes an empty -X outside a git checkout; the version
			// must still be a token rather than an empty line.
			name:   "an empty linked version falls back to dev",
			linked: Info{},
			bi:     stampedInfo("(devel)", "", ""),
			want:   Info{Version: devVersion},
		},
		{
			name:   "an empty linked version with no build info at all falls back to dev",
			linked: Info{},
			bi:     nil,
			want:   Info{Version: devVersion},
		},
		{
			name:   "release build keeps its tag version but gains the stamped commit",
			linked: Info{Version: "0.1.0", Tag: "v0.1.0"},
			bi:     stampedInfo("(devel)", fullSHA, stamped),
			want:   Info{Version: "0.1.0", Commit: fullSHA, Tag: "v0.1.0", BuildTime: stamped},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolve(tt.linked, tt.bi)
			if got != tt.want {
				t.Errorf("resolve() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPseudoVersionCommit(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{"v0.1.1-0.20260813194011-2917e735a634", "2917e735a634"},
		{"v0.0.0-20260813194011-2917e735a634", "2917e735a634"},
		{"v0.0.0-20260813194011-2917e735a634+dirty", "2917e735a634"},
		{"v1.2.3", ""},
		{"v1.2.3+dirty", ""},
		{"", ""},
		{"(devel)", ""},
		{"v1.2.3-rc.1", ""},
		// Right shape, wrong contents: a non-numeric timestamp and a short sha.
		{"v0.0.0-2026081319401x-2917e735a634", ""},
		{"v0.0.0-20260813194011-2917e735a", ""},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := pseudoVersionCommit(tt.version); got != tt.want {
				t.Errorf("pseudoVersionCommit(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestModuleTag(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{"v1.2.3", "v1.2.3"},
		{"v1.2.3-rc.1", "v1.2.3-rc.1"},
		{"v1.2.3+dirty", ""},
		{"v0.0.0-20260813194011-2917e735a634", ""},
		{"v0.0.0-20260813194011-2917e735a634+dirty", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := moduleTag(tt.version); got != tt.want {
				t.Errorf("moduleTag(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

// The four keys are the cross-tool contract: all of them are emitted even when
// the build knew none of the values.
func TestInfoAlwaysMarshalsFourKeys(t *testing.T) {
	b, err := json.Marshal(Info{})
	if err != nil {
		t.Fatalf("marshal Info: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal Info: %v", err)
	}
	for _, key := range []string{"version", "commit", "tag", "build_time"} {
		if _, present := doc[key]; !present {
			t.Errorf("key %q missing from %s", key, b)
		}
	}
	if len(doc) != 4 {
		t.Errorf("Info emitted %d keys (%s), want exactly 4", len(doc), b)
	}
}

// Get must always answer, whatever the binary was built with — a test binary
// carries no ldflags and no vcs stamps, and still reports the "dev" default.
func TestGetAlwaysReportsAVersion(t *testing.T) {
	if got := Get(); got.Version == "" {
		t.Errorf("Get() = %+v, want a non-empty version", got)
	}
}
