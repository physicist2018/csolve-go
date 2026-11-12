package solve

import (
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"time"
)

func nowSec() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}

var timeEnabled bool

func phase(name string, t0 float64) {
	if timeEnabled {
		fmt.Fprintf(os.Stderr, "[time] %-24s %.3f s\n", name, nowSec()-t0)
	}
}

// ---------------- демо ----------------
func mIndexOf(mre, mim []float64, re, im float64) int {
	for i := range mre {
		if mre[i] == re && mim[i] == im {
			return i
		}
	}
	return -1
}

func resolveDemoM(tre, tim *float64, a *Args, mre, mim []float64) {
	cand := []int{}
	for i := range mre {
		ok := true
		if a.HasMReal {
			if fmt.Sprintf("%.2f", a.MReal) != fmt.Sprintf("%.2f", mre[i]) {
				ok = false
			}
		}
		if ok && a.HasMImag {
			if fmt.Sprintf("%.5f", a.MImag) != fmt.Sprintf("%.5f", mim[i]) {
				ok = false
			}
		}
		if ok {
			cand = append(cand, i)
		}
	}
	if len(cand) == 0 {
		parts := make([]string, len(mre))
		for i := range mre {
			parts[i] = fmt.Sprintf("%g%+gj", mre[i], mim[i])
		}
		fatal("demo true m = %g%+gj is missing from the tabular m grid: %s — "+
			"set --m-real/--m-imag from this list", *tre, *tim,
			strings.Join(parts, ", "))
	}
	best := cand[0]
	for _, i := range cand[1:] {
		d1 := math.Abs(mre[i]-*tre) + math.Abs(mim[i]-*tim)
		d2 := math.Abs(mre[best]-*tre) + math.Abs(mim[best]-*tim)
		if d1 < d2 {
			best = i
		}
	}
	if mre[best] != *tre || mim[best] != *tim {
		fmt.Printf("[tabular mode] demo true m: using %g%+gj (closest to "+
			"%g%+gj in tables)\n", mre[best], mim[best], *tre, *tim)
	}
	*tre = mre[best]
	*tim = mim[best]
}

// forwardMeasurements — прямой расчёт g по демонстрационному распределению.
func forwardMeasurements(kt *ktable, nf func(float64) float64, rMin, rMax float64,
	order int, span float64) []float64 {
	nodes, weights := quadratureGrid([]float64{rMin, rMax}, order, span)
	out := make([]float64, kt.nch)
	for j := 0; j < kt.nch; j++ {
		s := 0.0
		for k := range nodes {
			r := nodes[k]
			q := 3.0 / (4.0 * csPi * r * r * r * r)
			s += weights[k] * ktableKernel(kt, j, r) * q * nf(r)
		}
		out[j] = s
	}
	return out
}

// ---------------- RNG (xorshift64*) ----------------
var rngState uint64

func rngNext() uint64 {
	rngState ^= rngState >> 12
	rngState ^= rngState << 25
	rngState ^= rngState >> 27
	return rngState * 2685821657736338717
}

func rngUniform() float64 {
	return float64(rngNext()>>11) * (1.0 / 9007199254740992.0)
}

func rngNormal() float64 {
	u1 := rngUniform()
	u2 := rngUniform()
	if u1 <= 0 {
		u1 = 1e-300
	}
	return math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*csPi*u2)
}

// Main — точка входа; fatalError печатается как "error: ...".
func Main(argv []string) (code int) {
	code = 0
	defer func() {
		if r := recover(); r != nil {
			if fe, ok := r.(fatalError); ok {
				fmt.Fprintf(os.Stderr, "error: %s\n", fe.msg)
				code = 1
				return
			}
			panic(r)
		}
	}()

	a, err := ParseArgs(argv)
	if errors.Is(err, errHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\nTry 'csolve --help' for more "+
			"information.\n", err)
		return 1
	}
	run(a)
	return 0
}

