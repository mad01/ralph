package config

import (
	"strings"
	"testing"
)

func TestTopologicalSort_NoDependencies_Alphabetical(t *testing.T) {
	builds := map[string]Build{
		"charlie": {Commands: []string{"echo c"}, Run: "always"},
		"alpha":   {Commands: []string{"echo a"}, Run: "always"},
		"bravo":   {Commands: []string{"echo b"}, Run: "always"},
	}
	packages := map[string]Package{}

	order, err := TopologicalSort(builds, packages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"builds.alpha", "builds.bravo", "builds.charlie"}
	if len(order) != len(want) {
		t.Fatalf("got %d items, want %d: %v", len(order), len(want), order)
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("order[%d] = %q, want %q", i, order[i], w)
		}
	}
}

func TestTopologicalSort_LinearChain(t *testing.T) {
	// A depends on B, B depends on C → [C, B, A]
	builds := map[string]Build{
		"a": {Commands: []string{"echo a"}, Run: "always", DependsOn: []string{"builds.b"}},
		"b": {Commands: []string{"echo b"}, Run: "always", DependsOn: []string{"builds.c"}},
		"c": {Commands: []string{"echo c"}, Run: "always"},
	}
	packages := map[string]Package{}

	order, err := TopologicalSort(builds, packages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"builds.c", "builds.b", "builds.a"}
	if len(order) != len(want) {
		t.Fatalf("got %d items, want %d: %v", len(order), len(want), order)
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("order[%d] = %q, want %q", i, order[i], w)
		}
	}
}

func TestTopologicalSort_Diamond(t *testing.T) {
	// A depends on B and C, B depends on D, C depends on D
	// Expected: D first, then B and C (alphabetical), then A
	builds := map[string]Build{
		"a": {Commands: []string{"echo a"}, Run: "always", DependsOn: []string{"builds.b", "builds.c"}},
		"b": {Commands: []string{"echo b"}, Run: "always", DependsOn: []string{"builds.d"}},
		"c": {Commands: []string{"echo c"}, Run: "always", DependsOn: []string{"builds.d"}},
		"d": {Commands: []string{"echo d"}, Run: "always"},
	}
	packages := map[string]Package{}

	order, err := TopologicalSort(builds, packages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"builds.d", "builds.b", "builds.c", "builds.a"}
	if len(order) != len(want) {
		t.Fatalf("got %d items, want %d: %v", len(order), len(want), order)
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("order[%d] = %q, want %q", i, order[i], w)
		}
	}
}

func TestTopologicalSort_Cycle(t *testing.T) {
	builds := map[string]Build{
		"a": {Commands: []string{"echo a"}, Run: "always", DependsOn: []string{"builds.b"}},
		"b": {Commands: []string{"echo b"}, Run: "always", DependsOn: []string{"builds.a"}},
	}
	packages := map[string]Package{}

	_, err := TopologicalSort(builds, packages)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention 'cycle', got: %v", err)
	}
}

func TestTopologicalSort_SelfCycle(t *testing.T) {
	builds := map[string]Build{
		"a": {Commands: []string{"echo a"}, Run: "always", DependsOn: []string{"builds.a"}},
	}
	packages := map[string]Package{}

	_, err := TopologicalSort(builds, packages)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention 'cycle', got: %v", err)
	}
}

func TestTopologicalSort_MixedBuildsAndPackages(t *testing.T) {
	// builds.x depends on packages.y → packages.y runs first
	builds := map[string]Build{
		"x": {Commands: []string{"echo x"}, Run: "always", DependsOn: []string{"packages.y"}},
	}
	packages := map[string]Package{
		"y": {Source: "local", WorkingDir: "/tmp", Build: []string{"make"}},
	}

	order, err := TopologicalSort(builds, packages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"packages.y", "builds.x"}
	if len(order) != len(want) {
		t.Fatalf("got %d items, want %d: %v", len(order), len(want), order)
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("order[%d] = %q, want %q", i, order[i], w)
		}
	}
}

func TestTopologicalSort_IndependentItemsAppearAlphabetically(t *testing.T) {
	// Items with no depends_on and not depended upon should still appear, in alphabetical position
	builds := map[string]Build{
		"zebra": {Commands: []string{"echo z"}, Run: "always"},
		"alpha": {Commands: []string{"echo a"}, Run: "always"},
	}
	packages := map[string]Package{
		"middle": {Source: "local", WorkingDir: "/tmp", Build: []string{"make"}},
	}

	order, err := TopologicalSort(builds, packages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"builds.alpha", "builds.zebra", "packages.middle"}
	if len(order) != len(want) {
		t.Fatalf("got %d items, want %d: %v", len(order), len(want), order)
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("order[%d] = %q, want %q", i, order[i], w)
		}
	}
}

func TestTopologicalSort_EmptyInput(t *testing.T) {
	order, err := TopologicalSort(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 0 {
		t.Errorf("expected empty order, got: %v", order)
	}
}

func TestTopologicalSort_PackageDependsOnPackage(t *testing.T) {
	builds := map[string]Build{}
	packages := map[string]Package{
		"app":  {Source: "local", WorkingDir: "/tmp", Build: []string{"make"}, DependsOn: []string{"packages.lib"}},
		"lib":  {Source: "local", WorkingDir: "/tmp", Build: []string{"make"}},
	}

	order, err := TopologicalSort(builds, packages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"packages.lib", "packages.app"}
	if len(order) != len(want) {
		t.Fatalf("got %d items, want %d: %v", len(order), len(want), order)
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("order[%d] = %q, want %q", i, order[i], w)
		}
	}
}

