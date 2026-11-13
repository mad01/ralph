package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mad01/ralph/internal/binversion"
	"github.com/mad01/ralph/internal/buildinfo"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/execx"
	"github.com/mad01/ralph/internal/gitutil"
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

		// A check writes nothing: no recipe source is cloned or moved.
		cfg, err := config.LoadConfigWithOptions(config.LoadOptions{ReadOnly: true})
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

		checkConfig(rpt, cfg)
		checkGit(rpt, cfg)
		checkStateFiles(rpt)
		checkRecipeSources(rpt, cfg)
		checkDotfiles(rpt, cfg)
		checkDirectories(rpt, cfg)
		checkDanglingLinks(rpt, cfg)
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

// checkConfig reports on config file resolution: the active config path and the
// config.local.toml overlay (loaded with its profiles, loaded without profiles,
// or missing). A machine with no profiles is a warning because profile-scoped
// recipes won't apply to it.
func checkConfig(rpt *report.Report, cfg *config.Config) {
	phase := rpt.AddPhase("Configuration")

	configPath, err := config.GetDefaultConfigPath()
	if err != nil {
		phase.AddResult("config path", "", report.StatusFail, fmt.Sprintf("%v", err), err)
		return
	}
	phase.AddResult("config.toml", "", report.StatusOK, configPath, nil)

	// A missing overlay and an overlay with no profiles are the same problem
	// from the operator's view — the machine has no profiles — so report it as a
	// single warning rather than two. Only when the overlay exists do we
	// distinguish "loaded" from "loaded but no profiles".
	localPath := config.LocalConfigPath(configPath)
	_, statErr := os.Stat(localPath)
	switch {
	case statErr == nil && len(cfg.Profiles) > 0:
		phase.AddResult(
			"config.local.toml",
			"",
			report.StatusOK,
			"loaded: "+strings.Join(cfg.Profiles, ", "),
			nil,
		)
	case statErr == nil:
		phase.AddResult(
			"config.local.toml",
			"",
			report.StatusWarn,
			"loaded but no profiles set",
			nil,
		)
	case os.IsNotExist(statErr):
		phase.AddResult(
			"config.local.toml",
			"",
			report.StatusWarn,
			"not found — machine has no profiles; create "+localPath,
			nil,
		)
	default:
		phase.AddResult(
			"config.local.toml",
			"",
			report.StatusWarn,
			fmt.Sprintf("%v", statErr),
			statErr,
		)
	}
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

		reason := config.SkipReason(df.Enable, df.Hosts, df.Profiles, currentHost, cfg.Profiles)
		if reason != "" {
			phase.AddResult(name, recipe, report.StatusSkip, doctorSkipText(reason), nil)
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

		reason := config.SkipReason(dir.Enable, dir.Hosts, dir.Profiles, currentHost, cfg.Profiles)
		if reason != "" {
			phase.AddResult(name, recipe, report.StatusSkip, doctorSkipText(reason), nil)
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

		reason := config.SkipReason(rp.Enable, rp.Hosts, rp.Profiles, currentHost, cfg.Profiles)
		if reason != "" {
			phase.AddResult(name, recipe, report.StatusSkip, doctorSkipText(reason), nil)
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

		reason := config.SkipReason(
			build.Enable,
			build.Hosts,
			build.Profiles,
			currentHost,
			cfg.Profiles,
		)
		if reason != "" {
			phase.AddResult(name, recipe, report.StatusSkip, doctorSkipText(reason), nil)
			continue
		}

		expandedDir, ok := checkBuildDir(phase, name, build)
		if !ok {
			continue
		}
		// A verify command is the authoritative state check: it inspects the
		// build's output rather than trusting the run mode.
		if strings.TrimSpace(build.Verify) != "" {
			phase.AddStep(buildVerifyStep(name, build, expandedDir))
			continue
		}
		phase.AddStep(buildStateStep(name, build, expandedDir, buildState))
	}
}

// checkBuildDir expands a build's working_dir, "" when it has none, and
// checks it exists. A problem is reported on phase and returns ok=false.
func checkBuildDir(phase *report.Phase, name string, build config.Build) (string, bool) {
	if build.WorkingDir == "" {
		return "", true
	}
	dir, err := config.ExpandPath(build.WorkingDir)
	if err != nil {
		msg := fmt.Sprintf("error expanding working_dir: %v", err)
		phase.AddResult(name, build.OwnerRecipe, report.StatusFail, msg, err)
		return "", false
	}
	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		msg := fmt.Sprintf("working_dir '%s' does not exist", dir)
		phase.AddResult(name, build.OwnerRecipe, report.StatusFail, msg, nil)
		return "", false
	}
	return dir, true
}

// buildVerifyStep runs a build's verify command. Drift is reconcilable by
// `ralph up`, so it surfaces as a Warn, not a Fail.
func buildVerifyStep(name string, build config.Build, workingDir string) report.StepResult {
	ok, out, verifyErr := runBuildVerify(build.Verify, workingDir)
	step := report.StepResult{Name: name, Recipe: build.OwnerRecipe}
	if ok {
		step.Status, step.Message = report.StatusOK, "verified"
		if line := firstLine(out); line != "" {
			step.Message = "verified: " + line
		}
		return step
	}
	step.Status, step.Message, step.Err = report.StatusWarn, "drift detected"+doctorRunUp, verifyErr
	if line := firstLine(out); line != "" {
		step.Message = line + doctorRunUp
	}
	return step
}

// doctorRunUp ends the message for drift a plain `ralph up` reconciles.
const doctorRunUp = " — run 'ralph up'"

// Drift messages shared by builds and packages.
const (
	msgSourceChanged = "source changed since last build" + doctorRunUp
	msgNoSourceHash  = "no recorded source hash — change detection inactive; run 'ralph up'"
	msgUncommitted   = "uncommitted changes" + doctorRunUp
)

// buildStateStep reports a build without a verify command by asking
// hooks.DecideBuild, the rule ralph up runs builds by. A build ralph up would
// rerun because its source moved is drift; a recorded build is otherwise
// reported with the time it completed.
func buildStateStep(
	name string,
	build config.Build,
	workingDir string,
	state *hooks.BuildState,
) report.StepResult {
	step := report.StepResult{Name: name, Recipe: build.OwnerRecipe}
	decision, err := hooks.DecideBuild(hooks.BuildInput{
		Name:       name,
		Build:      build,
		WorkingDir: workingDir,
		Facts:      hooks.StateFacts{State: state, Name: name, WorkingDir: workingDir},
	})
	if err != nil {
		step.Status, step.Message, step.Err = report.StatusFail, err.Error(), err
		return step
	}
	record, recorded := state.Builds[name]
	switch {
	case decision.Reason == hooks.ReasonSourceChanged:
		step.Status, step.Message = report.StatusWarn, msgSourceChanged
	case decision.Reason == hooks.ReasonUncommitted:
		step.Status, step.Message = report.StatusWarn, msgUncommitted
	case recorded:
		info, probed := installedBuild(build.InstallPaths, build.VersionCheck)
		step.Status = report.StatusOK
		step.Message = fmt.Sprintf(
			"completed at %s%s",
			record.CompletedAt.Format("2006-01-02 15:04:05"),
			installedVersionNote(info, probed),
		)
		step.Build = buildMeta(info, probed)
	case decision.Reason == hooks.ReasonNotYetRun:
		step.Status, step.Message = report.StatusWarn, string(hooks.ReasonNotYetRun)
	case decision.Reason == hooks.ReasonAlways:
		step.Status, step.Message = report.StatusOK, string(hooks.ReasonAlways)
	default:
		step.Status, step.Message = report.StatusSkip, string(hooks.ReasonManual)
	}
	return step
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

	cmd := execx.Command(ctx, "sh", "-c", verifyCmd)
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

		reason := config.SkipReason(pkg.Enable, pkg.Hosts, pkg.Profiles, currentHost, cfg.Profiles)
		if reason != "" {
			phase.AddResult(name, recipe, report.StatusSkip, doctorSkipText(reason), nil)
			continue
		}

		workDir, ok := checkPackageDir(phase, name, pkg, cfg.PackagesDir)
		if !ok {
			continue
		}
		phase.AddStep(packageStateStep(name, pkg, workDir, buildState))
	}
}

