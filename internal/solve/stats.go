package solve

import "math"

type moments struct {
	NCm3             float64
	VolMeanUm3       float64
	VolConcUm3PerCm3 float64
	REffUm           float64
	RCovMinUm        float64
	RCovMaxUm        float64
	CovFrac          float64
}

type csMode struct {
	V, Rm, Sigma float64
	OK           bool
	Method       string
}

type modes struct {
	Fine, Coarse, Overall csMode
}

func perCm3(v float64) float64 { return v / cm3PerM3 }

func qDvol(r float64) float64 { return 3.0 / (4.0 * csPi * r * r * r * r) }

func lognormalVolume(r, V, rg, sigma float64) float64 {
	s := math.Log(sigma)
	z := math.Log(r / rg)
	return V / (math.Sqrt(2.0*csPi) * s) * math.Exp(-(z*z)/(2.0*s*s))
}

func demoTrueDistribution(r float64) float64 {
	return lognormalVolume(r, 4.0e7, 0.15, 1.5) +
		lognormalVolume(r, 6.0e7, 1.2, 1.7)
}

func sizeMoments(nAvg, rGrid []float64) moments {
	n := len(nAvg)
	f := make([]float64, n)
	lnr := make([]float64, n)
	fr := make([]float64, n)
	nq := make([]float64, n)
	cov := 0
	rlo, rhi := math.NaN(), math.NaN()
	for i := 0; i < n; i++ {
		lnr[i] = math.Log(rGrid[i])
		if isFinite(nAvg[i]) {
			v := nAvg[i]
			if v < 0 {
				v = 0
			}
			f[i] = v
			fr[i] = v / rGrid[i]
			nq[i] = v * qDvol(rGrid[i])
			cov++
			if cov == 1 || rGrid[i] < rlo {
				rlo = rGrid[i]
			}
			if cov == 1 || rGrid[i] > rhi {
				rhi = rGrid[i]
			}
		}
	}
	vConc := trapz(f, lnr)
	mLn1 := trapz(fr, lnr)
	nNum := trapz(nq, rGrid)

	var m moments
	m.VolConcUm3PerCm3 = perCm3(vConc)
	m.NCm3 = perCm3(nNum)
	if nNum > 0 {
		m.VolMeanUm3 = vConc / nNum
	} else {
		m.VolMeanUm3 = math.NaN()
	}
	if mLn1 > 0 {
		m.REffUm = vConc / mLn1
	} else {
		m.REffUm = math.NaN()
	}
	m.RCovMinUm = rlo
	m.RCovMaxUm = rhi
	if n > 0 {
		m.CovFrac = float64(cov) / float64(n)
	}
	return m
}

func fitLognormalLn(x, y, w []float64) csMode {
	out := csMode{V: math.NaN(), Rm: math.NaN(), Sigma: math.NaN(),
		OK: false, Method: ""}
	n := len(x)
	if n < 3 {
		return out
	}
	a := make([]float64, n*3)
	b := make([]float64, n)
	for i := 0; i < n; i++ {
		sw := 1.0
		if w != nil {
			if w[i] <= 0 {
				continue
			}
			sw = math.Sqrt(w[i])
		}
		a[i*3+0] = sw
		a[i*3+1] = sw * x[i]
		a[i*3+2] = sw * x[i] * x[i]
		b[i] = sw * y[i]
	}
	coef := lstsq(a, n, 3, b)
	ca, cb, cc := coef[0], coef[1], coef[2]
	if cc >= 0 || !isFinite(cc) {
		return out
	}
	mu := -cb / (2.0 * cc)
	s2 := -1.0 / (2.0 * cc)
	s := math.Sqrt(s2)
	v := math.Exp(ca-cb*cb/(4.0*cc)) * math.Sqrt(2.0*csPi) * s
	if !(v > 0) || !isFinite(v) || !isFinite(mu) {
		return out
	}
	out.V = v
	out.Rm = math.Exp(mu)
	out.Sigma = math.Exp(s)
	out.OK = true
	out.Method = "fit"
	return out
}

