package solve

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	plotFmtPng = 0
	plotFmtPdf = 1
	plotFmtSvg = 2
)

type plotOpts struct {
	fmt      int
	dpi      float64
	fontSize float64
}

func plotFormatExt(f int) string {
	switch f {
	case plotFmtPdf:
		return "pdf"
	case plotFmtSvg:
		return "svg"
	default:
		return "png"
	}
}

// fmtG повторяет C-формат "%.17g".
func fmtG(v float64) string {
	return strconv.FormatFloat(v, 'g', 17, 64)
}

// gpPath приводит путь к прямым слэшам: такой вид понимает gnuplot на всех
// платформах, включая Windows (там обратные слэши в строках скрипта могут
// трактоваться как escape-последовательности).
func gpPath(p string) string { return filepath.ToSlash(p) }

// plotFontAttr — атрибут размера шрифта для gnuplot: " font ',14'".
func plotFontAttr(po *plotOpts) string {
	if po != nil && po.fontSize > 0 {
		return fmt.Sprintf(" font ',%.10g'", po.fontSize)
	}
	return ""
}

func setTerminal(sb *strings.Builder, po *plotOpts, wIn, hIn float64) {
	dpi := 150.0
	f := plotFmtPng
	if po != nil {
		if po.dpi > 0 {
			dpi = po.dpi
		}
		f = po.fmt
	}
	wp := int(wIn*dpi + 0.5)
	hp := int(hIn*dpi + 0.5)
	if wp < 1 {
		wp = 1
	}
	if hp < 1 {
		hp = 1
	}
	switch f {
	case plotFmtPdf:
		fmt.Fprintf(sb, "set terminal pdfcairo size %.3f,%.3f enhanced "+
			"font 'DejaVu Sans,10'\n", wIn, hIn)
	case plotFmtSvg:
		fmt.Fprintf(sb, "set terminal svg size %d,%d enhanced "+
			"font 'DejaVu Sans,10'\n", wp, hp)
	default:
		fmt.Fprintf(sb, "set terminal pngcairo size %d,%d enhanced "+
			"font 'DejaVu Sans,10'\n", wp, hp)
	}
}

func plotTempDir() (string, bool) {
	dir, err := os.MkdirTemp("", "csolve_")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[warning] failed to create temporary "+
			"directory for the plot\n")
		return "", false
	}
	return dir, true
}

func writeDataFile(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fatal("failed to write %s", path)
	}
}

func writeBars(ch []channel, g1, gFit []float64) string {
	var b strings.Builder
	for j := range ch {
		kind := 'a'
		if ch[j].kind == chBeta {
			kind = 'b'
		}
		fmt.Fprintf(&b, "%c%g %s %s\n", kind, ch[j].wlUm*1e3,
			fmtG(g1[j]), fmtG(gFit[j]))
	}
	return b.String()
}

func cleanupPlot(dir string, files ...string) {
	for _, f := range files {
		_ = os.Remove(f)
	}
	_ = os.Remove(dir)
}

func gnuplotOK(outPath, script string) bool {
	cmd := exec.Command("gnuplot", script)
	err := cmd.Run()
	if err != nil {
		return false
	}
	_, statErr := os.Stat(outPath)
	return statErr == nil
}

// plotProfile — компактный график одной высоты пакетного режима (2 панели).
func plotProfile(po *plotOpts, outPath string, rGrid, nAvg []float64,
	ch []channel, g1, gFit []float64, h float64) bool {
	dir, ok := plotTempDir()
	if !ok {
		return false
	}
	pProf := filepath.Join(dir, "prof.dat")
	pBars := filepath.Join(dir, "bars.dat")
	pScript := filepath.Join(dir, "plot.gp")

	var prof strings.Builder
	for i := range rGrid {
		v := nAvg[i] / cm3PerM3
		prof.WriteString(fmtG(rGrid[i]))
		prof.WriteByte(' ')
		switch {
		case math.IsNaN(v):
			prof.WriteString("NaN")
		case v > 1e-30:
			prof.WriteString(fmtG(v))
		default:
			prof.WriteString(fmtG(1e-30))
		}
		prof.WriteByte('\n')
	}
	writeDataFile(pProf, prof.String())
	writeDataFile(pBars, writeBars(ch, g1, gFit))

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

	writeDataFile(pScript, sb.String())
	if !gnuplotOK(outPath, pScript) {
		fmt.Fprintf(os.Stderr, "[warning] failed to build plot %s "+
			"(script: %s)\n", outPath, pScript)
		return false
	}
	cleanupPlot(dir, pProf, pBars, pScript)
	return true
}

