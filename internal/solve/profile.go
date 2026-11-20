package solve

import (
	"fmt"
	"math"
)

type profile struct {
	nr         int
	rGrid      []float64
	nAvg       []float64
	nAvgRaw    []float64
	vCum       []float64 // кумулятивный объём V(r) = ∫ n dln r, µm³/cm³
	nBest      [][]float64
	nBestCount int
	mRe, mIm   float64
	gFit       []float64
	moments    moments
	modes      modes
	count      []float64
	deltaL     []float64 // δ_l = (S11-S22)/(S11+S22)*100, %
	deltaLwl   []float64 // длины волн δ_l, µm
}

// mEps — допуск сравнения m: значения приходят из имён каталогов таблиц,
// поэтому точное == ненадёжно, а шаг сетки m на порядки больше.
const mEps = 1e-6

// mIndex — индекс пары (re, im) в сетке m; -1, если пары нет.
func mIndex(mre, mim []float64, re, im float64) int {
	for i := range mre {
		if math.Abs(mre[i]-re) <= mEps && math.Abs(mim[i]-im) <= mEps {
			return i
		}
	}
	return -1
}

// profileFinalize — итоговый профиль одной высоты (общий код одиночного и
// пакетного режимов). ds — ядра S11/S22 для δ_l (nil — не считаются).
func profileFinalize(a *Args, ch []channel, kts []*ktable, mre, mim []float64,
	sols *solutions, geom []*geom, rMin, rMax float64, ds *depolSet) (*profile, error) {
	_ = ch
	p := &profile{}
	nr := a.NrOut
	p.nr = nr
	p.rGrid = geomspace(rMin, rMax, nr)

	ens := ensembleAverage(sols, a.bestCount(sols.n), geom, p.rGrid, true)
	// Решение получено для выбранного порядка --kernel-order (y_k);
	// пересчитываем усреднённое распределение и все лучшие решения обратно
	// в dV/dln r до всякой обработки (сглаживание, моменты, графики).
	ko := normalizeKernelOrder(a.KernelOrder)
	applyKernelToVolume(ko, p.rGrid, ens.avg)
	for t := range ens.mat {
		applyKernelToVolume(ko, p.rGrid, ens.mat[t])
	}
	p.nBest = ens.mat
	p.nAvgRaw = ens.avg
	p.nAvg = append([]float64(nil), ens.avg...)
	p.mRe = ens.mRe
	p.mIm = ens.mIm
	if !a.NoSmooth {
		smoothDistribution(p.rGrid, p.nAvg, ens.count, a.SmoothLam)
		// монотонный метод по построению даёт n ≥ 0; сглаживающий сплайн
		// может дать небольшой отрицательный выброс у нуля — ограничиваем
		if sols.method == methodMonotone {
			clampNonNegative(p.nAvg)
		}
	}
	// кумулятивный объём строится по итоговому среднему n ≥ 0, поэтому V(r)
	// монотонна и V(r_end) совпадает с C_v
	p.vCum = cumulativeVolume(p.rGrid, p.nAvg)

	G := a.bestCount(sols.n)
	p.nBestCount = G

	bestIdx := make([]int, G)
	for t := 0; t < G; t++ {
		bestIdx[t] = mIndex(mre, mim, sols.sol[t].mRe, sols.sol[t].mIm)
		if bestIdx[t] < 0 {
			return nil, fmt.Errorf("internal error: m = %g%+gj is missing from the kernel grid",
				sols.sol[t].mRe, sols.sol[t].mIm)
		}
	}

	rq, wq := quadratureGrid([]float64{rMin, rMax}, a.QuadOrder, a.LogSpan)
	navg0 := make([]float64, nr)
	for i := 0; i < nr; i++ {
		if isFinite(p.nAvg[i]) {
			navg0[i] = p.nAvg[i]
		}
	}
	nqVals := make([]float64, len(rq))
	for k := range rq {
		nqVals[k] = interpClamped(rq[k], p.rGrid, navg0)
	}

	nch := len(sols.sol[0].gFit)
	p.gFit = make([]float64, nch)
	for j := 0; j < nch; j++ {
		s := 0.0
		for k := range rq {
			ksum := 0.0
			for t := 0; t < G; t++ {
				ksum += ktableKernel(kts[bestIdx[t]], j, rq[k])
			}
			kavg := ksum / float64(G)
			r := rq[k]
			q := 3.0 / (4.0 * math.Pi * r * r * r * r)
			s += wq[k] * kavg * q * nqVals[k]
		}
		p.gFit[j] = s
	}

	// δ_l — свёртка S11/S22 с тем же средним распределением
	if depolSetAvailable(ds) {
		p.deltaLwl = append([]float64(nil), ds.wl...)
		_, _, p.deltaL = depolFromProfile(ds, bestIdx, rq, wq, nqVals)
	}

	p.moments = sizeMoments(p.nAvg, p.rGrid)
	p.count = make([]float64, nr)
	for t := 0; t < G; t++ {
		s := &sols.sol[t]
		for i := 0; i < nr; i++ {
			if p.rGrid[i] >= s.r0 && p.rGrid[i] <= s.r1 {
				p.count[i] += 1.0
			}
		}
	}
	p.modes = modeAnalysis(p.rGrid, p.nAvg, a.RSplit, p.count, a.Modes)
	return p, nil
}
