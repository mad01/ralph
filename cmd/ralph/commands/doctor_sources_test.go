package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/paths"
	"github.com/mad01/ralph/internal/report"
)

// withSourceCheckout pins the state dir to a temp dir and creates the cached
// checkout directory of the named source, with a .git entry when isGit.
func withSourceCheckout(t *testing.T, name string, isGit bool) string {
	t.Helper()
	stateDir := t.TempDir()
	t.Setenv(paths.StateDirEnv, stateDir)
	checkout := filepath.Join(stateDir, "sources", name)
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatalf("create checkout: %v", err)
	}
	if isGit {
		if err := os.Mkdir(filepath.Join(checkout, ".git"), 0o755); err != nil {
			t.Fatalf("create .git: %v", err)
		}
	}
	return checkout
}

func TestCheckRecipeSources_NotAGitCheckoutWarns(t *testing.T) {
	checkout := withSourceCheckout(t, "shared", false)
	cfg := &config.Config{RecipeSources: []config.RecipeSource{{Name: "shared"}}}

	rpt := &report.Report{Command: "doctor"}
	checkRecipeSources(rpt, cfg)

	step := findStep(rpt, "Recipe sources", "shared")
	if step == nil || step.Status != report.StatusWarn {
		t.Fatalf("shared = %+v, want a warning", step)
	}
	if !strings.Contains(step.Message, checkout) ||
		!strings.Contains(step.Message, "not a git checkout") {
		t.Errorf(
			"message %q should name %s and say it is not a git checkout",
			step.Message,
			checkout,
		)
	}
}

func TestCheckRecipeSources_GitCheckoutOK(t *testing.T) {
	checkout := withSourceCheckout(t, "shared", true)
	cfg := &config.Config{RecipeSources: []config.RecipeSource{{Name: "shared"}}}

	rpt := &report.Report{Command: "doctor"}
	checkRecipeSources(rpt, cfg)

	step := findStep(rpt, "Recipe sources", "shared")
	if step == nil || step.Status != report.StatusOK || step.Message != checkout {
		t.Fatalf("shared = %+v, want OK with message %q", step, checkout)
	}
}

// Doctor loads read-only, so a source may be uncloned when it checks it.
func TestCheckRecipeSources_NotClonedWarns(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv(paths.StateDirEnv, stateDir)
	cfg := &config.Config{RecipeSources: []config.RecipeSource{{Name: "shared"}}}

	rpt := &report.Report{Command: "doctor"}
	checkRecipeSources(rpt, cfg)

	step := findStep(rpt, "Recipe sources", "shared")
	checkout := filepath.Join(stateDir, "sources", "shared")
	if step == nil || step.Status != report.StatusWarn ||
		!strings.Contains(step.Message, "not cloned at "+checkout) {
		t.Fatalf("shared = %+v, want a warning that %s is not cloned", step, checkout)
	}
}

// Doctor loads read-only, so a checkout may sit off its pinned ref.
func TestCheckRecipeSources_OffPinWarns(t *testing.T) {
	origin, checkout := makeSourceFixture(t, "moon", "main")
	cfg := &config.Config{
		RecipeSources: []config.RecipeSource{{Name: "moon", URL: origin, Ref: "v9.9.9"}},
	}

	rpt := &report.Report{Command: "doctor"}
	checkRecipeSources(rpt, cfg)

	step := findStep(rpt, "Recipe sources", "moon")
	if step == nil || step.Status != report.StatusWarn ||
		!strings.Contains(step.Message, checkout+" is not at ref 'v9.9.9'") {
		t.Fatalf("moon = %+v, want a warning that %s is not at ref v9.9.9", step, checkout)
	}
}

// A source gated to another profile has no checkout here, by design.
func TestCheckRecipeSources_OtherProfileSkips(t *testing.T) {
	t.Setenv(paths.StateDirEnv, t.TempDir())
	cfg := &config.Config{
		Profiles:      []string{"personal"},
		RecipeSources: []config.RecipeSource{{Name: "work", Profiles: []string{"work"}}},
	}

	rpt := &report.Report{Command: "doctor"}
	checkRecipeSources(rpt, cfg)

	step := findStep(rpt, "Recipe sources", "work")
	if step == nil || step.Status != report.StatusSkip || step.Message != "other profile" {
		t.Fatalf("work = %+v, want skip with 'other profile'", step)
	}
}

func TestCheckRecipeSources_MissingRecipesDirWarns(t *testing.T) {
	cfg := &config.Config{MissingRecipeDirs: []config.MissingRecipeDir{
		{Path: "/dots/recipes"},
		{Source: "shared", Path: "/state/sources/shared/recipes"},
	}}

	rpt := &report.Report{Command: "doctor"}
	checkRecipeSources(rpt, cfg)

	for name, path := range map[string]string{
		localRecipesName: "/dots/recipes",
		"shared":         "/state/sources/shared/recipes",
	} {
		step := findStep(rpt, "Recipe sources", name)
		if step == nil || step.Status != report.StatusWarn {
			t.Errorf("%s = %+v, want a warning", name, step)
			continue
		}
		if !strings.Contains(step.Message, "recipes dir missing: "+path) {
			t.Errorf("%s message %q should name %s", name, step.Message, path)
		}
	}
}

func TestCheckRecipeSources_NothingToReport(t *testing.T) {
	rpt := &report.Report{Command: "doctor"}
	checkRecipeSources(rpt, &config.Config{})

	if len(rpt.Phases) != 0 {
		t.Fatalf("no sources and no missing dirs should add no phase, got %+v", rpt.Phases)
	}
}
