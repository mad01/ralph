package config

import (
	"fmt"
	"sort"
	"strings"
)

// WaveGroup holds the builds and packages for one execution wave.
type WaveGroup struct {
	Builds   map[string]Build
	Packages map[string]Package
}

// GroupByWave partitions builds and packages by their Wave field. Each item is
// grouped under its exact Wave value: recipe items default to wave 1 (set during
// merge), and items declared directly in the main config keep the zero value
// (wave 0), so they run before recipe items. Lower wave numbers run first.
func GroupByWave(builds map[string]Build, packages map[string]Package) map[int]*WaveGroup {
	groups := make(map[int]*WaveGroup)

	for name, build := range builds {
		w := build.Wave
		if groups[w] == nil {
			groups[w] = &WaveGroup{
				Builds:   make(map[string]Build),
				Packages: make(map[string]Package),
			}
		}
		groups[w].Builds[name] = build
	}

	for name, pkg := range packages {
		w := pkg.Wave
		if groups[w] == nil {
			groups[w] = &WaveGroup{
				Builds:   make(map[string]Build),
				Packages: make(map[string]Package),
			}
		}
		groups[w].Packages[name] = pkg
	}

	return groups
}

// SortedWaveNumbers returns wave numbers in ascending order.
func SortedWaveNumbers(groups map[int]*WaveGroup) []int {
	nums := make([]int, 0, len(groups))
	for w := range groups {
		nums = append(nums, w)
	}
	sort.Ints(nums)
	return nums
}

// CrossWaveDependencyWarnings returns advisory warnings for depends_on edges
// whose target runs in a LATER wave than the item declaring it. Wave ordering
// then contradicts the dependency — the dependent runs before the thing it
// depends on, and TopologicalSort only orders within a wave, so nothing enforces
// the edge. Same-wave (handled by the topo sort) and earlier-wave (already
// completed) dependencies produce no warning. Unknown references are left to
// ValidateDependencies. Callers log these; they are not fatal.
func CrossWaveDependencyWarnings(builds map[string]Build, packages map[string]Package) []string {
	wave := make(map[string]int, len(builds)+len(packages))
	for name, b := range builds {
		wave["builds."+name] = b.Wave
	}
	for name, p := range packages {
		wave["packages."+name] = p.Wave
	}

	var warnings []string
	check := func(owner string, ownerWave int, deps []string) {
		for _, dep := range deps {
			depWave, ok := wave[dep]
			if !ok {
				continue
			}
			if depWave > ownerWave {
				warnings = append(warnings, fmt.Sprintf(
					"%s (wave %d) depends on %s (wave %d) which runs in a LATER wave — ordering is not enforced; move %s to wave %d or lower",
					owner, ownerWave, dep, depWave, dep, ownerWave))
			}
		}
	}
	for name, b := range builds {
		check("builds."+name, b.Wave, b.DependsOn)
	}
	for name, p := range packages {
		check("packages."+name, p.Wave, p.DependsOn)
	}
	sort.Strings(warnings)
	return warnings
}

// TopologicalSort returns a topologically sorted list of build and package
// keys in the form "builds.<name>" or "packages.<name>". Items with no
// dependencies come first, with ties broken alphabetically (Kahn's algorithm).
// Returns an error if a dependency cycle is detected.
func TopologicalSort(builds map[string]Build, packages map[string]Package) ([]string, error) {
	// Collect all node keys.
	nodes := make(map[string]bool)
	for name := range builds {
		nodes["builds."+name] = true
	}
	for name := range packages {
		nodes["packages."+name] = true
	}

	if len(nodes) == 0 {
		return nil, nil
	}

	// Build adjacency list (edges point from dependency → dependent)
	// and compute in-degree for each node.
	inDegree := make(map[string]int, len(nodes))
	dependents := make(map[string][]string, len(nodes)) // dependency → list of dependents

	for key := range nodes {
		inDegree[key] = 0
	}

	// Process build dependencies (skip cross-wave references — those items
	// are in a different wave group and already guaranteed to have completed).
	for name, build := range builds {
		key := "builds." + name
		for _, dep := range build.DependsOn {
			if !nodes[dep] {
				continue
			}
			dependents[dep] = append(dependents[dep], key)
			inDegree[key]++
		}
	}

	// Process package dependencies.
	for name, pkg := range packages {
		key := "packages." + name
		for _, dep := range pkg.DependsOn {
			if !nodes[dep] {
				continue
			}
			dependents[dep] = append(dependents[dep], key)
			inDegree[key]++
		}
	}

	// Kahn's algorithm: start with all nodes that have in-degree 0.
	var queue []string
	for key := range nodes {
		if inDegree[key] == 0 {
			queue = append(queue, key)
		}
	}
	sort.Strings(queue)

	var order []string
	for len(queue) > 0 {
		// Pop the first (alphabetically smallest) item.
		current := queue[0]
		queue = queue[1:]
		order = append(order, current)

		// For each dependent of current, reduce in-degree.
		for _, dep := range dependents[current] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
		// Re-sort the queue to maintain alphabetical tie-breaking.
		sort.Strings(queue)
	}

	if len(order) != len(nodes) {
		// Find the nodes involved in the cycle for a descriptive error.
		var cycleNodes []string
		for key := range nodes {
			if inDegree[key] > 0 {
				cycleNodes = append(cycleNodes, key)
			}
		}
		sort.Strings(cycleNodes)
		return nil, fmt.Errorf("dependency cycle detected among: %s", strings.Join(cycleNodes, ", "))
	}

	return order, nil
}
