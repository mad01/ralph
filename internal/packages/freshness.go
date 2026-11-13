package packages

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mad01/ralph/internal/binversion"
	"github.com/mad01/ralph/internal/buildinfo"
	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/gitutil"
)

// FreshnessCause names the rule that decided whether a package builds.
type FreshnessCause string

const (
	// CauseUpToDate: nothing changed; the package does not build.
	CauseUpToDate FreshnessCause = "up to date"
	// CauseForced: --force builds every package.
	CauseForced FreshnessCause = "forced"
	// CauseNotCloned: a remote or make package's working_dir is missing.
	// ralph up clones it in its sync step and then builds it; a run without
	// sync skips it.
	CauseNotCloned FreshnessCause = "not cloned"
	// CauseWorkDirMissing: a local package's working_dir is missing. No build
	// can fix that, so the package does not count as needing one; ralph up
	// reports it as an error.
	CauseWorkDirMissing FreshnessCause = "working_dir missing"
	// CauseNeverBuilt: no build is recorded, or the build state is
	// unreadable.
	CauseNeverBuilt FreshnessCause = "never built"
	// CauseNoSourceHash: the record carries no source hash (saved while git
	// was failing), so change detection is off until a build records one.
	CauseNoSourceHash FreshnessCause = "no recorded source hash"
	// CauseSourceChanged: the working_dir subtree's tree hash moved.
	CauseSourceChanged FreshnessCause = "git hash changed"
	// CauseUncommitted: the working_dir subtree has uncommitted changes.
	CauseUncommitted FreshnessCause = "uncommitted changes"
	// CauseVersionChanged: a go-install package's declared version differs
	// from the installed one.
	CauseVersionChanged FreshnessCause = "version changed"
	// CauseInstalled: the installed artifacts fail CheckInstalledPackage.
	CauseInstalled FreshnessCause = "installed artifact changed"
)

// Freshness is whether ralph up builds a package, and why.
type Freshness struct {
	NeedsBuild bool
	Cause      FreshnessCause
	// Reason is the cause as one short phrase for ralph list: the install
	// check's own reason for CauseInstalled, "never installed" for a
	// go-install package without a record, "" when up to date.
	Reason string
	// Was and Now are the recorded and current tree hashes for
	// CauseSourceChanged, and the installed and declared versions for
	// CauseVersionChanged.
	Was, Now string
}

// FreshnessInput is what DecidePackage reads about one package.
type FreshnessInput struct {
	Package config.Package // paths resolved with ResolvePackagePaths
	// Record is the package's last successful build; nil when there is none
	// or the build state is unreadable.
	Record *buildstate.BuildRecord
	// WorkDirExists reports that Package.WorkingDir exists. Unused for
	// go-install packages, which have no working dir.
	WorkDirExists bool
	TreeHash      string // tree hash of the working_dir subtree; "" when git cannot tell
	Dirty         bool   // the working_dir subtree has uncommitted, non-ignored changes
	// Installed checks the installed artifacts against Record. DecidePackage
	// calls it only when no other rule decides, since it may run the binary.
	// Nil skips the check.
	Installed func() InstallationCheck
	Force     bool
}

// DecidePackage decides whether ralph up builds a package that passed its
// enable, host and profile filters. ralph up, ralph list and ralph doctor
// all ask it, so they report the same verdict.
//
// Freshness is keyed on the tree hash of the working_dir subtree, not the
// repo-wide commit, so commits elsewhere in the repo don't force a rebuild.
func DecidePackage(in FreshnessInput) Freshness {
	if in.Package.Source == config.SourceGoInstall {
		return decideGoInstall(in)
	}
	if !in.WorkDirExists {
		if in.Package.Source.Clones() {
			return needs(CauseNotCloned)
		}
		return Freshness{Cause: CauseWorkDirMissing, Reason: string(CauseWorkDirMissing)}
	}
	if in.Force {
		return needs(CauseForced)
	}
	if in.Record == nil {
		return needs(CauseNeverBuilt)
	}
	switch {
	case in.TreeHash != "" && in.Record.GitHash == "":
		// A record without a hash could never match, so without this rebuild
		// the package would stay on its stale binary through every source
		// change. The rebuild records a real hash.
		return needs(CauseNoSourceHash)
	case in.TreeHash != "" && in.TreeHash != in.Record.GitHash:
		f := needs(CauseSourceChanged)
		f.Was, f.Now = in.Record.GitHash, in.TreeHash
		return f
	case in.Dirty:
		return needs(CauseUncommitted)
	}
	return checkInstalled(in)
}

