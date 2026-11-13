package apply

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
)

// nodeFixture runs applyBuildsAndPackages over builds and packages that
// append their name to one log file when they run.
type nodeFixture struct {
	home string
	log  string
	cfg  *config.Config
}

func newNodeFixture(t *testing.T) *nodeFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return &nodeFixture{
		home: home,
		log:  filepath.Join(home, "nodes.log"),
		cfg: &config.Config{
			Hooks:       config.HooksConfig{Builds: map[string]config.Build{}},
			Packages:    map[string]config.Package{},
			PackagesDir: filepath.Join(home, "pkg"),
		},
	}
}

// logCmd is a shell command that appends name to the fixture log.
func (f *nodeFixture) logCmd(name string) string {
	return fmt.Sprintf("echo %s >> %s", name, f.log)
}

// build adds a build that logs its name when it runs.
func (f *nodeFixture) build(name string, b config.Build) {
	b.Commands = append(b.Commands, f.logCmd(name))
	b.Run = "always"
	f.cfg.Hooks.Builds[name] = b
}

// pkg adds a local package that logs its name when it builds.
func (f *nodeFixture) pkg(t *testing.T, name string, p config.Package) {
	t.Helper()
	p.Source = "local"
	p.WorkingDir = filepath.Join(f.home, "src", name)
	if err := os.MkdirAll(p.WorkingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p.Build = append(p.Build, f.logCmd(name))
	f.cfg.Packages[name] = p
}

// run applies the fixture's builds and packages and returns the report.
func (f *nodeFixture) run(t *testing.T) *report.Report {
	t.Helper()
	rpt := &report.Report{Command: "test"}
	// Failures are echoed on the error writer; keep them out of the test output.
	s := newSequence(f.cfg, rpt, Options{Host: "thishost", Verbose: true, Err: io.Discard})
	applyBuildsAndPackages(s)
	return rpt
}

// ran returns the names that ran, in order.
func (f *nodeFixture) ran(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(f.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

// step returns the report step for name in phase, failing when it is missing.
func step(t *testing.T, rpt *report.Report, phase, name string) report.StepResult {
	t.Helper()
	for _, p := range rpt.Phases {
		if p.Name != phase {
			continue
		}
		for _, s := range p.Steps {
			if s.Name == name {
				return s
			}
		}
	}
	t.Fatalf("no %q step in phase %q", name, phase)
	return report.StepResult{}
}

func TestApplyBuildsAndPackages_RunsInDependencyOrder(t *testing.T) {
	f := newNodeFixture(t)
	f.build("app", config.Build{DependsOn: []string{"builds.codegen"}})
	f.build("codegen", config.Build{DependsOn: []string{"packages.lib"}})
	f.build("docs", config.Build{})
	f.pkg(t, "lib", config.Package{})

	f.run(t)

	// Dependencies first; items with no edge between them keep name order.
	want := []string{"docs", "lib", "codegen", "app"}
	if got := f.ran(t); !slices.Equal(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}

func TestApplyBuildsAndPackages_RunsWavesInOrder(t *testing.T) {
	f := newNodeFixture(t)
	f.build("alpha", config.Build{Wave: 1})
	f.build("zulu", config.Build{Wave: 0})

	f.run(t)

	if got, want := f.ran(t), []string{"zulu", "alpha"}; !slices.Equal(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}

func TestApplyBuildsAndPackages_SkipsDependentsOfAFailure(t *testing.T) {
	f := newNodeFixture(t)
	f.build("broken", config.Build{Commands: []string{"exit 1"}})
	f.build("direct", config.Build{DependsOn: []string{"builds.broken"}})
	f.build("indirect", config.Build{DependsOn: []string{"builds.direct"}})
	f.pkg(t, "lib", config.Package{DependsOn: []string{"builds.broken"}})
	f.build("other", config.Build{})

	rpt := f.run(t)

	if got, want := f.ran(t), []string{"other"}; !slices.Equal(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
	if s := step(t, rpt, "Builds", "broken"); s.Status != report.StatusFail {
		t.Errorf("broken status = %v, want fail", s.Status)
	}
	skips := []struct{ phase, name, msg string }{
		{"Builds", "direct", "dependency builds.broken failed"},
		{"Builds", "indirect", "dependency builds.direct failed"},
		{"Packages", "lib", "dependency builds.broken failed"},
	}
	for _, want := range skips {
		s := step(t, rpt, want.phase, want.name)
		if s.Status != report.StatusSkip || s.Message != want.msg {
			t.Errorf("%s = %v %q, want skip %q", want.name, s.Status, s.Message, want.msg)
		}
	}
}

func TestApplyBuildsAndPackages_SkipsFilteredItems(t *testing.T) {
	off := false
	tests := []struct {
		name     string
		enable   *bool
		hosts    []string
		profiles []string
		reason   string
	}{
		{name: "disabled", enable: &off, reason: "disabled"},
		{name: "otherhost", hosts: []string{"otherhost"}, reason: "host filter"},
		{name: "otherprofile", profiles: []string{"work"}, reason: "profile filter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newNodeFixture(t)
			f.build("b", config.Build{Enable: tt.enable, Hosts: tt.hosts, Profiles: tt.profiles})
			f.pkg(t, "p", config.Package{Enable: tt.enable, Hosts: tt.hosts, Profiles: tt.profiles})

			rpt := f.run(t)

			if got := f.ran(t); len(got) != 0 {
				t.Errorf("filtered items ran: %q", got)
			}
			if s := step(t, rpt, "Builds", "b"); s.Status != report.StatusSkip ||
				s.Message != tt.reason {
				t.Errorf("build = %v %q, want skip %q", s.Status, s.Message, tt.reason)
			}
			wantPkg := tt.reason + " [local]"
			if s := step(t, rpt, "Packages", "p"); s.Status != report.StatusSkip ||
				s.Message != wantPkg {
				t.Errorf("package = %v %q, want skip %q", s.Status, s.Message, wantPkg)
			}
		})
	}
}

// C008: firstFailedDependency reports the first depends_on entry that failed,
// so the caller can skip a node rather than build it against a missing artifact.
func TestFirstFailedDependency(t *testing.T) {
	failed := map[string]bool{"packages.broken": true}

	if dep := firstFailedDependency([]string{"builds.ok", "packages.broken"}, failed); dep != "packages.broken" {
		t.Errorf("expected to report the failed dependency, got %q", dep)
	}
	if dep := firstFailedDependency([]string{"builds.ok", "packages.fine"}, failed); dep != "" {
		t.Errorf("expected no failed dependency, got %q", dep)
	}
	if dep := firstFailedDependency(nil, failed); dep != "" {
		t.Errorf("expected empty for no deps, got %q", dep)
	}
}
