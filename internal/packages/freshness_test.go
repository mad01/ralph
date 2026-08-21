package packages

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/buildinfo"
	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/gitutil"
	"github.com/mad01/ralph/internal/testutil"
)

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeVersionExecutable(t *testing.T, path, version, commit string) {
	t.Helper()
	body := fmt.Sprintf(
		"#!/bin/sh\nprintf '%%s\\n' '{\"version\":\"%s\",\"commit\":\"%s\",\"tag\":\"\",\"build_time\":\"\"}'\n",
		version,
		commit,
	)
	writeExecutable(t, path, body)
}

func installRecord(t *testing.T, pkg config.Package) buildstate.BuildRecord {
	t.Helper()
	hash := computeInstallHash(pkg)
	if hash == "" {
		t.Fatal("expected a non-empty install hash")
	}
	return buildstate.BuildRecord{InstallHash: hash}
}

func wrongRevision(head string) string {
	first := byte('0')
	if head[0] == first {
		first = '1'
	}
	return string(first) + head[1:]
}

func TestCheckInstalledPackageVersionAndHash(t *testing.T) {
	tests := []struct {
		name       string
		version    func(string) string
		commit     func(string) string
		prepare    func(*testing.T, config.Package, *buildstate.BuildRecord)
		wantReason string
	}{
		{
			name:    "full commit match",
			version: func(head string) string { return head[:8] },
			commit:  func(head string) string { return head },
		},
		{
			name:       "full commit mismatch",
			version:    func(head string) string { return head[:8] },
			commit:     wrongRevision,
			wantReason: "does not match source HEAD",
		},
		{
			name:    "legacy short SHA match",
			version: func(head string) string { return head[:8] },
			commit:  func(string) string { return "" },
		},
		{
			name:       "legacy short SHA mismatch",
			version:    func(head string) string { return wrongRevision(head)[:8] },
			commit:     func(string) string { return "" },
			wantReason: "does not match source HEAD",
		},
		{
			name:       "semantic version is rejected",
			version:    func(string) string { return "v1.2.3" },
			commit:     func(string) string { return "" },
			wantReason: "did not report a SHA-shaped",
		},
		{
			name:    "semantic version rejection precedes hash mismatch",
			version: func(string) string { return "v1.2.3" },
			commit:  func(string) string { return "" },
			prepare: func(_ *testing.T, _ config.Package, record *buildstate.BuildRecord) {
				record.InstallHash = strings.Repeat("0", 64)
			},
			wantReason: "did not report a SHA-shaped",
		},
		{
			name:    "matching version without historical hash seeds integrity state",
			version: func(head string) string { return head[:8] },
			commit:  func(head string) string { return head },
			prepare: func(_ *testing.T, _ config.Package, record *buildstate.BuildRecord) {
				record.InstallHash = ""
			},
			wantReason: "no recorded install hash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			workDir := filepath.Join(root, "repo")
			head := testutil.InitGitRepo(t, workDir)
			binPath := filepath.Join(root, "bin", "tool")
			writeVersionExecutable(t, binPath, tt.version(head), tt.commit(head))
			pkg := config.Package{
				InstallPaths: []string{binPath},
				VersionCheck: true,
			}
			record := installRecord(t, pkg)
			if tt.prepare != nil {
				tt.prepare(t, pkg, &record)
			}

			check := CheckInstalledPackage(pkg, workDir, record)
			if !strings.Contains(check.Reason, tt.wantReason) {
				t.Fatalf("CheckInstalledPackage reason = %q, want substring %q", check.Reason, tt.wantReason)
			}
		})
	}
}

func TestCheckInstalledPackageRejectsUnsupportedProbe(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "repo")
	testutil.InitGitRepo(t, workDir)
	binPath := filepath.Join(root, "bin", "tool")
	writeExecutable(t, binPath, "#!/bin/sh\nexit 2\n")
	pkg := config.Package{InstallPaths: []string{binPath}, VersionCheck: true}
	record := installRecord(t, pkg)

	if check := CheckInstalledPackage(pkg, workDir, record); !strings.Contains(check.Reason, "does not support version -o json") {
		t.Fatalf("unsupported probe reason = %q", check.Reason)
	}
}

func TestCheckInstalledPackageRejectsNonSHARevision(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "repo")
	testutil.InitGitRepo(t, workDir)
	binPath := filepath.Join(root, "bin", "tool")
	writeVersionExecutable(t, binPath, "v1.2.3", "")
	pkg := config.Package{InstallPaths: []string{binPath}, VersionCheck: true}
	record := installRecord(t, pkg)

	if check := CheckInstalledPackage(pkg, workDir, record); !strings.Contains(check.Reason, "did not report a SHA-shaped") {
		t.Fatalf("non-SHA probe reason = %q", check.Reason)
	}
}

