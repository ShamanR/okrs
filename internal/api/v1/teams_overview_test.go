package v1

import "testing"

func TestCollectDescendantIDs(t *testing.T) {
	nodes := []teamNode{
		{
			ID: 1,
			Children: []teamNode{
				{ID: 2},
				{ID: 3, Children: []teamNode{{ID: 4}}},
			},
		},
		{ID: 5},
	}

	got := collectDescendantIDs(1, nodes)
	want := []int64{2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("expected %d descendants, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("descendants mismatch at %d: want %d, got %d", i, want[i], got[i])
		}
	}

	empty := collectDescendantIDs(999, nodes)
	if len(empty) != 0 {
		t.Fatalf("expected no descendants for unknown team, got %v", empty)
	}
}
