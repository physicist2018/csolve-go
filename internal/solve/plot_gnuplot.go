package solve

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func gnuplotOK(outPath, script string) bool {
	cmd := exec.Command("gnuplot", script)
	err := cmd.Run()
	if err != nil {
		return false
	}
	_, statErr := os.Stat(outPath)
	return statErr == nil
}

// gnuplotProfile — компактный график одной высоты пакетного режима (2 панели).
func gnuplotProfile(po *plotOpts, outPath string, rGrid, nAvg []float64,
	ch []channel, g1, gFit []float64, h float64) bool {
	dir, ok := po.tempDir()
	if !ok {
		return false
	}
	pProf := filepath.Join(dir, "prof.dat")
	pBars := filepath.Join(dir, "bars.dat")
	pScript := filepath.Join(dir, "plot.gp")

	var prof strings.Builder
	for i := range rGrid {
		v := nAvg[i] / cm3PerM3
		prof.WriteString(formatG17(rGrid[i]))
		prof.WriteByte(' ')
		switch {
		case math.IsNaN(v):
			prof.WriteString("NaN")
		case v > 1e-30:
			prof.WriteString(formatG17(v))
		default:
			prof.WriteString(formatG17(1e-30))
		}
		prof.WriteByte('\n')
	}
	po.writeDataFile(pProf, prof.String())
	po.writeDataFile(pBars, writeBars(ch, g1, gFit))

	fa := plotFontAttr(po)
	var sb strings.Builder
	sb.WriteString("set encoding utf8\n")
	setTerminal(&sb, po, 8.0, 3.0667)
	fmt.Fprintf(&sb, "set output %q\n", gpPath(outPath))
	sb.WriteString("set datafile missing 'NaN'\n")
	fmt.Fprintf(&sb, "set title 'H = %g m'\n", h)
	sb.WriteString("set multiplot layout 1,2\n")
	sb.WriteString("set logscale x\n")
	fmt.Fprintf(&sb, "set xlabel 'r, µm'%s\n", fa)
	fmt.Fprintf(&sb, "set ylabel 'dV/dln r, µm³/cm³'%s\n", fa)
	sb.WriteString("set grid\n")
	fmt.Fprintf(&sb, "plot %q using 1:2 with lines lw 2 lc rgb '#1f77b4' "+
		"notitle\n", gpPath(pProf))
	sb.WriteString("unset logscale x\n")
	sb.WriteString("set xlabel ''\n")
	fmt.Fprintf(&sb, "set ylabel 'coefficient'%s\n", fa)
	sb.WriteString("set style data histograms\n")
	sb.WriteString("set style fill solid 0.7 border -1\n")
	sb.WriteString("set boxwidth 0.4\n")
	sb.WriteString("set xtics rotate by 20 right\n")
	fmt.Fprintf(&sb, "set key top right%s\n", fa)
	fmt.Fprintf(&sb, "plot %q using 2:xtic(1) title 'measured g1', "+
		"'' using 3 title 'computed g'\n", gpPath(pBars))
	sb.WriteString("unset multiplot\n")

	po.writeDataFile(pScript, sb.String())
	if !gnuplotOK(outPath, pScript) {
		po.warnf("[warning] failed to build plot %s "+
			"(script: %s)\n", outPath, pScript)
		return false
	}
	cleanupPlot(dir, pProf, pBars, pScript)
	return true
}

// gnuplotRefractiveProfile — сводный вертикальный профиль m по высотам.
func gnuplotRefractiveProfile(po *plotOpts, outPath string, alt, mRe, mIm []float64) bool {
	n := len(alt)
	if n <= 0 {
		return false
	}
	dir, ok := po.tempDir()
	if !ok {
		return false
	}
	pData := filepath.Join(dir, "refr.dat")
	pScript := filepath.Join(dir, "plot.gp")

	idx := altitudeOrder(alt)
	reLo, reHi, imLo, imHi, hLo, hHi := refractiveAxes(alt, mRe, mIm)

	var data strings.Builder
	for k := 0; k < n; k++ {
		i := idx[k]
		fmt.Fprintf(&data, "%s %s %s\n", formatG17(alt[i]), formatG17(mRe[i]), formatG17(mIm[i]))
	}
	po.writeDataFile(pData, data.String())

	fa := plotFontAttr(po)
	var sb strings.Builder
	sb.WriteString("set encoding utf8\n")
	setTerminal(&sb, po, 6.0, 4.8)
	fmt.Fprintf(&sb, "set output %q\n", gpPath(outPath))
	sb.WriteString("set datafile missing 'NaN'\n")
	fmt.Fprintf(&sb, "set xlabel 'Re(m)'%s\n", fa)
	fmt.Fprintf(&sb, "set x2label 'Im(m)'%s\n", fa)
	fmt.Fprintf(&sb, "set ylabel 'Altitude H, m'%s\n", fa)
	sb.WriteString("set grid\n")
	sb.WriteString("set xtics nomirror\n")
	sb.WriteString("set x2tics\n")
	sb.WriteString("set ytics nomirror\n")
	fmt.Fprintf(&sb, "set key top right%s\n", fa)
	fmt.Fprintf(&sb, "set xrange [%.10g:%.10g]\n", reLo, reHi)
	fmt.Fprintf(&sb, "set x2range [%.10g:%.10g]\n", imLo, imHi)
	fmt.Fprintf(&sb, "set yrange [%.10g:%.10g]\n", hLo, hHi)
	sb.WriteString("set title 'Refractive index vs altitude'\n")
	fmt.Fprintf(&sb, "plot %q using 2:1 with linespoints axes x1y1 "+
		"lw 1.8 pt 7 ps 1.1 lc rgb '#1f77b4' title 'Re(m)', \\\n"+
		"     %q using 3:1 with linespoints axes x2y1 "+
		"lw 1.8 pt 5 ps 1.1 lc rgb '#d62728' title 'Im(m)'\n",
		gpPath(pData), gpPath(pData))

	po.writeDataFile(pScript, sb.String())
	if !gnuplotOK(outPath, pScript) {
		po.warnf("[warning] failed to build plot %s "+
			"(script: %s)\n", outPath, pScript)
		return false
	}
	cleanupPlot(dir, pData, pScript)
	return true
}

