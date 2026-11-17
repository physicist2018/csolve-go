package solve

import (
	"strings"
	"testing"
)

// TestArgsBest — разбор --best: количество (>= 1) или доля (< 1).
func TestArgsBest(t *testing.T) {
	{
		a, err := ParseArgs([]string{"--tables", "dir"})
		if err != nil {
			t.Fatalf("defaults: %v", err)
		}
		checkClose(t, "default --best", a.Best, 5, 0)
		if g := a.bestCount(100); g != 5 {
			t.Errorf("default bestCount(100) = %d, want 5", g)
		}
	}
	{
		a, err := ParseArgs([]string{"--tables", "dir", "--best", "0.1"})
		if err != nil {
			t.Fatalf("fraction --best: %v", err)
		}
		checkClose(t, "--best 0.1", a.Best, 0.1, 0)
		if g := a.bestCount(200); g != 20 {
			t.Errorf("bestCount(200) with 0.1 = %d, want 20", g)
		}
	}
	{
		a, err := ParseArgs([]string{"--tables", "dir", "--best", "7"})
		if err != nil {
			t.Fatalf("count --best: %v", err)
		}
		checkClose(t, "--best 7", a.Best, 7, 0)
		if g := a.bestCount(100); g != 7 {
			t.Errorf("bestCount(100) with 7 = %d, want 7", g)
		}
	}
	{
		// нецелое значение >= 1 — округляется с предупреждением
		a, err := ParseArgs([]string{"--tables", "dir", "--best", "2.5"})
		if err != nil {
			t.Fatalf("rounded --best: %v", err)
		}
		checkClose(t, "--best 2.5", a.Best, 3, 0)
	}
	for _, v := range []string{"0", "-1", "-0.5", "nan", "inf"} {
		if _, err := ParseArgs([]string{"--tables", "dir", "--best", v}); err == nil {
			t.Errorf("--best %s must be rejected", v)
		} else if !strings.Contains(err.Error(), "--best") {
			t.Errorf("--best %s: unexpected error: %v", v, err)
		}
	}
}

// TestBestCount — перевод --best в число решений с клампингом.
func TestBestCount(t *testing.T) {
	cases := []struct {
		best   float64
		nTotal int
		want   int
	}{
		{5, 100, 5},
		{5, 3, 3},   // вариантов меньше — берём все
		{1, 100, 1}, // 1 — это количество, не 100 %
		{0.1, 50, 5},
		{0.5, 3, 2},   // round(1.5) = 2
		{0.1, 4, 1},   // round(0.4) = 0 -> минимум 1
		{0.001, 1, 1}, // минимум 1
		{0.5, 100, 50},
		{1e6, 42, 42}, // больше найденного — все
	}
	for _, c := range cases {
		if got := bestCount(c.best, c.nTotal); got != c.want {
			t.Errorf("bestCount(%g, %d) = %d, want %d", c.best, c.nTotal, got,
				c.want)
		}
	}
}
