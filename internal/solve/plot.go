package solve

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// plotFormat — формат выходного файла графика (--plot-format).
type plotFormat string

const (
	plotFmtPng plotFormat = "png"
	plotFmtPdf plotFormat = "pdf"
	plotFmtSvg plotFormat = "svg"
)

// plotFormats — значения --plot-format в порядке справки.
var plotFormats = []plotFormat{plotFmtPng, plotFmtPdf, plotFmtSvg}

// plotBackend — способ отрисовки: внешний gnuplot или встроенный рендерер.
type plotBackend string

const (
	plottingGnuplot  plotBackend = "gnuplot"
	plottingInternal plotBackend = "internal"
)

// plotBackends — значения --plotting в порядке справки.
var plotBackends = []plotBackend{plottingInternal, plottingGnuplot}

type plotOpts struct {
	format   plotFormat
	dpi      float64
	fontSize float64
	backend  plotBackend
	con      *console
}

// warnf — предупреждение о графике; без консоли пишет в os.Stderr.
func (po *plotOpts) warnf(format string, a ...any) {
	if po != nil && po.con != nil {
		po.con.warnf(format, a...)
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, format, a...)
}

// printf — сообщение о готовом графике; без консоли пишет в os.Stdout.
func (po *plotOpts) printf(format string, a ...any) {
	if po != nil && po.con != nil {
		po.con.printf(format, a...)
		return
	}
	_, _ = fmt.Fprintf(os.Stdout, format, a...)
}

func plotFormatExt(f plotFormat) string {
	switch f {
	case plotFmtPdf:
		return "pdf"
	case plotFmtSvg:
		return "svg"
	default:
		return "png"
	}
}

// plotProfile — компактный график одной высоты пакетного режима (3 панели).
func plotProfile(po *plotOpts, outPath string, rGrid, nAvg, vCum []float64,
	ch []channel, g1, gFit []float64, h float64) bool {
	if po != nil && po.backend == plottingInternal {
		return internalProfile(po, outPath, rGrid, nAvg, vCum, ch, g1, gFit, h)
	}
	return gnuplotProfile(po, outPath, rGrid, nAvg, vCum, ch, g1, gFit, h)
}

// plotRefractiveProfile — сводный вертикальный профиль m по высотам.
func plotRefractiveProfile(po *plotOpts, outPath string, alt, mRe, mIm []float64) bool {
	if po != nil && po.backend == plottingInternal {
		return internalRefractiveProfile(po, outPath, alt, mRe, mIm)
	}
	return gnuplotRefractiveProfile(po, outPath, alt, mRe, mIm)
}

// fmtG повторяет C-формат "%.17g".
func formatG17(v float64) string {
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
		f = po.format
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

// tempDir создаёт временный каталог для вспомогательных файлов gnuplot.
func (po *plotOpts) tempDir() (string, bool) {
	dir, err := os.MkdirTemp("", "csolve_")
	if err != nil {
		po.warnf("[warning] failed to create temporary " +
			"directory for the plot\n")
		return "", false
	}
	return dir, true
}

// writeDataFile пишет служебный файл для gnuplot. Ошибка записи не прерывает
// расчёт: отрисовка всё равно завершится предупреждением.
func (po *plotOpts) writeDataFile(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		po.warnf("[warning] failed to write %s: %v\n", path, err)
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
			formatG17(g1[j]), formatG17(gFit[j]))
	}
	return b.String()
}