// gnuplotProfilesStyled — реализация plotProfilesStyled через gnuplot;
// dashed[s] добавляет к описанию линии атрибут "dt 2".
func gnuplotProfilesStyled(po *plotOpts, outPath, title, xlabel string,
	nSeries int, alt []float64, vals [][]float64, names []string,
	dashed []bool, logAuto bool) bool {
	n := len(alt)
	if n <= 0 || nSeries <= 0 {
		return false
	}
	dir, ok := po.tempDir()
	if !ok {
		return false
	}
	pData := filepath.Join(dir, "prof.dat")
	pScript := filepath.Join(dir, "plot.gp")

	var data strings.Builder
	for i := 0; i < n; i++ {
		data.WriteString(formatG17(alt[i]))
		for s := 0; s < nSeries; s++ {
			v := vals[s][i]
			data.WriteByte(' ')
			if isFinite(v) {
				data.WriteString(formatG17(v))
			} else {
				data.WriteString("NaN")
			}
		}
		data.WriteByte('\n')
	}
	po.writeDataFile(pData, data.String())

	lo, hi, hLo, hHi, useLog := profileAxes(alt, vals, nSeries, logAuto)

	colors := [6]string{"'#1f77b4'", "'#d62728'", "'#2ca02c'",
		"'#9467bd'", "'#ff7f0e'", "'#8c564b'"}
	points := [6]int{7, 5, 9, 11, 13, 15}

	fa := plotFontAttr(po)
	var sb strings.Builder
	sb.WriteString("set encoding utf8\n")
	setTerminal(&sb, po, 6.0, 4.8)
	fmt.Fprintf(&sb, "set output %q\n", gpPath(outPath))
	sb.WriteString("set datafile missing 'NaN'\n")
	fmt.Fprintf(&sb, "set title '%s'\n", title)
	fmt.Fprintf(&sb, "set xlabel '%s'%s\n", xlabel, fa)
	fmt.Fprintf(&sb, "set ylabel 'Altitude H, m'%s\n", fa)
	sb.WriteString("set grid\n")
	fmt.Fprintf(&sb, "set key top right%s\n", fa)
	if useLog {
		sb.WriteString("set logscale x\n")
	} else {
		sb.WriteString("unset logscale x\n")
	}
	fmt.Fprintf(&sb, "set xrange [%.10g:%.10g]\n", lo, hi)
	fmt.Fprintf(&sb, "set yrange [%.10g:%.10g]\n", hLo, hHi)
	sb.WriteString("plot ")
	for s := 0; s < nSeries; s++ {
		if s > 0 {
			sb.WriteString(", \\\n     ")
		}
		fmt.Fprintf(&sb, "%q using %d:1 with linespoints", gpPath(pData), s+2)
		if s < len(dashed) && dashed[s] {
			sb.WriteString(" dt 2")
		}
		fmt.Fprintf(&sb, " lw 1.8 pt %d ps 1.1 lc rgb %s title '%s'",
			points[s%6], colors[s%6], names[s])
	}
	sb.WriteByte('\n')

	po.writeDataFile(pScript, sb.String())
	if !gnuplotOK(outPath, pScript) {
		po.warnf("[warning] failed to build plot %s "+
			"(script: %s)\n", outPath, pScript)
		return false
	}
	cleanupPlot(dir, pData, pScript)
	return true
}

