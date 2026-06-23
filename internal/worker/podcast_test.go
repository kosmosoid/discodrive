package worker

import "testing"

func TestPruneStart(t *testing.T) {
	cases := []struct{ total, keep, want int }{
		{8, 5, 5},  // prune rows[5:8] = 3 oldest
		{3, 5, 3},  // fewer than keep → nothing pruned (start=total)
		{5, 5, 5},  // exactly keep → nothing
		{10, 0, 0}, // keep 0 → prune all
	}
	for _, c := range cases {
		if got := pruneStart(c.total, c.keep); got != c.want {
			t.Errorf("pruneStart(%d,%d)=%d want %d", c.total, c.keep, got, c.want)
		}
	}
}