func analyze(r, f, w []float64, n, method int) csMode {
	out := csMode{V: math.NaN(), Rm: math.NaN(), Sigma: math.NaN(),
		OK: false, Method: "insufficient points"}
	if n < 3 {
		return out
	}
	fr := make([]float64, n)
	lnr := make([]float64, n)
	for i := 0; i < n; i++ {
		if f[i] > 0 {
			fr[i] = f[i]
		}
		lnr[i] = math.Log(r[i])
	}
	v := trapz(fr, lnr)
	if !(v > 0) || !isFinite(v) {
		out.Method = "no positive volume"
		return out
	}
	out.V = v
	pos := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if f[i] > 0 {
			pos = append(pos, i)
		}
	}
	np := len(pos)
	if np < 3 {
		out.Method = "few points with F > 0"
		return out
	}
	if method != 2 {
		x := make([]float64, np)
		y := make([]float64, np)
		var wsub []float64
		if w != nil {
			wsub = make([]float64, np)
		}
		for k := 0; k < np; k++ {
			x[k] = math.Log(r[pos[k]])
			y[k] = math.Log(f[pos[k]])
			if wsub != nil {
				wsub[k] = w[pos[k]]
			}
		}
		fit := fitLognormalLn(x, y, wsub)
		if fit.OK {
			out.Rm = fit.Rm
			out.Sigma = fit.Sigma
			out.OK = true
			out.Method = "fit"
			return out
		}
	}
	// моментная оценка по объёмным весам F
	xf := make([]float64, np)
	yf := make([]float64, np)
	for k := 0; k < np; k++ {
		xf[k] = math.Log(r[pos[k]])
		yf[k] = f[pos[k]]
	}
	wsum := 0.0
	for k := 0; k < np; k++ {
		wsum += yf[k]
	}
	mu := 0.0
	for k := 0; k < np; k++ {
		mu += yf[k] * xf[k]
	}
	mu /= wsum
	s2 := 0.0
	for k := 0; k < np; k++ {
		d := xf[k] - mu
		s2 += yf[k] * d * d
	}
	s2 /= wsum
	if !(s2 > 0) || !isFinite(s2) {
		s2 = 0
	}
	out.Rm = math.Exp(mu)
	out.Sigma = math.Exp(math.Sqrt(s2))
	out.OK = true
	out.Method = "moments"
	return out
}

func analyzeFiltered(rGrid, f, w []float64, n int, lo, hi float64, method int) csMode {
	cnt := 0
	for i := 0; i < n; i++ {
		if rGrid[i] >= lo && rGrid[i] <= hi {
			cnt++
		}
	}
	r := make([]float64, cnt)
	fv := make([]float64, cnt)
	var wv []float64
	if w != nil {
		wv = make([]float64, cnt)
	}
	k := 0
	for i := 0; i < n; i++ {
		if rGrid[i] < lo || rGrid[i] > hi {
			continue
		}
		r[k] = rGrid[i]
		fv[k] = f[i]
		if wv != nil {
			wv[k] = w[i]
		}
		k++
	}
	return analyze(r, fv, wv, cnt, method)
}

func modeAnalysis(rGrid, fAvg []float64, rSplit float64, w []float64,
	method int) modes {
	var out modes
	n := len(rGrid)
	r := make([]float64, 0, n)
	f := make([]float64, 0, n)
	var wv []float64
	if w != nil {
		wv = make([]float64, 0, n)
	}
	for i := 0; i < n; i++ {
		if !isFinite(fAvg[i]) {
			continue
		}
		r = append(r, rGrid[i])
		f = append(f, fAvg[i])
		if wv != nil {
			wv = append(wv, w[i])
		}
	}
	cnt := len(r)
	if cnt < 4 {
		bad := csMode{V: math.NaN(), Rm: math.NaN(), Sigma: math.NaN(),
			OK: false, Method: "insufficient points"}
		out.Fine, out.Coarse, out.Overall = bad, bad, bad
		return out
	}
	r0, r1 := r[0], r[0]
	for i := 1; i < cnt; i++ {
		if r[i] < r0 {
			r0 = r[i]
		}
		if r[i] > r1 {
			r1 = r[i]
		}
	}
	out.Fine = analyzeFiltered(r, f, wv, cnt, r0, rSplit, method)
	out.Coarse = analyzeFiltered(r, f, wv, cnt, rSplit, r1, method)
	out.Overall = analyzeFiltered(r, f, wv, cnt, r0, r1, method)
	return out
}
