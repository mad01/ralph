package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mad01/ralph/internal/binversion"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/hooks"
	"github.com/mad01/ralph/internal/packages"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/shell"
	"github.com/mad01/ralph/internal/tool"
	"github.com/spf13/cobra"
)

var doctorAll bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the health of the ralph setup",
	Long:  `Performs a series of checks to ensure ralph is configured correctly and all managed items are in a healthy state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		rpt := &report.Report{Command: "doctor"}
		showAll := doctorAll || verbose

		cfg, err := config.LoadConfig()
		if err != nil {
			p := rpt.AddPhase("Configuration")
			p.AddResult(
				"config",
				"",
				report.StatusFail,
				fmt.Sprintf("failed to load: %v", err),
				err,
			)
			finishDoctor(rpt, showAll)
			return &ExitError{Code: 1}
		}

		checkDotfiles(rpt, cfg)
		checkDirectories(rpt, cfg)
		checkRepositories(rpt, cfg)
		checkBuilds(rpt, cfg)
		checkPackages(rpt, cfg)
		checkTools(rpt, cfg)
		checkRCFiles(rpt, cfg)

		finishDoctor(rpt, showAll)
		if code := rpt.ExitCode(); code != 0 {
			return &ExitError{Code: code}
		}
		return nil
	},
}

func checkDotfiles(rpt *report.Report, cfg *config.Config) {
	if len(cfg.Dotfiles) == 0 {
		return
	}
	phase := rpt.AddPhase("Dotfiles")
	expandedRepoPath, _ := config.ExpandPath(cfg.DotfilesRepoPath)
	currentHost := config.GetCurrentHost()

	dfNames := make([]string, 0, len(cfg.Dotfiles))
	for k := range cfg.Dotfiles {
		dfNames = append(dfNames, k)
	}
	sort.Strings(dfNames)

	for _, name := range dfNames {
		df := cfg.Dotfiles[name]
		recipe := df.OwnerRecipe

		if !config.IsEnabled(df.Enable) {
			phase.AddResult(name, recipe, report.StatusSkip, "disabled", nil)
			continue
		}

		if !config.ShouldApplyForHost(df.Hosts, currentHost) {
			phase.AddResult(name, recipe, report.StatusSkip, "other host", nil)
			continue
		}

		absoluteTarget, expandErr := config.ExpandPath(df.Target)
		if expandErr != nil {
			phase.AddResult(
				name,
				recipe,
				report.StatusFail,
				fmt.Sprintf("error expanding target path: %v", expandErr),
				expandErr,
			)
			continue
		}

		status, msg, err := validateDotfileTarget(df, absoluteTarget, expandedRepoPath)
		phase.AddResult(name, recipe, status, msg, err)
	}
}

func checkDirectories(rpt *report.Report, cfg *config.Config) {
	if len(cfg.Directories) == 0 {
		return
	}
	phase := rpt.AddPhase("Directories")
	currentHost := config.GetCurrentHost()

	dirNames := make([]string, 0, len(cfg.Directories))
	for k := range cfg.Directories {
		dirNames = append(dirNames, k)
	}
	sort.Strings(dirNames)

	for _, name := range dirNames {
		dir := cfg.Directories[name]
		recipe := dir.OwnerRecipe

		if !config.IsEnabled(dir.Enable) {
			phase.AddResult(name, recipe, report.StatusSkip, "disabled", nil)
			continue
		}

		if !config.ShouldApplyForHost(dir.Hosts, currentHost) {
			phase.AddResult(name, recipe, report.StatusSkip, "other host", nil)
			continue
		}

		absoluteTarget, expandErr := config.ExpandPath(dir.Target)
		if expandErr != nil {
			phase.AddResult(
				name,
				recipe,
				report.StatusFail,
				fmt.Sprintf("error expanding path: %v", expandErr),
				expandErr,
			)
			continue
		}
		info, statErr := os.Stat(absoluteTarget)
		if os.IsNotExist(statErr) {
			phase.AddResult(name, recipe, report.StatusWarn, "does not exist", nil)
		} else if statErr != nil {
			phase.AddResult(name, recipe, report.StatusFail, fmt.Sprintf("error checking: %v", statErr), statErr)
		} else if !info.IsDir() {
			phase.AddResult(name, recipe, report.StatusFail, "exists but is not a directory", nil)
		} else {
			phase.AddResult(name, recipe, report.StatusOK, "", nil)
		}
	}
}

func checkRepositories(rpt *report.Report, cfg *config.Config) {
	if len(cfg.Repos) == 0 {
		return
	}
	phase := rpt.AddPhase("Repositories")
	currentHost := config.GetCurrentHost()

	repoNames := make([]string, 0, len(cfg.Repos))
	for k := range cfg.Repos {
		repoNames = append(repoNames, k)
	}
	sort.Strings(repoNames)

	for _, name := range repoNames {
		rp := cfg.Repos[name]
		recipe := rp.OwnerRecipe

		if !config.IsEnabled(rp.Enable) {
			phase.AddResult(name, recipe, report.StatusSkip, "disabled", nil)
			continue
		}

		if !config.ShouldApplyForHost(rp.Hosts, currentHost) {
			phase.AddResult(name, recipe, report.StatusSkip, "other host", nil)
			continue
		}

		absoluteTarget, expandErr := config.ExpandPath(rp.Target)
		if expandErr != nil {
			phase.AddResult(
				name,
				recipe,
				report.StatusFail,
				fmt.Sprintf("error expanding path: %v", expandErr),
				expandErr,
			)
			continue
		}
		info, statErr := os.Stat(absoluteTarget)
		if os.IsNotExist(statErr) {
			phase.AddResult(name, recipe, report.StatusWarn, "not cloned", nil)
		} else if statErr != nil {
			phase.AddResult(name, recipe, report.StatusFail, fmt.Sprintf("error checking: %v", statErr), statErr)
		} else if !info.IsDir() {
			phase.AddResult(name, recipe, report.StatusFail, "target exists but is not a directory", nil)
		} else {
			gitDir := filepath.Join(absoluteTarget, ".git")
			if _, gitErr := os.Stat(gitDir); os.IsNotExist(gitErr) {
				phase.AddResult(name, recipe, report.StatusWarn, "directory exists but is not a git repository", nil)
			} else {
				phase.AddResult(name, recipe, report.StatusOK, "", nil)
			}
		}
	}
}

func checkBuilds(rpt *report.Report, cfg *config.Config) {
	if len(cfg.Hooks.Builds) == 0 {
		return
	}
	phase := rpt.AddPhase("Builds")

	buildState, stateErr := hooks.LoadBuildState()
	if stateErr != nil {
		phase.AddResult(
			"build-state",
			"",
			report.StatusFail,
			fmt.Sprintf("error loading build state: %v", stateErr),
			stateErr,
		)
		return
	}

	currentHost := config.GetCurrentHost()

	buildNames := make([]string, 0, len(cfg.Hooks.Builds))
	for k := range cfg.Hooks.Builds {
		buildNames = append(buildNames, k)
	}
	sort.Strings(buildNames)

	for _, name := range buildNames {
		build := cfg.Hooks.Builds[name]
		recipe := build.OwnerRecipe

		if !config.IsEnabled(build.Enable) {
			phase.AddResult(name, recipe, report.StatusSkip, "disabled", nil)
			continue
		}

		if !config.ShouldApplyForHost(build.Hosts, currentHost) {
			phase.AddResult(name, recipe, report.StatusSkip, "other host", nil)
			continue
		}

		var expandedDir string
		if build.WorkingDir != "" {
			ed, expandErr := config.ExpandPath(build.WorkingDir)
			if expandErr != nil {
				phase.AddResult(
					name,
					recipe,
					report.StatusFail,
					fmt.Sprintf("error expanding working_dir: %v", expandErr),
					expandErr,
				)
				continue
			}
			if _, statErr := os.Stat(ed); os.IsNotExist(statErr) {
				phase.AddResult(
					name,
					recipe,
					report.StatusFail,
					fmt.Sprintf("working_dir '%s' does not exist", ed),
					nil,
				)
				continue
			}
			expandedDir = ed
		}

		// A verify command is the authoritative state check: it inspects the
		// build's output rather than trusting the run mode. Drift is reconcilable
		// by `ralph up`, so it surfaces as a Warn, not a Fail.
		if strings.TrimSpace(build.Verify) != "" {
			ok, out, verifyErr := runBuildVerify(build.Verify, expandedDir)
			if ok {
				msg := "verified"
				if line := firstLine(out); line != "" {
					msg = "verified: " + line
				}
				phase.AddResult(name, recipe, report.StatusOK, msg, nil)
			} else {
				msg := "drift detected — run 'ralph up'"
				if line := firstLine(out); line != "" {
					msg = line + " — run 'ralph up'"
				}
				phase.AddResult(name, recipe, report.StatusWarn, msg, verifyErr)
			}
			continue
		}

		if record, exists := buildState.Builds[name]; exists {
			msg := fmt.Sprintf(
				"completed at %s%s",
				record.CompletedAt.Format("2006-01-02 15:04:05"),
				installedVersionNote(build.InstallPaths),
			)
			phase.AddResult(name, recipe, report.StatusOK, msg, nil)
		} else {
			switch build.Run {
			case "once":
				phase.AddResult(name, recipe, report.StatusWarn, "not yet run", nil)
			case "always":
				phase.AddResult(name, recipe, report.StatusOK, "runs every apply", nil)
			case "manual":
				phase.AddResult(name, recipe, report.StatusSkip, "manual", nil)
			}
		}
	}
}

// buildVerifyTimeout caps how long a verify command may run. Doctor is
// interactive, so this is intentionally short — verify commands should be cheap
// state inspections, not builds.
const buildVerifyTimeout = 30 * time.Second

// runBuildVerify runs a build's verify command via `sh -c` in workingDir.
// It returns ok=true when the command exits 0, along with the combined
// stdout+stderr (trimmed) for use in the doctor report.
func runBuildVerify(verifyCmd, workingDir string) (ok bool, output string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), buildVerifyTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", verifyCmd)
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	out := strings.TrimSpace(buf.String())
	if ctx.Err() == context.DeadlineExceeded {
		return false, out, fmt.Errorf("verify timed out after %s", buildVerifyTimeout)
	}
	return runErr == nil, out, runErr
}

// firstLine returns the first non-empty line of s, for one-line report messages.
func firstLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func checkPackages(rpt *report.Report, cfg *config.Config) {
	if len(cfg.Packages) == 0 {
		return
	}
	phase := rpt.AddPhase("Packages")

	buildState, stateErr := hooks.LoadBuildState()
	if stateErr != nil {
		phase.AddResult(
			"build-state",
			"",
			report.StatusFail,
			fmt.Sprintf("error loading build state: %v", stateErr),
			stateErr,
		)
		return
	}

	currentHost := config.GetCurrentHost()

	pkgNames := make([]string, 0, len(cfg.Packages))
	for k := range cfg.Packages {
		pkgNames = append(pkgNames, k)
	}
	sort.Strings(pkgNames)

	for _, name := range pkgNames {
		pkg := cfg.Packages[name]
		recipe := pkg.OwnerRecipe

		if !config.IsEnabled(pkg.Enable) {
			phase.AddResult(name, recipe, report.StatusSkip, "disabled", nil)
			continue
		}

		if !config.ShouldApplyForHost(pkg.Hosts, currentHost) {
			phase.AddResult(name, recipe, report.StatusSkip, "other host", nil)
			continue
		}

		workDir := pkg.WorkingDir
		if pkg.Source == "remote" && workDir == "" {
			resolved := packages.ResolvePackagePaths(name, pkg, cfg.PackagesDir)
			workDir = resolved.WorkingDir
		}

		if workDir != "" {
			expandedDir, expandErr := config.ExpandPath(workDir)
			if expandErr != nil {
				phase.AddResult(
					name,
					recipe,
					report.StatusFail,
					fmt.Sprintf("error expanding working_dir: %v", expandErr),
					expandErr,
				)
				continue
			}
			info, statErr := os.Stat(expandedDir)
			if os.IsNotExist(statErr) {
				if pkg.Source == "remote" {
					phase.AddResult(name, recipe, report.StatusWarn, "not cloned", nil)
				} else {
					phase.AddResult(name, recipe, report.StatusFail, fmt.Sprintf("working_dir '%s' does not exist", expandedDir), nil)
				}
				continue
			} else if statErr != nil {
				phase.AddResult(name, recipe, report.StatusFail, fmt.Sprintf("error checking: %v", statErr), statErr)
				continue
			} else if !info.IsDir() {
				phase.AddResult(name, recipe, report.StatusFail, "path exists but is not a directory", nil)
				continue
			}

			if pkg.Source == "remote" {
				gitDir := filepath.Join(expandedDir, ".git")
				if _, gitErr := os.Stat(gitDir); os.IsNotExist(gitErr) {
					phase.AddResult(
						name,
						recipe,
						report.StatusWarn,
						"directory exists but is not a git repository",
						nil,
					)
					continue
				}
			}
		}

		stateKey := "pkg:" + name
		if record, exists := buildState.Builds[stateKey]; exists {
			msg := fmt.Sprintf(
				"last built at %s%s",
				record.CompletedAt.Format("2006-01-02 15:04:05"),
				installedVersionNote(pkg.InstallPaths),
			)
			phase.AddResult(name, recipe, report.StatusOK, msg, nil)
		} else {
			phase.AddResult(name, recipe, report.StatusWarn, "never built", nil)
		}
	}
}

// installedVersionNote probes the first install_paths binary for the build it
// reports (the `version -o json` convention) and returns a short, purely
// informational suffix like " (installed deadbeef)". It returns "" when the
// item declares no install_paths or the binary doesn't implement the
// convention. This is identity info only — it is deliberately NOT used to flag
// staleness, because a binary embeds the commit it was built from while build
// freshness is keyed on the working_dir subtree (see internal/gitutil).
func installedVersionNote(installPaths []string) string {
	if len(installPaths) == 0 {
		return ""
	}
	binPath, err := config.ExpandPath(installPaths[0])
	if err != nil {
		return ""
	}
	sha, ok := binversion.Probe(binPath)
	if !ok {
		return ""
	}
	if len(sha) > 8 {
		sha = sha[:8]
	}
	return fmt.Sprintf(" (installed %s)", sha)
}

func checkTools(rpt *report.Report, cfg *config.Config) {
	if len(cfg.Tools) == 0 {
		return
	}
	phase := rpt.AddPhase("Tools")
	currentHost := config.GetCurrentHost()

	for _, t := range cfg.Tools {
		recipe := t.OwnerRecipe
		if !config.IsEnabled(t.Enable) {
			phase.AddResult(t.Name, recipe, report.StatusSkip, "disabled", nil)
			continue
		}
		if !config.ShouldApplyForHost(t.Hosts, currentHost) {
			phase.AddResult(t.Name, recipe, report.StatusSkip, "other host", nil)
			continue
		}
		if tool.CheckStatus(t.CheckCommand) {
			phase.AddResult(t.Name, recipe, report.StatusOK, "installed", nil)
		} else {
			msg := "not installed"
			if t.InstallHint != "" {
				msg = fmt.Sprintf("not installed (hint: %s)", t.InstallHint)
			}
			phase.AddResult(t.Name, recipe, report.StatusWarn, msg, nil)
		}
	}
}

func checkRCFiles(rpt *report.Report, cfg *config.Config) {
	phase := rpt.AddPhase("RC files")
	shellsToTest := shell.ResolveShell(cfg.Shell.Name)

	for _, s := range shellsToTest {
		shellName := string(s)
		rcPath, err := shell.GetRCFilePath(s)
		if err != nil {
			phase.AddResult(shellName, "", report.StatusSkip, "could not get RC file path", nil)
			continue
		}
		if _, err := os.Stat(rcPath); os.IsNotExist(err) {
			phase.AddResult(shellName, "", report.StatusSkip, "RC file does not exist", nil)
			continue
		}
		content, err := os.ReadFile(rcPath)
		if err != nil {
			phase.AddResult(
				shellName,
				"",
				report.StatusFail,
				fmt.Sprintf("could not read RC file: %v", err),
				err,
			)
			continue
		}

		contentStr := string(content)
		if strings.Contains(contentStr, shell.RalphBlockBeginMarker) &&
			strings.Contains(contentStr, shell.RalphBlockEndMarker) {
			blockStartIndex := strings.Index(contentStr, shell.RalphBlockBeginMarker)
			blockEndIndex := strings.Index(contentStr, shell.RalphBlockEndMarker)
			blockContent := contentStr[blockStartIndex+len(shell.RalphBlockBeginMarker) : blockEndIndex]
			blockLines := strings.Split(blockContent, "\n")
			foundMissingSourceFiles := false
			sourcedFilesExpected := (len(cfg.Shell.Aliases) > 0 || len(cfg.Shell.Functions) > 0)
			sourcedFilesFoundInBlock := 0

			for _, line := range blockLines {
				trimmedLine := strings.TrimSpace(line)
				var sourcedFile string
				if after, ok := strings.CutPrefix(trimmedLine, "source "); ok {
					sourcedFile = after
				} else if after, ok := strings.CutPrefix(trimmedLine, ". "); ok {
					sourcedFile = after
				}

				if sourcedFile != "" {
					sourcedFilesFoundInBlock++
					expandedSourcedFile, expErr := config.ExpandPath(sourcedFile)
					if expErr != nil {
						foundMissingSourceFiles = true
						continue
					}
					if _, statErr := os.Stat(expandedSourcedFile); statErr != nil {
						foundMissingSourceFiles = true
					}
				}
			}

			if foundMissingSourceFiles {
				phase.AddResult(shellName, "", report.StatusFail, "sourced file(s) missing", nil)
			} else if sourcedFilesExpected && sourcedFilesFoundInBlock == 0 {
				phase.AddResult(shellName, "", report.StatusWarn, "block found but no source commands detected", nil)
			} else {
				phase.AddResult(shellName, "", report.StatusOK, "", nil)
			}
		} else {
			if len(cfg.Shell.Aliases) > 0 || len(cfg.Shell.Functions) > 0 {
				phase.AddResult(shellName, "", report.StatusWarn, "ralph block missing but aliases/functions configured (run apply to fix)", nil)
			} else {
				phase.AddResult(shellName, "", report.StatusWarn, "ralph block not found", nil)
			}
		}
	}
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorAll, "all", false, "show all items, not just problems")
	rootCmd.AddCommand(doctorCmd)
}
