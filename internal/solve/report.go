// Copyright (C) 2026 Шмирко Константин Александрович <kshmirko@dvo.ru>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package solve

import (
	"math"
	"strconv"
)

// reportEnsembleSize — общее число найденных решений и число усредняемых.
// Печатается один раз на прогон (высоту / пакет).
func reportEnsembleSize(a *Args, nTot int, c *console) {
	g := a.bestCount(nTot)
	c.printf("solutions found: %d; averaging over G = %d", nTot, g)
	if a.Best < 1 {
		c.printf(" (fraction --best=%g)", a.Best)
	}
	c.printf("\n")
}

// reportSummary — сводка по ансамблю решений и согласию по каналам и по
// деполяризации (измеренной δ_l против рассчитанной).
func reportSummary(a *Args, sols *solutions, g1 []float64, ch []channel,
	nAvg, nLow, nHigh, vCum, gFit, rGrid []float64, mAvgRe, mAvgIm float64,
	depWl, depVal, calcWl, calcDelta []float64, c *console) {
	nch := len(ch)
	G := a.bestCount(sols.n)
	c.printf("\n=== Solution ensemble (scan over m, r0, r1) ===\n")
	if sols.method == methodMonotone {
		c.printf("%4s %13s %6s %9s %9s %10s %9s\n",
			"rank", "m", "bins", "r0, µm", "r1, µm", "gamma", "Err")
	} else {
		c.printf("%4s %13s %7s %9s %9s %10s %9s\n",
			"rank", "m", "degree", "r0, µm", "r1, µm", "gamma", "Err")
	}
	for t := 0; t < sols.n; t++ {
		s := &sols.sol[t]
		mark := ""
		if t == 0 {
			mark = " <--"
		}
		if sols.method == methodMonotone {
			c.printf("%4d %6.3f%+7.3fj %6d %9.4g %9.4g %10.3g %9.5f%s\n",
				t+1, s.mRe, s.mIm, s.bins, s.r0, s.r1, s.gamma, s.err, mark)
		} else {
			c.printf("%4d %6.3f%+7.3fj %7d %9.4g %9.4g %10.3g %9.5f%s\n",
				t+1, s.mRe, s.mIm, s.degree, s.r0, s.r1, s.gamma, s.err, mark)
		}
		if t+1 >= G {
			break
		}
	}
	c.printf("\nmean refractive index (over %d best): m = %.3f%+.3fj\n",
		G, mAvgRe, mAvgIm)

	c.printf("\n=== Distribution function (dV/dln r) over the best solutions ===\n")
	c.printf("key points (full grid — in --json, if requested):\n")
	c.printf("  %9s %12s %12s %12s  µm³/cm³\n", "r, µm", "mean", "lower",
		"upper")
	nr := len(nAvg)
	npts := nr
	if npts > 7 {
		npts = 7
	}
	for k := 0; k < npts; k++ {
		i := 0
		if npts > 1 {
			i = int(float64(k)*float64(nr-1)/float64(npts-1) + 1e-9)
		}
		if i >= nr {
			i = nr - 1
		}
		r := rGrid[i]
		if !isFinite(nAvg[i]) {
			c.printf("  r = %9.4g µm,  dV/dln r = no coverage "+
				"(outside the windows of the best solutions)\n", r)
			continue
		}
		lo, hi := math.NaN(), math.NaN()
		if i < len(nLow) {
			lo = nLow[i]
		}
		if i < len(nHigh) {
			hi = nHigh[i]
		}
		c.printf("  %9.4g %12.4g %12.4g %12.4g\n", r, nAvg[i]/cm3PerM3,
			lo/cm3PerM3, hi/cm3PerM3)
	}
	c.printf("  (lower/upper — min/max over the G = %d best solutions "+
		"covering r, before smoothing)\n", G)

	reportCumulativeVolume(vCum, rGrid, sols.method == methodMonotone, c)

	c.printf("\n=== Channel agreement (mean solution) ===\n")
	errAvg := 0.0
	for j := 0; j < nch; j++ {
		rel := absf(gFit[j]-g1[j]) / g1[j] * 100.0
		c.printf("  %-14s g1 = %.6g   g = %.6g   |g-g1|/g1 = %.2f%%\n",
			ch[j].name(), g1[j], gFit[j], rel)
		errAvg += absf(gFit[j]-g1[j]) / g1[j]
	}
	// деполяризация: измеренная δ_l (на всех длинах волн из блока
	// depolarization) против рассчитанной по среднему решению
	nVal := nch
	for k, wl := range depWl {
		meas := 0.0
		hasMeas := false
		if k < len(depVal) && isFinite(depVal[k]) {
			meas, hasMeas = depVal[k], true
		}
		calc := 0.0
		hasCalc := false
		if i := depolWlIndex(calcWl, wl); i >= 0 && i < len(calcDelta) &&
			isFinite(calcDelta[i]) {
			calc, hasCalc = calcDelta[i], true
		}
		switch {
		case !hasMeas:
			// измерения нет (nan) — только расчётный коэффициент,
			// в функционал не добавляется
			if hasCalc {
				c.printf("  %-14s δ_calc = %.4g %%   (measured n/a)\n",
					depolName(wl), calc)
			} else {
				c.printf("  %-14s no data (measured n/a, no S11/S22)\n",
					depolName(wl))
			}
		case !hasCalc:
			c.printf("  %-14s δ_meas = %.4g %%   δ_calc = no data\n",
				depolName(wl), meas)
		case meas == 0:
			// относительная невязка не определена — только абсолютная
			c.printf("  %-14s δ_meas = %.4g %%   δ_calc = %.4g %%   "+
				"|δ-δ_meas| = %.4g п.п.\n", depolName(wl), meas, calc,
				absf(calc-meas))
		default:
			rel := absf(calc-meas) / absf(meas) * 100.0
			c.printf("  %-14s δ_meas = %.4g %%   δ_calc = %.4g %%   "+
				"|δ-δ_meas|/δ_meas = %.2f%%\n", depolName(wl), meas, calc, rel)
			errAvg += absf(calc-meas) / absf(meas)
			nVal++
		}
	}
	errAvg /= float64(nVal)
	c.printf("  Err (mean solution) = %.1f%%\n", errAvg*100.0)
}

