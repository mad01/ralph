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

// GroupByWave partitions builds and packages by their Wave field.
// Items with Wave <= 0 are placed in wave 2 (the default).
func GroupByWave(builds map[string]Build, packages map[string]Package) map[int]*WaveGroup {
	groups := make(map[int]*WaveGroup)

	for name, build := range builds {
		w := build.Wave
		if w <= 0 {
			w = 2
		}
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
		if w <= 0 {
			w = 2
		}
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

	// Process build dependencies.
	for name, build := range builds {
		key := "builds." + name
		for _, dep := range build.DependsOn {
			dependents[dep] = append(dependents[dep], key)
			inDegree[key]++
		}
	}

	// Process package dependencies.
	for name, pkg := range packages {
		key := "packages." + name
		for _, dep := range pkg.DependsOn {
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
