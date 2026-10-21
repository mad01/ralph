package commands

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
)

func symlink(t *testing.T, dest, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(link), err)
	}
	if err := os.Symlink(dest, link); err != nil {
		t.Fatalf("symlink %s -> %s: %v", link, dest, err)
	}
}

func TestDanglingScanDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &config.Config{
		DirsMirror: map[string]config.DirMirror{
			"skills": {Target: "~/.claude/skills", OwnerRecipe: "claude"},
		},
		Directories: map[string]config.Directory{
			"styles": {Target: "~/.claude/output-styles", OwnerRecipe: "claude"},
		},
		Dotfiles: map[string]config.Dotfile{
			"style":     {Target: "~/.claude/output-styles/a.md", OwnerRecipe: "claude"},
			"zshrc":     {Target: "~/.zshrc", OwnerRecipe: "zsh"},
			"gitconfig": {Target: "~/.gitconfig", OwnerRecipe: "git"},
		},
	}

	got := danglingScanDirs(cfg)
	want := []scanDir{
		{path: home},
		{path: filepath.Join(home, ".claude", "output-styles"), recipe: "claude"},
		{path: filepath.Join(home, ".claude", "skills"), recipe: "claude"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("danglingScanDirs() = %+v, want %+v", got, want)
	}
}

func TestDanglingLinksIn(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "repo")
	dir := filepath.Join(tmp, "home", ".claude", "output-styles")
	createTestFile(t, filepath.Join(root, "live.md"), "live")

	symlink(t, filepath.Join(root, "gone.md"), filepath.Join(dir, "gone.md"))
	symlink(t, "../../../repo/rel.md", filepath.Join(dir, "rel.md"))
	symlink(t, filepath.Join(tmp, "elsewhere", "x.md"), filepath.Join(dir, "other.md"))
	symlink(t, filepath.Join(root, "live.md"), filepath.Join(dir, "live.md"))
	symlink(t, filepath.Join(root, "declared.md"), filepath.Join(dir, "declared.md"))
	createTestFile(t, filepath.Join(dir, "plain.txt"), "plain")

	declared := map[string]bool{filepath.Join(dir, "declared.md"): true}
	got, err := danglingLinksIn(dir, []string{root}, declared)
	if err != nil {
		t.Fatalf("danglingLinksIn() error = %v", err)
	}
	want := []danglingLink{
		{path: filepath.Join(dir, "gone.md"), target: filepath.Join(root, "gone.md")},
		{path: filepath.Join(dir, "rel.md"), target: filepath.Join(root, "rel.md")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("danglingLinksIn() = %+v, want %+v", got, want)
	}

	missing, err := danglingLinksIn(filepath.Join(tmp, "absent"), []string{root}, nil)
	if err != nil || missing != nil {
		t.Errorf("danglingLinksIn(missing dir) = %+v, %v; want nil, nil", missing, err)
	}
}

// danglingFixture is a fresh HOME holding a dotfiles repo and a styles
// directory with one live, declared dotfile link.
type danglingFixture struct {
	home, repo, styles string
	cfg                *config.Config
}

func newDanglingFixture(t *testing.T) danglingFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	f := danglingFixture{
		home:   home,
		repo:   filepath.Join(home, "dotfiles"),
		styles: filepath.Join(home, ".claude", "output-styles"),
	}
	createTestFile(t, filepath.Join(f.repo, "claude", "kept.md"), "kept")
	symlink(t, filepath.Join(f.repo, "claude", "kept.md"), filepath.Join(f.styles, "kept.md"))
	f.cfg = &config.Config{
		DotfilesRepoPath: "~/dotfiles",
		Dotfiles: map[string]config.Dotfile{
			"kept": {
				Source:      "claude/kept.md",
				Target:      "~/.claude/output-styles/kept.md",
				OwnerRecipe: "claude",
			},
		},
	}
	return f
}