// plotRefractiveProfile — сводный вертикальный профиль m по высотам.
func plotRefractiveProfile(po *plotOpts, outPath string, alt, mRe, mIm []float64) bool {
	n := len(alt)
	if n <= 0 {
		return false
	}
	dir, ok := plotTempDir()
	if !ok {
		return false
	}
	pData := filepath.Join(dir, "refr.dat")
	pScript := filepath.Join(dir, "plot.gp")

	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return alt[idx[a]] < alt[idx[b]] })

	reLo, reHi := mRe[idx[0]], mRe[idx[0]]
	imLo, imHi := mIm[idx[0]], mIm[idx[0]]
	hLo, hHi := alt[idx[0]], alt[idx[0]]
	for k := 1; k < n; k++ {
		i := idx[k]
		if mRe[i] < reLo {
			reLo = mRe[i]
		}
		if mRe[i] > reHi {
			reHi = mRe[i]
		}
		if mIm[i] < imLo {
			imLo = mIm[i]
		}
		if mIm[i] > imHi {
			imHi = mIm[i]
		}
		if alt[i] < hLo {
			hLo = alt[i]
		}
		if alt[i] > hHi {
			hHi = alt[i]
		}
	}
	reMag := math.Max(math.Abs(reLo), math.Abs(reHi))
	imMag := math.Max(math.Abs(imLo), math.Abs(imHi))
	rePad := 0.06 * (reHi - reLo)
	if 0.02*reMag > rePad {
		rePad = 0.02 * reMag
	}
	if rePad <= 0 {
		rePad = 0.01
	}
	imPad := 0.06 * (imHi - imLo)
	if 0.02*imMag > imPad {
		imPad = 0.02 * imMag
	}
	if imPad <= 0 {
		imPad = 0.001
	}
	hMag := math.Max(math.Abs(hLo), math.Abs(hHi))
	hPad := 0.04 * (hHi - hLo)
	if 0.02*hMag > hPad {
		hPad = 0.02 * hMag
	}
	if hPad <= 0 {
		hPad = 10.0
	}

	var data strings.Builder
	for k := 0; k < n; k++ {
		i := idx[k]
		fmt.Fprintf(&data, "%s %s %s\n", fmtG(alt[i]), fmtG(mRe[i]), fmtG(mIm[i]))
	}
	writeDataFile(pData, data.String())

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
	fmt.Fprintf(&sb, "set xrange [%.10g:%.10g]\n", reLo-rePad, reHi+rePad)
	fmt.Fprintf(&sb, "set x2range [%.10g:%.10g]\n", imLo-imPad, imHi+imPad)
	fmt.Fprintf(&sb, "set yrange [%.10g:%.10g]\n", hLo-hPad, hHi+hPad)
	sb.WriteString("set title 'Refractive index vs altitude'\n")
	fmt.Fprintf(&sb, "plot %q using 2:1 with linespoints axes x1y1 "+
		"lw 1.8 pt 7 ps 1.1 lc rgb '#1f77b4' title 'Re(m)', \\\n"+
		"     %q using 3:1 with linespoints axes x2y1 "+
		"lw 1.8 pt 5 ps 1.1 lc rgb '#d62728' title 'Im(m)'\n",
		gpPath(pData), gpPath(pData))

	writeDataFile(pScript, sb.String())
	if !gnuplotOK(outPath, pScript) {
		fmt.Fprintf(os.Stderr, "[warning] failed to build plot %s "+
			"(script: %s)\n", outPath, pScript)
		return false
	}
	cleanupPlot(dir, pData, pScript)
	return true
}

// anyFiniteSeries — есть ли хоть одно конечное значение в наборе серий.
func anyFiniteSeries(vals [][]float64) bool {
	for _, s := range vals {
		for _, v := range s {
			if isFinite(v) {
				return true
			}
		}
	}
	return false
}

// plotProfiles — сводный вертикальный профиль нескольких серий.
// logAuto — выбирать логарифмическую шкалу x автоматически (для величин
// вроде деполяризации в процентах это нежелательно).
func plotProfiles(po *plotOpts, outPath, title, xlabel string, nSeries int,
	alt []float64, vals [][]float64, names []string, logAuto bool) bool {
	return plotProfilesStyled(po, outPath, title, xlabel, nSeries, alt, vals,
		names, nil, logAuto)
}

