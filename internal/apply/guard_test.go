package apply

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
	"github.com/mad01/ralph/internal/testutil"
)

// guardFixture is a config with an item of every phase that writes to disk,
// half of them owned by recipe "kit" and half straight in config.toml. Every
// target sits under root, written with ~ so the manifest has to expand it
// the way the phases do.
type guardFixture struct {
	home string
	root string // where every target of the fixture lives
	cfg  *config.Config
}

func newGuardFixture(t *testing.T) guardFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "dotfiles")
	for _, f := range []string{
		"rc.txt", "copy.txt", "linkdir/inner.txt",
		"mirror/a.conf", "mirror/b.conf", "mirror/.hidden",
	} {
		writeGuardFile(t, filepath.Join(repo, f))
	}
	origin := filepath.Join(t.TempDir(), "origin")
	testutil.InitGitRepo(t, origin)
	pkgDir := filepath.Join(home, "src", "lib")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "t")
	bin := filepath.Join(root, "bin")

	cfg := &config.Config{
		DotfilesRepoPath: repo,
		LoadedRecipes:    []config.LoadedRecipeInfo{{Name: "kit"}},
		Directories: map[string]config.Directory{
			"made": {Target: "~/t/made", OwnerRecipe: "kit"},
		},
		DirsMirror: map[string]config.DirMirror{
			"conf": {Source: "mirror", Target: "~/t/mirror", OwnerRecipe: "kit"},
		},
		Repos: map[string]config.Repo{
			"extra": {URL: origin, Target: "~/t/repo", OwnerRecipe: "kit"},
		},
		Dotfiles: map[string]config.Dotfile{
			"rc":      {Source: "rc.txt", Target: "~/t/rc", OwnerRecipe: "kit"},
			"copy":    {Source: "copy.txt", Target: "~/t/copy", Action: "copy"},
			"linkdir": {Source: "linkdir", Target: "~/t/linkdir", Action: "symlink_dir"},
		},
		Shell: config.ShellConfig{
			Name: "zsh",
			Aliases: map[string]config.ShellAlias{
				"ll": {Command: "ls -l", OwnerRecipe: "kit"},
			},
			Functions: map[string]config.ShellFunction{
				"greet": {Body: "echo hi", OwnerRecipe: "kit"},
			},
		},
		Hooks: config.HooksConfig{Builds: map[string]config.Build{
			"marker": {
				Commands:     []string{fmt.Sprintf("mkdir -p %s && touch %s/built", bin, bin)},
				Run:          "always",
				InstallPaths: []string{"~/t/bin/built"},
				OwnerRecipe:  "kit",
			},
		}},
		Packages: map[string]config.Package{
			"lib": {
				Source:       "local",
				WorkingDir:   pkgDir,
				Build:        []string{fmt.Sprintf("mkdir -p %s && touch %s/lib", bin, bin)},
				InstallPaths: []string{"~/t/bin/lib"},
			},
		},
	}
	return guardFixture{home: home, root: root, cfg: cfg}
}

// writeGuardFile writes a small file at path, creating its parent directories.
func writeGuardFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// declaredPaths returns every filesystem path the manifest declares: the
// recipe-owned paths of every kind, repos included, and the ownerless ones.
func declaredPaths(next *state.RecipeState, unowned map[string]bool) map[string]bool {
	paths := map[string]bool{}
	for p := range unowned {
		paths[p] = true
	}
	for _, art := range next.Recipes {
		for _, list := range [][]string{
			art.Symlinks, art.DirSymlinks, art.Copies, art.Directories,
			art.InstallPaths, art.Repos, art.PackageClones,
		} {
			for _, p := range list {
				paths[filepath.Clean(p)] = true
			}
		}
	}
	return paths
}