func TestCheckInstalledPackageDoesNotProbeWithoutOptIn(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "repo")
	testutil.InitGitRepo(t, workDir)
	marker := filepath.Join(root, "probed")
	binPath := filepath.Join(root, "bin", "tool")
	writeExecutable(t, binPath, fmt.Sprintf("#!/bin/sh\ntouch %q\nexit 2\n", marker))
	pkg := config.Package{InstallPaths: []string{binPath}}
	record := installRecord(t, pkg)

	if check := CheckInstalledPackage(pkg, workDir, record); check.Reason != "" {
		t.Fatalf("check reason = %q, want empty", check.Reason)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("non-opted-in binary was executed, stat error = %v", err)
	}
}

func TestReportedGitRevisionRejectsSemanticVersion(t *testing.T) {
	if got := ReportedGitRevision(buildinfo.Info{Version: "v1.2.3"}); got != "" {
		t.Fatalf("ReportedGitRevision(semantic version) = %q, want empty", got)
	}
}

func TestGitRevisionMatchesSHA256Head(t *testing.T) {
	head := strings.Repeat("a", 64)
	if !GitRevisionMatchesHead(head, head) {
		t.Fatal("full SHA-256 revision should match its HEAD")
	}
	if !GitRevisionMatchesHead(head[:12], head) {
		t.Fatal("abbreviated SHA-256 revision should match its HEAD")
	}
}

func TestBuildPackageRejectsStaleBinaryAfterNoOpInstall(t *testing.T) {
	home := testutil.WithHome(t)
	workDir := filepath.Join(home, "repo")
	head := testutil.InitGitRepo(t, workDir)
	binPath := filepath.Join(home, "code", "bin", "tool")
	writeVersionExecutable(t, binPath, wrongRevision(head)[:8], "")
	pkg := config.Package{
		Source:       "local",
		WorkingDir:   workDir,
		Build:        []string{"true"},
		Install:      []string{"true"},
		InstallPaths: []string{binPath},
		VersionCheck: true,
	}
	record := installRecord(t, pkg)
	record.CompletedAt = time.Unix(1, 0)
	record.GitHash = gitutil.GetTreeHash(workDir)
	testutil.SaveBuildStateJSON(t, home, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{"pkg:tool": record},
	})

	var output bytes.Buffer
	result := BuildPackage(context.Background(), &output, "tool", pkg, BuildOptions{})
	if result.Action != "error" || result.Message != "installed version validation failed" {
		t.Fatalf("BuildPackage result = %+v, want post-install validation error\n%s", result, output.String())
	}
	state, err := buildstate.LoadBuildState()
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Builds["pkg:tool"].CompletedAt; !got.Equal(record.CompletedAt) {
		t.Fatalf("failed validation changed successful-build state: got %v want %v", got, record.CompletedAt)
	}
}

func TestBuildPackageAcceptsMatchingVersionAfterInstall(t *testing.T) {
	home := testutil.WithHome(t)
	workDir := filepath.Join(home, "repo")
	head := testutil.InitGitRepo(t, workDir)
	binPath := filepath.Join(home, "code", "bin", "tool")
	writeVersionExecutable(t, binPath, head[:8], head)
	pkg := config.Package{
		Source:       "local",
		WorkingDir:   workDir,
		Build:        []string{"true"},
		Install:      []string{"true"},
		InstallPaths: []string{binPath},
		VersionCheck: true,
	}

	var output bytes.Buffer
	result := BuildPackage(context.Background(), &output, "tool", pkg, BuildOptions{Force: true})
	if result.Action != "built" {
		t.Fatalf("BuildPackage result = %+v, want built\n%s", result, output.String())
	}
	state, err := buildstate.LoadBuildState()
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Builds["pkg:tool"].InstallHash; got == "" {
		t.Fatal("successful version validation did not save install hash")
	}
}

func TestEmptyInstallHashRebuildsOnceThenSkips(t *testing.T) {
	home := testutil.WithHome(t)
	workDir := filepath.Join(home, "repo")
	head := testutil.InitGitRepo(t, workDir)
	binPath := filepath.Join(home, "code", "bin", "tool")
	writeVersionExecutable(t, binPath, head[:8], head)
	pkg := config.Package{
		Source:       "local",
		WorkingDir:   workDir,
		Build:        []string{"true"},
		Install:      []string{"true"},
		InstallPaths: []string{binPath},
		VersionCheck: true,
	}
	testutil.SaveBuildStateJSON(t, home, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:tool": {GitHash: gitutil.GetTreeHash(workDir)},
		},
	})

	var output bytes.Buffer
	first := BuildPackage(context.Background(), &output, "tool", pkg, BuildOptions{})
	if first.Action != "built" {
		t.Fatalf("first BuildPackage result = %+v, want built\n%s", first, output.String())
	}
	output.Reset()
	second := BuildPackage(context.Background(), &output, "tool", pkg, BuildOptions{})
	if second.Action != "up-to-date" {
		t.Fatalf("second BuildPackage result = %+v, want up-to-date\n%s", second, output.String())
	}
}