// plotProfilesStyled — то же, что plotProfiles, но с дополнительными
// gnuplot-атрибутами по сериям (styles[s] добавляется к описанию линии,
// например "dt 2"; пустая строка или nil — стиль по умолчанию).
func plotProfilesStyled(po *plotOpts, outPath, title, xlabel string,
	nSeries int, alt []float64, vals [][]float64, names []string,
	styles []string, logAuto bool) bool {
	n := len(alt)
	if n <= 0 || nSeries <= 0 {
		return false
	}
	dir, ok := plotTempDir()
	if !ok {
		return false
	}
	pData := filepath.Join(dir, "prof.dat")
	pScript := filepath.Join(dir, "plot.gp")

	var data strings.Builder
	for i := 0; i < n; i++ {
		data.WriteString(fmtG(alt[i]))
		for s := 0; s < nSeries; s++ {
			v := vals[s][i]
			data.WriteByte(' ')
			if isFinite(v) {
				data.WriteString(fmtG(v))
			} else {
				data.WriteString("NaN")
			}
		}
		data.WriteByte('\n')
	}
	writeDataFile(pData, data.String())

	lo, hi := math.Inf(1), math.Inf(-1)
	hLo, hHi := math.Inf(1), math.Inf(-1)
	any := false
	for s := 0; s < nSeries; s++ {
		for i := 0; i < n; i++ {
			v := vals[s][i]
			if !isFinite(v) {
				continue
			}
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
			any = true
		}
	}
	for i := 0; i < n; i++ {
		if !isFinite(alt[i]) {
			continue
		}
		if alt[i] < hLo {
			hLo = alt[i]
		}
		if alt[i] > hHi {
			hHi = alt[i]
		}
	}
	if !any {
		lo, hi = 0, 1
	}
	useLog := logAuto && lo > 0 && hi > 0 && hi/lo > 30.0
	mag := math.Max(math.Abs(lo), math.Abs(hi))
	if useLog {
		lo /= 1.3
		hi *= 1.3
	} else if hi > lo {
		pad := 0.08 * (hi - lo)
		if 0.05*mag > pad {
			pad = 0.05 * mag
		}
		lo -= pad
		hi += pad
	} else {
		pad := 0.5
		if mag > 0 {
			pad = 0.05 * mag
		}
		lo -= pad
		hi += pad
	}
	if !(hi > lo) {
		lo -= 0.5
		hi += 0.5
	}
	hMag := math.Max(math.Abs(hLo), math.Abs(hHi))
	hPad := 0.04 * (hHi - hLo)
	if 0.02*hMag > hPad {
		hPad = 0.02 * hMag
	}
	if hPad <= 0 {
		hPad = 10.0
	}
	hLo -= hPad
	hHi += hPad

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
		if s < len(styles) && styles[s] != "" {
			sb.WriteString(" " + styles[s])
		}
		fmt.Fprintf(&sb, " lw 1.8 pt %d ps 1.1 lc rgb %s title '%s'",
			points[s%6], colors[s%6], names[s])
	}
	sb.WriteByte('\n')

	writeDataFile(pScript, sb.String())
	if !gnuplotOK(outPath, pScript) {
		fmt.Fprintf(os.Stderr, "[warning] failed to build plot %s "+
			"(script: %s)\n", outPath, pScript)
		return false
	}
	cleanupPlot(dir, pData, pScript)
	return true
}

// plotResults — основной график одиночного режима (2 панели).
func plotResults(po *plotOpts, outPath string, rGrid []float64,
	nTrue, nAvg, nAvgRaw []float64, nBest [][]float64, nBestCount int,
	ch []channel, g1, gFit []float64, mAvgRe, mAvgIm float64) bool {
	dir, ok := plotTempDir()
	if !ok {
		return false
	}
	pAvg := filepath.Join(dir, "avg.dat")
	pTrue := filepath.Join(dir, "true.dat")
	pBars := filepath.Join(dir, "bars.dat")
	pScript := filepath.Join(dir, "plot.gp")

	var avg strings.Builder
	for i := range rGrid {
		avg.WriteString(fmtG(rGrid[i]))
		avg.WriteByte(' ')
		v := nAvg[i] / cm3PerM3
		if isFinite(v) {
			avg.WriteString(fmtG(v))
		} else {
			avg.WriteString("NaN")
		}
		avg.WriteByte(' ')
		rv := nAvgRaw[i] / cm3PerM3
		if isFinite(rv) {
			avg.WriteString(fmtG(rv))
		} else {
			avg.WriteString("NaN")
		}
		avg.WriteByte('\n')
	}
	writeDataFile(pAvg, avg.String())

	haveTrue := nTrue != nil
	if haveTrue {
		var ta strings.Builder
		for i := range rGrid {
			fmt.Fprintf(&ta, "%s %s\n", fmtG(rGrid[i]), fmtG(nTrue[i]/cm3PerM3))
		}
		writeDataFile(pTrue, ta.String())
	}
	writeDataFile(pBars, writeBars(ch, g1, gFit))

	ns := nBestCount
	if ns > 64 {
		ns = 64
	}
	solFiles := make([]string, ns)
	for t := 0; t < ns; t++ {
		solFiles[t] = filepath.Join(dir, fmt.Sprintf("sol_%d.dat", t))
		var sb strings.Builder
		for i := range rGrid {
			fmt.Fprintf(&sb, "%s %s\n", fmtG(rGrid[i]),
				fmtG(nBest[t][i]/cm3PerM3))
		}
		writeDataFile(solFiles[t], sb.String())
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

	writeDataFile(pScript, sb.String())
	if !gnuplotOK(outPath, pScript) {
		fmt.Fprintf(os.Stderr, "[warning] failed to build plot "+
			"(script kept for diagnostics: %s)\n", pScript)
		return false
	}
	files := append([]string{pAvg, pTrue, pBars, pScript}, solFiles...)
	cleanupPlot(dir, files...)
	fmt.Printf("plot saved: %s\n", outPath)
	return true
}
