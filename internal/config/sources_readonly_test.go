package config

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/gitutil"
)

// useSourceConfig writes a config.toml with one recipe source, points config
// loading at it and HOME at a temp dir, and returns the source's checkout
// path under the default state dir.
func useSourceConfig(t *testing.T, url, ref string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	configPath := filepath.Join(t.TempDir(), "config.toml")
	body := "dotfiles_repo_path = \"" + t.TempDir() + "\"\n\n" +
		"[[recipe_sources]]\nname = \"moon\"\nurl = \"" + url + "\"\nref = \"" + ref + "\"\n"
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := GetDefaultConfigPath
	GetDefaultConfigPath = func() (string, error) { return configPath, nil }
	t.Cleanup(func() { GetDefaultConfigPath = orig })

	return filepath.Join(home, ".config", "ralph", "sources", "moon")
}

// A read-only load clones nothing: the sources dir is never created, and the
// source's recipes dir is recorded missing so cleanup keeps its recipes. A
// normal load of the same config still clones.
func TestLoadConfigWithOptions_ReadOnlyDoesNotClone(t *testing.T) {
	src, _, _ := makeSourceRepo(t, "app")
	checkout := useSourceConfig(t, src, "main")

	cfg, err := LoadConfigWithOptions(LoadOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("read-only load: %v", err)
	}
	if _, err := os.Lstat(filepath.Dir(checkout)); !os.IsNotExist(err) {
		t.Errorf("read-only load created %s (stat err %v)", filepath.Dir(checkout), err)
	}
	want := MissingRecipeDir{Source: "moon", Path: filepath.Join(checkout, DefaultRecipesDir)}
	if len(cfg.MissingRecipeDirs) != 1 || cfg.MissingRecipeDirs[0] != want {
		t.Errorf("MissingRecipeDirs = %+v, want [%+v]", cfg.MissingRecipeDirs, want)
	}
	if _, ok := cfg.Dotfiles["app_conf"]; ok {
		t.Error("read-only load merged a recipe from a source it did not clone")
	}

	cfg, err = LoadConfig()
	if err != nil {
		t.Fatalf("normal load: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checkout, ".git")); err != nil {
		t.Errorf("normal load did not clone the source: %v", err)
	}
	if _, ok := cfg.Dotfiles["app_conf"]; !ok {
		t.Error("normal load did not merge the source's recipe")
	}
}

// A read-only load leaves a checkout off its pin where it is and loads the
// recipes found there, without fetching a ref it does not know. A normal
// load then moves it.
func TestProcessRecipes_ReadOnlyLeavesCheckoutOffPin(t *testing.T) {
	src, first, second := makeSourceRepo(t, "app")
	checkout := useSourceConfig(t, src, "main")
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("initial load: %v", err)
	}
	gitTag(t, src, "v2.0.0")

	for _, ref := range []string{"v1.0.0", "v2.0.0"} {
		cfg := &Config{
			DotfilesRepoPath: t.TempDir(),
			RecipeSources:    []RecipeSource{{Name: "moon", URL: src, Ref: ref}},
		}
		if err := processRecipes(cfg, "testhost", true); err != nil {
			t.Fatalf("read-only load at %s: %v", ref, err)
		}
		if got := gitutil.GetGitHash(checkout); got != second {
			t.Errorf("read-only load at %s moved HEAD to %s, want %s", ref, got, second)
		}
		if _, ok := cfg.Dotfiles["app_conf"]; !ok {
			t.Errorf("read-only load at %s did not load the recipes in the checkout", ref)
		}
		if len(cfg.MissingRecipeDirs) != 0 {
			t.Errorf("MissingRecipeDirs = %+v, want none", cfg.MissingRecipeDirs)
		}
	}
	if got := gitutil.ResolveRef(checkout, "v2.0.0"); got != "" {
		t.Errorf("read-only load fetched tag v2.0.0 (%s)", got)
	}

	cfg := &Config{
		DotfilesRepoPath: t.TempDir(),
		RecipeSources:    []RecipeSource{{Name: "moon", URL: src, Ref: "v1.0.0"}},
	}
	if err := ProcessRecipes(cfg, "testhost"); err != nil {
		t.Fatalf("normal load: %v", err)
	}
	if got := gitutil.GetGitHash(checkout); got != first {
		t.Errorf("normal load left HEAD at %s, want the v1.0.0 commit %s", got, first)
	}
}

// Each case a read-only load cannot match a real run warns once, naming the
// source; a checkout at its pin says nothing.
func TestReadOnlySourceCheckout_Warnings(t *testing.T) {
	src, _, _ := makeSourceRepo(t, "app")
	sourcesDir := t.TempDir()

	var buf bytes.Buffer
	missing := RecipeSource{Name: "app", URL: src, Ref: "main"}
	if _, present := readOnlySourceCheckout(&buf, missing, sourcesDir); present {
		t.Error("an uncloned source reported present")
	}
	assertWarning(t, buf.String(), "recipe source 'app' is not cloned")

	if _, err := EnsureSourceCheckout(io.Discard, missing, sourcesDir); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	buf.Reset()
	if _, present := readOnlySourceCheckout(&buf, missing, sourcesDir); !present {
		t.Error("a cloned source reported missing")
	}
	if buf.Len() != 0 {
		t.Errorf("a checkout at its pin warned: %q", buf.String())
	}

	buf.Reset()
	offPin := RecipeSource{Name: "app", URL: src, Ref: "v1.0.0"}
	if _, present := readOnlySourceCheckout(&buf, offPin, sourcesDir); !present {
		t.Error("a checkout off its pin reported missing")
	}
	assertWarning(t, buf.String(), "recipe source 'app' is not at ref 'v1.0.0'")
}

func assertWarning(t *testing.T, got, want string) {
	t.Helper()
	if strings.Count(got, "\n") != 1 || !strings.HasPrefix(got, "Warning: "+want) ||
		!strings.Contains(got, "dry run or check") ||
		!strings.Contains(got, "preview may be incomplete") {
		t.Errorf("warning = %q, want one line starting %q", got, "Warning: "+want)
	}
}

func gitTag(t *testing.T, repo, tag string) {
	t.Helper()
	cmd := exec.Command("git", "tag", tag)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git tag %s: %v\n%s", tag, err, out)
	}
}