func TestCheckDanglingLinks(t *testing.T) {
	disabled := false
	tests := []struct {
		name     string
		setup    func(t *testing.T, f danglingFixture)
		want     []report.Status
		wantName string
		wantCode int
	}{
		{
			name:     "passes when every link resolves",
			setup:    func(*testing.T, danglingFixture) {},
			want:     []report.Status{report.StatusOK},
			wantCode: 0,
		},
		{
			name: "reports a link no recipe declares",
			setup: func(t *testing.T, f danglingFixture) {
				symlink(
					t,
					filepath.Join(f.repo, "claude", "me-fable.md"),
					filepath.Join(f.styles, "me-fable.md"),
				)
			},
			want:     []report.Status{report.StatusWarn},
			wantName: "~/.claude/output-styles/me-fable.md",
			wantCode: 2,
		},
		{
			name: "reports a dangling link at a disabled dotfile's target",
			setup: func(t *testing.T, f danglingFixture) {
				f.cfg.Dotfiles["old"] = config.Dotfile{
					Source:      "claude/old.md",
					Target:      "~/.claude/output-styles/old.md",
					Enable:      &disabled,
					OwnerRecipe: "claude",
				}
				symlink(
					t,
					filepath.Join(f.repo, "claude", "old.md"),
					filepath.Join(f.styles, "old.md"),
				)
			},
			want:     []report.Status{report.StatusWarn},
			wantName: "~/.claude/output-styles/old.md",
			wantCode: 2,
		},
		{
			name: "reports a link into the recipe sources cache",
			setup: func(t *testing.T, f danglingFixture) {
				dest := filepath.Join(
					f.home,
					".config",
					"ralph",
					"sources",
					"src",
					"recipes",
					"a.md",
				)
				symlink(t, dest, filepath.Join(f.styles, "a.md"))
			},
			want:     []report.Status{report.StatusWarn},
			wantName: "~/.claude/output-styles/a.md",
			wantCode: 2,
		},
		{
			name: "leaves a directories target that is a file to the Directories phase",
			setup: func(t *testing.T, f danglingFixture) {
				createTestFile(t, filepath.Join(f.home, "not-a-dir"), "file")
				f.cfg.Directories = map[string]config.Directory{
					"bad": {Target: "~/not-a-dir", OwnerRecipe: "claude"},
				}
			},
			want:     []report.Status{report.StatusOK},
			wantCode: 0,
		},
		{
			name: "fails without an OK row when a directory can't be read",
			setup: func(t *testing.T, f danglingFixture) {
				if os.Geteuid() == 0 {
					t.Skip("root reads a mode-000 directory")
				}
				if err := os.Chmod(f.styles, 0o000); err != nil {
					t.Fatalf("chmod: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(f.styles, 0o755) })
			},
			want:     []report.Status{report.StatusFail},
			wantName: "~/.claude/output-styles",
			wantCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newDanglingFixture(t)
			tt.setup(t, f)

			rpt := &report.Report{Command: "doctor"}
			checkDanglingLinks(rpt, f.cfg)

			steps := danglingSteps(t, rpt)
			got := make([]report.Status, len(steps))
			for i, step := range steps {
				got[i] = step.Status
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("statuses = %v, want %v; steps %+v", got, tt.want, steps)
			}
			if tt.wantName != "" && (steps[0].Name != tt.wantName || steps[0].Recipe != "claude") {
				t.Errorf("step = %+v, want name %q under recipe claude", steps[0], tt.wantName)
			}
			if code := rpt.ExitCode(); code != tt.wantCode {
				t.Errorf("ExitCode() = %d, want %d", code, tt.wantCode)
			}
		})
	}
}

func danglingSteps(t *testing.T, rpt *report.Report) []report.StepResult {
	t.Helper()
	for _, phase := range rpt.Phases {
		if phase.Name == "Dangling links" {
			return phase.Steps
		}
	}
	t.Fatalf("no Dangling links phase in %+v", rpt.Phases)
	return nil
}
