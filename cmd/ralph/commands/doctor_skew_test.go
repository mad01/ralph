package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/buildinfo"
	"github.com/mad01/ralph/internal/gitutil"
	"github.com/mad01/ralph/internal/testutil"
)

// packageSkew is doctor's staleness verdict for a built package. These tests
// pin the three skew classes (poisoned record, moved subtree, outdated
// installed binary) and the fresh/no-verdict paths.

// commitChange writes a file and commits it, returning the new commit hash.
func commitChange(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.RunGitCmd(t, dir, "add", ".")
	testutil.RunGitCmd(t, dir, "commit", "-m", "change "+name)
	return gitutil.GetGitHash(dir)
}

func TestPackageSkew_EmptyRecordedHash(t *testing.T) {
	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)

	msg, skewed := packageSkew(dir, "", "")
	if !skewed || !strings.Contains(msg, "no recorded source hash") {
		t.Errorf(
			"packageSkew with empty recorded hash = (%q, %v), want poisoned-record warning",
			msg,
			skewed,
		)
	}
}

func TestPackageSkew_SourceChanged(t *testing.T) {
	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)
	staleHash := gitutil.GetTreeHash(dir)
	commitChange(t, dir, "new.txt", "v2")

	msg, skewed := packageSkew(dir, staleHash, "")
	if !skewed || !strings.Contains(msg, "source changed since last build") {
		t.Errorf(
			"packageSkew with moved subtree = (%q, %v), want source-changed warning",
			msg,
			skewed,
		)
	}
}

func TestPackageSkew_BinaryPredatesChanges(t *testing.T) {
	dir := t.TempDir()
	oldCommit := testutil.InitGitRepo(t, dir)
	commitChange(t, dir, "new.txt", "v2")

	// State is healthy (recorded hash matches HEAD) but the installed binary
	// reports a commit from before the subtree changed.
	msg, skewed := packageSkew(dir, gitutil.GetTreeHash(dir), oldCommit)
	if !skewed || !strings.Contains(msg, "predates source changes") {
		t.Errorf(
			"packageSkew with outdated binary = (%q, %v), want binary-skew warning",
			msg,
			skewed,
		)
	}
}

func TestPackageSkew_Fresh(t *testing.T) {
	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)
	head := gitutil.GetGitHash(dir)

	if msg, skewed := packageSkew(dir, gitutil.GetTreeHash(dir), head); skewed {
		t.Errorf("packageSkew on a fresh package = (%q, %v), want no skew", msg, skewed)
	}
}

func TestPackageSkew_NoVerdictOutsideGit(t *testing.T) {
	if msg, skewed := packageSkew(t.TempDir(), "somehash", ""); skewed {
		t.Errorf("packageSkew outside a git repo = (%q, %v), want no verdict", msg, skewed)
	}
	if msg, skewed := packageSkew("", "somehash", ""); skewed {
		t.Errorf("packageSkew with empty workDir = (%q, %v), want no verdict", msg, skewed)
	}
}

// A ref git cannot resolve — a release tag from the tool's own repo, say —
// must not produce a verdict, in either direction.
func TestPackageSkew_UnresolvableRefGivesNoVerdict(t *testing.T) {
	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)
	commitChange(t, dir, "new.txt", "v2")

	if msg, skewed := packageSkew(dir, gitutil.GetTreeHash(dir), "keep/v0.4.0"); skewed {
		t.Errorf("packageSkew with an unresolvable ref = (%q, %v), want no verdict", msg, skewed)
	}
}

// stalenessRef is what makes the skew check work across the contract change:
// new binaries report a commit, older ones only a version.
func TestStalenessRef(t *testing.T) {
	tests := []struct {
		name string
		info buildinfo.Info
		want string
	}{
		{
			name: "prefers the reported commit",
			info: buildinfo.Info{Version: "abc1234", Commit: "abc1234def5678", Tag: "v0.4.0"},
			want: "abc1234def5678",
		},
		{
			name: "falls back to version when the binary reports no commit",
			info: buildinfo.Info{Version: "abc1234"},
			want: "abc1234",
		},
		{
			name: "nothing reported yields no ref",
			info: buildinfo.Info{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stalenessRef(tt.info); got != tt.want {
				t.Errorf("stalenessRef(%+v) = %q, want %q", tt.info, got, tt.want)
			}
		})
	}
}

func TestInstalledVersionNote(t *testing.T) {
	tests := []struct {
		name   string
		info   buildinfo.Info
		probed bool
		want   string
	}{
		{
			name: "full metadata",
			info: buildinfo.Info{
				Version:   "abc1234",
				Commit:    "abc1234def5678901234567890123456789abcde",
				Tag:       "keep/v0.4.0",
				BuildTime: "2026-08-13T19:40:11Z",
			},
			probed: true,
			want:   " (installed abc1234d, tag keep/v0.4.0, built 2026-08-13T19:40Z)",
		},
		{
			name:   "version only, as older tools report",
			info:   buildinfo.Info{Version: "abc1234"},
			probed: true,
			want:   " (installed abc1234)",
		},
		{
			name:   "no tag, with a build time",
			info:   buildinfo.Info{Version: "abc1234", BuildTime: "2026-08-13T19:40:11Z"},
			probed: true,
			want:   " (installed abc1234, built 2026-08-13T19:40Z)",
		},
		{
			name:   "unparseable build time is passed through",
			info:   buildinfo.Info{Version: "abc1234", BuildTime: "yesterday"},
			probed: true,
			want:   " (installed abc1234, built yesterday)",
		},
		{
			name:   "no probe, no note",
			info:   buildinfo.Info{Version: "abc1234"},
			probed: false,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := installedVersionNote(tt.info, tt.probed); got != tt.want {
				t.Errorf("installedVersionNote() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A non-UTC build time is normalised, so notes from different builders compare.
func TestShortBuildTimeNormalisesToUTC(t *testing.T) {
	if got := shortBuildTime("2026-08-13T21:40:11+02:00"); got != "2026-08-13T19:40Z" {
		t.Errorf("shortBuildTime() = %q, want %q", got, "2026-08-13T19:40Z")
	}
}

func TestBuildMeta(t *testing.T) {
	info := buildinfo.Info{
		Version:   "abc1234",
		Commit:    "abc1234def5678901234567890123456789abcde",
		Tag:       "keep/v0.4.0",
		BuildTime: "2026-08-13T19:40:11Z",
	}

	got := buildMeta(info, true)
	if got == nil {
		t.Fatal("buildMeta(probed) = nil, want the projected metadata")
	}
	if got.Version != info.Version || got.Commit != info.Commit || got.Tag != info.Tag ||
		got.BuildTime != info.BuildTime {
		t.Errorf("buildMeta() = %+v, want the same four fields as %+v", got, info)
	}
	if buildMeta(info, false) != nil {
		t.Error("buildMeta(unprobed) should be nil so JSON omits the field")
	}
}
