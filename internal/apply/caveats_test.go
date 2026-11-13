package apply

import (
	"testing"

	"github.com/mad01/ralph/internal/config"
)

func TestCollectCaveat_MatchesRecipeByName(t *testing.T) {
	s := &sequence{
		cfg: &config.Config{
			LoadedRecipes: []config.LoadedRecipeInfo{
				{Name: "window-cycle", Caveats: "Re-grant Accessibility permission"},
				{Name: "glow", Caveats: "Re-grant Input Monitoring"},
			},
		},
	}

	s.collectCaveat("window-cycle")

	if len(s.caveats) != 1 {
		t.Fatalf("expected 1 caveat, got %d", len(s.caveats))
	}
	if s.caveats[0].Recipe != "window-cycle" {
		t.Errorf("expected recipe=window-cycle, got %q", s.caveats[0].Recipe)
	}
	if s.caveats[0].Text != "Re-grant Accessibility permission" {
		t.Errorf("unexpected caveat text: %q", s.caveats[0].Text)
	}
}

func TestCollectCaveat_SkipsRecipeWithoutCaveats(t *testing.T) {
	s := &sequence{
		cfg: &config.Config{
			LoadedRecipes: []config.LoadedRecipeInfo{
				{Name: "bionic", Caveats: ""},
			},
		},
	}

	s.collectCaveat("bionic")

	if len(s.caveats) != 0 {
		t.Errorf("expected 0 caveats for recipe without caveats, got %d", len(s.caveats))
	}
}

func TestCollectCaveat_SkipsUnknownRecipe(t *testing.T) {
	s := &sequence{
		cfg: &config.Config{
			LoadedRecipes: []config.LoadedRecipeInfo{
				{Name: "glow", Caveats: "some caveat"},
			},
		},
	}

	s.collectCaveat("unknown-recipe")

	if len(s.caveats) != 0 {
		t.Errorf("expected 0 caveats for unknown recipe, got %d", len(s.caveats))
	}
}

func TestCollectCaveat_MultipleCalls(t *testing.T) {
	s := &sequence{
		cfg: &config.Config{
			LoadedRecipes: []config.LoadedRecipeInfo{
				{Name: "window-cycle", Caveats: "caveat A"},
				{Name: "glow", Caveats: "caveat B"},
			},
		},
	}

	s.collectCaveat("window-cycle")
	s.collectCaveat("glow")

	if len(s.caveats) != 2 {
		t.Fatalf("expected 2 caveats, got %d", len(s.caveats))
	}
	if s.caveats[0].Recipe != "window-cycle" || s.caveats[1].Recipe != "glow" {
		t.Errorf("unexpected caveat order: %v", s.caveats)
	}
}

func TestCollectCaveat_DuplicateCallsSameRecipe(t *testing.T) {
	s := &sequence{
		cfg: &config.Config{
			LoadedRecipes: []config.LoadedRecipeInfo{
				{Name: "window-cycle", Caveats: "caveat A"},
			},
		},
	}

	s.collectCaveat("window-cycle")
	s.collectCaveat("window-cycle")

	// collectCaveat appends each time; dedup happens at display time
	if len(s.caveats) != 2 {
		t.Fatalf("expected 2 raw entries (dedup at display), got %d", len(s.caveats))
	}
}
