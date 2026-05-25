package config

import "sort"

// RecipeNode represents a recipe in the dependency graph.
type RecipeNode struct {
	Name     string
	Wave     int
	Builds   int
	Packages int
}

// RecipeGraph is the recipe dependency DAG grouped by wave.
type RecipeGraph struct {
	Waves map[int][]RecipeNode
}

// BuildRecipeGraph constructs a RecipeGraph from the loaded config.
func BuildRecipeGraph(cfg *Config) *RecipeGraph {
	g := &RecipeGraph{Waves: make(map[int][]RecipeNode)}

	recipeCounts := make(map[string]struct{ builds, packages int })

	for _, b := range cfg.Hooks.Builds {
		c := recipeCounts[b.OwnerRecipe]
		c.builds++
		recipeCounts[b.OwnerRecipe] = c
	}
	for _, p := range cfg.Packages {
		c := recipeCounts[p.OwnerRecipe]
		c.packages++
		recipeCounts[p.OwnerRecipe] = c
	}

	for _, lr := range cfg.LoadedRecipes {
		c := recipeCounts[lr.Name]
		node := RecipeNode{
			Name:     lr.Name,
			Wave:     lr.Wave,
			Builds:   c.builds,
			Packages: c.packages,
		}
		g.Waves[lr.Wave] = append(g.Waves[lr.Wave], node)
	}

	for w := range g.Waves {
		sort.Slice(g.Waves[w], func(i, j int) bool {
			return g.Waves[w][i].Name < g.Waves[w][j].Name
		})
	}

	return g
}