func TestTopologicalSort_ComplexMixed(t *testing.T) {
	// builds.brain_index depends on packages.brain
	// builds.csl_hooks depends on packages.csl
	// No cross-dependencies between the two chains, so alphabetical within tiers
	builds := map[string]Build{
		"brain_index":  {Commands: []string{"brain index"}, Run: "always", DependsOn: []string{"packages.brain"}},
		"csl_hooks":    {Commands: []string{"csl hooks install"}, Run: "always", DependsOn: []string{"packages.csl"}},
	}
	packages := map[string]Package{
		"brain": {Source: "remote", Repo: "git@example.com:brain.git", Build: []string{"make"}},
		"csl":   {Source: "remote", Repo: "git@example.com:csl.git", Build: []string{"make"}},
	}

	order, err := TopologicalSort(builds, packages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Initial in-degree 0: packages.brain, packages.csl (alpha order)
	// Pop packages.brain → builds.brain_index becomes in-degree 0
	// Queue is now: [builds.brain_index, packages.csl] (re-sorted alpha)
	// Pop builds.brain_index → nothing new
	// Queue is now: [packages.csl]
	// Pop packages.csl → builds.csl_hooks becomes in-degree 0
	// Pop builds.csl_hooks
	want := []string{"packages.brain", "builds.brain_index", "packages.csl", "builds.csl_hooks"}
	if len(order) != len(want) {
		t.Fatalf("got %d items, want %d: %v", len(order), len(want), order)
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("order[%d] = %q, want %q", i, order[i], w)
		}
	}
}

// --- Validation tests for dependencies ---

func TestValidateDependencies_ValidReference(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Hooks: HooksConfig{
			Builds: map[string]Build{
				"indexer": {
					Commands:  []string{"brain index"},
					Run:       "always",
					DependsOn: []string{"packages.brain"},
				},
			},
		},
		Packages: map[string]Package{
			"brain": {Source: "remote", Repo: "git@example.com:brain.git", Build: []string{"make"}},
		},
	}
	if err := ValidateDependencies(cfg); err != nil {
		t.Errorf("valid depends_on should pass, got: %v", err)
	}
}

func TestValidateDependencies_DanglingReference(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Hooks: HooksConfig{
			Builds: map[string]Build{
				"indexer": {
					Commands:  []string{"brain index"},
					Run:       "always",
					DependsOn: []string{"packages.nonexistent"},
				},
			},
		},
		Packages: map[string]Package{},
	}
	err := ValidateDependencies(cfg)
	if err == nil {
		t.Fatal("expected error for dangling reference, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the dangling reference, got: %v", err)
	}
}

func TestValidateDependencies_InvalidFormat(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Hooks: HooksConfig{
			Builds: map[string]Build{
				"indexer": {
					Commands:  []string{"brain index"},
					Run:       "always",
					DependsOn: []string{"invalid_format"},
				},
			},
		},
	}
	err := ValidateDependencies(cfg)
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
	if !strings.Contains(err.Error(), "builds.") && !strings.Contains(err.Error(), "packages.") {
		t.Errorf("error should mention required format, got: %v", err)
	}
}

func TestValidateDependencies_CycleDetected(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Hooks: HooksConfig{
			Builds: map[string]Build{
				"a": {Commands: []string{"echo a"}, Run: "always", DependsOn: []string{"builds.b"}},
				"b": {Commands: []string{"echo b"}, Run: "always", DependsOn: []string{"builds.a"}},
			},
		},
	}
	err := ValidateDependencies(cfg)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention 'cycle', got: %v", err)
	}
}

func TestValidateDependencies_CrossTypeCycle(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Hooks: HooksConfig{
			Builds: map[string]Build{
				"x": {Commands: []string{"echo x"}, Run: "always", DependsOn: []string{"packages.y"}},
			},
		},
		Packages: map[string]Package{
			"y": {Source: "local", WorkingDir: "/tmp", Build: []string{"make"}, DependsOn: []string{"builds.x"}},
		},
	}
	err := ValidateDependencies(cfg)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention 'cycle', got: %v", err)
	}
}

func TestValidateDependencies_NoDeps(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Hooks: HooksConfig{
			Builds: map[string]Build{
				"a": {Commands: []string{"echo a"}, Run: "always"},
			},
		},
		Packages: map[string]Package{
			"b": {Source: "local", WorkingDir: "/tmp", Build: []string{"make"}},
		},
	}
	if err := ValidateDependencies(cfg); err != nil {
		t.Errorf("no dependencies should pass validation, got: %v", err)
	}
}

func TestValidateDependencies_PackageDanglingReference(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Packages: map[string]Package{
			"app": {Source: "local", WorkingDir: "/tmp", Build: []string{"make"}, DependsOn: []string{"builds.nonexistent"}},
		},
	}
	err := ValidateDependencies(cfg)
	if err == nil {
		t.Fatal("expected error for dangling reference from package, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the dangling reference, got: %v", err)
	}
}

func TestValidateMergedConfig_IncludesDependencyValidation(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Hooks: HooksConfig{
			Builds: map[string]Build{
				"a": {Commands: []string{"echo a"}, Run: "always", DependsOn: []string{"packages.nonexistent"}},
			},
		},
	}
	err := ValidateMergedConfig(cfg)
	if err == nil {
		t.Fatal("ValidateMergedConfig should catch dangling dependency references")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention 'nonexistent', got: %v", err)
	}
}
