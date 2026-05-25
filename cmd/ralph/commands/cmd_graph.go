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
	isDefault := len(waveNums) == 1 && waveNums[0] == 2

	for i, waveNum := range waveNums {
		nodes := g.Waves[waveNum]
		if len(nodes) == 0 {
			continue
		}

		label := fmt.Sprintf("Wave %d", waveNum)
		if waveNum == 2 && !isDefault {
			label += " (default)"
		} else if isDefault {
			label += " (all recipes)"
		}
		bold.Fprintf(w, "%s\n", label)

		boxes := make([][]string, len(nodes))
		maxHeight := 0
		for j, node := range nodes {
			boxes[j] = renderRecipeBox(node)
			if len(boxes[j]) > maxHeight {
				maxHeight = len(boxes[j])
			}
		}

		for row := 0; row < maxHeight; row++ {
			for j, box := range boxes {
				if j > 0 {
					fmt.Fprint(w, "  ")
				}
				if row < len(box) {
					fmt.Fprint(w, box[row])
				}
			}
			fmt.Fprintln(w)
		}

		if i < len(waveNums)-1 {
			fmt.Fprintln(w)
			dim.Fprintln(w, "       │")
			dim.Fprintln(w, "       ▼")
			fmt.Fprintln(w)
		}
	}
}

func renderRecipeBox(node config.RecipeNode) []string {
	var summary []string
	if node.Builds > 0 {
		summary = append(summary, fmt.Sprintf("%d build", node.Builds))
		if node.Builds > 1 {
			summary[len(summary)-1] += "s"
		}
	}
	if node.Packages > 0 {
		summary = append(summary, fmt.Sprintf("%d pkg", node.Packages))
		if node.Packages > 1 {
			summary[len(summary)-1] += "s"
		}
	}
	summaryStr := strings.Join(summary, ", ")
	if summaryStr == "" {
		summaryStr = "config only"
	}

	contentWidth := len(node.Name)
	if len(summaryStr) > contentWidth {
		contentWidth = len(summaryStr)
	}
	boxWidth := contentWidth + 4
	if boxWidth < 14 {
		boxWidth = 14
	}

	top := "┌" + strings.Repeat("─", boxWidth) + "┐"
	nameLine := "│  " + node.Name + strings.Repeat(" ", boxWidth-len(node.Name)-2) + "│"
	summaryLine := "│  " + summaryStr + strings.Repeat(" ", boxWidth-len(summaryStr)-2) + "│"
	bottom := "└" + strings.Repeat("─", boxWidth) + "┘"

	return []string{top, nameLine, summaryLine, bottom}
}

func wavesToWaveGroups(g *config.RecipeGraph) map[int]*config.WaveGroup {
	groups := make(map[int]*config.WaveGroup)
	for w := range g.Waves {
		groups[w] = &config.WaveGroup{}
	}
	return groups
}
