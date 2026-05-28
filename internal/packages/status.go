package packages

import (
	"os"
	"sort"
	"time"

	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/gitutil"
)

// PackageStatus holds the read-only status of a single package.
type PackageStatus struct {
	Name         string
	Source       string // "local" or "remote"
	WorkingDir   string
	Repo         string
	Enabled      bool
	HostMatch    bool
	Cloned       bool // target dir exists (remote) or working_dir exists (local)
	NeedsBuild   bool
	NeedReason   string // "never built", "git hash changed", "uncommitted changes", "not cloned", "working_dir missing"
	LastBuiltAt  *time.Time
	CurrentHash  string
	RecordedHash string
}

// CheckPackageStatuses checks the status of all packages without modifying anything.
func CheckPackageStatuses(packages map[string]config.Package, packagesDir, currentHost string) []PackageStatus {
	if len(packages) == 0 {
		return nil
	}

	state, stateErr := buildstate.LoadBuildState()

	var statuses []PackageStatus
	for name, pkg := range packages {
		s := checkSinglePackageStatus(name, pkg, packagesDir, currentHost, state, stateErr)
		statuses = append(statuses, s)
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})

	return statuses
}

func checkSinglePackageStatus(name string, pkg config.Package, packagesDir, currentHost string, state *buildstate.BuildState, stateErr error) PackageStatus {
	resolved := ResolvePackagePaths(name, pkg, packagesDir)

	s := PackageStatus{
		Name:       name,
		Source:     pkg.Source,
		WorkingDir: resolved.WorkingDir,
		Repo:       pkg.Repo,
		Enabled:    config.IsEnabled(pkg.Enable),
		HostMatch:  config.ShouldApplyForHost(pkg.Hosts, currentHost),
	}

	if !s.Enabled || !s.HostMatch {
		return s
	}

	stateKey := "pkg:" + name

	switch pkg.Source {
	case "go-install":
		s = checkGoInstallStatus(s, pkg, stateKey, state, stateErr)
	case "remote", "make":
		s = checkRemoteStatus(s, resolved, stateKey, state, stateErr)
	case "local":
		s = checkLocalStatus(s, resolved, stateKey, state, stateErr)
	}

	return s
}

func checkRemoteStatus(s PackageStatus, pkg config.Package, stateKey string, state *buildstate.BuildState, stateErr error) PackageStatus {
	target := pkg.Target

	// Check if cloned
	if _, err := os.Stat(target); os.IsNotExist(err) {
		s.Cloned = false
		s.NeedsBuild = true
		s.NeedReason = "not cloned"
		return s
	}
	s.Cloned = true

	// Get current git hash
	s.CurrentHash = gitutil.GetGitHash(pkg.WorkingDir)

	// Check build state
	if stateErr != nil {
		s.NeedsBuild = true
		s.NeedReason = "never built"
		return s
	}

	record, exists := state.Builds[stateKey]
	if !exists {
		s.NeedsBuild = true
		s.NeedReason = "never built"
		return s
	}

	t := record.CompletedAt
	s.LastBuiltAt = &t
	s.RecordedHash = record.GitHash

	// Compare hashes
	if s.CurrentHash != "" && s.RecordedHash != "" && s.CurrentHash != s.RecordedHash {
		s.NeedsBuild = true
		s.NeedReason = "git hash changed"
		return s
	}

	return s
}

func checkGoInstallStatus(s PackageStatus, pkg config.Package, stateKey string, state *buildstate.BuildState, stateErr error) PackageStatus {
	s.Cloned = true // go-install doesn't clone; treat as always "available"

	if stateErr != nil {
		s.NeedsBuild = true
		s.NeedReason = "never installed"
		return s
	}

	record, exists := state.Builds[stateKey]
	if !exists {
		s.NeedsBuild = true
		s.NeedReason = "never installed"
		return s
	}

	t := record.CompletedAt
	s.LastBuiltAt = &t

	if record.Version != pkg.Version {
		s.NeedsBuild = true
		s.NeedReason = "version changed"
		return s
	}

	return s
}

func checkLocalStatus(s PackageStatus, pkg config.Package, stateKey string, state *buildstate.BuildState, stateErr error) PackageStatus {
	workDir := pkg.WorkingDir

	// Check if working dir exists
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		s.Cloned = false
		s.NeedsBuild = false
		s.NeedReason = "working_dir missing"
		return s
	}
	s.Cloned = true

	// Get current git hash
	s.CurrentHash = gitutil.GetGitHash(workDir)

	// Check build state
	if stateErr != nil {
		s.NeedsBuild = true
		s.NeedReason = "never built"
		return s
	}

	record, exists := state.Builds[stateKey]
	if !exists {
		s.NeedsBuild = true
		s.NeedReason = "never built"
		return s
	}

	t := record.CompletedAt
	s.LastBuiltAt = &t
	s.RecordedHash = record.GitHash

	// Compare hashes
	if s.CurrentHash != "" && s.RecordedHash != "" && s.CurrentHash != s.RecordedHash {
		s.NeedsBuild = true
		s.NeedReason = "git hash changed"
		return s
	}

	// Check uncommitted changes
	if gitutil.HasGitChanges(workDir) {
		s.NeedsBuild = true
		s.NeedReason = "uncommitted changes"
		return s
	}

	return s
}
