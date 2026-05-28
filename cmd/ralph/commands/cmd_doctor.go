package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
			p.AddResult("config", "", report.StatusFail, fmt.Sprintf("failed to load: %v", err), err)
			rpt.PrintDoctorSummary(os.Stdout, summaryVerbosity(), showAll)
			return &ExitError{Code: 1}
		}

		checkDotfiles(rpt, cfg)
		checkDirectories(rpt, cfg)
		checkRepositories(rpt, cfg)
		checkBuilds(rpt, cfg)
		checkPackages(rpt, cfg)
		checkTools(rpt, cfg)
		checkRCFiles(rpt, cfg)

		rpt.PrintDoctorSummary(os.Stdout, summaryVerbosity(), showAll)
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

	dfNames := make([]string, 0, len(cfg.Dotfiles))
	for k := range cfg.Dotfiles {
		dfNames = append(dfNames, k)
	}
	sort.Strings(dfNames)

	for _, name := range dfNames {
		df := cfg.Dotfiles[name]
		recipe := df.OwnerRecipe

		absoluteTarget, expandErr := config.ExpandPath(df.Target)
		if expandErr != nil {
			phase.AddResult(name, recipe, report.StatusFail, fmt.Sprintf("error expanding target path: %v", expandErr), expandErr)
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

	dirNames := make([]string, 0, len(cfg.Directories))
	for k := range cfg.Directories {
		dirNames = append(dirNames, k)
	}
	sort.Strings(dirNames)

	for _, name := range dirNames {
		dir := cfg.Directories[name]
		recipe := dir.OwnerRecipe

		absoluteTarget, expandErr := config.ExpandPath(dir.Target)
		if expandErr != nil {
			phase.AddResult(name, recipe, report.StatusFail, fmt.Sprintf("error expanding path: %v", expandErr), expandErr)
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

	repoNames := make([]string, 0, len(cfg.Repos))
	for k := range cfg.Repos {
		repoNames = append(repoNames, k)
	}
	sort.Strings(repoNames)

	for _, name := range repoNames {
		rp := cfg.Repos[name]
		recipe := rp.OwnerRecipe

		absoluteTarget, expandErr := config.ExpandPath(rp.Target)
		if expandErr != nil {
			phase.AddResult(name, recipe, report.StatusFail, fmt.Sprintf("error expanding path: %v", expandErr), expandErr)
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
		phase.AddResult("build-state", "", report.StatusFail, fmt.Sprintf("error loading build state: %v", stateErr), stateErr)
		return
	}

	buildNames := make([]string, 0, len(cfg.Hooks.Builds))
	for k := range cfg.Hooks.Builds {
		buildNames = append(buildNames, k)
	}
	sort.Strings(buildNames)

	for _, name := range buildNames {
		build := cfg.Hooks.Builds[name]
		recipe := build.OwnerRecipe

		if build.WorkingDir != "" {
			expandedDir, expandErr := config.ExpandPath(build.WorkingDir)
			if expandErr != nil {
				phase.AddResult(name, recipe, report.StatusFail, fmt.Sprintf("error expanding working_dir: %v", expandErr), expandErr)
				continue
			}
			if _, statErr := os.Stat(expandedDir); os.IsNotExist(statErr) {
				phase.AddResult(name, recipe, report.StatusFail, fmt.Sprintf("working_dir '%s' does not exist", expandedDir), nil)
				continue
			}
		}

		if record, exists := buildState.Builds[name]; exists {
			phase.AddResult(name, recipe, report.StatusOK, fmt.Sprintf("completed at %s", record.CompletedAt.Format("2006-01-02 15:04:05")), nil)
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

func checkPackages(rpt *report.Report, cfg *config.Config) {
	if len(cfg.Packages) == 0 {
		return
	}
	phase := rpt.AddPhase("Packages")

	buildState, stateErr := hooks.LoadBuildState()
	if stateErr != nil {
		phase.AddResult("build-state", "", report.StatusFail, fmt.Sprintf("error loading build state: %v", stateErr), stateErr)
		return
	}

	pkgNames := make([]string, 0, len(cfg.Packages))
	for k := range cfg.Packages {
		pkgNames = append(pkgNames, k)
	}
	sort.Strings(pkgNames)

	for _, name := range pkgNames {
		pkg := cfg.Packages[name]
		recipe := pkg.OwnerRecipe

		workDir := pkg.WorkingDir
		if pkg.Source == "remote" && workDir == "" {
			resolved := packages.ResolvePackagePaths(name, pkg, cfg.PackagesDir)
			workDir = resolved.WorkingDir
		}

		if workDir != "" {
			expandedDir, expandErr := config.ExpandPath(workDir)
			if expandErr != nil {
				phase.AddResult(name, recipe, report.StatusFail, fmt.Sprintf("error expanding working_dir: %v", expandErr), expandErr)
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
					phase.AddResult(name, recipe, report.StatusWarn, "directory exists but is not a git repository", nil)
					continue
				}
			}
		}

		stateKey := "pkg:" + name
		if record, exists := buildState.Builds[stateKey]; exists {
			phase.AddResult(name, recipe, report.StatusOK, fmt.Sprintf("last built at %s", record.CompletedAt.Format("2006-01-02 15:04:05")), nil)
		} else {
			phase.AddResult(name, recipe, report.StatusWarn, "never built", nil)
		}
	}
}

func checkTools(rpt *report.Report, cfg *config.Config) {
	if len(cfg.Tools) == 0 {
		return
	}
	phase := rpt.AddPhase("Tools")

	for _, t := range cfg.Tools {
		recipe := t.OwnerRecipe
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
			phase.AddResult(shellName, "", report.StatusFail, fmt.Sprintf("could not read RC file: %v", err), err)
			continue
		}

		contentStr := string(content)
		if strings.Contains(contentStr, shell.RalphBlockBeginMarker) && strings.Contains(contentStr, shell.RalphBlockEndMarker) {
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
