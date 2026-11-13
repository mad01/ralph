package packages

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/execx"
	"github.com/mad01/ralph/internal/gitutil"
)

// OutdatedStatus is how a package compares with its upstream.
type OutdatedStatus string

const (
	StatusUpToDate OutdatedStatus = "up to date"
	StatusOutdated OutdatedStatus = "outdated"
	StatusSkipped  OutdatedStatus = "skipped"
	StatusError    OutdatedStatus = "error"
)

// OutdatedResult holds the version comparison result for a single package.
type OutdatedResult struct {
	Name    string               `json:"name"`
	Source  config.PackageSource `json:"source"`
	Current string               `json:"current"`
	Latest  string               `json:"latest"`
	Status  OutdatedStatus       `json:"status"`
	Error   string               `json:"error,omitempty"`
}

// buildOutdatedResult constructs an OutdatedResult, comparing current and latest
// to determine whether the package is "outdated" or "up to date".
func buildOutdatedResult(
	name string,
	source config.PackageSource,
	current, latest string,
) OutdatedResult {
	status := StatusUpToDate
	if current != latest {
		status = StatusOutdated
	}
	return OutdatedResult{
		Name:    name,
		Source:  source,
		Current: current,
		Latest:  latest,
		Status:  status,
	}
}

// CheckOutdated checks all packages for newer upstream versions.
// It skips disabled, host-filtered, profile-filtered, and local packages.
func CheckOutdated(
	ctx context.Context,
	packages map[string]config.Package,
	packagesDir string,
	currentHost string,
	machineProfiles []string,
) []OutdatedResult {
	var results []OutdatedResult

	keys := sortedPackageKeys(packages)
	for _, name := range keys {
		pkg := packages[name]

		source := pkg.Source.OrLocal()

		// Skip disabled, host-filtered and profile-filtered packages
		if !config.Applies(pkg.Enable, pkg.Hosts, pkg.Profiles, currentHost, machineProfiles) {
			results = append(results, OutdatedResult{
				Name:    name,
				Source:  source,
				Current: "-",
				Latest:  "-",
				Status:  StatusSkipped,
			})
			continue
		}

		// Skip local packages
		if source == config.SourceLocal {
			results = append(results, OutdatedResult{
				Name:    name,
				Source:  source,
				Current: "-",
				Latest:  "-",
				Status:  StatusSkipped,
			})
			continue
		}

		switch source {
		case config.SourceGoInstall:
			results = append(results, checkGoInstallOutdated(ctx, name, pkg))
		case config.SourceRemote, config.SourceMake:
			resolved := ResolvePackagePaths(name, pkg, packagesDir)
			results = append(results, checkGitOutdated(ctx, name, resolved))
		default:
			results = append(results, OutdatedResult{
				Name:    name,
				Source:  source,
				Current: "-",
				Latest:  "-",
				Status:  StatusSkipped,
			})
		}
	}

	return results
}

// goListResult is the subset of `go list -m -json` output we need.
type goListResult struct {
	Version string `json:"Version"`
}

// checkGoInstallOutdated checks a go-install package against the latest module version.
func checkGoInstallOutdated(ctx context.Context, name string, pkg config.Package) OutdatedResult {
	current := pkg.Version
	if current == "" {
		return OutdatedResult{
			Name:    name,
			Source:  config.SourceGoInstall,
			Current: "-",
			Latest:  "-",
			Status:  StatusError,
			Error:   "no version configured",
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := execx.Command(ctx, "go", "list", "-m", "-json", pkg.Module+"@latest")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return OutdatedResult{
			Name:    name,
			Source:  config.SourceGoInstall,
			Current: current,
			Latest:  "",
			Status:  StatusError,
			Error:   errMsg,
		}
	}

	var result goListResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return OutdatedResult{
			Name:    name,
			Source:  config.SourceGoInstall,
			Current: current,
			Latest:  "",
			Status:  StatusError,
			Error:   fmt.Sprintf("failed to parse go list output: %v", err),
		}
	}

	return buildOutdatedResult(name, config.SourceGoInstall, current, result.Version)
}

