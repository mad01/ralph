package commands

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/report"
)

func printDryRunBanner(w io.Writer) {
	fmt.Fprintln(w, color.CyanString("\n*** DRY RUN MODE ENABLED ***"))
	fmt.Fprintln(w, color.CyanString("No actual changes will be made."))
	fmt.Fprintln(w, color.CyanString("****************************\n"))
}

func verboseWriter(verbose, dryRun bool) io.Writer {
	if verbose || dryRun {
		return os.Stdout
	}
	return io.Discard
}

func printReportSummary(rpt *report.Report) {
	ok, warn, fail, skip := rpt.TotalCounts()
	parts := []string{color.GreenString("%d ok", ok)}
	if warn > 0 {
		parts = append(parts, color.YellowString("%d warnings", warn))
	}
	if fail > 0 {
		parts = append(parts, color.RedString("%d failed", fail))
	}
	if skip > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skip))
	}
	fmt.Printf("\n%s: %s\n", rpt.Command, strings.Join(parts, ", "))
}
