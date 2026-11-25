package solve

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// runInput — каналы и измерения одиночного прогона (JSON или демо).
type runInput struct {
	channels []channel
	g1       []float64
	depWl    []float64 // длины волн измеренной δ_l, µm
	depVal   []float64 // измеренная δ_l, %
	demo     bool
	trueRe   float64 // истинный Re(m) (для демо)
	trueIm   float64 // истинный Im(m) (для демо)
}

// loadRunInput — чтение измерений из JSON или синтетические демо-каналы и
// отчёт о входных данных.
func loadRunInput(a *Args, c *console) (*runInput, error) {
	in := &runInput{demo: a.Data == "", trueRe: 1.45, trueIm: -0.005}
	if !in.demo {
		mi, err := loadJSONInput(a.Data)
		if err != nil {
			return nil, err
		}
		in.channels, in.g1 = mi.channels, mi.g1
		in.depWl, in.depVal = mi.depWl, mi.depVal
		if mi.mRe != nil {
			in.trueRe = *mi.mRe
		}
		if mi.mIm != nil {
			in.trueIm = *mi.mIm
		}
		c.printf("data: %s; channels: %d\n", a.Data, len(in.channels))
	} else {
		if a.HasMReal {
			in.trueRe = a.MReal
		}
		if a.HasMImag {
			in.trueIm = a.MImag
		}
		for _, wl := range []float64{355.0, 532.0, 1064.0} {
			in.channels = append(in.channels, channel{
				kind:       chBeta,
				wlUm:       wl * 1e-3,
				refWlUm:    tableRefWlUm,
				cbackScale: 1.0,
			})
		}
		for _, wl := range []float64{355.0, 532.0} {
			in.channels = append(in.channels, channel{
				kind:       chAlpha,
				wlUm:       wl * 1e-3,
				refWlUm:    tableRefWlUm,
				cbackScale: 1.0,
			})
		}
		c.printf("demo on synthetic data\n")
	}

	if a.DepolRatioCons && len(in.depVal) == 0 && a.DepolRatioExplicit {
		if in.demo {
			return nil, errors.New("option --depol-ratio-cons requires measured " +
				"depolarization, but demo data has none — provide --data JSON " +
				"with a \"depolarization\" group")
		}
		return nil, errors.New("option --depol-ratio-cons requires measured depolarization: " +
			"add a \"depolarization\" group with wavelengths_nm/coefficients " +
			"to the input JSON")
	}

	for j := range in.channels {
		c.printf("  channel: %s\n", in.channels[j].name())
	}
	c.printf("input units: β [1/(m·sr)], α [1/m]\n")
	if a.NoSmooth {
		c.printf("smoothing of the mean over best solutions: disabled " +
			"(--no-smooth)\n")
	} else {
		c.printf("smoothing of the mean over best solutions: enabled "+
			"(rel-lam=%g)\n", a.SmoothLam)
	}
	c.printf("kernel order: %d (%s; result converted to dV/dln r)\n",
		normalizeKernelOrder(a.KernelOrder), kernelOrderLabel(a.KernelOrder))
	if a.Method == methodMonotone {
		if normalizeKernelOrder(a.KernelOrder) == kernelOrderVolume {
			c.printf("method: monotone functions — V(r) = ∫ dV/dln r′ dln r′ is " +
				"reconstructed from non-negative bin increments\n")
		} else {
			c.printf("method: monotone functions — %s is reconstructed from "+
				"non-negative bin increments, then converted to dV/dln r\n",
				kernelOrderLabel(a.KernelOrder))
		}
		c.printf("size bins per window: %d (regularization order %d)\n",
			binsFor(a.VBins, len(in.channels)), a.RegOrder)
		if a.VBins == 0 {
			c.printf("  (--v-bins 0: automatic count max(2·channels, 8), "+
				"at most %d)\n", vBinsMax)
		}
		if a.NoZeroBoundary {
			c.warnf("[warning] --no-zero-boundary has no effect with " +
				"--method monotone: V(r0) = 0 is kept and n at the window " +
				"edges is not forced to zero\n")
		}
	} else {
		if a.NoZeroBoundary {
			c.printf("boundary conditions n(r0)=n(r1)=0 (each elementary " +
				"solution): disabled (--no-zero-boundary)\n")
		} else {
			c.printf("boundary conditions n(r0)=n(r1)=0 (each elementary " +
				"solution): enabled\n")
		}
		nBasis := a.NBasis
		if nBasis <= 0 {
			nBasis = len(in.channels)
		}
		if !a.NoZeroBoundary {
			c.printf("basis functions per window: %d (unknowns with BC: %d)\n",
				nBasis, nBasis-2)
		} else {
			c.printf("basis functions per window: %d\n", nBasis)
		}
	}
	return in, nil
}

// runSetup — таблицы, ядра и δ_l-ядра одиночного прогона.
type runSetup struct {
	src       *tableSource
	mre, mim  []float64
	kts       []*ktable
	ds        *depolSet
	r1MaxEff  float64
	cbackScal float64
	mRe, mIm  float64 // истинные m (для демо — приведённые к сетке таблиц)
}

