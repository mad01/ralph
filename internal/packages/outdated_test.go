package packages

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

func TestCompareVersions_Outdated(t *testing.T) {
	r := buildOutdatedResult("test_pkg", "go-install", "v1.0.0", "v1.1.0")
	if r.Status != "outdated" {
		t.Errorf("expected status=outdated, got %s", r.Status)
	}
	if r.Current != "v1.0.0" {
		t.Errorf("expected current=v1.0.0, got %s", r.Current)
	}
	if r.Latest != "v1.1.0" {
		t.Errorf("expected latest=v1.1.0, got %s", r.Latest)
	}
}

func TestCompareVersions_UpToDate(t *testing.T) {
	r := buildOutdatedResult("test_pkg", "go-install", "v1.0.0", "v1.0.0")
	if r.Status != "up to date" {
		t.Errorf("expected status='up to date', got %s", r.Status)
	}
}

func TestCompareVersions_Hashes(t *testing.T) {
	localHash := "abc1234def5678"
	remoteHash := "def5678abc1234"
	r := buildOutdatedResult("test_pkg", "make", localHash[:7], remoteHash[:7])
	if r.Status != "outdated" {
		t.Errorf("expected status=outdated for different hashes, got %s", r.Status)
	}
}

func TestCompareVersions_SameHash(t *testing.T) {
	hash := "abc1234"
	r := buildOutdatedResult("test_pkg", "make", hash, hash)
	if r.Status != "up to date" {
		t.Errorf("expected status='up to date' for same hash, got %s", r.Status)
	}
}

func TestCheckOutdated_LocalPackageSkipped(t *testing.T) {
	pkgs := map[string]config.Package{
		"local_pkg": {
			Source:     "local",
			WorkingDir: "/some/path",
		},
	}

	results := CheckOutdated(pkgs, "", "testhost")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Status != "skipped" {
		t.Errorf("expected status=skipped for local package, got %s", r.Status)
	}
	if r.Current != "-" {
		t.Errorf("expected current='-' for local package, got %s", r.Current)
	}
	if r.Latest != "-" {
		t.Errorf("expected latest='-' for local package, got %s", r.Latest)
	}
}

func TestCheckOutdated_DisabledPackageSkipped(t *testing.T) {
	disabled := false
	pkgs := map[string]config.Package{
		"disabled_pkg": {
			Source:  "go-install",
			Module:  "github.com/example/tool",
			Version: "v1.0.0",
			Enable:  &disabled,
		},
	}

	results := CheckOutdated(pkgs, "", "testhost")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Status != "skipped" {
		t.Errorf("expected status=skipped for disabled package, got %s", r.Status)
	}
}

func TestCheckOutdated_HostFilteredSkipped(t *testing.T) {
	pkgs := map[string]config.Package{
		"filtered_pkg": {
			Source:  "go-install",
			Module:  "github.com/example/tool",
			Version: "v1.0.0",
			Hosts:   []string{"otherhost"},
		},
	}

	results := CheckOutdated(pkgs, "", "myhost")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Status != "skipped" {
		t.Errorf("expected status=skipped for host-filtered package, got %s", r.Status)
	}
}

func TestCheckOutdated_EmptySourceTreatedAsLocal(t *testing.T) {
	pkgs := map[string]config.Package{
		"empty_source": {
			WorkingDir: "/some/path",
		},
	}

	results := CheckOutdated(pkgs, "", "testhost")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Status != "skipped" {
		t.Errorf("expected status=skipped for empty source (local), got %s", r.Status)
	}
	if r.Source != "local" {
		t.Errorf("expected source=local for empty source, got %s", r.Source)
	}
}

func TestCheckOutdated_SortedAlphabetically(t *testing.T) {
	pkgs := map[string]config.Package{
		"zebra": {Source: "local"},
		"alpha": {Source: "local"},
		"mango": {Source: "local"},
	}

	results := CheckOutdated(pkgs, "", "testhost")
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Name != "alpha" {
		t.Errorf("expected first result to be 'alpha', got '%s'", results[0].Name)
	}
	if results[1].Name != "mango" {
		t.Errorf("expected second result to be 'mango', got '%s'", results[1].Name)
	}
	if results[2].Name != "zebra" {
		t.Errorf("expected third result to be 'zebra', got '%s'", results[2].Name)
	}
}