// checkPackageDir checks a package's working dir and returns it expanded,
// "" for a go-install package, which has none. A problem is reported on
// phase and returns ok=false.
func checkPackageDir(
	phase *report.Phase,
	name string,
	pkg config.Package,
	packagesDir string,
) (workDir string, ok bool) {
	recipe := pkg.OwnerRecipe
	workDir = pkg.WorkingDir
	if pkg.Source.Clones() && workDir == "" {
		workDir = packages.ResolvePackagePaths(name, pkg, packagesDir).WorkingDir
	}
	if workDir == "" {
		return "", true
	}

	expandedDir, expandErr := config.ExpandPath(workDir)
	if expandErr != nil {
		msg := fmt.Sprintf("error expanding working_dir: %v", expandErr)
		phase.AddResult(name, recipe, report.StatusFail, msg, expandErr)
		return "", false
	}
	info, statErr := os.Stat(expandedDir)
	switch {
	case os.IsNotExist(statErr) && pkg.Source.Clones():
		phase.AddResult(name, recipe, report.StatusWarn, "not cloned", nil)
		return "", false
	case os.IsNotExist(statErr):
		msg := fmt.Sprintf("working_dir '%s' does not exist", expandedDir)
		phase.AddResult(name, recipe, report.StatusFail, msg, nil)
		return "", false
	case statErr != nil:
		msg := fmt.Sprintf("error checking: %v", statErr)
		phase.AddResult(name, recipe, report.StatusFail, msg, statErr)
		return "", false
	case !info.IsDir():
		phase.AddResult(name, recipe, report.StatusFail, "path exists but is not a directory", nil)
		return "", false
	}

	if pkg.Source.Clones() {
		if _, gitErr := os.Stat(filepath.Join(expandedDir, ".git")); os.IsNotExist(gitErr) {
			msg := "directory exists but is not a git repository"
			phase.AddResult(name, recipe, report.StatusWarn, msg, nil)
			return "", false
		}
	}
	return expandedDir, true
}

