package cleanup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

func TestCarryForwardFrozenRecipes_PreventsOrphanDeletion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// A symlink owned by a recipe that is host-filtered on this host: it must
	// survive cleanup even though it is absent from the next manifest.
	link := filepath.Join(dir, "pi.link")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("pi", state.KindSymlink, link)
	prev.SetMetadata("pi", time.Now(), "delete")
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	// Freeze "pi" (host-filtered on this host).
	frozen := map[string]bool{"pi": true}
	carryForwardFrozenRecipes(prev, next, frozen, nil)

	// The frozen recipe's artifacts must now be tracked in next so it isn't
	// diffed as an orphan, and must persist for future runs.
	if _, ok := next.Recipes["pi"]; !ok {
		t.Fatalf("expected frozen recipe carried into next manifest, got %v", next.Recipes)
	}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	Run(prev, next, next.AllPaths(), false, logger, phase)

	if _, err := os.Lstat(link); err != nil {
		t.Errorf("expected host-filtered recipe's symlink to survive, got %v", err)
	}
}

func TestCarryForwardFrozenRecipes_SourceProvenanceAvoidsNameCollision(t *testing.T) {
	prev := &state.RecipeState{
		Version: state.CurrentRecipeStateVersion,
		Recipes: map[string]state.RecipeArtifacts{},
	}
	prev.AddArtifact("moon/app", state.KindPackage, "app")
	prev.SetRecipeSource("moon/app", "moon")
	prev.AddArtifact("moon/tool", state.KindPackage, "local-with-slash")
	prev.AddArtifact("sun/app", state.KindPackage, "other-source")
	prev.SetRecipeSource("sun/app", "sun")
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	carryForwardFrozenRecipes(prev, next, nil, map[string]bool{"moon": true})

	if _, ok := next.Recipes["moon/app"]; !ok {
		t.Error("recipe from filtered source moon was not carried forward")
	}
	for _, name := range []string{"moon/tool", "sun/app"} {
		if _, ok := next.Recipes[name]; ok {
			t.Errorf("unrelated recipe %q was carried forward", name)
		}
	}
	orphans := state.Diff(prev, next)
	for _, name := range []string{"moon/tool", "sun/app"} {
		if _, ok := orphans[name]; !ok {
			t.Errorf("unrelated recipe %q is not eligible for cleanup", name)
		}
	}
}

func TestCarryForwardFrozenRecipes_MigratesLegacySourceProvenance(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), ".recipe_state")
	originalStatePath := state.GetStatePath
	state.GetStatePath = func() (string, error) { return statePath, nil }
	t.Cleanup(func() { state.GetStatePath = originalStatePath })

	legacyJSON := `{
  "recipes": {
    "moon/app": {
      "delete_behavior": "delete",
      "packages": ["app"]
    },
    "sun/app": {
      "delete_behavior": "delete",
      "packages": ["other-source"]
    },
    "moonlight/app": {
      "delete_behavior": "delete",
      "packages": ["similar-prefix"]
    }
  }
}`
	if err := os.WriteFile(statePath, []byte(legacyJSON), 0o644); err != nil {
		t.Fatalf("write legacy recipe state: %v", err)
	}
	prev, err := state.Load()
	if err != nil {
		t.Fatalf("load legacy recipe state: %v", err)
	}
	if prev.Version != 0 {
		t.Fatalf("legacy state Version = %d, want 0", prev.Version)
	}
	next := &state.RecipeState{
		Version: state.CurrentRecipeStateVersion,
		Recipes: map[string]state.RecipeArtifacts{},
	}

	carryForwardFrozenRecipes(prev, next, nil, map[string]bool{"moon": true})

	moon, ok := next.Recipes["moon/app"]
	if !ok {
		t.Fatal("legacy recipe from filtered source moon was not carried forward")
	}
	if moon.RecipeSource != "moon" {
		t.Errorf("migrated RecipeSource = %q, want moon", moon.RecipeSource)
	}
	orphans := state.Diff(prev, next)
	for _, name := range []string{"sun/app", "moonlight/app"} {
		if _, ok := orphans[name]; !ok {
			t.Errorf("unrelated legacy recipe %q is not eligible for cleanup", name)
		}
	}
	if err := state.Save(next); err != nil {
		t.Fatalf("save upgraded recipe state: %v", err)
	}
	upgraded, err := state.Load()
	if err != nil {
		t.Fatalf("reload upgraded recipe state: %v", err)
	}
	if upgraded.Version != state.CurrentRecipeStateVersion {
		t.Errorf(
			"upgraded Version = %d, want %d",
			upgraded.Version,
			state.CurrentRecipeStateVersion,
		)
	}
	if source := upgraded.Recipes["moon/app"].RecipeSource; source != "moon" {
		t.Errorf("persisted RecipeSource = %q, want moon", source)
	}
}

// MAD-587: one warning per missing recipes directory that had recipes kept,
// naming the directory and the count; none when nothing was kept.
func TestKeepFrozenRecipes_WarnsPerMissingDirWithKeptRecipes(t *testing.T) {
	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{
		"editors":  {Symlinks: []string{"/home/u/.vimrc"}},
		"shell":    {Symlinks: []string{"/home/u/.zshrc"}},
		"moon/app": {RecipeSource: "moon", Symlinks: []string{"/home/u/.app"}},
	}}
	cfg := &config.Config{MissingRecipeDirs: []config.MissingRecipeDir{
		{Path: "/dotfiles/recipes"},
		{Source: "sun", Path: "/sources/sun/recipes"},
	}}
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	var errW bytes.Buffer
	KeepFrozenRecipes(cfg, prev, next, &errW)

	lines := strings.Split(strings.TrimSpace(errW.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d warning line(s), want 1: %q", len(lines), errW.String())
	}
	if !strings.Contains(lines[0], "/dotfiles/recipes") || !strings.Contains(lines[0], "2 recipe") {
		t.Errorf("warning does not name the directory and the count: %q", lines[0])
	}
	if strings.Contains(errW.String(), "/sources/sun/recipes") {
		t.Errorf("warned about a directory with nothing kept: %q", errW.String())
	}
}