// reportDepolarization — линейная деполяризация обратного рассеяния,
// посчитанная по среднему распределению и столбцам S11/S22 таблиц.
func reportDepolarization(p *profile, c *console) {
	c.printf("\n=== Linear depolarization at 180° ===\n")
	if len(p.deltaLwl) == 0 {
		c.printf("  not computed: tables have no valid S11/S22 columns\n")
		return
	}
	c.printf("  δ_l = (S11-S22)/(S11+S22)*100; S11/S22 — свёртка со " +
		"средним распределением\n")
	for i, wl := range p.deltaLwl {
		v := p.deltaL[i]
		if isFinite(v) {
			c.printf("  δ_l(%g nm) = %.4g %%\n", wl*1e3, v)
		} else {
			c.printf("  δ_l(%g nm) = no data (no S11/S22 for the mean m)\n",
				wl*1e3)
		}
	}
}

// reportCumulativeVolume — восстановленная монотонная функция V(r) в
// ключевых точках и контроль её согласия с C_v.
func reportCumulativeVolume(vCum, rGrid []float64, monotone bool, c *console) {
	if len(vCum) == 0 {
		return
	}
	c.printf("\n=== Cumulative volume V(r) = ∫ dV/dln r′ dln r′ ===\n")
	if total := volumeTotal(vCum); isFinite(total) {
		c.printf("  total over the covered range V(r_end) = %.4g µm³/cm³\n",
			total/cm3PerM3)
	}
	minStep, viol := monotoneCheck(vCum)
	switch {
	case !monotone:
	case viol == 0:
		c.printf("  monotone: yes (V(r) does not decrease)\n")
	default:
		c.printf("  monotone: %d decreasing step(s), worst = %.3g of the total\n",
			viol, minStep)
	}
	nr := len(vCum)
	npts := nr
	if npts > 7 {
		npts = 7
	}
	for k := 0; k < npts; k++ {
		i := 0
		if npts > 1 {
			i = int(float64(k)*float64(nr-1)/float64(npts-1) + 1e-9)
		}
		if i >= nr {
			i = nr - 1
		}
		if !isFinite(vCum[i]) {
			continue
		}
		c.printf("  r = %9.4g µm,  V(r) = %.4g µm³/cm³\n", rGrid[i],
			vCum[i]/cm3PerM3)
	}
}

func reportMoments(m moments, c *console) {
	c.printf("\n=== Integral characteristics " +
		"(estimates over the covered region) ===\n")
	c.printf("  covered radius range: %.4g–%.4g µm (grid fraction: %.0f%%)\n",
		m.RCovMinUm, m.RCovMaxUm, m.CovFrac*100.0)
	c.printf("  number concentration N      = %.4g cm⁻³\n", m.NCm3)
	c.printf("  volume concentration C_v    = %.4g µm³/cm³\n",
		m.VolConcUm3PerCm3)
	c.printf("  mean particle volume ⟨V⟩    = %.4g µm³ (%.4g m³)\n",
		m.VolMeanUm3, m.VolMeanUm3*um3ToM3)
	c.printf("  effective radius r_eff      = %.4g µm\n", m.REffUm)
}

func reportModes(m modes, rSplit float64, method modesKind, c *console) {
	c.printf("\n=== Log-normal mode parameters (dV/dln r) ===\n")
	c.printf("  mode split boundary: %g µm (--r-split)\n", rSplit)
	c.printf("  parameter estimation method: %s (--modes)\n", method)
	c.printf("  %-8s %12s %9s %7s   note\n",
		"mode", "V, µm³/cm³", "rm, µm", "sigma")
	names := [3]string{"fine", "coarse", "overall"}
	vals := [3]*csMode{&m.Fine, &m.Coarse, &m.Overall}
	for i := 0; i < 3; i++ {
		v := vals[i]
		if v.OK {
			note := ""
			if v.Method != "fit" {
				note = "(" + v.Method + ")"
			}
			c.printf("  %-8s %12.4g %9.4g %7.4g   %s\n", names[i],
				v.V/cm3PerM3, v.Rm, v.Sigma, note)
		} else {
			c.printf("  %-8s %12s %9s %7s   %s\n", names[i], "—", "—", "—",
				v.Method)
		}
	}
}

// depolName — подпись строки деполяризации для заданной длины волны (µm).
func depolName(wlUm float64) string {
	return "δ_l(" + strconv.FormatFloat(wlUm*1e3, 'g', -1, 64) + " nm)"
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
