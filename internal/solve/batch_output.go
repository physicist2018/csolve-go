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

// distDepol — скалярные δ_l на 532 нм для файла dist_<H>.txt.
type distDepol struct {
	calc    float64
	meas    float64
	hasMeas bool
}

func writeProfile(outDir string, h float64, b *batchInput, g1 []float64,
	p *profile, dd distDepol) (float64, error) {
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
	sb.WriteString("\n# r, µm  dV/dln r, µm³/cm³  depol_calc_532")
	if dd.hasMeas {
		sb.WriteString("  depol_meas_532")
	}
	sb.WriteString("\n# depol — линейная деполяризация на 532 нм, % (nan если нет " +
		"S11/S22 или измерения)\n")
	for i := 0; i < p.nr; i++ {
		nv := p.nAvg[i] / cm3PerM3
		if isFinite(nv) {
			fmt.Fprintf(&sb, "%s  %s  %s", formatG6(p.rGrid[i]), formatG6(nv), formatG6(dd.calc))
			if dd.hasMeas {
				fmt.Fprintf(&sb, "  %s", formatG6(dd.meas))
			}
			sb.WriteString("\n")
		}
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
	S              []float64   // nl x nAlt
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
	c *console) error {
	prof, err := profileFinalize(a, solveCh, kts, mre, mim, sols, geom, a.R0Min,
		r1Max, ds)
	if err != nil {
		return err
	}
	dd := distDepol{calc: math.NaN(), meas: math.NaN()}
	if i := depolWlIndex(prof.deltaLwl, 0.532); i >= 0 && i < len(prof.deltaL) {
		dd.calc = prof.deltaL[i]
	}
	if k := depolWlIndex(b.depWl, 0.532); k >= 0 {
		dd.meas = b.dep[hi*b.ndep+k]
		dd.hasMeas = true
	}
	errSum, err := writeProfile(outDir, h, b, g1, prof, dd)
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
		plotProfile(&po, pp, prof.rGrid, prof.nAvg, solveCh, g1, prof.gFit, h)
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
