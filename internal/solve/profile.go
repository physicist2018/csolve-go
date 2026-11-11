package solve

type profile struct {
	nr         int
	rGrid      []float64
	nAvg       []float64
	nAvgRaw    []float64
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

func mIndex(mre, mim []float64, re, im float64) int {
	for i := range mre {
		if mre[i] == re && mim[i] == im {
			return i
		}
	}
	return -1
}

// profileFinalize — итоговый профиль одной высоты (общий код одиночного и
// пакетного режимов). ds — ядра S11/S22 для δ_l (nil — не считаются).
func profileFinalize(a *Args, ch []channel, kts []*ktable, mre, mim []float64,
	sols *solutions, geom []*geom, rMin, rMax float64, ds *depolSet) *profile {
	_ = ch
	p := &profile{}
	nr := a.NrOut
	p.nr = nr
	p.rGrid = geomspace(rMin, rMax, nr)

	ens := ensembleAverage(sols, a.Best, geom, p.rGrid, true)
	p.nBest = ens.mat
	p.nAvgRaw = ens.avg
	p.nAvg = append([]float64(nil), ens.avg...)
	p.mRe = ens.mRe
	p.mIm = ens.mIm
	if !a.NoSmooth {
		smoothDistribution(p.rGrid, p.nAvg, ens.count, a.SmoothLam)
	}

	G := a.Best
	if G < 1 {
		G = 1
	}
	if G > sols.n {
		G = sols.n
	}
	p.nBestCount = G

	bestIdx := make([]int, G)
	for t := 0; t < G; t++ {
		bestIdx[t] = mIndex(mre, mim, sols.sol[t].mRe, sols.sol[t].mIm)
		if bestIdx[t] < 0 {
			fatal("internal error: m = %g%+gj is missing from the kernel grid",
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
			q := 3.0 / (4.0 * csPi * r * r * r * r)
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
	return p
}
