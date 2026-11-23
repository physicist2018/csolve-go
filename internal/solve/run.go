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
	out, errOut, rep := reportWriters(a)
	c := newConsole(out, errOut, os.Getenv("CSOLVE_TIME") != "")
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

	in, err := loadRunInput(a, c)
	if err != nil {
		return err
	}
	baseCh, g1 := in.channels, in.g1
	depWl, depVal := in.depWl, in.depVal
	nch := len(baseCh)

	st, err := prepareRun(a, c, in)
	if err != nil {
		return err
	}
	mre, mim := st.mre, st.mim
	kts, ds, r1MaxEff := st.kts, st.ds, st.r1MaxEff

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
	if in.demo {
		if g1, err = demoMeasurements(a, c, st, baseCh); err != nil {
			return err
		}
		nTrueFn = demoTrueDistribution
	}

	// ---- перебор ----
	r0Grid := geomspace(a.R0Min, a.R0Max, a.NR0)
	r1Grid := geomspace(a.R1Min, r1MaxEff, a.NR1)

	c.printf("scan (m, r0, r1, degree)...\n")
	tPhase := nowSec()
	design, err := designBuild(baseCh, nch, kts, mre, mim, r0Grid, r1Grid,
		a.MinRatio, designSpec{
			method:       a.Method,
			degrees:      a.Degrees,
			nBasis:       a.NBasis,
			bins:         a.VBins,
			zeroBoundary: !a.NoZeroBoundary,
			quadOrder:    a.QuadOrder,
			maxLogSpan:   a.LogSpan,
			regOrder:     a.RegOrder,
			kernelOrder:  a.KernelOrder,
		})
	if err != nil {
		return err
	}
	c.phase("design", tPhase)
	reportCondition(design, c)

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
		a.R0Min, r1MaxEff, ds, st.src, g1, depWl, depVal)
	if err != nil {
		return err
	}
	c.phase("output", tPhase)

	reportSummary(a, sols, g1, baseCh, prof.nAvg, prof.nLow, prof.nHigh,
		prof.vCum, prof.gFit, prof.rGrid, prof.mRe, prof.mIm, depWl, depVal,
		prof.deltaLwl, prof.deltaL, c)
	reportMoments(prof.moments, c)
	reportDepolarization(prof, c)

	reportModes(prof.modes, a.RSplit, a.Modes, c)

	if err := writeRunOutputs(a, c, baseCh, g1, sols, prof, nTrueFn); err != nil {
		return err
	}

	c.phase("total", tAll)
	if rep != nil {
		if err := writeReportDoc(a, c, rep, prof, baseCh, g1, nTrueFn); err != nil {
			return err
		}
	}
	return nil
}