// decideGoInstall decides for a go-install package, which has no source
// tree: the declared version and the installed artifacts decide.
func decideGoInstall(in FreshnessInput) Freshness {
	switch {
	case in.Force:
		return needs(CauseForced)
	case in.Record == nil:
		f := needs(CauseNeverBuilt)
		f.Reason = "never installed"
		return f
	case in.Record.Version != in.Package.Version:
		f := needs(CauseVersionChanged)
		f.Was, f.Now = in.Record.Version, in.Package.Version
		return f
	}
	return checkInstalled(in)
}

func checkInstalled(in FreshnessInput) Freshness {
	if in.Installed != nil {
		if check := in.Installed(); check.Reason != "" {
			return Freshness{NeedsBuild: true, Cause: CauseInstalled, Reason: check.Reason}
		}
	}
	return Freshness{Cause: CauseUpToDate}
}

func needs(cause FreshnessCause) Freshness {
	return Freshness{NeedsBuild: true, Cause: cause, Reason: string(cause)}
}

// ObservePackage gathers DecidePackage's input for a resolved package from
// disk and git. record is its last successful build, nil when none. The
// install check is left lazy; Force is left false.
func ObservePackage(pkg config.Package, record *buildstate.BuildRecord) FreshnessInput {
	in := FreshnessInput{Package: pkg, Record: record, Installed: installedCheck(pkg, record)}
	if pkg.Source != config.SourceGoInstall {
		in.WorkDirExists = statWorkDir(pkg.WorkingDir) == nil
		if in.WorkDirExists {
			in.TreeHash, in.Dirty = sourceState(pkg.WorkingDir)
		}
	}
	return in
}

// statWorkDir returns the error when dir does not exist, else nil. Any
// other stat failure counts as present, so the build reports it.
func statWorkDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return err
	}
	return nil
}

// sourceState returns the tree hash of dir's subtree and whether the subtree
// has uncommitted changes. Ignored build output does not count as a change.
func sourceState(dir string) (treeHash string, dirty bool) {
	return gitutil.GetTreeHash(dir), gitutil.HasGitChangesInPath(dir)
}

// installedCheck returns the lazy install check for a package and its
// record. A go-install package has no source repo to compare against.
func installedCheck(pkg config.Package, record *buildstate.BuildRecord) func() InstallationCheck {
	return func() InstallationCheck {
		var rec buildstate.BuildRecord
		if record != nil {
			rec = *record
		}
		workDir := pkg.WorkingDir
		if pkg.Source == config.SourceGoInstall {
			workDir = ""
		}
		return CheckInstalledPackage(pkg, workDir, rec)
	}
}

// InstallationCheck describes whether an installed package still agrees with
// the source and the state recorded after the last successful install.
type InstallationCheck struct {
	BuildInfo     buildinfo.Info
	VersionProbed bool
	Reason        string
}

// ReportedGitRevision returns the SHA-shaped revision reported by a binary.
// The explicit commit wins; legacy binaries may report a 7-40 character short
// SHA in version. Full SHA-256 object IDs are also accepted. Release versions
// such as v1.2.3 do not produce a verdict.
func ReportedGitRevision(info buildinfo.Info) string {
	if isGitRevision(info.Commit) {
		return strings.ToLower(info.Commit)
	}
	if isGitRevision(info.Version) {
		return strings.ToLower(info.Version)
	}
	return ""
}

