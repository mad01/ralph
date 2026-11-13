package apply

import (
	"context"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/hooks"
	"github.com/mad01/ralph/internal/packages"
	"github.com/mad01/ralph/internal/progress"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

// applyBuildsAndPackages executes builds and packages in topologically sorted
// order so that items can depend on each other via depends_on. When a specific
// build is requested via --build, only that build runs (skipping topological sort).
func applyBuildsAndPackages(s *sequence) {
	buildOpts := s.buildOptions()
	if len(s.cfg.Hooks.Builds) == 0 && len(s.cfg.Packages) == 0 &&
		buildOpts.SpecificBuild == "" {
		return
	}

	// If a specific build is requested, just run that one (no topological sort).
	if buildOpts.SpecificBuild != "" {
		applyBuilds(s, buildOpts)
		return
	}

	r := &nodeRunner{
		s:          s,
		buildOpts:  buildOpts,
		force:      s.opts.Force,
		buildPhase: s.rpt.AddPhase("Builds"),
		pkgPhase:   s.rpt.AddPhase("Packages"),
		failed:     map[string]bool{},
	}

	// Group by wave and execute each wave fully before the next.
	groups := config.GroupByWave(s.cfg.Hooks.Builds, s.cfg.Packages)
	waveNums := config.SortedWaveNumbers(groups)

	totalItems := len(s.cfg.Hooks.Builds) + len(s.cfg.Packages)
	prog := progress.NewOrQuiet("Builds & Packages", totalItems, s.opts.Verbose || s.opts.DryRun)

	// Cross-wave depends_on edges (a dependency in a LATER wave) are not
	// enforced by the per-wave topological sort: warn so a silently-unordered
	// dependency is visible.
	for _, warning := range config.CrossWaveDependencyWarnings(s.cfg.Hooks.Builds, s.cfg.Packages) {
		fmt.Fprintln(s.errOut, color.YellowString("Warning: %s", warning))
	}

	for _, waveNum := range waveNums {
		r.runWave(waveNum, groups[waveNum], len(waveNums) > 1, prog)
	}
	prog.Done()

	for _, f := range r.buildFailures {
		fmt.Fprintf(s.errOut, "  build %s: %v\n", f.Name, f.Err)
	}
}

// buildOptions are the options every build of this run gets.
func (s *sequence) buildOptions() hooks.BuildOptions {
	return hooks.BuildOptions{
		DryRun:        s.opts.DryRun,
		Force:         s.opts.Force,
		SpecificBuild: s.opts.Build,
		Verbose:       s.opts.Verbose,
	}
}

// nodeRunner carries the state one builds+packages phase shares across waves.
type nodeRunner struct {
	s          *sequence
	buildOpts  hooks.BuildOptions
	force      bool
	buildPhase *report.Phase
	pkgPhase   *report.Phase
	// failed tracks node keys ("builds.x"/"packages.x") that failed or were
	// skipped because a dependency failed, so dependents in this and later
	// waves skip instead of building against a missing artifact.
	failed        map[string]bool
	buildFailures []hooks.BuildResult
}

// runWave runs one wave's builds and packages in dependency order.
func (r *nodeRunner) runWave(
	waveNum int,
	group *config.WaveGroup,
	multiWave bool,
	prog *progress.Counter,
) {
	order, err := config.TopologicalSort(group.Builds, group.Packages)
	if err != nil {
		phase := r.s.rpt.AddPhase(fmt.Sprintf("Wave %d", waveNum))
		phase.AddFail("dependency-sort", err.Error(), err)
		fmt.Fprintln(
			r.s.errOut,
			color.RedString("Error sorting wave %d dependencies: %v", waveNum, err),
		)
		return
	}
	if len(order) == 0 {
		return
	}
	if r.s.opts.Verbose && multiWave {
		fmt.Fprintf(r.s.out, "  Wave %d (%d items)\n", waveNum, len(order))
	}
	for _, key := range order {
		kind, name, _ := strings.Cut(key, ".")
		prog.TickWith(name)
		switch kind {
		case "builds":
			r.runBuild(key, name, group.Builds[name])
		case "packages":
			r.runPackage(key, name, group.Packages[name])
		}
	}
}

// runBuild runs one build unless a filter or a failed dependency skips it.
// Disabled and filtered builds are reported as skips, not "completed".
func (r *nodeRunner) runBuild(key, name string, build config.Build) {
	if reason := r.s.skipReason(build.Enable, build.Hosts, build.Profiles); reason != "" {
		fmt.Fprintf(r.s.out, "  Skipping build: %s (%s)\n", name, reason)
		r.buildPhase.AddSkip(name, reason)
		return
	}
	if dep := firstFailedDependency(build.DependsOn, r.failed); dep != "" {
		fmt.Fprintf(r.s.out, "  Skipping build: %s (dependency %s failed)\n", name, dep)
		r.buildPhase.AddSkip(name, fmt.Sprintf("dependency %s failed", dep))
		r.failed[key] = true // cascade to this build's own dependents
		return
	}
	s := r.s
	err := hooks.RunBuild(
		context.Background(), s.out, name, build, s.opts.Host, s.cfg.Profiles, r.buildOpts,
	)
	if err != nil {
		r.buildFailures = append(r.buildFailures, hooks.BuildResult{Name: name, Err: err})
		r.buildPhase.AddFail(name, err.Error(), err)
		r.failed[key] = true
		return
	}
	r.buildPhase.AddOK(name, "completed")
}

// runPackage builds one package unless a filter or a failed dependency skips it.
func (r *nodeRunner) runPackage(key, name string, pkg config.Package) {
	source := pkg.Source
	if source == "" {
		source = "local"
	}
	if reason := r.s.skipReason(pkg.Enable, pkg.Hosts, pkg.Profiles); reason != "" {
		fmt.Fprintf(r.s.out, "  Skipping package: %s [%s] (%s)\n", name, source, reason)
		r.pkgPhase.AddSkip(name, fmt.Sprintf("%s [%s]", reason, source))
		return
	}
	if dep := firstFailedDependency(pkg.DependsOn, r.failed); dep != "" {
		fmt.Fprintf(
			r.s.out,
			"  Skipping package: %s [%s] (dependency %s failed)\n",
			name,
			source,
			dep,
		)
		r.pkgPhase.AddSkip(name, fmt.Sprintf("dependency %s failed", dep))
		r.failed[key] = true // cascade to this package's own dependents
		return
	}
	resolved := packages.ResolvePackagePaths(name, pkg, r.s.cfg.PackagesDir)
	opts := packages.BuildOptions{
		DryRun:  r.s.opts.DryRun,
		Force:   r.force,
		Verbose: r.s.opts.Verbose,
	}
	r.recordPackage(
		key,
		pkg,
		packages.BuildPackage(context.Background(), r.s.out, name, resolved, opts),
	)
}

// recordPackage reports one package build result and marks a failure so the
// package's dependents skip.
func (r *nodeRunner) recordPackage(key string, pkg config.Package, res packages.BuildResult) {
	switch res.Action {
	case packages.ActionError:
		fmt.Fprintln(r.s.errOut, color.RedString("  %s: %s: %v", res.Name, res.Message, res.Err))
		r.pkgPhase.AddFail(res.Name, res.Message, res.Err)
		r.failed[key] = true
	case packages.ActionSkipped:
		r.pkgPhase.AddSkip(res.Name, res.Message)
	case packages.ActionUpToDate:
		r.pkgPhase.AddOK(res.Name, res.Message)
	default:
		fmt.Fprintf(r.s.out, "  %s: %s\n", res.Name, res.Message)
		r.pkgPhase.AddOK(res.Name, res.Message)
		r.s.collectCaveat(pkg.OwnerRecipe)
	}
}

// firstFailedDependency returns the first depends_on entry that is in the failed
// set (a build/package that errored or was itself skipped for a failed dep), or
// "" if none. Used to skip a node whose dependency didn't succeed rather than
// build it against a missing or stale artifact.
func firstFailedDependency(deps []string, failed map[string]bool) string {
	for _, dep := range deps {
		if failed[dep] {
			return dep
		}
	}
	return ""
}

// applyBuilds executes build hooks with the given options.
func applyBuilds(s *sequence, buildOpts hooks.BuildOptions) {
	if len(s.cfg.Hooks.Builds) == 0 && buildOpts.SpecificBuild == "" {
		return
	}
	buildPhase := s.rpt.AddPhase("Builds")
	if err := hooks.RunBuilds(context.Background(), s.out, s.cfg.Hooks.Builds, s.opts.Host, s.cfg.Profiles, buildOpts); err != nil {
		fmt.Fprintln(s.errOut, color.RedString("Error executing builds: %v", err))
		buildPhase.AddFail("builds", err.Error(), err)
	} else {
		buildPhase.AddOK("builds", "completed")
	}
}

// declareBuildsAndPackages records what every applying build and package
// declares, whether or not a dependency of it fails this run: the manifest is
// the config's intent, not the run's outcome. Install paths are tracked for
// owned and ownerless items alike; names and package clones only for owned
// ones.
func declareBuildsAndPackages(m *manifest) {
	for name, pkg := range m.cfg.Packages {
		if !m.applies(pkg.Enable, pkg.Hosts, pkg.Profiles) {
			continue
		}
		m.trackInstallPaths(pkg.OwnerRecipe, pkg.InstallPaths)
		if pkg.OwnerRecipe == "" {
			continue
		}
		m.own(pkg.OwnerRecipe, state.KindPackage, name)
		// Remote/make packages clone into packages_dir. Track that clone dir so
		// removing the package surfaces it as an orphan (abandon-with-log): it
		// is otherwise invisible and leaks a full checkout. ResolvePackagePaths
		// is the single source of truth for where the clone lands, here and in
		// runPackage.
		if pkg.Source.Clones() {
			resolved := packages.ResolvePackagePaths(name, pkg, m.cfg.PackagesDir)
			if resolved.Target != "" {
				m.own(pkg.OwnerRecipe, state.KindPackageClone, resolved.Target)
			}
		}
	}

	for name, b := range m.cfg.Hooks.Builds {
		if !m.applies(b.Enable, b.Hosts, b.Profiles) {
			continue
		}
		m.trackInstallPaths(b.OwnerRecipe, b.InstallPaths)
		m.own(b.OwnerRecipe, state.KindBuild, name)
	}
}
