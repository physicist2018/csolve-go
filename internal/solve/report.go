package solve

import "fmt"

// reportSummary — сводка по ансамблю решений и согласию по каналам.
func reportSummary(a *Args, sols *solutions, g1 []float64, ch []channel,
	nAvg, gFit, rGrid []float64, mAvgRe, mAvgIm float64) {
	nch := len(ch)
	G := a.Best
	if G > sols.n {
		G = sols.n
	}
	fmt.Printf("\n=== Solution ensemble (scan over m, r0, r1) ===\n")
	fmt.Printf("variants scanned: %d; averaging: G = %d\n", sols.n, G)
	fmt.Printf("%4s %13s %7s %9s %9s %10s %9s\n",
		"rank", "m", "degree", "r0, µm", "r1, µm", "gamma", "Err")
	for t := 0; t < sols.n; t++ {
		s := &sols.sol[t]
		mark := ""
		if t == 0 {
			mark = " <--"
		}
		fmt.Printf("%4d %6.3f%+7.3fj %7d %9.4g %9.4g %10.3g %9.5f%s\n",
			t+1, s.mRe, s.mIm, s.degree, s.r0, s.r1, s.gamma, s.err, mark)
		if t+1 >= G {
			break
		}
	}
	fmt.Printf("\nmean refractive index (over %d best): m = %.3f%+.3fj\n",
		G, mAvgRe, mAvgIm)

	fmt.Printf("\n=== Mean distribution function (dV/dln r) ===\n")
	fmt.Printf("key points (full grid — in --json, if requested):\n")
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
			fmt.Printf("  r = %9.4g µm,  dV/dln r = no coverage "+
				"(outside the windows of the best solutions)\n", r)
			continue
		}
		fmt.Printf("  r = %9.4g µm,  dV/dln r = %.4g µm³/cm³\n", r,
			nAvg[i]/cm3PerM3)
	}

	fmt.Printf("\n=== Channel agreement (mean solution) ===\n")
	errAvg := 0.0
	for j := 0; j < nch; j++ {
		rel := absf(gFit[j]-g1[j]) / g1[j] * 100.0
		fmt.Printf("  %-14s g1 = %.6g   g = %.6g   |g-g1|/g1 = %.2f%%\n",
			ch[j].name(), g1[j], gFit[j], rel)
		errAvg += absf(gFit[j]-g1[j]) / g1[j]
	}
	errAvg /= float64(nch)
	fmt.Printf("  Err (mean solution) = %.5f\n", errAvg)
}

// reportDepolarization — линейная деполяризация обратного рассеяния,
// посчитанная по среднему распределению и столбцам S11/S22 таблиц.
func reportDepolarization(p *profile) {
	fmt.Printf("\n=== Linear depolarization at 180° ===\n")
	if len(p.deltaLwl) == 0 {
		fmt.Printf("  not computed: tables have no valid S11/S22 columns\n")
		return
	}
	fmt.Printf("  δ_l = (S11-S22)/(S11+S22)*100; S11/S22 — свёртка со " +
		"средним распределением\n")
	for i, wl := range p.deltaLwl {
		v := p.deltaL[i]
		if isFinite(v) {
			fmt.Printf("  δ_l(%g nm) = %.4g %%\n", wl*1e3, v)
		} else {
			fmt.Printf("  δ_l(%g nm) = no data (no S11/S22 for the mean m)\n",
				wl*1e3)
		}
	}
}

func reportMoments(m moments) {
	fmt.Printf("\n=== Integral characteristics " +
		"(estimates over the covered region) ===\n")
	fmt.Printf("  covered radius range: %.4g–%.4g µm (grid fraction: %.0f%%)\n",
		m.RCovMinUm, m.RCovMaxUm, m.CovFrac*100.0)
	fmt.Printf("  number concentration N      = %.4g cm⁻³\n", m.NCm3)
	fmt.Printf("  volume concentration C_v    = %.4g µm³/cm³\n",
		m.VolConcUm3PerCm3)
	fmt.Printf("  mean particle volume ⟨V⟩    = %.4g µm³ (%.4g m³)\n",
		m.VolMeanUm3, m.VolMeanUm3*um3ToM3)
	fmt.Printf("  effective radius r_eff      = %.4g µm\n", m.REffUm)
}

func reportModes(m modes, rSplit float64, method string) {
	fmt.Printf("\n=== Log-normal mode parameters (dV/dln r) ===\n")
	fmt.Printf("  mode split boundary: %g µm (--r-split)\n", rSplit)
	fmt.Printf("  parameter estimation method: %s (--modes)\n", method)
	fmt.Printf("  %-8s %12s %9s %7s   note\n",
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
			fmt.Printf("  %-8s %12.4g %9.4g %7.4g   %s\n", names[i],
				v.V/cm3PerM3, v.Rm, v.Sigma, note)
		} else {
			fmt.Printf("  %-8s %12s %9s %7s   %s\n", names[i], "—", "—", "—",
				v.Method)
		}
	}
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