func isGitRevision(value string) bool {
	if len(value) < 7 || (len(value) > 40 && len(value) != 64) {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// GitRevisionMatchesHead compares a full or abbreviated SHA with a full HEAD
// SHA. Both inputs must be SHA-shaped; a short revision matches a HEAD prefix.
func GitRevisionMatchesHead(revision, head string) bool {
	if !isGitRevision(revision) || !isGitRevision(head) || len(revision) > len(head) {
		return false
	}
	return strings.HasPrefix(strings.ToLower(head), strings.ToLower(revision))
}

// CheckInstalledPackage applies the installed-artifact part of the freshness
// policy (see DecidePackage). Version probing is opt-in because some
// binaries interpret `version -o json` as a mutating command. When enabled, a
// usable reported revision is compared with the repository-wide HEAD. The
// content hash remains an integrity guard, including when versions match.
func CheckInstalledPackage(
	pkg config.Package,
	workDir string,
	record buildstate.BuildRecord,
) InstallationCheck {
	var check InstallationCheck
	if len(pkg.InstallPaths) == 0 {
		return check
	}
	expanded, reason := statInstallPaths(pkg.InstallPaths)
	if reason != "" {
		check.Reason = reason
		return check
	}
	if pkg.VersionCheck {
		check.BuildInfo, check.VersionProbed = binversion.Probe(expanded[0])
		check.Reason = revisionMismatch(check.BuildInfo, check.VersionProbed, workDir)
		if check.Reason != "" {
			return check
		}
	}
	check.Reason = installHashMismatch(expanded, record.InstallHash)
	return check
}

// statInstallPaths expands install paths and checks each exists. It returns
// the expanded paths, or why one is unusable.
func statInstallPaths(paths []string) (expanded []string, reason string) {
	expanded = make([]string, 0, len(paths))
	for _, path := range paths {
		resolved, err := config.ExpandPath(path)
		if err != nil {
			return nil, fmt.Sprintf("cannot expand install_path %q", path)
		}
		if _, err := os.Stat(resolved); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Sprintf("install_path missing (%s)", resolved)
			}
			return nil, fmt.Sprintf("cannot inspect install_path %s", resolved)
		}
		expanded = append(expanded, resolved)
	}
	return expanded, ""
}

// revisionMismatch compares the revision a probed binary reports with the
// source repository's HEAD. It returns "" when they match, else why not.
func revisionMismatch(info buildinfo.Info, probed bool, workDir string) string {
	if !probed {
		return "installed binary does not support version -o json"
	}
	revision := ReportedGitRevision(info)
	if revision == "" {
		return "installed binary did not report a SHA-shaped commit or version"
	}
	head := gitutil.GetGitHash(workDir)
	if head == "" {
		return "cannot determine source repository HEAD"
	}
	if !GitRevisionMatchesHead(revision, head) {
		return fmt.Sprintf(
			"installed revision %s does not match source HEAD %s",
			short(revision),
			short(head),
		)
	}
	return ""
}

// installHashMismatch compares the installed content with the hash recorded
// after the last successful install. It returns "" when they match.
func installHashMismatch(expanded []string, recorded string) string {
	if recorded == "" {
		return "no recorded install hash"
	}
	current, err := buildstate.ComputeInstallHash(expanded)
	if err != nil {
		return "cannot verify installed content"
	}
	if current != recorded {
		return "installed content differs from the last successful install"
	}
	return ""
}

// validateInstalledVersion checks, after an install, that an opted-in
// binary reports the source HEAD it was just built from.
func validateInstalledVersion(pkg config.Package, workDir string) error {
	if !pkg.VersionCheck {
		return nil
	}
	if len(pkg.InstallPaths) == 0 {
		return errors.New("version_check requires install_paths")
	}
	path, err := config.ExpandPath(pkg.InstallPaths[0])
	if err != nil {
		return fmt.Errorf("expand version_check install_path: %w", err)
	}
	info, ok := binversion.Probe(path)
	if reason := revisionMismatch(info, ok, workDir); reason != "" {
		return errors.New(reason)
	}
	return nil
}
