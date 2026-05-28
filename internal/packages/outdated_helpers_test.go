package packages

import "testing"

func TestHasOutdatedAndHasErrors(t *testing.T) {
	none := []OutdatedResult{{Status: "up to date"}, {Status: "skipped"}}
	if HasOutdated(none) {
		t.Error("HasOutdated should be false when nothing is outdated")
	}
	if HasErrors(none) {
		t.Error("HasErrors should be false when there are no errors")
	}

	mixed := []OutdatedResult{{Status: "outdated"}, {Status: "error"}}
	if !HasOutdated(mixed) {
		t.Error("HasOutdated should be true with an outdated result")
	}
	if !HasErrors(mixed) {
		t.Error("HasErrors should be true with an error result")
	}
}

func TestSortResults_OrdersByStatusThenName(t *testing.T) {
	results := []OutdatedResult{
		{Name: "z-skip", Status: "skipped"},
		{Name: "b-ok", Status: "up to date"},
		{Name: "a-err", Status: "error"},
		{Name: "m-out", Status: "outdated"},
		{Name: "a-out", Status: "outdated"},
	}
	SortResults(results)

	wantOrder := []string{"a-out", "m-out", "a-err", "b-ok", "z-skip"}
	for i, want := range wantOrder {
		if results[i].Name != want {
			t.Errorf("position %d = %q, want %q (full: %v)", i, results[i].Name, want, results)
		}
	}
}
