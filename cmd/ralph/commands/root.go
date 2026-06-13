package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/mad01/ralph/internal/progress"
	"github.com/mad01/ralph/internal/report"
	"github.com/spf13/cobra"
)

// ExitError is returned by RunE commands to signal a non-zero exit code
// without calling os.Exit directly, making commands testable.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit %d", e.Code)
}

var rootCmd = &cobra.Command{
	Use:   "ralph",
	Short: "ralph is a tool for managing dotfiles and shell configurations.",
	Long: `ralph helps you manage your dotfiles, shell tools, rc files, and helper functions seamlessly.
Inspired by tools like Starship, it uses a TOML configuration file to define how your environment is set up.`,
	// A non-zero exit (notably the routine exit-2 warning path) must not dump
	// the full usage text, and Execute() already prints the error once — so
	// silence cobra's own usage+error printing.
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Default action when ralph is run without subcommands
		fmt.Println("Use 'ralph --help' for more information.")
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		switch outputFormat {
		case "text", "json":
		default:
			return fmt.Errorf("invalid --output %q: must be \"text\" or \"json\"", outputFormat)
		}
		// In JSON mode, suppress progress rendering so stdout carries only JSON.
		progress.SetSilent(outputJSON())
		return nil
	},
}

var (
	dryRun       bool   // Global variable for the dry-run flag
	verbose      bool   // Show all items in summary (including OK and skip)
	quiet        bool   // Show only failures in summary
	outputFormat string // Output format: "text" (default) or "json"
)

// outputJSON reports whether machine-readable JSON output was requested.
func outputJSON() bool { return outputFormat == "json" }

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() { // This init is for the package, not a specific command
	rootCmd.PersistentFlags().
		BoolVarP(&dryRun, "dry-run", "n", false, "Show what changes would be made without actually making them")
	rootCmd.PersistentFlags().
		BoolVarP(&verbose, "verbose", "v", false, "Show all items in summary (including OK and skip)")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Show only failures in summary")
	rootCmd.PersistentFlags().
		StringVarP(&outputFormat, "output", "o", "text", "Output format: text or json")
}

// summaryVerbosity returns the report verbosity level based on --verbose/--quiet flags.
func summaryVerbosity() report.Verbosity {
	if verbose {
		return report.VerbosityVerbose
	}
	if quiet {
		return report.VerbosityQuiet
	}
	return report.VerbosityNormal
}
