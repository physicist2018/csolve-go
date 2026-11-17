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

// ---------------- демо ----------------

// sameToDecimals — совпадение чисел с точностью до decimals знаков после
// запятой (повторяет сравнение печатных форм "%.Nf" в C-версии).
func sameToDecimals(a, b float64, decimals int) bool {
	tol := 0.5 * math.Pow(10, -float64(decimals))
	return math.Abs(a-b) <= tol
}

// resolveDemoM — приведение запрошенного m к ближайшему значению сетки таблиц.
func resolveDemoM(trueRe, trueIm float64, a *Args, mre, mim []float64,
	c *console) (float64, float64, error) {
	var cand []int
	for i := range mre {
		ok := true
		if a.HasMReal && !sameToDecimals(a.MReal, mre[i], 2) {
			ok = false
		}
		if ok && a.HasMImag && !sameToDecimals(a.MImag, mim[i], 5) {
			ok = false
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
		return 0, 0, fmt.Errorf("demo true m = %g%+gj is missing from the tabular m grid: %s — "+
			"set --m-real/--m-imag from this list", trueRe, trueIm,
			strings.Join(parts, ", "))
	}
	best := cand[0]
	for _, i := range cand[1:] {
		d1 := math.Abs(mre[i]-trueRe) + math.Abs(mim[i]-trueIm)
		d2 := math.Abs(mre[best]-trueRe) + math.Abs(mim[best]-trueIm)
		if d1 < d2 {
			best = i
		}
	}
	if !sameToDecimals(mre[best], trueRe, 2) || !sameToDecimals(mim[best], trueIm, 5) {
		c.printf("[tabular mode] demo true m: using %g%+gj (closest to "+
			"%g%+gj in tables)\n", mre[best], mim[best], trueRe, trueIm)
	}
	return mre[best], mim[best], nil
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
			q := 3.0 / (4.0 * math.Pi * r * r * r * r)
			s += weights[k] * ktableKernel(kt, j, r) * q * nf(r)
		}
		out[j] = s
	}
	return out
}

// ---------------- RNG (xorshift64*) ----------------

// xorshift — детерминированный ГПСЧ демо-шума; состояние локальное, чтобы
// расчёт был воспроизводим и не зависел от глобального состояния.
type xorshift struct{ state uint64 }

// newXorshift — генератор с зерном как в C-версии (0 — фиксированное зерно).
func newXorshift(seed int) *xorshift {
	if seed == 0 {
		return &xorshift{state: 88172645463325252}
	}
	return &xorshift{state: uint64(seed)*6364136223846793005 + 1442695040888963407}
}

func (r *xorshift) next() uint64 {
	r.state ^= r.state >> 12
	r.state ^= r.state << 25
	r.state ^= r.state >> 27
	return r.state * 2685821657736338717
}

func (r *xorshift) uniform() float64 {
	return float64(r.next()>>11) * (1.0 / 9007199254740992.0)
}

func (r *xorshift) normal() float64 {
	u1 := r.uniform()
	u2 := r.uniform()
	if u1 <= 0 {
		u1 = 1e-300
	}
	return math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
}