func TestGoInstallDoesNotOverwriteMalformedBuildState(t *testing.T) {
	home := testutil.WithHome(t)
	statePath := filepath.Join(home, ".config", "ralph", ".builds_state")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	malformed := []byte("not-json")
	if err := os.WriteFile(statePath, malformed, 0o644); err != nil {
		t.Fatal(err)
	}

	fakeBin := filepath.Join(home, "fake-bin")
	writeExecutable(
		t,
		filepath.Join(fakeBin, "go"),
		"#!/bin/sh\nprintf 'new-binary' > \"$GOBIN/tool\"\nchmod 755 \"$GOBIN/tool\"\n",
	)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	pkg := config.Package{
		Source:       "go-install",
		Module:       "example.invalid/tool",
		Version:      "v1.0.0",
		InstallPaths: []string{filepath.Join(home, "code", "bin", "tool")},
	}
	var output bytes.Buffer
	result := BuildPackage(context.Background(), &output, "tool", pkg, BuildOptions{})
	if result.Action != "error" || result.Message != "failed to load build state" {
		t.Fatalf("BuildPackage result = %+v, want build-state load error\n%s", result, output.String())
	}
	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, malformed) {
		t.Fatalf("malformed state was overwritten: got %q want %q", got, malformed)
	}
}

func TestBuildPackageRejectsMissingInstallPathAfterInstall(t *testing.T) {
	home := testutil.WithHome(t)
	workDir := filepath.Join(home, "repo")
	testutil.InitGitRepo(t, workDir)
	binPath := filepath.Join(home, "code", "bin", "missing")
	recordedAt := time.Unix(1, 0)
	testutil.SaveBuildStateJSON(t, home, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:tool": {
				CompletedAt: recordedAt,
				GitHash:     gitutil.GetTreeHash(workDir),
				InstallHash: strings.Repeat("0", 64),
			},
		},
	})
	pkg := config.Package{
		Source:       "local",
		WorkingDir:   workDir,
		Build:        []string{"true"},
		Install:      []string{"true"},
		InstallPaths: []string{binPath},
	}

	var output bytes.Buffer
	result := BuildPackage(context.Background(), &output, "tool", pkg, BuildOptions{})
	if result.Action != "error" || result.Message != "installed artifact validation failed" {
		t.Fatalf("BuildPackage result = %+v, want artifact validation error\n%s", result, output.String())
	}
	state, err := buildstate.LoadBuildState()
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Builds["pkg:tool"].CompletedAt; !got.Equal(recordedAt) {
		t.Fatalf("failed install changed successful-build state: got %v want %v", got, recordedAt)
	}
}

func TestBuildPackageGoInstallReinstallsOnHashDrift(t *testing.T) {
	home := testutil.WithHome(t)
	binPath := filepath.Join(home, "code", "bin", "tool")
	writeExecutable(t, binPath, "stale")
	fakeBin := filepath.Join(home, "fake-bin")
	writeExecutable(t, filepath.Join(fakeBin, "go"), "#!/bin/sh\nprintf fresh > \"$GOBIN/tool\"\n")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	pkg := config.Package{
		Source:       "go-install",
		Module:       "example.invalid/tool",
		Version:      "v1.0.0",
		InstallPaths: []string{binPath},
	}
	testutil.SaveBuildStateJSON(t, home, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:tool": {
				CompletedAt: time.Now(),
				Version:     pkg.Version,
				InstallHash: strings.Repeat("0", 64),
			},
		},
	})

	var output bytes.Buffer
	result := BuildPackage(context.Background(), &output, "tool", pkg, BuildOptions{})
	if result.Action != "built" {
		t.Fatalf("BuildPackage action = %q, want built: %v\n%s", result.Action, result.Err, output.String())
	}
	content, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "fresh" {
		t.Fatalf("installed content = %q, want fresh", content)
	}
}

func TestPackageStatusUsesInstalledFreshnessPolicy(t *testing.T) {
	home := testutil.WithHome(t)
	workDir := filepath.Join(home, "repo")
	head := testutil.InitGitRepo(t, workDir)
	binPath := filepath.Join(home, "code", "bin", "tool")
	writeVersionExecutable(t, binPath, wrongRevision(head)[:8], "")
	pkg := config.Package{
		Source:       "local",
		WorkingDir:   workDir,
		InstallPaths: []string{binPath},
		VersionCheck: true,
	}
	record := installRecord(t, pkg)
	record.CompletedAt = time.Now()
	record.GitHash = gitutil.GetTreeHash(workDir)
	testutil.SaveBuildStateJSON(t, home, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{"pkg:tool": record},
	})

	statuses := CheckPackageStatuses(map[string]config.Package{"tool": pkg}, "", "testhost")
	if len(statuses) != 1 || !statuses[0].NeedsBuild {
		t.Fatalf("status = %+v, want installed-version rebuild", statuses)
	}
	if !strings.Contains(statuses[0].NeedReason, "does not match source HEAD") {
		t.Fatalf("status reason = %q", statuses[0].NeedReason)
	}
}