func cleanupPlot(dir string, files ...string) {
	for _, f := range files {
		_ = os.Remove(f)
	}
	_ = os.Remove(dir)
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

// plotProfiles — сводный вертикальный профиль нескольких серий с
// автоматическим выбором логарифмической шкалы x. Для величин вроде
// деполяризации в процентах логарифмическая шкала нежелательна, поэтому там
// используется plotProfilesStyled с logAuto=false.
func plotProfiles(po *plotOpts, outPath, title, xlabel string, nSeries int,
	alt []float64, vals [][]float64, names []string) {
	plotProfilesStyled(po, outPath, title, xlabel, nSeries, alt, vals,
		names, nil, true)
}

// plotProfilesStyled — то же, что plotProfiles, но с пометкой штриховых
// серий: dashed[s] рисует серию штриховой линией (измеренные величины).
func plotProfilesStyled(po *plotOpts, outPath, title, xlabel string,
	nSeries int, alt []float64, vals [][]float64, names []string,
	dashed []bool, logAuto bool) bool {
	if po != nil && po.backend == plottingInternal {
		return internalProfiles(po, outPath, title, xlabel, nSeries, alt, vals,
			names, dashed, logAuto)
	}
	return gnuplotProfilesStyled(po, outPath, title, xlabel, nSeries, alt, vals,
		names, dashed, logAuto)
}

// profileAxes вычисляет дополненные отступом диапазоны по значениям (x) и по
// высоте (y) и автоматическое решение о логарифмической шкале x.
func profileAxes(alt []float64, vals [][]float64, nSeries int,
	logAuto bool) (lo, hi, hLo, hHi float64, useLog bool) {
	n := len(alt)
	lo, hi = math.Inf(1), math.Inf(-1)
	hLo, hHi = math.Inf(1), math.Inf(-1)
	found := false
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
			found = true
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
	if !found {
		lo, hi = 0, 1
	}
	useLog = logAuto && lo > 0 && hi > 0 && hi/lo > 30.0
	mag := math.Max(math.Abs(lo), math.Abs(hi))
	switch {
	case useLog:
		lo /= 1.3
		hi *= 1.3
	case hi > lo:
		pad := 0.08 * (hi - lo)
		if 0.05*mag > pad {
			pad = 0.05 * mag
		}
		lo -= pad
		hi += pad
	default:
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
	return lo, hi, hLo, hHi, useLog
}

// altitudeOrder возвращает индексы точек в порядке возрастания высоты.
func altitudeOrder(alt []float64) []int {
	idx := make([]int, len(alt))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int { return compareFloats(alt[a], alt[b]) })
	return idx
}

// refractiveAxes вычисляет дополненные отступом диапазоны Re(m), Im(m) и
// высоты для сводного профиля.
func refractiveAxes(alt, mRe, mIm []float64) (reLo, reHi, imLo, imHi, hLo, hHi float64) {
	n := len(alt)
	reLo, reHi = mRe[0], mRe[0]
	imLo, imHi = mIm[0], mIm[0]
	hLo, hHi = alt[0], alt[0]
	for i := 1; i < n; i++ {
		reLo = math.Min(reLo, mRe[i])
		reHi = math.Max(reHi, mRe[i])
		imLo = math.Min(imLo, mIm[i])
		imHi = math.Max(imHi, mIm[i])
		hLo = math.Min(hLo, alt[i])
		hHi = math.Max(hHi, alt[i])
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
	return reLo - rePad, reHi + rePad, imLo - imPad, imHi + imPad,
		hLo - hPad, hHi + hPad
}

// plotResults — основной график одиночного режима (3 панели).
func plotResults(po *plotOpts, outPath string, rGrid []float64,
	nTrue, nAvg, nAvgRaw, vCum []float64, nBest [][]float64, nBestCount int,
	ch []channel, g1, gFit []float64, mAvgRe, mAvgIm float64) bool {
	if po != nil && po.backend == plottingInternal {
		ok := internalResults(po, outPath, rGrid, nTrue, nAvg, nAvgRaw, vCum,
			nBest, nBestCount, ch, g1, gFit, mAvgRe, mAvgIm)
		if ok {
			po.printf("plot saved: %s\n", outPath)
		}
		return ok
	}
	return gnuplotResults(po, outPath, rGrid, nTrue, nAvg, nAvgRaw, vCum, nBest,
		nBestCount, ch, g1, gFit, mAvgRe, mAvgIm)
}
