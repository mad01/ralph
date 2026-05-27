package commands

import (
	"testing"

	"github.com/mad01/ralph/internal/config"
)

func TestCollectCaveat_MatchesRecipeByName(t *testing.T) {
	ctx := &applyContext{
		cfg: &config.Config{
			LoadedRecipes: []config.LoadedRecipeInfo{
				{Name: "window-cycle", Caveats: "Re-grant Accessibility permission"},
				{Name: "glow", Caveats: "Re-grant Input Monitoring"},
			},
		},
	}

	ctx.collectCaveat("window-cycle")

	if len(ctx.caveats) != 1 {
		t.Fatalf("expected 1 caveat, got %d", len(ctx.caveats))
	}
	if ctx.caveats[0].recipe != "window-cycle" {
		t.Errorf("expected recipe=window-cycle, got %q", ctx.caveats[0].recipe)
	}
	if ctx.caveats[0].text != "Re-grant Accessibility permission" {
		t.Errorf("unexpected caveat text: %q", ctx.caveats[0].text)
	}
}

func TestCollectCaveat_SkipsRecipeWithoutCaveats(t *testing.T) {
	ctx := &applyContext{
		cfg: &config.Config{
			LoadedRecipes: []config.LoadedRecipeInfo{
				{Name: "bionic", Caveats: ""},
			},
		},
	}

	ctx.collectCaveat("bionic")

	if len(ctx.caveats) != 0 {
		t.Errorf("expected 0 caveats for recipe without caveats, got %d", len(ctx.caveats))
	}
}

func TestCollectCaveat_SkipsUnknownRecipe(t *testing.T) {
	ctx := &applyContext{
		cfg: &config.Config{
			LoadedRecipes: []config.LoadedRecipeInfo{
				{Name: "glow", Caveats: "some caveat"},
			},
		},
	}

	ctx.collectCaveat("unknown-recipe")

	if len(ctx.caveats) != 0 {
		t.Errorf("expected 0 caveats for unknown recipe, got %d", len(ctx.caveats))
	}
}

func TestCollectCaveat_MultipleCalls(t *testing.T) {
	ctx := &applyContext{
		cfg: &config.Config{
			LoadedRecipes: []config.LoadedRecipeInfo{
				{Name: "window-cycle", Caveats: "caveat A"},
				{Name: "glow", Caveats: "caveat B"},
			},
		},
	}

	ctx.collectCaveat("window-cycle")
	ctx.collectCaveat("glow")

	if len(ctx.caveats) != 2 {
		t.Fatalf("expected 2 caveats, got %d", len(ctx.caveats))
	}
	if ctx.caveats[0].recipe != "window-cycle" || ctx.caveats[1].recipe != "glow" {
		t.Errorf("unexpected caveat order: %v", ctx.caveats)
	}
}

func TestCollectCaveat_DuplicateCallsSameRecipe(t *testing.T) {
	ctx := &applyContext{
		cfg: &config.Config{
			LoadedRecipes: []config.LoadedRecipeInfo{
				{Name: "window-cycle", Caveats: "caveat A"},
			},
		},
	}

	ctx.collectCaveat("window-cycle")
	ctx.collectCaveat("window-cycle")

	// collectCaveat appends each time; dedup happens at display time
	if len(ctx.caveats) != 2 {
		t.Fatalf("expected 2 raw entries (dedup at display), got %d", len(ctx.caveats))
	}
}
