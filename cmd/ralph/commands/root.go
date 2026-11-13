package commands

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mad01/ralph/internal/execx"
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
	// This hook runs for every subcommand. A subcommand that defines its own
	// PersistentPreRunE would shadow it and skip the JSON gate; the command
	// tree walk in output_support_test.go fails if that happens.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		switch outputFormat {
		case "text", "json":
		default:
			return fmt.Errorf("invalid --output %q: must be \"text\" or \"json\"", outputFormat)
		}
		if err := requireOutputSupported(cmd); err != nil {
			return err
		}
		// In JSON mode, suppress progress rendering so stdout carries only JSON.
		progress.SetSilent(outputJSON())
		return nil
	},
}

// jsonOutputAnnotation marks a command whose output honours --output json.
const jsonOutputAnnotation = "ralph/json-output"

// supportJSONOutput opts commands in to --output json. A command without the
// marker rejects -o json before it runs, so a command added later fails safe
// instead of printing text to a reader that expects JSON.
func supportJSONOutput(cmds ...*cobra.Command) {
	for _, c := range cmds {
		if c.Annotations == nil {
			c.Annotations = map[string]string{}
		}
		c.Annotations[jsonOutputAnnotation] = "true"
	}
}

// requireOutputSupported returns an error when -o json was requested for a
// command that has no JSON form.
func requireOutputSupported(cmd *cobra.Command) error {
	if !outputJSON() || cmd.Annotations[jsonOutputAnnotation] == "true" {
		return nil
	}
	return fmt.Errorf("JSON output (-o json) is not supported for %q", cmd.CommandPath())
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
	stopOnSignal()
	if err := executeRoot(); err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// executeRoot runs the command tree. Execute and the in-process tests share
// it, so a test runs the tree exactly as the binary does.
func executeRoot() error {
	// cobra adds its completion group lazily inside Execute; add it now so the
	// walk below covers it.
	rootCmd.InitDefaultCompletionCmd()
	runGroupsAsHelp(rootCmd)
	return rootCmd.Execute()
}

// runGroupsAsHelp gives every command that only groups subcommands (state,
// profile, completion) a RunE that prints its help. For a command with no run
// function cobra prints help and exits 0 without running PersistentPreRunE,
// so `ralph state -o json` would put help text on stdout instead of failing
// the JSON gate. The walk runs at execute time, after every init has
// registered its commands, so a group added later is covered too.
func runGroupsAsHelp(cmd *cobra.Command) {
	if !cmd.Runnable() {
		cmd.RunE = func(c *cobra.Command, _ []string) error { return c.Help() }
	}
	for _, c := range cmd.Commands() {
		runGroupsAsHelp(c)
	}
}

// stopOnSignal makes Ctrl-C, SIGTERM and a terminal hangup stop ralph and
// every command it started. Commands run in their own session (internal/execx),
// so the terminal's SIGINT reaches ralph alone; the handler terminates their
// process groups, shows the cursor again, and exits with 128 plus the signal
// number, as a process the signal killed would.
func stopOnSignal() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		sig := <-signals
		execx.TerminateAll()
		progress.RestoreCursor()
		os.Exit(signalExitCode(sig))
	}()
}

// signalExitCode is the shell's exit status for a process killed by sig.
func signalExitCode(sig os.Signal) int {
	const signalExitBase = 128
	if s, ok := sig.(syscall.Signal); ok {
		return signalExitBase + int(s)
	}
	return 1
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
