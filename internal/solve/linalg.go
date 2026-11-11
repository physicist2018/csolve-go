package solve

import (
	"math"

	"gonum.org/v1/gonum/mat"
)

// svdNorm2 — спектральная норма (||A||_2) через SVD gonum.
func svdNorm2(a []float64, m, n int) float64 {
	if m <= 0 || n <= 0 {
		return 0
	}
	data := append([]float64(nil), a[:m*n]...)
	var svd mat.SVD
	if !svd.Factorize(mat.NewDense(m, n, data), mat.SVDNone) {
		return 0
	}
	vals := svd.Values(nil)
	if len(vals) == 0 {
		return 0
	}
	return vals[0]
}

// lstsq — МНК min ||a x - b|| с отсечкой как в numpy lstsq(rcond=None):
// rank = число s_i > eps*max(m,n)*s_max.
func lstsq(a []float64, m, n int, b []float64) []float64 {
	x := make([]float64, n)
	if m <= 0 || n <= 0 {
		return x
	}
	data := append([]float64(nil), a[:m*n]...)
	var svd mat.SVD
	if !svd.Factorize(mat.NewDense(m, n, data), mat.SVDThin) {
		return x
	}
	vals := svd.Values(nil)
	smax := 0.0
	if len(vals) > 0 {
		smax = vals[0]
	}
	rcond := csEps * float64(max(m, n))
	rank := 0
	if smax > 0 {
		for _, v := range vals {
			if v > rcond*smax && v != 0 {
				rank++
			} else {
				break
			}
		}
	}
	if rank == 0 {
		return x
	}
	bb := mat.NewVecDense(m, append([]float64(nil), b[:m]...))
	dst := mat.NewVecDense(n, nil)
	svd.SolveVecTo(dst, bb, rank)
	copy(x, dst.RawVector().Data)
	return x
}

// bandCholSolve — факторизация Холецкого ленточной СПД-матрицы и решение.
// Полуширина ленты kb; band[i*(2kb+1) + (j-i+kb)] = M[i][j].
// Матрица band факторизуется на месте.
func bandCholSolve(band []float64, n, kb int, b []float64) []float64 {
	w := 2*kb + 1
	for i := 0; i < n; i++ {
		j0 := 0
		if i-kb > 0 {
			j0 = i - kb
		}
		for j := j0; j <= i; j++ {
			s := band[i*w+(j-i+kb)]
			k0 := 0
			if i-kb > 0 {
				k0 = i - kb
			}
			if j-kb > k0 {
				k0 = j - kb
			}
			for k := k0; k < j; k++ {
				s -= band[i*w+(k-i+kb)] * band[j*w+(k-j+kb)]
			}
			if j == i {
				if !(s > 0) {
					s = 1e-300 // страховка от потери положительности
				}
				band[i*w+kb] = math.Sqrt(s)
			} else {
				band[i*w+(j-i+kb)] = s / band[j*w+kb]
			}
		}
	}
	x := make([]float64, n)
	for i := 0; i < n; i++ {
		s := b[i]
		k0 := 0
		if i-kb > 0 {
			k0 = i - kb
		}
		for k := k0; k < i; k++ {
			s -= band[i*w+(k-i+kb)] * x[k]
		}
		x[i] = s / band[i*w+kb]
	}
	for i := n - 1; i >= 0; i-- {
		s := x[i]
		for k := i + 1; k < n && k <= i+kb; k++ {
			s -= band[k*w+(i-k+kb)] * x[k]
		}
		x[i] = s / band[i*w+kb]
	}
	return x
}

// lstsqSubset — МНК по подмножеству столбцов cols.
func lstsqSubset(a []float64, m, n int, cols []int, b []float64) []float64 {
	np := len(cols)
	sub := make([]float64, m*np)
	for i := 0; i < m; i++ {
		for j := 0; j < np; j++ {
			sub[i*np+j] = a[i*n+cols[j]]
		}
	}
	return lstsq(sub, m, np, b)
}

// nnls — неотрицательный МНК (Lawson–Hanson): min ||a x - b||, x >= 0.
func nnls(a []float64, m, n int, b []float64) []float64 {
	x := make([]float64, n)
	if n <= 0 {
		return x
	}
	passive := make([]bool, n)
	cols := make([]int, n)
	res := make([]float64, m)

	// начальная норма градиента — для относительного допуска
	g0 := 0.0
	for j := 0; j < n; j++ {
		s := 0.0
		for i := 0; i < m; i++ {
			s += a[i*n+j] * b[i]
		}
		if math.Abs(s) > g0 {
			g0 = math.Abs(s)
		}
	}
	if g0 == 0 {
		g0 = 1
	}
	tol := 1e-12 * g0
	maxIter := 3*n + 10

	for iter := 0; iter < maxIter; iter++ {
		// градиент A^T (b - A x)
		for i := 0; i < m; i++ {
			s := b[i]
			for j := 0; j < n; j++ {
				s -= a[i*n+j] * x[j]
			}
			res[i] = s
		}
		best := -1
		bestv := tol
		for j := 0; j < n; j++ {
			if passive[j] {
				continue
			}
			s := 0.0
			for i := 0; i < m; i++ {
				s += a[i*n+j] * res[i]
			}
			if s > bestv {
				bestv = s
				best = j
			}
		}
		if best < 0 {
			break
		}
		passive[best] = true

		for inner := 0; inner < maxIter; inner++ {
			np := 0
			for j := 0; j < n; j++ {
				if passive[j] {
					cols[np] = j
					np++
				}
			}
			z := lstsqSubset(a, m, n, cols[:np], b)
			allpos := true
			alpha := 1.0
			for k := 0; k < np; k++ {
				j := cols[k]
				if z[k] <= 0 {
					allpos = false
					denom := x[j] - z[k]
					if denom > 0 {
						r := x[j] / denom
						if r < alpha {
							alpha = r
						}
					} else {
						alpha = 0
					}
				}
			}
			if allpos {
				for k := 0; k < np; k++ {
					x[cols[k]] = z[k]
				}
				break
			}
			for k := 0; k < np; k++ {
				j := cols[k]
				x[j] += alpha * (z[k] - x[j])
			}
			for j := 0; j < n; j++ {
				if passive[j] && x[j] <= 1e-14 {
					passive[j] = false
				}
			}
		}
	}
	return x
}