// Main — точка входа; ошибки печатаются как "error: ...".
func Main(argv []string) int {
	a, err := ParseArgs(argv)
	if errors.Is(err, errHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\nTry 'csolve --help' for more "+
			"information.\n", err)
		return 1
	}
	if err := run(a); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func run(a *Args) error {
	c := newConsole(os.Stdout, os.Stderr, os.Getenv("CSOLVE_TIME") != "")
	tAll := nowSec()

	slices.Sort(a.Degrees)
	a.Degrees = slices.Compact(a.Degrees)

	if a.LidarRatioCons && a.Batch == "" {
		c.printf("[warning] --lidar-ratio-cons only has an effect in batch " +
			"mode (--batch); ignored\n")
	}

	if a.Batch != "" {
		err := runBatch(a, c)
		c.phase("total", tAll)
		return err
	}

	// ---- каналы и измерения ----
	var baseCh []channel
	var g1 []float64
	var depWl, depVal []float64
	demo := a.Data == ""
	trueRe, trueIm := 1.45, -0.005

	if !demo {
		in, err := loadJSONInput(a.Data)
		if err != nil {
			return err
		}
		baseCh, g1, depWl, depVal = in.channels, in.g1, in.depWl, in.depVal
		if in.mRe != nil {
			trueRe = *in.mRe
		}
		if in.mIm != nil {
			trueIm = *in.mIm
		}
		c.printf("data: %s; channels: %d\n", a.Data, len(baseCh))
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
			baseCh = append(baseCh, channel{
				kind:       chBeta,
				wlUm:       wl * 1e-3,
				refWlUm:    tableRefWlUm,
				cbackScale: 1.0,
			})
		}
		for _, wl := range awl {
			baseCh = append(baseCh, channel{
				kind:       chAlpha,
				wlUm:       wl * 1e-3,
				refWlUm:    tableRefWlUm,
				cbackScale: 1.0,
			})
		}
		c.printf("demo on synthetic data\n")
	}
	nch := len(baseCh)

	if a.DepolRatioCons && len(depVal) == 0 {
		if demo {
			return errors.New("option --depol-ratio-cons requires measured " +
				"depolarization, but demo data has none — provide --data JSON " +
				"with a \"depolarization\" group")
		}
		return errors.New("option --depol-ratio-cons requires measured depolarization: " +
			"add a \"depolarization\" group with wavelengths_nm/coefficients " +
			"to the input JSON")
	}

	for j := range baseCh {
		c.printf("  channel: %s\n", baseCh[j].name())
	}
	c.printf("input units: β [1/(m·sr)], α [1/m]\n")
	if a.NoZeroBoundary {
		c.printf("boundary conditions n(r0)=n(r1)=0 (each elementary " +
			"solution): disabled (--no-zero-boundary)\n")
	} else {
		c.printf("boundary conditions n(r0)=n(r1)=0 (each elementary " +
			"solution): enabled\n")
	}
	if a.NoSmooth {
		c.printf("smoothing of the mean over best solutions: disabled " +
			"(--no-smooth)\n")
	} else {
		c.printf("smoothing of the mean over best solutions: enabled "+
			"(rel-lam=%g)\n", a.SmoothLam)
	}
	nBasis := a.NBasis
	if nBasis <= 0 {
		nBasis = nch
	}
	if !a.NoZeroBoundary {
		c.printf("basis functions per window: %d (unknowns with BC: %d)\n",
			nBasis, nBasis-2)
	} else {
		c.printf("basis functions per window: %d\n", nBasis)
	}

	// ---- сетка m из таблиц ----
	src, err := a.tableSource(c)
	if err != nil {
		return err
	}
	mre, mim, err := tableMGrid(src)
	if err != nil {
		return err
	}
	nm := len(mre)
	if nm == 0 {
		return errors.New("refractive index grid is empty")
	}
	if demo {
		trueRe, trueIm, err = resolveDemoM(trueRe, trueIm, a, mre, mim, c)
		if err != nil {
			return err
		}
	}

	c.printf("refractive index m (initial/true) = %g %+gj\n", trueRe, trueIm)
	c.printf("refractive index grid (scan):\n")
	for i := 0; i < nm; i++ {
		c.printf("  m = %.3f%+.3fj\n", mre[i], mim[i])
	}

	// ---- покрытие таблиц и построение ядер ----
	cbackScale := 1.0
	if a.TableCback4pi {
		cbackScale = 1.0 / (4.0 * math.Pi)
	}
	c.printf("building kernel tables (%d m values)...\n", nm)
	capmin := math.Inf(1)
	for i := 0; i < nm; i++ {
		chs, err := loadTableChannels(src, mre[i], mim[i], baseCh,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		if err != nil {
			return err
		}
		if chs.cap < capmin {
			capmin = chs.cap
		}
	}
	r1MaxEff := math.Min(a.R1Max, capmin)
	if r1MaxEff < a.R1Min {
		return fmt.Errorf("table coverage in x limits r1_max = %g µm < "+
			"r1_min = %g — decrease --r1-min, choose another --type, "+
			"or extend the tables (%s)", r1MaxEff, a.R1Min, src.describe())
	}

	c.printf("tabular kernel mode (external cross sections), %s\n",
		src.describe())
	c.printf("  table λ_ref = %g µm (cross sections rescaled as (λ/λ_ref)²)\n",
		a.TableRefWl)
	if a.TableCback4pi {
		c.printf("  Cback convention = 4pi\n")
	} else {
		c.printf("  Cback convention = dsigma\n")
	}
	{
		c0, err := loadTableChannels(src, mre[0], mim[0], baseCh,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		if err != nil {
			return err
		}
		for j := 0; j < nch; j++ {
			c.printf("  %-14s x_max = %g, r_max = %.3g µm\n",
				c0.ch[j].name(), c0.ch[j].tab.xMax,
				c0.ch[j].tab.xMax*c0.ch[j].wlUm/(2.0*math.Pi))
		}
	}
	c.printf("  effective r1_max = %g µm (requested: %g) — windows and "+
		"r grid limited\n", r1MaxEff, a.R1Max)

	kts := make([]*ktable, nm)
	tPhase := nowSec()
	for i := 0; i < nm; i++ {
		chs, err := loadTableChannels(src, mre[i], mim[i], baseCh,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		if err != nil {
			return err
		}
		kt, err := ktableBuild(chs, a.R0Min, r1MaxEff, a.DxMax)
		if err != nil {
			return err
		}
		kts[i] = kt
	}
	c.printf("  table: %d points in r per m\n", kts[0].n)
	c.phase("kernels", tPhase)

	// ядра S11/S22 для линейной деполяризации (независимо от каналов)
	tPhase = nowSec()
	ds, err := depolSetBuild(src, mre, mim, a.TableRefWl, a.R0Min, r1MaxEff,
		a.DxMax, a.TableExtrapolate)
	if err != nil {
		return err
	}
	depolSetReport(ds, src, nm, c)
	c.phase("depolarization", tPhase)

	// измеренное δ_l для ограничения --depol-ratio-cons
	var depIw, depCol []int
	if a.DepolRatioCons {
		if !depolSetAvailable(ds) {
			c.warnf("[warning] --depol-ratio-cons: tables have no " +
				"valid S11/S22 columns; constraint is not applied\n")
		} else {
			depIw, depCol = depolColumnMatch(ds, depWl, "input data", c)
			switch {
			case len(depIw) == 0:
				c.warnf("[warning] --depol-ratio-cons: no measured " +
					"depolarization matching S11/S22 wavelengths; constraint is " +
					"not applied\n")
			case !(a.DepolRatioW > 0):
				c.printf("depolarization ratio constraint δ_l: disabled " +
					"(--depol-ratio-w 0)\n")
			default:
				var sb strings.Builder
				for k, c := range depCol {
					if k > 0 {
						sb.WriteString(", ")
					}
					fmt.Fprintf(&sb, "%.6g nm = %g %%", ds.wl[depIw[k]]*1e3,
						depVal[c])
				}
				c.printf("depolarization ratio constraint δ_l: enabled "+
					"(weight %g, mode %s; measured %s)\n", a.DepolRatioW,
					a.DepolRatioMode, sb.String())
			}
		}
	}

	// ---- измерения ----
	var nTrueFn func(float64) float64
	if demo {
		idx := mIndex(mre, mim, trueRe, trueIm)
		var tk *ktable
		if idx >= 0 {
			tk = kts[idx]
		} else {
			c.printf("  building table for demo true m: m = %g%+gj\n",
				trueRe, trueIm)
			chs, err := loadTableChannels(src, trueRe, trueIm, baseCh,
				a.TableExtrapolate, a.TableRefWl, cbackScale)
			if err != nil {
				return err
			}
			tk, err = ktableBuild(chs, a.R0Min, r1MaxEff, a.DxMax)
			if err != nil {
				return err
			}
		}
		g1 = forwardMeasurements(tk, demoTrueDistribution, a.R0Min,
			r1MaxEff, 24, 0.05)
		if a.Noise > 0 {
			rng := newXorshift(a.Seed)
			for j := range g1 {
				g1[j] *= 1.0 + a.Noise*rng.normal()
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
		c.print(b.String())
		nTrueFn = demoTrueDistribution
	}

	// ---- перебор ----
	r0Grid := geomspace(a.R0Min, a.R0Max, a.NR0)
	r1Grid := geomspace(a.R1Min, r1MaxEff, a.NR1)

	c.printf("scan (m, r0, r1, degree)...\n")
	tPhase = nowSec()
	design, err := designBuild(baseCh, nch, kts, mre, mim, a.Degrees, r0Grid,
		r1Grid, a.MinRatio, a.NBasis, !a.NoZeroBoundary, a.QuadOrder,
		a.LogSpan)
	if err != nil {
		return err
	}
	c.phase("design", tPhase)

	// ограничение по измеренному δ_l: строка в функционале или ранжирование
	var depProj *depolProj
	var depTargets []float64
	if a.DepolRatioCons && len(depIw) > 0 && a.DepolRatioW > 0 {
		if proj := depolProjBuild(ds, design, depIw); proj != nil {
			depProj = proj
			depTargets = make([]float64, len(depCol))
			for k, c := range depCol {
				depTargets[k] = depVal[c]
			}
		}
	}
	var dc *depolCons
	if depProj != nil && a.DepolRatioMode == depolModeFunc {
		dc = &depolCons{proj: depProj, w: a.DepolRatioW, targets: depTargets}
	}

	tPhase = nowSec()
	sols, err := scanVariants(design, g1, nch, a.GammaSelect, a.GammaRel,
		a.GammaMin, a.GammaMax, a.NGamma, a.Nonneg, !a.Absolute, dc, c)
	if err != nil {
		return err
	}
	c.phase("scan", tPhase)
	if sols.n == 0 {
		return errors.New("no valid window [r0, r1] — check the bounds and min-ratio")
	}
	reportEnsembleSize(a, sols.n, c)
	if depProj != nil && a.DepolRatioMode == depolModeRank {
		rankByDepol(sols, depProj, mre, mim, depTargets, a.DepolRatioW)
	}

	tPhase = nowSec()
	prof, err := profileFinalize(a, baseCh, kts, mre, mim, sols, design.geom,
		a.R0Min, r1MaxEff, ds)
	if err != nil {
		return err
	}
	c.phase("output", tPhase)

	reportSummary(a, sols, g1, baseCh, prof.nAvg, prof.gFit, prof.rGrid,
		prof.mRe, prof.mIm, c)
	reportMoments(prof.moments, c)
	reportDepolarization(prof, c)

	reportModes(prof.modes, a.RSplit, a.Modes, c)

	// ---- выходные файлы ----
	var plotPath string
	var plotInDir bool
	if !a.NoPlot {
		name := a.Out
		if name == "" {
			name = "result." + plotFormatExt(a.PlotFormat)
		}
		plotPath, plotInDir = resolveOutPath(a.OutDir, name)
	}
	jsonPath, jsonInDir := resolveOutPath(a.OutDir, a.JSON)
	if plotInDir || jsonInDir {
		if err := mkdirP(a.OutDir); err != nil {
			return err
		}
	}

	if a.JSON != "" {
		if err := writeJSON(jsonPath, sols, prof.rGrid, prof.nAvg, prof.nAvgRaw, baseCh,
			g1, prof.gFit, prof.mRe, prof.mIm, prof.moments, prof.modes,
			prof.deltaLwl, prof.deltaL); err != nil {
			return err
		}
		c.printf("results saved: %s\n", jsonPath)
	}

	if !a.NoPlot {
		var nTrueVals []float64
		if nTrueFn != nil {
			nTrueVals = make([]float64, prof.nr)
			for i := 0; i < prof.nr; i++ {
				nTrueVals[i] = nTrueFn(prof.rGrid[i])
			}
		}
		po := plotOpts{format: a.PlotFormat, dpi: a.PlotDpi,
			fontSize: a.PlotFontSize, backend: a.Plotting, con: c}
		plotResults(&po, plotPath, prof.rGrid, nTrueVals, prof.nAvg,
			prof.nAvgRaw, prof.nBest, prof.nBestCount, baseCh, g1,
			prof.gFit, prof.mRe, prof.mIm)
	}

	c.phase("total", tAll)
	return nil
}
