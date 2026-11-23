package solve

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func mkdirP(path string) error {
	if err := os.MkdirAll(path, 0777); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return nil
}

func defaultOutDir(batchPath string) (string, error) {
	abs, err := filepath.Abs(batchPath)
	if err != nil {
		return "", errors.New("failed to get current directory")
	}
	return filepath.Dir(abs), nil
}

func openAppend(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for writing: %w", err)
	}
	return f, nil
}

// closeFile закрывает файл и сообщает ошибку записи, отложенную до Close.
func closeFile(f *os.File) error {
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

// writeSpectrum — файл spectrum_<H>.txt: длины волн и mean/low/high для
// α(λ), β(λ) и δ_l(λ). Отсутствующие величины пишутся как nan, чтобы набор
// столбцов был стабильным.
func writeSpectrum(outDir string, h float64, sp *spectra) error {
	if sp == nil {
		return nil
	}
	path := filepath.Join(outDir, fmt.Sprintf("spectrum_%s.txt", formatG6(h)))
	var sb strings.Builder
	fmt.Fprintf(&sb, "# altitude H = %g m\n", h)
	sb.WriteString("# alpha [1/m], beta [1/(m·sr)], delta_l [%]\n")
	sb.WriteString("# measured:")
	writeMeas := func(prefix string, pts []wlPoint) {
		for _, p := range pts {
			fmt.Fprintf(&sb, " %s%g=%s", prefix, p.wlNm, formatG6(p.value))
		}
	}
	if sp.alpha != nil {
		writeMeas("a", sp.alpha.meas)
	}
	if sp.beta != nil {
		writeMeas("b", sp.beta.meas)
	}
	if sp.delta != nil {
		writeMeas("dep", sp.delta.meas)
	}
	sb.WriteString("\n# lambda_nm alpha_mean alpha_low alpha_high beta_mean " +
		"beta_low beta_high delta_mean delta_low delta_high\n")
	val := func(set *spectrumSet, i int) float64 {
		if set == nil || i >= len(set.mean) {
			return math.NaN()
		}
		return set.mean[i]
	}
	valLo := func(set *spectrumSet, i int) float64 {
		if set == nil || i >= len(set.low) {
			return math.NaN()
		}
		return set.low[i]
	}
	valHi := func(set *spectrumSet, i int) float64 {
		if set == nil || i >= len(set.high) {
			return math.NaN()
		}
		return set.high[i]
	}
	for i, wl := range sp.wlNm {
		fmt.Fprintf(&sb, "%s  %s  %s  %s  %s  %s  %s  %s  %s  %s\n",
			formatG6(wl), formatG6(val(sp.alpha, i)),
			formatG6(valLo(sp.alpha, i)), formatG6(valHi(sp.alpha, i)),
			formatG6(val(sp.beta, i)), formatG6(valLo(sp.beta, i)),
			formatG6(valHi(sp.beta, i)), formatG6(val(sp.delta, i)),
			formatG6(valLo(sp.delta, i)), formatG6(valHi(sp.delta, i)))
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("failed to open file for writing: %w", err)
	}
	return nil
}

// writeProfile — запись dist_<H>.txt и накопление строк в сводных файлах.
// calcWl/calcVals — расчётные δ_l по длинам волн, measVals — измеренные
// значения, выровненные с b.depWl.
func writeProfile(outDir string, h float64, b *batchInput, g1 []float64,
	p *profile, calcWl, calcVals, measVals []float64) (float64, error) {
	path := filepath.Join(outDir, fmt.Sprintf("dist_%s.txt", formatG6(h)))
	var sb strings.Builder
	fmt.Fprintf(&sb, "# altitude H = %g m\n", h)
	sb.WriteString("# input data (β [1/(m·sr)], α [1/m]): ")
	for j := 0; j < b.nch; j++ {
		if j > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%s=%s", b.ch[j].name, formatG6(g1[j]))
	}
	sb.WriteString("\n# r, µm  dV/dln r, µm³/cm³  V, µm³/cm³")
	for _, wl := range calcWl {
		fmt.Fprintf(&sb, "  depol_calc_%g", wl*1e3)
	}
	for _, wl := range b.depWl {
		fmt.Fprintf(&sb, "  depol_meas_%g", wl*1e3)
	}
	sb.WriteString("  n_low  n_high")
	sb.WriteString("\n# V — кумулятивный объём ∫ dV/dln r′ dln r′ (монотонный)\n")
	sb.WriteString("# depol — линейная деполяризация, % (nan если нет " +
		"S11/S22 или измерения)\n")
	sb.WriteString("# n_low/n_high — нижняя/верхняя огибающие по лучшим решениям " +
		"(min/max, до сглаживания)\n")
	for i := 0; i < p.nr; i++ {
		nv := p.nAvg[i] / cm3PerM3
		if !isFinite(nv) {
			continue
		}
		vv := math.NaN()
		if i < len(p.vCum) {
			vv = p.vCum[i] / cm3PerM3
		}
		fmt.Fprintf(&sb, "%s  %s  %s", formatG6(p.rGrid[i]), formatG6(nv),
			formatG6(vv))
		for _, v := range calcVals {
			fmt.Fprintf(&sb, "  %s", formatG6(v))
		}
		for _, v := range measVals {
			fmt.Fprintf(&sb, "  %s", formatG6(v))
		}
		lo := math.NaN()
		if i < len(p.nLow) {
			lo = p.nLow[i] / cm3PerM3
		}
		hi := math.NaN()
		if i < len(p.nHigh) {
			hi = p.nHigh[i] / cm3PerM3
		}
		fmt.Fprintf(&sb, "  %s  %s", formatG6(lo), formatG6(hi))
		sb.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return 0, fmt.Errorf("failed to open file for writing: %w", err)
	}

	mf := &p.modes.Fine
	mc := &p.modes.Coarse
	mo := &p.modes.Overall
	fm, err := openAppend(filepath.Join(outDir, "moments.txt"))
	if err != nil {
		return 0, err
	}
	fmt.Fprintf(fm, "%s  %s  %s  %s  %s  %s  %s  %s  %s  %s  %s  %s\n",
		formatG6(h), formatG6(mf.V/cm3PerM3), formatG6(mc.V/cm3PerM3),
		formatG6(p.moments.VolConcUm3PerCm3), formatG6(mf.Rm), formatG6(mc.Rm), formatG6(mo.Rm),
		formatG6(p.moments.REffUm), formatG6(p.moments.NCm3), formatG6(mf.Sigma),
		formatG6(mc.Sigma), formatG6(mo.Sigma))
	if err := closeFile(fm); err != nil {
		return 0, err
	}

	fr, err := openAppend(filepath.Join(outDir, "refractive.txt"))
	if err != nil {
		return 0, err
	}
	fmt.Fprintf(fr, "%s  %s  %s\n", formatG6(h), formatG6(p.mRe), formatG6(p.mIm))
	if err := closeFile(fr); err != nil {
		return 0, err
	}

	fe, err := openAppend(filepath.Join(outDir, "errors.txt"))
	if err != nil {
		return 0, err
	}
	fmt.Fprintf(fe, "%s", formatG6(h))
	errSum := 0.0
	for j := 0; j < b.nch; j++ {
		rel := math.Abs(p.gFit[j]-g1[j]) / g1[j]
		fmt.Fprintf(fe, "  %s", formatG6(rel))
		errSum += rel
	}
	errSum /= float64(b.nch)
	fmt.Fprintf(fe, "  %s\n", formatG6(errSum))
	if err := closeFile(fe); err != nil {
		return 0, err
	}
	return errSum, nil
}

type metrics struct {
	err, V, Vc, Vf []float64
	rmf, rmc, reff []float64
	Nc             []float64
	sgf, sgc, sgo  []float64
	mRe, mIm       []float64
	S              []float64   // nl × nAlt
	dpol           [][]float64 // ndepol x nAlt, δ_l, %
	dpolWl         []float64   // длины волн δ_l, µm
	dpolMeas       [][]float64 // ndep x nAlt, измеренная δ_l, %
	dpolMeasWl     []float64   // длины волн измеренной δ_l, µm
	nAlt, nl       int
}

func emitProfile(a *Args, outDir string, h float64, b *batchInput,
	solveCh []channel, g1 []float64, sols *solutions, kts []*ktable,
	mre, mim []float64, geom []*geom, r1Max float64, common []commonWl,
	nl int, writeLidar bool, met *metrics, hi int, ds *depolSet,
	src *tableSource, c *console) error {
	// измеренные δ_l по всем столбцам dep<nm> для текущей высоты
	var measVals []float64
	if b.ndep > 0 {
		measVals = b.dep[hi*b.ndep : (hi+1)*b.ndep]
	}
	prof, err := profileFinalize(a, solveCh, kts, mre, mim, sols, geom, a.R0Min,
		r1Max, ds, src, g1, b.depWl, measVals)
	if err != nil {
		return err
	}
	// расчётные δ_l на всех длинах волн набора (nan без S11/S22)
	var calcWl []float64
	if ds != nil {
		calcWl = ds.wl
	}
	calcVals := make([]float64, len(calcWl))
	for i, wl := range calcWl {
		calcVals[i] = math.NaN()
		if j := depolWlIndex(prof.deltaLwl, wl); j >= 0 && j < len(prof.deltaL) {
			calcVals[i] = prof.deltaL[j]
		}
	}
	errSum, err := writeProfile(outDir, h, b, g1, prof, calcWl, calcVals, measVals)
	if err != nil {
		return err
	}

	dStr := ""
	if ds != nil && len(ds.wl) > 0 {
		fd, err := openAppend(filepath.Join(outDir, "delpol.txt"))
		if err != nil {
			return err
		}
		fmt.Fprintf(fd, "%s", formatG6(h))
		for i := range ds.wl {
			v := math.NaN()
			if i < len(prof.deltaL) {
				v = prof.deltaL[i]
			}
			fmt.Fprintf(fd, "  %s", formatG6(v))
		}
		for k := 0; k < b.ndep; k++ {
			fmt.Fprintf(fd, "  %s", formatG6(b.dep[hi*b.ndep+k]))
		}
		fmt.Fprint(fd, "\n")
		if err := closeFile(fd); err != nil {
			return err
		}
	}

	if len(prof.deltaL) > 0 {
		var acc strings.Builder
		acc.WriteString("  δ_l = ")
		for i, v := range prof.deltaL {
			if i > 0 {
				acc.WriteString("/")
			}
			if isFinite(v) {
				fmt.Fprintf(&acc, "%.4g", v)
			} else {
				acc.WriteString("nan")
			}
		}
		acc.WriteString(" %")
		dStr = acc.String()
		if met != nil {
			for i := range prof.deltaL {
				if i < len(met.dpol) {
					met.dpol[i][hi] = prof.deltaL[i]
				}
			}
		}
	}

	if met != nil {
		met.err[hi] = errSum
		met.V[hi] = prof.moments.VolConcUm3PerCm3
		met.Vc[hi] = prof.modes.Coarse.V / cm3PerM3
		met.Vf[hi] = prof.modes.Fine.V / cm3PerM3
		met.rmf[hi] = prof.modes.Fine.Rm
		met.rmc[hi] = prof.modes.Coarse.Rm
		met.reff[hi] = prof.moments.REffUm
		met.Nc[hi] = prof.moments.NCm3
		met.sgf[hi] = prof.modes.Fine.Sigma
		met.sgc[hi] = prof.modes.Coarse.Sigma
		met.sgo[hi] = prof.modes.Overall.Sigma
		met.mRe[hi] = prof.mRe
		met.mIm[hi] = prof.mIm
		for k := 0; k < nl && met.nl > 0; k++ {
			met.S[k*met.nAlt+hi] = lidarRatioValue(prof.gFit, common, k)
		}
	}

	if !a.NoPlot {
		po := plotOpts{format: a.PlotFormat, dpi: a.PlotDpi,
			fontSize: a.PlotFontSize, backend: a.Plotting, con: c}
		pp := filepath.Join(outDir,
			fmt.Sprintf("plot_%s.%s", formatG6(h), plotFormatExt(a.PlotFormat)))
		plotProfile(&po, pp, prof.rGrid, prof.nAvg, prof.vCum, solveCh, g1,
			prof.gFit, h)
		if prof.spectra != nil {
			ps := filepath.Join(outDir, fmt.Sprintf("spectrum_%s.%s",
				formatG6(h), plotFormatExt(a.PlotFormat)))
			plotSpectraPanels(&po, ps, prof.spectra, h)
		}
	}

	if prof.spectra != nil {
		if err := writeSpectrum(outDir, h, prof.spectra); err != nil {
			return err
		}
	}

	sStr := ""
	if writeLidar {
		fl, err := openAppend(filepath.Join(outDir, "lidar_ratios.txt"))
		if err != nil {
			return err
		}
		fmt.Fprintf(fl, "%s", formatG6(h))
		for k := 0; k < nl; k++ {
			v := lidarRatioValue(prof.gFit, common, k)
			fmt.Fprintf(fl, "  %s", formatG6(v))
		}
		fmt.Fprint(fl, "\n")
		if err := closeFile(fl); err != nil {
			return err
		}

		var acc strings.Builder
		acc.WriteString("  S = ")
		for k := 0; k < nl; k++ {
			if k > 0 {
				acc.WriteString(", ")
			}
			v := lidarRatioValue(prof.gFit, common, k)
			if isFinite(v) {
				fmt.Fprintf(&acc, "%g", v)
			} else {
				acc.WriteString("nan")
			}
		}
		sStr = acc.String()
	}

	c.printf("  altitude %s m: ⟨m⟩ = %.3f%+.3fj, N = %.3g cm⁻³%s%s\n",
		formatG6(h), prof.mRe, prof.mIm, prof.moments.NCm3, sStr, dStr)
	return nil
}