// packageStateStep reports a package whose working dir checks out by asking
// packages.DecidePackage, the rule ralph up builds by, then packageSkew for
// a binary built before its source last changed.
func packageStateStep(
	name string,
	pkg config.Package,
	workDir string,
	state *hooks.BuildState,
) report.StepResult {
	step := report.StepResult{Name: name, Recipe: pkg.OwnerRecipe, Status: report.StatusWarn}
	record, exists := state.Builds["pkg:"+name]
	if !exists {
		step.Message = string(packages.CauseNeverBuilt)
		return step
	}

	installCheck := packages.CheckInstalledPackage(pkg, workDir, record)
	info, probed := installCheck.BuildInfo, installCheck.VersionProbed
	step.Build = buildMeta(info, probed)

	observed := pkg
	observed.WorkingDir = workDir
	in := packages.ObservePackage(observed, &record)
	in.Installed = func() packages.InstallationCheck { return installCheck }
	if d := packages.DecidePackage(in); d.NeedsBuild {
		step.Message = packageDriftMessage(d)
		return step
	}
	if msg, skewed := packageSkew(workDir, record.GitHash, stalenessRef(info)); skewed {
		step.Message = msg
		return step
	}

	step.Status = report.StatusOK
	step.Message = fmt.Sprintf(
		"last built at %s%s",
		record.CompletedAt.Format("2006-01-02 15:04:05"),
		installedVersionNote(info, probed),
	)
	return step
}

