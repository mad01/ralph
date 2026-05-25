package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/packages"
	"github.com/spf13/cobra"
)

var outdatedJSON bool

var outdatedCmd = &cobra.Command{
	Use:   "outdated",
	Short: "Check for newer versions of managed packages",
	Long:  `Checks each managed package for newer upstream versions. For go-install packages, queries the Go module proxy. For remote/make packages, compares local HEAD against the remote.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error loading configuration: %v", err))
			os.Exit(2)
		}

		if len(cfg.Packages) == 0 {
			if outdatedJSON {
				fmt.Println("[]")
			} else {
				fmt.Println("No packages configured.")
			}
			return
		}

		currentHost := config.GetCurrentHost()

		if !outdatedJSON {
			fmt.Println("Checking for outdated packages...")
		}

		results := packages.CheckOutdated(cfg.Packages, cfg.PackagesDir, currentHost)

		if outdatedJSON {
			data, err := json.MarshalIndent(results, "", "  ")
			if err != nil {
				fmt.Fprintln(os.Stderr, color.RedString("Error marshaling JSON: %v", err))
				os.Exit(2)
			}
			fmt.Println(string(data))
		} else {
			// Sort: outdated first, then errors, then up to date, then skipped
			packages.SortResults(results)

			// Print colorized table
			printOutdatedTable(results)

			// Summary line
			fmt.Println()
			outdated, upToDate, skipped, errors := countStatuses(results)
			var parts []string
			if outdated > 0 {
				parts = append(parts, color.YellowString("%d outdated", outdated))
			}
			if upToDate > 0 {
				parts = append(parts, color.GreenString("%d up to date", upToDate))
			}
			if errors > 0 {
				parts = append(parts, color.RedString("%d errors", errors))
			}
			if skipped > 0 {
				parts = append(parts, color.CyanString("%d skipped", skipped))
			}
			fmt.Printf("Outdated check complete — %s\n", strings.Join(parts, "  "))
		}

		// Exit codes: 0 = all up to date, 1 = some outdated, 2 = errors
		if packages.HasErrors(results) {
			os.Exit(2)
		}
		if packages.HasOutdated(results) {
			os.Exit(1)
		}
	},
}

func printOutdatedTable(results []packages.OutdatedResult) {
	if len(results) == 0 {
		fmt.Println("No packages configured.")
		return
	}

	// Column headers
	headers := []string{"Package", "Source", "Current", "Latest", "Status"}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}

	type row struct {
		cells  []string
		result packages.OutdatedResult
	}

	rows := make([]row, len(results))
	for i, r := range results {
		rows[i] = row{
			cells:  []string{r.Name, r.Source, r.Current, r.Latest, r.Status},
			result: r,
		}
		for j, cell := range rows[i].cells {
			if len(cell) > widths[j] {
				widths[j] = len(cell)
			}
		}
	}

	// Build format string
	fmtParts := make([]string, len(widths))
	for i, w := range widths {
		fmtParts[i] = fmt.Sprintf("%%-%ds", w)
	}
	rowFmt := strings.Join(fmtParts, "  ")

	// Print header
	headerArgs := make([]any, len(headers))
	for i, h := range headers {
		headerArgs[i] = h
	}
	fmt.Println(color.New(color.Bold).Sprintf(rowFmt, headerArgs...))

	// Separator
	sepParts := make([]any, len(widths))
	for i, w := range widths {
		sepParts[i] = strings.Repeat("-", w)
	}
	fmt.Printf(rowFmt+"\n", sepParts...)

	// Data rows
	for _, r := range rows {
		rowArgs := make([]any, len(r.cells))
		for i, cell := range r.cells {
			rowArgs[i] = cell
		}
		line := fmt.Sprintf(rowFmt, rowArgs...)

		// Colorize based on status
		switch r.result.Status {
		case "outdated":
			fmt.Println(color.YellowString(line))
		case "error":
			fmt.Println(color.RedString(line))
		case "up to date":
			fmt.Println(color.GreenString(line))
		case "skipped":
			fmt.Println(color.CyanString(line))
		default:
			fmt.Println(line)
		}
	}
}

func countStatuses(results []packages.OutdatedResult) (outdated, upToDate, skipped, errors int) {
	for _, r := range results {
		switch r.Status {
		case "outdated":
			outdated++
		case "up to date":
			upToDate++
		case "skipped":
			skipped++
		case "error":
			errors++
		}
	}
	return
}

func init() {
	rootCmd.AddCommand(outdatedCmd)
	outdatedCmd.Flags().BoolVar(&outdatedJSON, "json", false, "Output results as JSON")
}
