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
	_, _ = fmt.Fprintln(w, color.CyanString("\n*** DRY RUN MODE ENABLED ***"))
	_, _ = fmt.Fprintln(w, color.CyanString("No actual changes will be made."))
	_, _ = fmt.Fprintln(w, color.CyanString("****************************\n"))
}

// verboseWriter returns the writer for progress/phase chatter. In JSON output
// mode it always discards so stdout carries only the JSON document.
func verboseWriter(verbose, dryRun bool) io.Writer {
	if outputJSON() {
		return io.Discard
	}
	if verbose || dryRun {
		return os.Stdout
	}
	return io.Discard
}

// uiOut returns the writer for decorative banners/headers printed directly to
// the user. In JSON output mode it discards them to keep stdout pure JSON.
func uiOut() io.Writer {
	if outputJSON() {
		return io.Discard
	}
	return os.Stdout
}

// finishReport emits the end-of-run result for apply-style commands (up, apply,
// clean, down). In JSON mode it writes the machine-readable report to stdout;
// otherwise it prints the human-readable summary line and report.
func finishReport(rpt *report.Report, ctx *applyContext, isDryRun, isVerbose bool) {
	if outputJSON() {
		_ = rpt.WriteJSON(os.Stdout, isDryRun)
		return
	}
	printApplyResult(rpt, ctx, isDryRun, isVerbose)
}

// finishDoctor emits the end-of-run result for the doctor command. In JSON mode
// it writes the machine-readable report; otherwise the recipe-grouped summary.
func finishDoctor(rpt *report.Report, showAll bool) {
	if outputJSON() {
		_ = rpt.WriteJSON(os.Stdout, dryRun)
		return
	}
	rpt.PrintDoctorSummary(os.Stdout, summaryVerbosity(), showAll)
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