// packageDriftMessage words a DecidePackage verdict that needs a build.
func packageDriftMessage(d packages.Freshness) string {
	switch d.Cause {
	case packages.CauseNoSourceHash:
		return msgNoSourceHash
	case packages.CauseSourceChanged:
		return msgSourceChanged
	case packages.CauseVersionChanged:
		return fmt.Sprintf("version changed (%s → %s)%s", d.Was, d.Now, doctorRunUp)
	default:
		return d.Reason + doctorRunUp
	}
}

// packageSkew checks a built package for staleness the presence of a build
// record alone cannot rule out: a record with no source hash (change detection
// inactive), a working-dir subtree that moved since the last build, and an
// installed binary whose build predates subtree changes. installedRef is the
// commit-ish the installed binary reports (see stalenessRef), empty when
// unknown. It reports skewed=false when everything checks out or no verdict is
// possible (empty or non-git working dir). Every skew is reconcilable by
// `ralph up`, so callers report it as a warning, not a failure.
func packageSkew(workDir, recordedHash, installedRef string) (msg string, skewed bool) {
	if workDir == "" {
		return "", false
	}
	currentHash := gitutil.GetTreeHash(workDir)
	if currentHash == "" {
		return "", false
	}
	if recordedHash == "" {
		return msgNoSourceHash, true
	}
	if currentHash != recordedHash {
		return msgSourceChanged, true
	}
	if installedRef != "" {
		// A ref git cannot resolve here — a release tag, a version string from a
		// tool that reports no commit — comes back ok=false, so an unresolvable
		// ref costs the check, never buys a wrong verdict.
		if changed, ok := gitutil.SubtreeChangedSince(workDir, installedRef); ok && changed {
			return fmt.Sprintf(
				"installed binary %s predates source changes — run 'ralph up'",
				shortSHA(installedRef),
			), true
		}
	}
	return "", false
}

// installedBuild probes an opted-in build's first install_paths binary for the
// build it reports. Probing is never implicit because arbitrary binaries may
// interpret `version -o json` as a mutating command.
func installedBuild(installPaths []string, versionCheck bool) (info buildinfo.Info, ok bool) {
	if !versionCheck || len(installPaths) == 0 {
		return buildinfo.Info{}, false
	}
	binPath, err := config.ExpandPath(installPaths[0])
	if err != nil {
		return buildinfo.Info{}, false
	}
	return binversion.Probe(binPath)
}

// stalenessRef picks the commit-ish to date an installed binary by. The probed
// commit is the answer whenever the binary reports one; binaries built before
// the four-field contract report only version, which for a fleet build is the
// short commit and works the same way. Anything else is not a commit-ish and
// yields no verdict downstream.
func stalenessRef(info buildinfo.Info) string {
	return packages.ReportedGitRevision(info)
}

