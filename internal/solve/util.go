package solve

import (
	"fmt"
	"math"
)

const (
	csPi         = math.Pi
	csEps        = 2.2204460492503131e-16
	um2ToM2      = 1e-12
	tableRefWlUm = 2.0 * csPi
	cm3PerM3     = 1e6
	um3ToM3      = 1e-18
	// nextAfterNegativeInf — предел слева, как в C-версии (nextafter(x, -inf)).
)

func linspace(a, b float64, n int) []float64 {
	if n <= 0 {
		return []float64{}
	}
	out := make([]float64, n)
	if n == 1 {
		out[0] = a
		return out
	}
	step := (b - a) / float64(n-1)
	for i := 0; i < n; i++ {
		out[i] = a + step*float64(i)
	}
	out[n-1] = b
	return out
}

func geomspace(a, b float64, n int) []float64 {
	if n <= 0 {
		return []float64{}
	}
	out := make([]float64, n)
	if n == 1 {
		out[0] = a
		return out
	}
	la, lb := math.Log(a), math.Log(b)
	step := (lb - la) / float64(n-1)
	for i := 0; i < n; i++ {
		out[i] = math.Exp(la + step*float64(i))
	}
	out[n-1] = math.Exp(lb)
	return out
}

// cG повторяет C-формат "%g" (6 значащих цифр, нижний регистр nan/inf).
// Используется в текстовых результатах, чтобы они совпадали с C-версией.
func cG(v float64) string {
	switch {
	case math.IsNaN(v):
		return "nan"
	case math.IsInf(v, 1):
		return "inf"
	case math.IsInf(v, -1):
		return "-inf"
	}
	return fmt.Sprintf("%.6g", v)
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func trapz(y, x []float64) float64 {
	n := len(y)
	sum := 0.0
	for i := 0; i < n-1; i++ {
		sum += 0.5 * (x[i+1] - x[i]) * (y[i] + y[i+1])
	}
	return sum
}

// interpClamped — линейная интерполяция с зажимом на концах (как np.interp).
func interpClamped(x float64, xs, ys []float64) float64 {
	n := len(xs)
	if n <= 0 {
		return math.NaN()
	}
	if x <= xs[0] {
		return ys[0]
	}
	if x >= xs[n-1] {
		return ys[n-1]
	}
	lo, hi := 0, n-1
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if x < xs[mid] {
			hi = mid
		} else {
			lo = mid
		}
	}
	dx := xs[hi] - xs[lo]
	w := 0.0
	if dx > 0 {
		w = (x - xs[lo]) / dx
	}
	return ys[lo] + w*(ys[hi]-ys[lo])
}
