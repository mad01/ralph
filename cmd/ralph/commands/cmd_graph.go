package commands

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/spf13/cobra"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Render the recipe dependency DAG",
	Long:  `Displays the recipe dependency graph as horizontal wave layers, showing which recipes run in each wave.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		g := config.BuildRecipeGraph(cfg)
		renderGraph(os.Stdout, g)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(graphCmd)
}

func renderGraph(w io.Writer, g *config.RecipeGraph) {
	if len(g.Waves) == 0 {
		fmt.Fprintln(w, "No recipes loaded.")
		return
	}

	bold := color.New(color.Bold)
	dim := color.New(color.FgHiBlack)

	bold.Fprintln(w, "Recipe Dependency Graph")
	fmt.Fprintln(w)

	waveNums := config.SortedWaveNumbers(wavesToWaveGroups(g))
	isDefault := len(waveNums) == 1 && waveNums[0] == 1

	for i, waveNum := range waveNums {
		nodes := g.Waves[waveNum]
		if len(nodes) == 0 {
			continue
		}

		label := fmt.Sprintf("Wave %d", waveNum)
		if waveNum == 1 && !isDefault {
			label += " (default)"
		} else if isDefault {
			label += " (all recipes)"
		}
		bold.Fprintf(w, "%s\n", label)

		nameWidth := 0
		for _, node := range nodes {
			if len(node.Name) > nameWidth {
				nameWidth = len(node.Name)
			}
		}

		for _, node := range nodes {
			summary := recipeSummary(node)
			padding := strings.Repeat(" ", nameWidth-len(node.Name))
			dim.Fprintf(w, "  %s%s  %s\n", node.Name, padding, summary)
		}

		if i < len(waveNums)-1 {
			fmt.Fprintln(w)
			dim.Fprintln(w, "  │")
			dim.Fprintln(w, "  ▼")
			fmt.Fprintln(w)
		}
	}
}

func recipeSummary(node config.RecipeNode) string {
	var parts []string
	if node.Builds > 0 {
		s := fmt.Sprintf("%d build", node.Builds)
		if node.Builds > 1 {
			s += "s"
		}
		parts = append(parts, s)
	}
	if node.Packages > 0 {
		s := fmt.Sprintf("%d pkg", node.Packages)
		if node.Packages > 1 {
			s += "s"
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return "config only"
	}
	return strings.Join(parts, ", ")
}

func wavesToWaveGroups(g *config.RecipeGraph) map[int]*config.WaveGroup {
	groups := make(map[int]*config.WaveGroup)
	for w := range g.Waves {
		groups[w] = &config.WaveGroup{}
	}
	return groups
}