// shortSHA truncates a commit sha to 8 characters for one-line messages.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// installedVersionNote renders probed build metadata as a suffix for a one-line
// report message: " (installed abc1234, tag keep/v0.4.0, built 2026-08-13T19:40Z)".
// Parts the binary did not report are dropped, and an unprobed binary yields "".
// The commit alone never flags staleness — a binary embeds the commit it was
// built from, which legitimately lags HEAD when the subtree is unchanged;
// packageSkew judges staleness with a subtree diff instead.
func installedVersionNote(info buildinfo.Info, probed bool) string {
	if !probed {
		return ""
	}
	parts := make([]string, 0, 3)
	if ref := stalenessRef(info); ref != "" {
		parts = append(parts, "installed "+shortSHA(ref))
	}
	if info.Tag != "" {
		parts = append(parts, "tag "+info.Tag)
	}
	if info.BuildTime != "" {
		parts = append(parts, "built "+shortBuildTime(info.BuildTime))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// shortBuildTime renders an RFC3339 build timestamp at minute precision in UTC.
// A value that does not parse is passed through as reported.
func shortBuildTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.UTC().Format("2006-01-02T15:04Z")
}

// buildMeta projects probed build metadata into the report's wire shape.
// An unprobed binary carries no metadata at all, which is how a JSON consumer
// tells "not probed" from "probed but unknown".
func buildMeta(info buildinfo.Info, probed bool) *report.BuildMeta {
	if !probed {
		return nil
	}
	return &report.BuildMeta{
		Version:   info.Version,
		Commit:    info.Commit,
		Tag:       info.Tag,
		BuildTime: info.BuildTime,
	}
}

func checkTools(rpt *report.Report, cfg *config.Config) {
	if len(cfg.Tools) == 0 {
		return
	}
	phase := rpt.AddPhase("Tools")
	currentHost := config.GetCurrentHost()

	for _, t := range cfg.Tools {
		recipe := t.OwnerRecipe
		reason := config.SkipReason(t.Enable, t.Hosts, t.Profiles, currentHost, cfg.Profiles)
		if reason != "" {
			phase.AddResult(t.Name, recipe, report.StatusSkip, doctorSkipText(reason), nil)
			continue
		}
		installed, checkErr := tool.CheckStatus(t.CheckCommand)
		if checkErr != nil {
			phase.AddResult(t.Name, recipe, report.StatusWarn, checkErr.Error(), nil)
		} else if installed {
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

// doctorSkipText words a config.SkipReason the way doctor reports it: an item
// filtered out by hosts or profiles belongs to another host or profile.
func doctorSkipText(reason string) string {
	switch reason {
	case config.SkipHost:
		return "other host"
	case config.SkipProfile:
		return "other profile"
	}
	return reason
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
		status, msg, err := rcBlockStatus(rcPath, content, cfg)
		phase.AddResult(shellName, "", status, msg, err)
	}
}

// rcBlockStatus checks the ralph managed block in content, the rc file at
// rcPath. It reads the block with the parser InjectSourceLines uses, so
// doctor fails exactly the files apply refuses to edit.
func rcBlockStatus(
	rcPath string,
	content []byte,
	cfg *config.Config,
) (report.Status, string, error) {
	sourcesExpected := len(cfg.Shell.Aliases) > 0 || len(cfg.Shell.Functions) > 0
	body, found, err := shell.ManagedBlockBody(content)
	switch {
	case err != nil:
		return report.StatusFail, fmt.Sprintf(
			"%s: %v; %s",
			rcPath,
			err,
			shell.MalformedBlockFix,
		), err
	case !found && sourcesExpected:
		return report.StatusWarn, "ralph block missing but aliases/functions configured (run apply to fix)", nil
	case !found:
		return report.StatusWarn, "ralph block not found", nil
	}

	sourced, missing := countSourcedFiles(body)
	switch {
	case missing > 0:
		return report.StatusFail, "sourced file(s) missing", nil
	case sourcesExpected && sourced == 0:
		return report.StatusWarn, "block found but no source commands detected", nil
	}
	return report.StatusOK, "", nil
}

// countSourcedFiles counts the `source <file>` and `. <file>` lines in a
// managed block body, and how many of those files do not exist.
func countSourcedFiles(body []string) (sourced, missing int) {
	for _, line := range body {
		trimmed := strings.TrimSpace(line)
		file, ok := strings.CutPrefix(trimmed, "source ")
		if !ok {
			file, ok = strings.CutPrefix(trimmed, ". ")
		}
		if !ok || file == "" {
			continue
		}
		sourced++
		expanded, err := config.ExpandPath(file)
		if err != nil {
			missing++
			continue
		}
		if _, err := os.Stat(expanded); err != nil {
			missing++
		}
	}
	return sourced, missing
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorAll, "all", false, "show all items, not just problems")
	rootCmd.AddCommand(doctorCmd)
	supportJSONOutput(doctorCmd)
}
