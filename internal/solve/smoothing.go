package solve

import "math"

// smoothingSplineFit — кубический сглаживающий сплайн (эквивалент scipy
// make_smoothing_spline):
//
//	min_f sum_i w_i (f(x_i) - y_i)^2 + lam * integral (f'')^2 dx
//
// Симметричная положительно определённая ленточная система решается
// Холецким. Возвращает значения в точках xeval.
func smoothingSplineFit(x, y, w []float64, lam float64, xeval []float64) []float64 {
	out := make([]float64, len(xeval))
	for k := range out {
		out[k] = math.NaN()
	}
	n := len(x)
	if n < 5 {
		return out // scipy требует >= 5 точек
	}

	const p = 3
	nbf := n + 2
	nk := nbf + p + 1
	U := make([]float64, nk)
	U[0], U[1], U[2] = x[0], x[0], x[0]
	for i := 0; i < n; i++ {
		U[3+i] = x[i]
	}
	U[nk-1], U[nk-2], U[nk-3] = x[n-1], x[n-1], x[n-1]

	const kb = 3
	bw := 2*kb + 1
	M := make([]float64, nbf*bw)
	rhs := make([]float64, nbf)

	mb := func(i, j int) int { return i*bw + (j - i + kb) }

	// член данных B^T W B и правая часть B^T W y
	for i := 0; i < n; i++ {
		wi := 1.0
		if w != nil {
			wi = w[i]
		}
		loc, span := bsplineDerivWindow(U, p, 0, x[i])
		base := span - p
		for a := 0; a < p+1; a++ {
			ia := base + a
			if ia < 0 || ia >= nbf {
				continue
			}
			rhs[ia] += wi * y[i] * loc[a]
			for b := 0; b < p+1; b++ {
				ib := base + b
				if ib < 0 || ib >= nbf {
					continue
				}
				if absInt(ia-ib) <= kb {
					M[mb(ia, ib)] += wi * loc[a] * loc[b]
				}
			}
		}
	}

	// штраф lam * int (f'')^2 dx по пролётам между узлами x
	gl3 := [3]float64{-0.7745966692414834, 0.0, 0.7745966692414834}
	gw3 := [3]float64{0.5555555555555556, 0.8888888888888888, 0.5555555555555556}
	for i := 0; i < n-1; i++ {
		a, b := x[i], x[i+1]
		mid := 0.5 * (a + b)
		half := 0.5 * (b - a)
		for q := 0; q < 3; q++ {
			node := half*gl3[q] + mid
			d2, span := bsplineDerivWindow(U, p, 2, node)
			base := span - p
			wq := half * gw3[q] * lam
			for a1 := 0; a1 < p+1; a1++ {
				i1 := base + a1
				if i1 < 0 || i1 >= nbf {
					continue
				}
				for b1 := 0; b1 < p+1; b1++ {
					j1 := base + b1
					if j1 < 0 || j1 >= nbf {
						continue
					}
					if absInt(i1-j1) <= kb {
						M[mb(i1, j1)] += wq * d2[a1] * d2[b1]
					}
				}
			}
		}
	}

	c := bandCholSolve(M, nbf, kb, rhs)

	for k := range xeval {
		loc, span := bsplineDerivWindow(U, p, 0, xeval[k])
		base := span - p
		val := 0.0
		for a := 0; a < p+1; a++ {
			ia := base + a
			if ia >= 0 && ia < nbf {
				val += c[ia] * loc[a]
			}
		}
		out[k] = val
	}
	return out
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