func run(a *Args) {
	timeEnabled = os.Getenv("CSOLVE_TIME") != ""
	tAll := nowSec()

	if a.Best < 1 {
		a.Best = 1
	}
	slices.Sort(a.Degrees)
	a.Degrees = slices.Compact(a.Degrees)

	if a.LidarRatioCons && a.Batch == "" {
		fmt.Printf("[warning] --lidar-ratio-cons only has an effect in batch " +
			"mode (--batch); ignored\n")
	}

	if a.Batch != "" {
		runBatch(a)
		phase("total", tAll)
		return
	}

	// ---- каналы и измерения ----
	var baseCh []channel
	var g1 []float64
	demo := a.Data == ""
	trueRe, trueIm := 1.45, -0.005

	if !demo {
		baseCh, g1 = loadJSONInput(a.Data, &trueRe, &trueIm)
		fmt.Printf("data: %s; channels: %d\n", a.Data, len(baseCh))
	} else {
		if a.HasMReal {
			trueRe = a.MReal
		}
		if a.HasMImag {
			trueIm = a.MImag
		}
		bwl := []float64{355.0, 532.0, 1064.0}
		awl := []float64{355.0, 532.0}
		for _, wl := range bwl {
			baseCh = append(baseCh, channel{chBeta, wl * 1e-3, nil,
				tableRefWlUm, 1.0})
		}
		for _, wl := range awl {
			baseCh = append(baseCh, channel{chAlpha, wl * 1e-3, nil,
				tableRefWlUm, 1.0})
		}
		fmt.Printf("demo on synthetic data\n")
	}
	nch := len(baseCh)

	for j := range baseCh {
		fmt.Printf("  channel: %s\n", baseCh[j].name())
	}
	fmt.Printf("input units: β [1/(m·sr)], α [1/m]\n")
	if a.NoZeroBoundary {
		fmt.Printf("boundary conditions n(r0)=n(r1)=0 (each elementary " +
			"solution): disabled (--no-zero-boundary)\n")
	} else {
		fmt.Printf("boundary conditions n(r0)=n(r1)=0 (each elementary " +
			"solution): enabled\n")
	}
	if a.NoSmooth {
		fmt.Printf("smoothing of the mean over best solutions: disabled " +
			"(--no-smooth)\n")
	} else {
		fmt.Printf("smoothing of the mean over best solutions: enabled "+
			"(rel-lam=%g)\n", a.SmoothLam)
	}
	nb := a.NBasis
	if nb <= 0 {
		nb = nch
	}
	if !a.NoZeroBoundary {
		fmt.Printf("basis functions per window: %d (unknowns with BC: %d)\n",
			nb, nb-2)
	} else {
		fmt.Printf("basis functions per window: %d\n", nb)
	}

	// ---- сетка m из таблиц ----
	src := a.tableSource()
	mre, mim := tableMGrid(src)
	nm := len(mre)
	if nm == 0 {
		fatal("refractive index grid is empty")
	}
	if demo {
		resolveDemoM(&trueRe, &trueIm, a, mre, mim)
	}

	fmt.Printf("refractive index m (initial/true) = %g %+gj\n", trueRe, trueIm)
	fmt.Printf("refractive index grid (scan):\n")
	for i := 0; i < nm; i++ {
		fmt.Printf("  m = %.3f%+.3fj\n", mre[i], mim[i])
	}

	// ---- покрытие таблиц и построение ядер ----
	cbackScale := 1.0
	if a.TableCback4pi {
		cbackScale = 1.0 / (4.0 * csPi)
	}
	fmt.Printf("building kernel tables (%d m values)...\n", nm)
	capmin := math.Inf(1)
	for i := 0; i < nm; i++ {
		chs := loadTableChannels(src, mre[i], mim[i], baseCh,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		if chs.cap < capmin {
			capmin = chs.cap
		}
	}
	r1MaxEff := math.Min(a.R1Max, capmin)
	if r1MaxEff < a.R1Min {
		fatal("table coverage in x limits r1_max = %g µm < "+
			"r1_min = %g — decrease --r1-min, choose another --type, "+
			"or extend the tables (%s)", r1MaxEff, a.R1Min, src.describe())
	}

	fmt.Printf("tabular kernel mode (external cross sections), %s\n",
		src.describe())
	fmt.Printf("  table λ_ref = %g µm (cross sections rescaled as (λ/λ_ref)²)\n",
		a.TableRefWl)
	if a.TableCback4pi {
		fmt.Printf("  Cback convention = 4pi\n")
	} else {
		fmt.Printf("  Cback convention = dsigma\n")
	}
	{
		c0 := loadTableChannels(src, mre[0], mim[0], baseCh,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		for j := 0; j < nch; j++ {
			fmt.Printf("  %-14s x_max = %g, r_max = %.3g µm\n",
				c0.ch[j].name(), c0.ch[j].tab.xMax,
				c0.ch[j].tab.xMax*c0.ch[j].wlUm/(2.0*csPi))
		}
	}
	fmt.Printf("  effective r1_max = %g µm (requested: %g) — windows and "+
		"r grid limited\n", r1MaxEff, a.R1Max)

	kts := make([]*ktable, nm)
	tPhase := nowSec()
	for i := 0; i < nm; i++ {
		chs := loadTableChannels(src, mre[i], mim[i], baseCh,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		kts[i] = ktableBuild(chs, a.R0Min, r1MaxEff, a.DxMax)
	}
	fmt.Printf("  table: %d points in r per m\n", kts[0].n)
	phase("kernels", tPhase)

	// ядра S11/S22 для линейной деполяризации (независимо от каналов)
	tPhase = nowSec()
	ds := depolSetBuild(src, mre, mim, a.TableRefWl, a.R0Min, r1MaxEff,
		a.DxMax, a.TableExtrapolate)
	depolSetReport(ds, src, nm)
	phase("depolarization", tPhase)

	// ---- измерения ----
	var nTrueFn func(float64) float64
	if demo {
		idx := mIndexOf(mre, mim, trueRe, trueIm)
		var tk *ktable
		if idx >= 0 {
			tk = kts[idx]
		} else {
			fmt.Printf("  building table for demo true m: m = %g%+gj\n",
				trueRe, trueIm)
			chs := loadTableChannels(src, trueRe, trueIm, baseCh,
				a.TableExtrapolate, a.TableRefWl, cbackScale)
			tk = ktableBuild(chs, a.R0Min, r1MaxEff, a.DxMax)
		}
		g1 = forwardMeasurements(tk, demoTrueDistribution, a.R0Min,
			r1MaxEff, 24, 0.05)
		if a.Noise > 0 {
			if a.Seed != 0 {
				rngState = uint64(a.Seed)*6364136223846793005 +
					1442695040888963407
			} else {
				rngState = 88172645463325252
			}
			for j := range g1 {
				g1[j] *= 1.0 + a.Noise*rngNormal()
			}
		}
		var b strings.Builder
		b.WriteString("measured coefficients g1: [")
		for j := range g1 {
			if j > 0 {
				b.WriteByte(' ')
			}
			fmt.Fprintf(&b, "%.6g", g1[j])
		}
		b.WriteString("]\n")
		fmt.Print(b.String())
		nTrueFn = demoTrueDistribution
	}

	// ---- перебор ----
	r0Grid := geomspace(a.R0Min, a.R0Max, a.NR0)
	r1Grid := geomspace(a.R1Min, r1MaxEff, a.NR1)

	fmt.Printf("scan (m, r0, r1, degree)...\n")
	tPhase = nowSec()
	design := designBuild(baseCh, nch, kts, mre, mim, a.Degrees, r0Grid,
		r1Grid, a.MinRatio, a.NBasis, !a.NoZeroBoundary, a.QuadOrder,
		a.LogSpan)
	phase("design", tPhase)

	tPhase = nowSec()
	sols := scanVariants(design, g1, nch, a.GammaSelect, a.GammaRel,
		a.GammaMin, a.GammaMax, a.NGamma, a.Nonneg, !a.Absolute)
	phase("scan", tPhase)
	if sols.n == 0 {
		fatal("no valid window [r0, r1] — check the bounds and min-ratio")
	}

	tPhase = nowSec()
	prof := profileFinalize(a, baseCh, kts, mre, mim, sols, design.geom,
		a.R0Min, r1MaxEff, ds)
	phase("output", tPhase)

	reportSummary(a, sols, g1, baseCh, prof.nAvg, prof.gFit, prof.rGrid,
		prof.mRe, prof.mIm)
	reportMoments(prof.moments)
	reportDepolarization(prof)

	modeName := "auto"
	if a.Modes == 1 {
		modeName = "fit"
	} else if a.Modes == 2 {
		modeName = "stat"
	}
	reportModes(prof.modes, a.RSplit, modeName)

	if a.JSON != "" {
		writeJSON(a.JSON, sols, prof.rGrid, prof.nAvg, prof.nAvgRaw, baseCh,
			g1, prof.gFit, prof.mRe, prof.mIm, prof.moments, prof.modes,
			prof.deltaLwl, prof.deltaL)
		fmt.Printf("results saved: %s\n", a.JSON)
	}

	if !a.NoPlot {
		var nTrueVals []float64
		if nTrueFn != nil {
			nTrueVals = make([]float64, prof.nr)
			for i := 0; i < prof.nr; i++ {
				nTrueVals[i] = nTrueFn(prof.rGrid[i])
			}
		}
		po := plotOpts{a.PlotFormat, a.PlotDpi, a.PlotFontSize}
		outPath := a.Out
		if outPath == "" {
			outPath = "result." + plotFormatExt(a.PlotFormat)
		}
		plotResults(&po, outPath, prof.rGrid, nTrueVals, prof.nAvg,
			prof.nAvgRaw, prof.nBest, prof.nBestCount, baseCh, g1,
			prof.gFit, prof.mRe, prof.mIm)
	}

	phase("total", tAll)
}