// checkGitOutdated checks a remote/make package against the latest remote HEAD.
func checkGitOutdated(ctx context.Context, name string, pkg config.Package) OutdatedResult {
	source := pkg.Source
	if source == "" {
		source = config.SourceRemote
	}

	// Get local HEAD hash
	workDir := pkg.WorkingDir
	if workDir == "" {
		workDir = pkg.Target
	}

	localHash := gitutil.GetGitHash(workDir)
	if localHash == "" {
		return OutdatedResult{
			Name:    name,
			Source:  source,
			Current: "-",
			Latest:  "-",
			Status:  StatusError,
			Error:   notClonedMessage,
		}
	}

	// Get remote HEAD hash
	repo := pkg.Repo
	if repo == "" {
		return OutdatedResult{
			Name:    name,
			Source:  source,
			Current: short(localHash),
			Latest:  "-",
			Status:  StatusError,
			Error:   "no repo URL configured",
		}
	}
	if !gitutil.IsSafeRemoteURL(repo) {
		return OutdatedResult{
			Name:    name,
			Source:  source,
			Current: short(localHash),
			Latest:  "-",
			Status:  StatusError,
			Error:   "unsafe repo URL (leading '-' or ext::/fd:: transport)",
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := execx.TerminalCommand(ctx, "git", "ls-remote", "--", repo, "HEAD")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return OutdatedResult{
			Name:    name,
			Source:  source,
			Current: short(localHash),
			Latest:  "",
			Status:  StatusError,
			Error:   errMsg,
		}
	}

	// Parse the first field (hash) from the output
	output := strings.TrimSpace(stdout.String())
	fields := strings.Fields(output)
	if len(fields) == 0 {
		return OutdatedResult{
			Name:    name,
			Source:  source,
			Current: short(localHash),
			Latest:  "",
			Status:  StatusError,
			Error:   "empty response from git ls-remote",
		}
	}
	remoteHash := fields[0]

	localShort := short(localHash)
	remoteShort := short(remoteHash)

	return buildOutdatedResult(name, source, localShort, remoteShort)
}

// FormatOutdatedTable formats a slice of OutdatedResult as a column-aligned table.
func FormatOutdatedTable(results []OutdatedResult) string {
	if len(results) == 0 {
		return "No packages configured."
	}

	// Column headers
	headers := []string{"Package", "Source", "Current", "Latest", "Status"}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}

	rows := make([][]string, len(results))
	for i, r := range results {
		rows[i] = []string{r.Name, string(r.Source), r.Current, r.Latest, string(r.Status)}
		for j, cell := range rows[i] {
			if len(cell) > widths[j] {
				widths[j] = len(cell)
			}
		}
	}

	// Build format string
	fmtParts := make([]string, len(widths))
	for i, w := range widths {
		fmtParts[i] = fmt.Sprintf("%%-%ds", w)
	}
	rowFmt := strings.Join(fmtParts, "  ")

	var sb strings.Builder

	// Header
	headerArgs := make([]any, len(headers))
	for i, h := range headers {
		headerArgs[i] = h
	}
	fmt.Fprintf(&sb, rowFmt, headerArgs...)
	sb.WriteString("\n")

	// Separator
	sepParts := make([]any, len(widths))
	for i, w := range widths {
		sepParts[i] = strings.Repeat("-", w)
	}
	fmt.Fprintf(&sb, rowFmt, sepParts...)
	sb.WriteString("\n")

	// Data rows
	for _, row := range rows {
		rowArgs := make([]any, len(row))
		for i, cell := range row {
			rowArgs[i] = cell
		}
		fmt.Fprintf(&sb, rowFmt, rowArgs...)
		sb.WriteString("\n")
	}

	return sb.String()
}

// HasOutdated returns true if any result has status "outdated".
func HasOutdated(results []OutdatedResult) bool {
	for _, r := range results {
		if r.Status == StatusOutdated {
			return true
		}
	}
	return false
}

// HasErrors returns true if any result has status "error".
func HasErrors(results []OutdatedResult) bool {
	for _, r := range results {
		if r.Status == StatusError {
			return true
		}
	}
	return false
}

// SortResults sorts results: outdated first, then errors, then up to date, then skipped.
func SortResults(results []OutdatedResult) {
	order := map[OutdatedStatus]int{
		StatusOutdated: 0,
		StatusError:    1,
		StatusUpToDate: 2,
		StatusSkipped:  3,
	}
	sort.SliceStable(results, func(i, j int) bool {
		oi, oj := order[results[i].Status], order[results[j].Status]
		if oi != oj {
			return oi < oj
		}
		return results[i].Name < results[j].Name
	})
}
