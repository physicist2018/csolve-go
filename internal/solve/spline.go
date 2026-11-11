package solve

import "math"

// spline — кубический сплайн с краевыми условиями not-a-knot.
// Повторяет scipy.interpolate.CubicSpline(bc_type='not-a-knot'), n=3 —
// параболическая интерполяция, n=2 — линейная.
type spline struct {
	n       int
	x, y    []float64
	b, c, d []float64 // коэффициенты на интервалах 0..n-2
}

func newSpline(x, y []float64) *spline {
	n := len(x)
	s := &spline{
		n: n,
		x: append([]float64(nil), x...),
		y: append([]float64(nil), y...),
	}
	if n <= 1 {
		return s
	}
	s.b = make([]float64, n-1)
	s.c = make([]float64, n-1)
	s.d = make([]float64, n-1)

	if n == 2 {
		h := x[1] - x[0]
		s.b[0] = (y[1] - y[0]) / h
		return s
	}

	M := make([]float64, n)
	if n == 3 {
		h0, h1 := x[1]-x[0], x[2]-x[1]
		d0, d1 := (y[1]-y[0])/h0, (y[2]-y[1])/h1
		m := 2.0 * (d1 - d0) / (h0 + h1)
		M[0], M[1], M[2] = m, m, m
	} else {
		// Неизвестные M_1..M_{n-2}: трёхдиагональная система (n-2)x(n-2).
		m := n - 2
		a := make([]float64, m)
		b := make([]float64, m)
		c := make([]float64, m)
		r := make([]float64, m)
		h := make([]float64, n-1)
		delta := make([]float64, n-1)
		for i := 0; i < n-1; i++ {
			h[i] = x[i+1] - x[i]
			delta[i] = (y[i+1] - y[i]) / h[i]
		}
		// k = 0 — узел 1
		b[0] = (h[0] + h[1]) * (h[0] + 2.0*h[1])
		c[0] = h[1]*h[1] - h[0]*h[0]
		r[0] = 6.0 * h[1] * (delta[1] - delta[0])
		// внутренние строки, узел i = k+1, i = 2..n-3
		for k := 1; k <= m-2; k++ {
			i := k + 1
			a[k] = h[i-1]
			b[k] = 2.0 * (h[i-1] + h[i])
			c[k] = h[i]
			r[k] = 6.0 * (delta[i] - delta[i-1])
		}
		// k = m-1 — узел n-2
		if m-1 > 0 {
			k := m - 1
			a[k] = h[n-3]*h[n-3] - h[n-2]*h[n-2]
			b[k] = (h[n-3] + h[n-2]) * (2.0*h[n-3] + h[n-2])
			r[k] = 6.0 * h[n-3] * (delta[n-2] - delta[n-3])
		}
		// прямая прогонка
		for k := 1; k < m; k++ {
			w := a[k] / b[k-1]
			b[k] -= w * c[k-1]
			r[k] -= w * r[k-1]
		}
		sol := make([]float64, m)
		sol[m-1] = r[m-1] / b[m-1]
		for k := m - 2; k >= 0; k-- {
			sol[k] = (r[k] - c[k]*sol[k+1]) / b[k]
		}
		for k := 0; k < m; k++ {
			M[k+1] = sol[k]
		}
		// восстановление M_0 и M_{n-1}
		M[0] = ((h[0]+h[1])*M[1] - h[0]*M[2]) / h[1]
		M[n-1] = (-h[n-2]*M[n-3] + (h[n-2]+h[n-3])*M[n-2]) / h[n-3]
	}
	s.fillCoeffs(M)
	return s
}

func (s *spline) fillCoeffs(M []float64) {
	for i := 0; i < s.n-1; i++ {
		h := s.x[i+1] - s.x[i]
		delta := (s.y[i+1] - s.y[i]) / h
		s.c[i] = 0.5 * M[i]
		s.b[i] = delta - (2.0*M[i]+M[i+1])*h/6.0
		s.d[i] = (M[i+1] - M[i]) / (6.0 * h)
	}
}

func (s *spline) eval(t float64) float64 {
	n := s.n
	if n <= 0 {
		return math.NaN()
	}
	if n == 1 {
		return s.y[0]
	}
	var lo int
	if t <= s.x[0] {
		lo = 0
	} else if t >= s.x[n-1] {
		lo = n - 2
	} else {
		hi := n - 1
		lo = 0
		for hi-lo > 1 {
			mid := (lo + hi) / 2
			if t < s.x[mid] {
				hi = mid
			} else {
				lo = mid
			}
		}
	}
	dx := t - s.x[lo]
	return s.y[lo] + dx*(s.b[lo]+dx*(s.c[lo]+dx*s.d[lo]))
}

// evalHint — то же, но с подсказкой интервала для монотонно возрастающих t.
func (s *spline) evalHint(t float64, hint *int) float64 {
	n := s.n
	if n <= 0 {
		return math.NaN()
	}
	if n == 1 {
		return s.y[0]
	}
	lo := *hint
	if lo < 0 {
		lo = 0
	}
	if lo > n-2 {
		lo = n - 2
	}
	if t <= s.x[0] {
		lo = 0
	} else if t >= s.x[n-1] {
		lo = n - 2
	} else {
		for lo > 0 && t < s.x[lo] {
			lo--
		}
		for lo < n-2 && t >= s.x[lo+1] {
			lo++
		}
	}
	*hint = lo
	dx := t - s.x[lo]
	return s.y[lo] + dx*(s.b[lo]+dx*(s.c[lo]+dx*s.d[lo]))
}
