package config

import "testing"

func TestBuildRecipeGraph_EmptyConfig(t *testing.T) {
	cfg := &Config{DotfilesRepoPath: "~/.dotfiles"}
	g := BuildRecipeGraph(cfg)
	if len(g.Waves) != 0 {
		t.Errorf("expected empty waves, got %d", len(g.Waves))
	}
}

func TestBuildRecipeGraph_SingleWave(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		LoadedRecipes: []LoadedRecipeInfo{
			{Name: "packages", Wave: 1},
		},
		Packages: map[string]Package{
			"csl":   {Source: "make", OwnerRecipe: "packages"},
			"brain": {Source: "make", OwnerRecipe: "packages"},
		},
	}
	g := BuildRecipeGraph(cfg)
	if len(g.Waves) != 1 {
		t.Fatalf("expected 1 wave, got %d", len(g.Waves))
	}
	nodes := g.Waves[1]
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node in wave 1, got %d", len(nodes))
	}
	if nodes[0].Name != "packages" {
		t.Errorf("expected name='packages', got %q", nodes[0].Name)
	}
	if nodes[0].Packages != 2 {
		t.Errorf("expected 2 packages, got %d", nodes[0].Packages)
	}
}

func TestBuildRecipeGraph_TwoWaves(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		LoadedRecipes: []LoadedRecipeInfo{
			{Name: "packages", Wave: 1},
			{Name: "brain", Wave: 2},
		},
		Hooks: HooksConfig{
			Builds: map[string]Build{
				"brain_index": {
					Commands:    []string{"brain index"},
					Run:         "always",
					OwnerRecipe: "brain",
				},
			},
		},
		Packages: map[string]Package{
			"csl": {Source: "make", OwnerRecipe: "packages"},
		},
	}
	g := BuildRecipeGraph(cfg)
	if len(g.Waves) != 2 {
		t.Fatalf("expected 2 waves, got %d", len(g.Waves))
	}
	w1 := g.Waves[1]
	if len(w1) != 1 || w1[0].Name != "packages" {
		t.Errorf("wave 1: expected [packages], got %v", w1)
	}
	if w1[0].Packages != 1 {
		t.Errorf("wave 1 packages: expected 1, got %d", w1[0].Packages)
	}
	w2 := g.Waves[2]
	if len(w2) != 1 || w2[0].Name != "brain" {
		t.Errorf("wave 2: expected [brain], got %v", w2)
	}
	if w2[0].Builds != 1 {
		t.Errorf("wave 2 brain builds: expected 1, got %d", w2[0].Builds)
	}
}

func TestBuildRecipeGraph_SortedWithinWave(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		LoadedRecipes: []LoadedRecipeInfo{
			{Name: "zebra", Wave: 2},
			{Name: "alpha", Wave: 2},
		},
	}
	g := BuildRecipeGraph(cfg)
	nodes := g.Waves[2]
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Name != "alpha" || nodes[1].Name != "zebra" {
		t.Errorf("expected [alpha, zebra], got [%s, %s]", nodes[0].Name, nodes[1].Name)
	}
}