// gnuplotResults — основной график одиночного режима (2 панели), gnuplot.
func gnuplotResults(po *plotOpts, outPath string, rGrid []float64,
	nTrue, nAvg, nAvgRaw []float64, nBest [][]float64, nBestCount int,
	ch []channel, g1, gFit []float64, mAvgRe, mAvgIm float64) bool {
	dir, ok := po.tempDir()
	if !ok {
		return false
	}
	pAvg := filepath.Join(dir, "avg.dat")
	pTrue := filepath.Join(dir, "true.dat")
	pBars := filepath.Join(dir, "bars.dat")
	pScript := filepath.Join(dir, "plot.gp")

	var avg strings.Builder
	for i := range rGrid {
		avg.WriteString(formatG17(rGrid[i]))
		avg.WriteByte(' ')
		v := nAvg[i] / cm3PerM3
		if isFinite(v) {
			avg.WriteString(formatG17(v))
		} else {
			avg.WriteString("NaN")
		}
		avg.WriteByte(' ')
		rv := nAvgRaw[i] / cm3PerM3
		if isFinite(rv) {
			avg.WriteString(formatG17(rv))
		} else {
			avg.WriteString("NaN")
		}
		avg.WriteByte('\n')
	}
	po.writeDataFile(pAvg, avg.String())

	haveTrue := nTrue != nil
	if haveTrue {
		var ta strings.Builder
		for i := range rGrid {
			fmt.Fprintf(&ta, "%s %s\n", formatG17(rGrid[i]), formatG17(nTrue[i]/cm3PerM3))
		}
		po.writeDataFile(pTrue, ta.String())
	}
	po.writeDataFile(pBars, writeBars(ch, g1, gFit))

	ns := nBestCount
	if ns > 64 {
		ns = 64
	}
	solFiles := make([]string, ns)
	for t := 0; t < ns; t++ {
		solFiles[t] = filepath.Join(dir, fmt.Sprintf("sol_%d.dat", t))
		var sb strings.Builder
		for i := range rGrid {
			fmt.Fprintf(&sb, "%s %s\n", formatG17(rGrid[i]),
				formatG17(nBest[t][i]/cm3PerM3))
		}
		po.writeDataFile(solFiles[t], sb.String())
	}

	fa := plotFontAttr(po)
	var sb strings.Builder
	sb.WriteString("set encoding utf8\n")
	setTerminal(&sb, po, 8.6667, 3.4667)
	fmt.Fprintf(&sb, "set output %q\n", gpPath(outPath))
	sb.WriteString("set datafile missing 'NaN'\n")
	sb.WriteString("set multiplot layout 1,2\n")
	sb.WriteString("set logscale x\n")
	fmt.Fprintf(&sb, "set xlabel 'r, µm'%s\n", fa)
	fmt.Fprintf(&sb, "set ylabel 'dV/dln r, µm³/cm³'%s\n", fa)
	fmt.Fprintf(&sb, "set title 'Volume particle size distribution, "+
		"⟨m⟩ = %.3f%+.3fj'\n", mAvgRe, mAvgIm)
	sb.WriteString("set grid\n")
	fmt.Fprintf(&sb, "set key top right%s\n", fa)
	sb.WriteString("plot ")
	first := true
	sep := func() {
		if first {
			first = false
			return
		}
		sb.WriteString(", \\\n     ")
	}
	if haveTrue {
		sep()
		fmt.Fprintf(&sb, "%q using 1:2 with lines dt 2 lw 1.6 "+
			"lc rgb 'black' title 'true n(r)'", gpPath(pTrue))
	}
	for t := 0; t < ns; t++ {
		sep()
		fmt.Fprintf(&sb, "%q using 1:2 with lines lw 1 lc rgb '#bbbbbb' "+
			"notitle", gpPath(solFiles[t]))
	}
	sep()
	fmt.Fprintf(&sb, "%q using 1:3 with lines lw 1 dt 3 "+
		"lc rgb 'dark-orange' title 'mean (unsmoothed)'", gpPath(pAvg))
	sep()
	fmt.Fprintf(&sb, "%q using 1:2 with lines lw 2.6 "+
		"lc rgb '#1f77b4' title 'mean of best solutions'\n", gpPath(pAvg))
	sb.WriteString("unset logscale x\n")
	sb.WriteString("set xlabel ''\n")
	fmt.Fprintf(&sb, "set ylabel 'coefficient, rel. units'%s\n", fa)
	sb.WriteString("set title 'Measured vs computed coefficients'\n")
	sb.WriteString("set style data histograms\n")
	sb.WriteString("set style fill solid 0.7 border -1\n")
	sb.WriteString("set boxwidth 0.4\n")
	sb.WriteString("set xtics rotate by 20 right\n")
	fmt.Fprintf(&sb, "plot %q using 2:xtic(1) title 'measured g1', "+
		"'' using 3 title 'computed g (mean)'\n", gpPath(pBars))
	sb.WriteString("unset multiplot\n")

	po.writeDataFile(pScript, sb.String())
	if !gnuplotOK(outPath, pScript) {
		po.warnf("[warning] failed to build plot "+
			"(script kept for diagnostics: %s)\n", pScript)
		return false
	}
	files := append([]string{pAvg, pTrue, pBars, pScript}, solFiles...)
	cleanupPlot(dir, files...)
	po.printf("plot saved: %s\n", outPath)
	return true
}