func TestOutdatedResult_JSONSerialization(t *testing.T) {
	results := []OutdatedResult{
		{
			Name:    "test_pkg",
			Source:  "go-install",
			Current: "v1.0.0",
			Latest:  "v1.1.0",
			Status:  "outdated",
		},
		{
			Name:    "local_pkg",
			Source:  "local",
			Current: "-",
			Latest:  "-",
			Status:  "skipped",
		},
		{
			Name:    "error_pkg",
			Source:  "go-install",
			Current: "v1.0.0",
			Latest:  "",
			Status:  "error",
			Error:   "network timeout",
		},
	}

	data, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded []OutdatedResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(decoded) != 3 {
		t.Fatalf("expected 3 results, got %d", len(decoded))
	}

	// Verify first result
	if decoded[0].Name != "test_pkg" || decoded[0].Status != "outdated" {
		t.Errorf("unexpected first result: %+v", decoded[0])
	}

	// Verify error field omitted when empty
	jsonStr := string(data)
	// The local_pkg entry should not have an "error" key
	if strings.Contains(jsonStr, `"error":""`) {
		t.Error("expected error field to be omitted when empty (omitempty)")
	}

	// The error_pkg entry should have an "error" key
	if !strings.Contains(jsonStr, `"error":"network timeout"`) {
		t.Error("expected error field to be present for error_pkg")
	}
}

func TestFormatOutdatedTable(t *testing.T) {
	results := []OutdatedResult{
		{Name: "github_mcp_server", Source: "go-install", Current: "v1.0.5", Latest: "v1.1.0", Status: "outdated"},
		{Name: "vale", Source: "go-install", Current: "v3.14.2", Latest: "v3.14.2", Status: "up to date"},
		{Name: "kitty_session", Source: "make", Current: "abc1234", Latest: "abc1234", Status: "up to date"},
		{Name: "ralph", Source: "make", Current: "abc1234", Latest: "def5678", Status: "outdated"},
		{Name: "obsidian_search", Source: "local", Current: "-", Latest: "-", Status: "skipped"},
	}

	output := FormatOutdatedTable(results)

	// Check header is present
	if !strings.Contains(output, "Package") {
		t.Error("expected table to contain 'Package' header")
	}
	if !strings.Contains(output, "Source") {
		t.Error("expected table to contain 'Source' header")
	}
	if !strings.Contains(output, "Current") {
		t.Error("expected table to contain 'Current' header")
	}
	if !strings.Contains(output, "Latest") {
		t.Error("expected table to contain 'Latest' header")
	}
	if !strings.Contains(output, "Status") {
		t.Error("expected table to contain 'Status' header")
	}

	// Check all package names appear
	for _, r := range results {
		if !strings.Contains(output, r.Name) {
			t.Errorf("expected table to contain package name '%s'", r.Name)
		}
	}

	// Check all statuses appear
	if !strings.Contains(output, "outdated") {
		t.Error("expected table to contain 'outdated' status")
	}
	if !strings.Contains(output, "up to date") {
		t.Error("expected table to contain 'up to date' status")
	}
	if !strings.Contains(output, "skipped") {
		t.Error("expected table to contain 'skipped' status")
	}

	// Verify table structure: header, separator, data rows
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 7 { // header + separator + 5 data rows
		t.Fatalf("expected 7 lines, got %d", len(lines))
	}

	// Separator line should contain only dashes and spaces
	for _, c := range lines[1] {
		if c != '-' && c != ' ' {
			t.Errorf("separator line contains unexpected character: %c", c)
			break
		}
	}
}

func TestFormatOutdatedTable_Empty(t *testing.T) {
	output := FormatOutdatedTable(nil)
	if !strings.Contains(output, "No packages") {
		t.Errorf("expected empty results message, got: %s", output)
	}
}
