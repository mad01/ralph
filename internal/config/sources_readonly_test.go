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
	"github.com/mad01/ralph/internal/testutil"
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

// writeRecipe writes recipes/<name>/recipe.toml under root.
func writeRecipe(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, DefaultRecipesDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "recipe.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// goInstallRecipe is a recipe.toml declaring one go-install package, named
// after the recipe, with an optional depends_on entry.
func goInstallRecipe(name, dependsOn string) string {
	body := "[recipe]\nname = \"" + name + "\"\n\n[packages." + name + "]\n" +
		"source = \"go-install\"\nmodule = \"example.com/" + name + "\"\nversion = \"v1.0.0\"\n"
	if dependsOn != "" {
		body += "depends_on = [\"" + dependsOn + "\"]\n"
	}
	return body
}

// useDependsOnSourceConfig sets up a dotfiles repo whose local recipe app has
// a package depending on packages.lib, which only recipe source moon
// declares. It returns the source's checkout path.
func useDependsOnSourceConfig(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	writeRecipe(t, src, "lib", goInstallRecipe("lib", ""))
	testutil.InitGitRepo(t, src)

	dotfiles := t.TempDir()
	writeRecipe(t, dotfiles, "app", goInstallRecipe("app", "packages.lib"))

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	configPath := filepath.Join(t.TempDir(), "config.toml")
	body := "dotfiles_repo_path = \"" + dotfiles + "\"\n\n" +
		"[recipes_config]\nauto_discover = true\n\n" +
		"[[recipe_sources]]\nname = \"moon\"\nurl = \"" + src + "\"\n"
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := GetDefaultConfigPath
	GetDefaultConfigPath = func() (string, error) { return configPath, nil }
	t.Cleanup(func() { GetDefaultConfigPath = orig })
	return filepath.Join(home, ".config", "ralph", "sources", "moon")
}

// A dry run before the first clone, where a local package depends on a
// package only the uncloned source declares, loads with a warning instead of
// failing validation. The normal load that clones the source validates it.
func TestLoadConfigWithOptions_ReadOnlyDependsOnUnclonedSource(t *testing.T) {
	checkout := useDependsOnSourceConfig(t)

	var cfg *Config
	var err error
	stderr := captureStderr(t, func() {
		cfg, err = LoadConfigWithOptions(LoadOptions{ReadOnly: true})
	})
	if err != nil {
		t.Fatalf("read-only load: %v", err)
	}
	if _, err := os.Lstat(checkout); !os.IsNotExist(err) {
		t.Errorf("read-only load cloned the source (stat err %v)", err)
	}
	if _, ok := cfg.Packages["app"]; !ok {
		t.Error("read-only load dropped the local package")
	}
	if !strings.Contains(stderr, "Warning: package 'app': depends_on references 'packages.lib'") ||
		!strings.Contains(stderr, "preview may be incomplete") {
		t.Errorf("stderr does not warn about the dependency:\n%s", stderr)
	}

	cfg, err = LoadConfig()
	if err != nil {
		t.Fatalf("normal load: %v", err)
	}
	if _, ok := cfg.Packages["lib"]; !ok {
		t.Error("normal load did not merge the source's package")
	}
}

// Only a read-only load that left a recipe source unloaded tolerates a
// depends_on entry naming nothing; every other load still fails on it, and
// a malformed entry fails even then.
func TestValidateLoaded_UnknownDependency(t *testing.T) {
	missingSource := []MissingRecipeDir{{Source: "moon", Path: "/sources/moon/recipes"}}
	missingLocal := []MissingRecipeDir{{Path: "/dotfiles/recipes"}}
	tests := []struct {
		name     string
		readOnly bool
		missing  []MissingRecipeDir
		dep      string
		wantErr  bool
	}{
		{"read-only, source missing", true, missingSource, "packages.lib", false},
		{"normal, source missing", false, missingSource, "packages.lib", true},
		{"read-only, nothing missing", true, nil, "packages.lib", true},
		{"read-only, local recipes missing", true, missingLocal, "packages.lib", true},
		{"read-only, source missing, malformed", true, missingSource, "lib", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Packages: map[string]Package{"app": {
					Source:    SourceGoInstall,
					Module:    "example.com/app",
					Version:   "v1.0.0",
					DependsOn: []string{tt.dep},
				}},
				MissingRecipeDirs: tt.missing,
			}
			var warn bytes.Buffer
			err := validateLoaded(&warn, cfg, tt.readOnly)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateLoaded err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !strings.Contains(warn.String(), "packages.lib") {
				t.Errorf("no warning names the dependency: %q", warn.String())
			}
		})
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