// holdsDeclared reports whether dir is a parent of a declared path.
func holdsDeclared(dir string, declared map[string]bool) bool {
	for p := range declared {
		if strings.HasPrefix(p, dir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// The guard against drift between what apply creates and what cleanup diffs
// against: after a real apply run (no dry run, no sync), every path under the
// fixture's targets is declared by the intended manifest, owned or unowned,
// and every path the manifest declares there exists. A phase that creates an
// artifact its declare step does not record, or records under another path,
// fails here.
func TestIntendedManifest_DeclaresWhatApplyCreates(t *testing.T) {
	f := newGuardFixture(t)
	rpt := &report.Report{Command: "test"}
	var errOut bytes.Buffer

	if _, err := Run(f.cfg, rpt, Options{Host: "testhost", Out: io.Discard, Err: &errOut}); err != nil {
		t.Fatalf("Run: %v\nstderr: %s", err, errOut.String())
	}
	if rpt.HasFailures() {
		var buf bytes.Buffer
		rpt.PrintSummary(&buf, report.VerbosityVerbose)
		t.Fatalf("apply failed:\n%s\nstderr: %s", buf.String(), errOut.String())
	}

	next, unowned, err := IntendedManifest(f.cfg, "testhost", time.Now())
	if err != nil {
		t.Fatalf("IntendedManifest: %v", err)
	}
	declared := declaredPaths(next, unowned)

	created := 0
	walkErr := filepath.WalkDir(f.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == f.root {
			return err
		}
		created++
		switch {
		case declared[p] && d.IsDir():
			return filepath.SkipDir
		case declared[p], d.IsDir() && holdsDeclared(p, declared):
			return nil
		}
		t.Errorf("apply created %s, which the intended manifest does not declare", p)
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", f.root, walkErr)
	}
	if created == 0 {
		t.Fatal("apply created nothing under the fixture targets")
	}

	for p := range declared {
		if !strings.HasPrefix(p, f.root+string(filepath.Separator)) {
			t.Errorf("manifest declares %s, outside the fixture targets %s", p, f.root)
			continue
		}
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("manifest declares %s, which apply did not create: %v", p, err)
		}
	}

	kit := next.Recipes["kit"]
	if !slices.Contains(kit.ShellAliases, "ll") || !slices.Contains(kit.ShellFunctions, "greet") {
		t.Errorf("kit's shell aliases %v and functions %v, want ll and greet",
			kit.ShellAliases, kit.ShellFunctions)
	}
	if !slices.Contains(kit.Builds, "marker") {
		t.Errorf("kit's builds = %v, want marker", kit.Builds)
	}
}

// Every phase that declares tracked artifacts is in the one phase list: the
// manifest walk reaches each item type in the fixture.
func TestIntendedManifest_WalksEveryDeclaringPhase(t *testing.T) {
	f := newGuardFixture(t)
	next, unowned, err := IntendedManifest(f.cfg, "testhost", time.Now())
	if err != nil {
		t.Fatalf("IntendedManifest: %v", err)
	}
	kit := next.Recipes["kit"]
	home := f.home
	checks := []struct {
		kind string
		got  []string
		want string
	}{
		{"directory", kit.Directories, filepath.Join(home, "t", "made")},
		{"mirror entry", kit.Symlinks, filepath.Join(home, "t", "mirror", "a.conf")},
		{"mirror entry", kit.Symlinks, filepath.Join(home, "t", "mirror", "b.conf")},
		{"repo", kit.Repos, filepath.Join(home, "t", "repo")},
		{"dotfile", kit.Symlinks, filepath.Join(home, "t", "rc")},
		{"build install path", kit.InstallPaths, filepath.Join(home, "t", "bin", "built")},
	}
	for _, c := range checks {
		if !slices.Contains(c.got, c.want) {
			t.Errorf("%s %s not declared under kit: %v", c.kind, c.want, c.got)
		}
	}
	for _, p := range []string{"copy", "linkdir", filepath.Join("bin", "lib")} {
		if want := filepath.Join(home, "t", p); !unowned[want] {
			t.Errorf("ownerless %s not in the unowned set: %v", want, unowned)
		}
	}
}
