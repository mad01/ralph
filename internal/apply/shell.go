package apply

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/progress"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/shell"
	"github.com/mad01/ralph/internal/state"
)

// applyShellConfig generates shell alias, function, and env files, then injects source lines.
func applyShellConfig(s *sequence) {
	fmt.Fprintln(s.out, "\nProcessing shell configurations...")
	shellPhase := s.rpt.AddPhase("Shell config")
	defer func() {
		if !s.opts.Verbose && !s.opts.DryRun {
			_, _, fail, _ := shellPhase.Counts()
			progress.StatusLine("Shell config", fail == 0)
		}
	}()
	resolvedShells := shell.ResolveShell(s.cfg.Shell.Name)
	currentShell := resolvedShells[0]
	if len(resolvedShells) > 1 {
		// Fallback to all shells means we couldn't determine a single shell
		fmt.Fprintln(
			s.errOut,
			color.YellowString("Could not determine current shell. Skipping shell configuration."),
		)
		shellPhase.AddSkip("shell", "could not determine shell")
		return
	}

	fmt.Fprintf(s.out, "  Detected shell: %s\n", currentShell)
	linesToSource, ok := generateShellFiles(s, currentShell, shellPhase)
	if !ok {
		return
	}

	if len(linesToSource) == 0 {
		fmt.Fprintln(s.out, "  No shell aliases, functions, or env vars configured to source.")
		shellPhase.AddOK(string(currentShell), "nothing to source")
		return
	}

	fmt.Fprintf(s.out, "  Injecting source lines into %s rc file...\n", currentShell)
	if err := shell.InjectSourceLines(s.out, currentShell, linesToSource, s.opts.DryRun); err != nil {
		fmt.Fprintln(
			s.errOut,
			color.RedString(
				"  Error injecting source lines into %s rc file: %v",
				currentShell,
				err,
			),
		)
		shellPhase.AddFail(string(currentShell), fmt.Sprintf("inject source lines: %v", err), err)
	} else {
		shellPhase.AddOK(string(currentShell), "")
	}
}

// generateShellFiles writes the generated env, alias and function files for
// currentShell and returns the source lines the rc file needs, env first so
// aliases and functions can reference env vars. A failure is printed and
// recorded in shellPhase, and ok is false.
func generateShellFiles(
	s *sequence,
	currentShell shell.SupportedShell,
	shellPhase *report.Phase,
) (linesToSource []string, ok bool) {
	aliasFile, funcFile, genErr := shell.GenerateShellConfigs(
		s.out,
		s.cfg,
		currentShell,
		s.opts.DryRun,
	)

	if genErr != nil {
		fmt.Fprintln(
			s.errOut,
			color.RedString("  Error generating shell configs for %s: %v", currentShell, genErr),
		)
		shellPhase.AddFail(
			string(currentShell),
			fmt.Sprintf("generate configs: %v", genErr),
			genErr,
		)
		return nil, false
	}

	// Generate env file -- sourced before aliases and functions so they can reference env vars.
	envFilePath, envPathErr := shell.GetEnvFilePath()
	if envPathErr != nil {
		fmt.Fprintln(s.errOut, color.RedString("  Error getting env file path: %v", envPathErr))
		shellPhase.AddFail(
			string(currentShell),
			fmt.Sprintf("env file path: %v", envPathErr),
			envPathErr,
		)
		return nil, false
	}
	if envErr := shell.GenerateEnvFile(s.out, s.cfg.Shell.Env, envFilePath, s.opts.DryRun); envErr != nil {
		fmt.Fprintln(s.errOut, color.RedString("  Error generating env file: %v", envErr))
		shellPhase.AddFail(
			string(currentShell),
			fmt.Sprintf("generate env file: %v", envErr),
			envErr,
		)
		return nil, false
	}

	return sourceLines(s, envFilePath, aliasFile, funcFile), true
}

// sourceLines returns the rc-file lines that source the generated files: env
// first so aliases and functions can reference env vars, then aliases, then
// functions. A file is sourced only when it has entries, or on a dry run when
// it was named.
func sourceLines(s *sequence, envFile, aliasFile, funcFile string) []string {
	lines := []string{}
	if len(s.cfg.Shell.Env) > 0 {
		lines = append(lines, fmt.Sprintf("source %s", toPortablePath(envFile)))
	}
	if aliasFile != "" && (len(s.cfg.Shell.Aliases) > 0 || (s.opts.DryRun && aliasFile != "")) {
		lines = append(lines, fmt.Sprintf("source %s", toPortablePath(aliasFile)))
	}
	if funcFile != "" && (len(s.cfg.Shell.Functions) > 0 || (s.opts.DryRun && funcFile != "")) {
		lines = append(lines, fmt.Sprintf("source %s", toPortablePath(funcFile)))
	}
	return lines
}

// declareShell records, by name, every applying shell alias and function a
// recipe owns. shell.env is a flat map[string]string with no per-entry owner,
// so env vars are not recorded: they are rarely orphaned, and the right fix is
// a typed ShellConfig.Env.
func declareShell(m *manifest) {
	for name, alias := range m.cfg.Shell.Aliases {
		if alias.OwnerRecipe == "" ||
			!m.applies(alias.Enable, alias.Hosts, alias.Profiles) {
			continue
		}
		m.own(alias.OwnerRecipe, state.KindShellAlias, name)
	}
	for name, fn := range m.cfg.Shell.Functions {
		if fn.OwnerRecipe == "" || !m.applies(fn.Enable, fn.Hosts, fn.Profiles) {
			continue
		}
		m.own(fn.OwnerRecipe, state.KindShellFunc, name)
	}
}

// toPortablePath converts an absolute path to use $HOME instead of the expanded home directory.
// This makes the path portable across different users/machines.
func toPortablePath(path string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, homeDir) {
		return "$HOME" + path[len(homeDir):]
	}
	return path
}
