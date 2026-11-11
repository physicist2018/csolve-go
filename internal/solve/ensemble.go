package solve

import "math"

// distAt — значение распределения решения s в точке r (0 вне окна).
func distAt(s *solution, g *geom, r float64) float64 {
	if r < s.r0 || r > s.r1 {
		return 0
	}
	full := bsplineValues(g.knots, g.degree, r)
	off := 0
	if g.interiorOnly {
		off = 1
	}
	sum := 0.0
	for i := 0; i < g.nUnknown; i++ {
		sum += s.C[i] * full[off+i]
	}
	return sum
}

func solutionDistribution(s *solution, g *geom, r float64) float64 {
	return distAt(s, g, r)
}

// smoothDistribution — сглаживание среднего (scipy make_smoothing_spline):
//
//	min sum_i w_i (y_i - f_i)^2 + lam * int (f'')^2 dx
//
// с нормированными координатами и усиленными весами краёв (1e3).
func smoothDistribution(rGrid, nAvg, w []float64, lamRel float64) {
	n := len(rGrid)
	if n == 0 {
		return
	}
	ok := make([]bool, n)
	cnt := 0
	for i := 0; i < n; i++ {
		if isFinite(nAvg[i]) {
			ok[i] = true
			cnt++
		}
	}
	if cnt < 4 {
		return
	}

	t := make([]float64, cnt)
	y := make([]float64, cnt)
	wts := make([]float64, cnt)
	k := 0
	for i := 0; i < n; i++ {
		if !ok[i] {
			continue
		}
		t[k] = math.Log(rGrid[i])
		y[k] = nAvg[i]
		if w != nil {
			wts[k] = w[i]
		} else {
			wts[k] = 1.0
		}
		k++
	}
	t0, t1 := t[0], t[cnt-1]
	if !(t1 > t0) {
		return
	}
	for i := 0; i < cnt; i++ {
		t[i] = (t[i] - t0) / (t1 - t0)
	}

	scale := 0.0
	for i := 0; i < cnt; i++ {
		if math.Abs(y[i]) > scale {
			scale = math.Abs(y[i])
		}
	}
	if !(scale > 0) || !isFinite(scale) {
		return
	}
	ys := make([]float64, cnt)
	for i := 0; i < cnt; i++ {
		ys[i] = y[i] / scale
	}

	wmax := 0.0
	for i := 0; i < cnt; i++ {
		if wts[i] > wmax {
			wmax = wts[i]
		}
	}
	wden := wmax
	if wden <= 1e-300 {
		wden = 1e-300
	}
	for i := 0; i < cnt; i++ {
		wts[i] /= wden
	}
	wts[0] = 1e3
	wts[cnt-1] = 1e3

	if cnt < 5 {
		return
	}

	fit := smoothingSplineFit(t, ys, wts, lamRel, t)

	k = 0
	for i := 0; i < n; i++ {
		if !ok[i] {
			continue
		}
		nAvg[i] = fit[k] * scale
		k++
	}
}

// ensembleResult — результат усреднения по лучшим решениям.
type ensembleResult struct {
	avg, count []float64
	mat        [][]float64 // G x nr, если запрошено
	mRe, mIm   float64
}

// ensembleAverage — среднее только по решениям, чьё окно покрывает r;
// точки без покрытия — NaN.
func ensembleAverage(sols *solutions, nBest int, geom []*geom,
	rGrid []float64, wantMat bool) ensembleResult {
	G := nBest
	if G < 1 {
		G = 1
	}
	if G > sols.n {
		G = sols.n
	}
	nr := len(rGrid)
	total := make([]float64, nr)
	count := make([]float64, nr)

	var mat [][]float64
	if wantMat {
		mat = make([][]float64, G)
		for t := 0; t < G; t++ {
			mat[t] = make([]float64, nr)
		}
	}

	mre, mim := 0.0, 0.0
	for t := 0; t < G; t++ {
		s := &sols.sol[t]
		g := geom[s.geomIndex]
		for i := 0; i < nr; i++ {
			v := distAt(s, g, rGrid[i])
			if wantMat {
				mat[t][i] = v
			}
			if rGrid[i] >= s.r0 && rGrid[i] <= s.r1 {
				total[i] += v
				count[i] += 1.0
			}
		}
		mre += s.mRe
		mim += s.mIm
	}
	if G > 0 {
		mre /= float64(G)
		mim /= float64(G)
	}
	avg := make([]float64, nr)
	for i := 0; i < nr; i++ {
		if count[i] > 0 {
			avg[i] = total[i] / count[i]
		} else {
			avg[i] = math.NaN()
		}
	}
	return ensembleResult{avg: avg, count: count, mat: mat, mRe: mre, mIm: mim}
}
