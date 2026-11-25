package solve

import "testing"

func TestPlotCurves(t *testing.T) {
	cases := []struct{ n, limit, want int }{
		{2072, 0, 2072},  // 0 — без ограничения
		{2072, -1, 2072}, // отрицательное значение тоже трактуется как «все»
		{2072, 64, 64},   // явный предел
		{10, 64, 10},     // предел больше числа решений
		{0, 64, 0},       // нет решений
	}
	for _, c := range cases {
		if got := plotCurves(c.n, c.limit); got != c.want {
			t.Errorf("plotCurves(%d, %d) = %d, want %d",
				c.n, c.limit, got, c.want)
		}
	}
}

func TestPlotBestLimitFlag(t *testing.T) {
	a, err := ParseArgs([]string{"--plot-best-limit", "64"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if a.PlotBestLimit != 64 {
		t.Errorf("PlotBestLimit = %d, want 64", a.PlotBestLimit)
	}

	// по умолчанию — без ограничения
	def, err := DefaultArgs()
	if err != nil {
		t.Fatalf("DefaultArgs: %v", err)
	}
	if def.PlotBestLimit != 0 {
		t.Errorf("default PlotBestLimit = %d, want 0", def.PlotBestLimit)
	}

	if _, err := ParseArgs([]string{"--plot-best-limit", "-1"}); err == nil {
		t.Error("negative --plot-best-limit must be rejected")
	}
}
