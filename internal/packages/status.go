package packages

import (
	"sort"
	"time"

	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/config"
)

// PackageStatus holds the read-only status of a single package.
type PackageStatus struct {
	Name         string
	Source       string // as configured: "local", "remote", "make" or "go-install"
	WorkingDir   string
	Repo         string
	Enabled      bool
	HostMatch    bool
	ProfileMatch bool
	Cloned       bool // working_dir exists; always true for go-install
	NeedsBuild   bool // ralph up would build it (DecidePackage)
	NeedReason   string
	LastBuiltAt  *time.Time
	CurrentHash  string
	RecordedHash string
}

// CheckPackageStatuses checks the status of all packages without modifying
// anything. NeedsBuild and NeedReason come from DecidePackage, the rule
// ralph up builds by.
func CheckPackageStatuses(
	packages map[string]config.Package,
	packagesDir, currentHost string,
	machineProfiles []string,
) []PackageStatus {
	if len(packages) == 0 {
		return nil
	}

	state, stateErr := buildstate.LoadBuildState()

	var statuses []PackageStatus
	for name, pkg := range packages {
		var record *buildstate.BuildRecord
		if stateErr == nil {
			if rec, ok := state.Builds[stateKey(name)]; ok {
				record = &rec
			}
		}
		s := checkSinglePackageStatus(
			name,
			pkg,
			packagesDir,
			currentHost,
			machineProfiles,
			record,
		)
		statuses = append(statuses, s)
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})

	return statuses
}

func checkSinglePackageStatus(
	name string,
	pkg config.Package,
	packagesDir, currentHost string,
	machineProfiles []string,
	record *buildstate.BuildRecord,
) PackageStatus {
	resolved := ResolvePackagePaths(name, pkg, packagesDir)

	s := PackageStatus{
		Name:         name,
		Source:       string(pkg.Source),
		WorkingDir:   resolved.WorkingDir,
		Repo:         pkg.Repo,
		Enabled:      config.IsEnabled(pkg.Enable),
		HostMatch:    config.ShouldApplyForHost(pkg.Hosts, currentHost),
		ProfileMatch: config.ShouldApplyForProfiles(pkg.Profiles, machineProfiles),
	}
	if !s.Enabled || !s.HostMatch || !s.ProfileMatch {
		return s
	}

	in := ObservePackage(resolved, record)
	d := DecidePackage(in)
	s.Cloned = pkg.Source == config.SourceGoInstall || in.WorkDirExists
	s.CurrentHash = in.TreeHash
	if record != nil {
		t := record.CompletedAt
		s.LastBuiltAt = &t
		s.RecordedHash = record.GitHash
	}
	s.NeedsBuild = d.NeedsBuild
	s.NeedReason = d.Reason
	return s
}