// prepareRun — сетка m, покрытие таблиц, ядра Cext/Cback и ядра S11/S22.
func prepareRun(a *Args, c *console, in *runInput) (*runSetup, error) {
	st := &runSetup{mRe: in.trueRe, mIm: in.trueIm, cbackScal: 1.0}
	if a.TableCback4pi {
		st.cbackScal = 1.0 / (4.0 * math.Pi)
	}

	src, err := a.tableSource(c)
	if err != nil {
		return nil, err
	}
	st.src = src
	st.mre, st.mim, err = tableMGrid(src)
	if err != nil {
		return nil, err
	}
	nm := len(st.mre)
	if nm == 0 {
		return nil, errors.New("refractive index grid is empty")
	}
	if in.demo {
		st.mRe, st.mIm, err = resolveDemoM(in.trueRe, in.trueIm, a, st.mre,
			st.mim, c)
		if err != nil {
			return nil, err
		}
	}

	c.printf("refractive index m (initial/true) = %g %+gj\n", st.mRe, st.mIm)
	c.printf("refractive index grid (scan):\n")
	for i := 0; i < nm; i++ {
		c.printf("  m = %.3f%+.3fj\n", st.mre[i], st.mim[i])
	}

	c.printf("building kernel tables (%d m values)...\n", nm)
	capmin := math.Inf(1)
	for i := 0; i < nm; i++ {
		chs, err := loadTableChannels(src, st.mre[i], st.mim[i], in.channels,
			a.TableExtrapolate, a.TableRefWl, st.cbackScal)
		if err != nil {
			return nil, err
		}
		if chs.cap < capmin {
			capmin = chs.cap
		}
	}
	st.r1MaxEff = math.Min(a.R1Max, capmin)
	if st.r1MaxEff < a.R1Min {
		return nil, fmt.Errorf("table coverage in x limits r1_max = %g µm < "+
			"r1_min = %g — decrease --r1-min, choose another --type, "+
			"or extend the tables (%s)", st.r1MaxEff, a.R1Min, src.describe())
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
		c0, err := loadTableChannels(src, st.mre[0], st.mim[0], in.channels,
			a.TableExtrapolate, a.TableRefWl, st.cbackScal)
		if err != nil {
			return nil, err
		}
		for j := 0; j < len(in.channels); j++ {
			c.printf("  %-14s x_max = %g, r_max = %.3g µm\n",
				c0.ch[j].name(), c0.ch[j].tab.xMax,
				c0.ch[j].tab.xMax*c0.ch[j].wlUm/(2.0*math.Pi))
		}
	}
	c.printf("  effective r1_max = %g µm (requested: %g) — windows and "+
		"r grid limited\n", st.r1MaxEff, a.R1Max)

	st.kts = make([]*ktable, nm)
	tPhase := nowSec()
	for i := 0; i < nm; i++ {
		chs, err := loadTableChannels(src, st.mre[i], st.mim[i], in.channels,
			a.TableExtrapolate, a.TableRefWl, st.cbackScal)
		if err != nil {
			return nil, err
		}
		kt, err := ktableBuild(chs, a.R0Min, st.r1MaxEff, a.DxMax)
		if err != nil {
			return nil, err
		}
		st.kts[i] = kt
	}
	c.printf("  table: %d points in r per m\n", st.kts[0].n)
	c.phase("kernels", tPhase)

	// ядра S11/S22 для линейной деполяризации (независимо от каналов);
	// считаем δ_l на всех длинах волн входных данных
	tPhase = nowSec()
	depWls := make([]float64, 0, len(in.channels)+len(in.depWl))
	for _, ch := range in.channels {
		depWls = append(depWls, ch.wlUm)
	}
	depWls = append(depWls, in.depWl...)
	st.ds, err = depolSetBuild(src, st.mre, st.mim, depWls, a.TableRefWl,
		a.R0Min, st.r1MaxEff, a.DxMax, a.TableExtrapolate)
	if err != nil {
		return nil, err
	}
	depolSetReport(st.ds, src, nm, c)
	c.phase("depolarization", tPhase)
	return st, nil
}

// demoMeasurements — синтетические g1 по демонстрационному распределению.
func demoMeasurements(a *Args, c *console, st *runSetup,
	baseCh []channel) ([]float64, error) {
	idx := mIndex(st.mre, st.mim, st.mRe, st.mIm)
	var tk *ktable
	if idx >= 0 {
		tk = st.kts[idx]
	} else {
		c.printf("  building table for demo true m: m = %g%+gj\n",
			st.mRe, st.mIm)
		chs, err := loadTableChannels(st.src, st.mRe, st.mIm, baseCh,
			a.TableExtrapolate, a.TableRefWl, st.cbackScal)
		if err != nil {
			return nil, err
		}
		tk, err = ktableBuild(chs, a.R0Min, st.r1MaxEff, a.DxMax)
		if err != nil {
			return nil, err
		}
	}
	g1 := forwardMeasurements(tk, demoTrueDistribution, a.R0Min,
		st.r1MaxEff, 24, 0.05)
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
	return g1, nil
}

// writeRunOutputs — запись JSON и графика одиночного прогона.
func writeRunOutputs(a *Args, c *console, baseCh []channel, g1 []float64,
	sols *solutions, prof *profile, nTrueFn func(float64) float64) error {
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
		if err := writeJSON(jsonPath, sols, prof.rGrid, prof.nAvg, prof.nAvgRaw,
			prof.nLow, prof.nHigh, prof.vCum, baseCh, g1, prof.gFit, prof.mRe,
			prof.mIm, prof.moments, prof.modes, prof.deltaLwl, prof.deltaL,
			prof.spectra); err != nil {
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
			fontSize: a.PlotFontSize, backend: a.Plotting, con: c,
			bestLimit: a.PlotBestLimit}
		plotResults(&po, plotPath, prof.rGrid, nTrueVals, prof.nAvg,
			prof.nAvgRaw, prof.nLow, prof.nHigh, prof.vCum, prof.nBest,
			prof.nBestCount, baseCh, g1, prof.gFit, prof.mRe, prof.mIm)
		// отдельные графики спектров α(λ), β(λ) и δ_l(λ) рядом с основным
		for _, np := range prof.spectra.namedPanels() {
			sp := spectrumPlotPath(plotPath, np.suffix, a.PlotFormat)
			plotSpectrum(&po, sp, np.panel)
		}
	}
	return nil
}
