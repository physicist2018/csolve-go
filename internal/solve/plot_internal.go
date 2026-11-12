package solve

// Встроенный рендерер графиков: PDF средствами Go (gonum.org/v1/plot +
// собственный UTF-8 canvas), без внешнего gnuplot и без cgo. Шрифт —
// DejaVu Sans: латиница, греческий, кириллица, µ, ³, ⁻, ⟨⟩.

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"strings"
	"sync"

	"codeberg.org/go-fonts/dejavu/dejavusans"
	"golang.org/x/image/font/opentype"

	"gonum.org/v1/plot"
	plotfont "gonum.org/v1/plot/font"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
)

// internalFontPt — размер шрифта по умолчанию (как "DejaVu Sans,10" в
// gnuplot-скриптах.
const internalFontPt = 10.0

// internalTypeface — шрифт встроенного рендерера: DejaVu Sans покрывает
// латиницу, греческий, кириллицу, µ, ³, ⁻ и ⟨⟩.
const internalTypeface = "DejaVu Sans"

var (
	internalFontsOnce sync.Once
	// internalNoGlyphs включается, если DejaVu недоступен и приходится
	// использовать Liberation Sans без ⁻ и ⟨⟩.
	internalNoGlyphs bool
)

// initInternalPlotFonts регистрирует DejaVu Sans в кэше шрифтов gonum/plot.
func initInternalPlotFonts() {
	internalFontsOnce.Do(func() {
		face, err := opentype.Parse(dejavusans.TTF)
		if err != nil {
			internalNoGlyphs = true
			plot.DefaultFont = plotfont.Font{Typeface: "Liberation",
				Variant: "Sans"}
			return
		}
		plotfont.DefaultCache.Add(plotfont.Collection{{
			Font: plotfont.Font{Typeface: internalTypeface},
			Face: face,
		}})
		plot.DefaultFont = plotfont.Font{Typeface: internalTypeface}
	})
}

// Палитра и маркеры повторяют gnuplot-оформление.
var internalPalette = []color.RGBA{
	{0x1f, 0x77, 0xb4, 0xff},
	{0xd6, 0x27, 0x28, 0xff},
	{0x2c, 0xa0, 0x2c, 0xff},
	{0x94, 0x67, 0xbd, 0xff},
	{0xff, 0x7f, 0x0e, 0xff},
	{0x8c, 0x56, 0x4b, 0xff},
}

var internalGlyphs = []draw.GlyphDrawer{
	draw.RingGlyph{}, draw.SquareGlyph{}, draw.TriangleGlyph{},
	draw.PyramidGlyph{}, draw.PlusGlyph{}, draw.CrossGlyph{},
}

// internalReplacer — запасной вариант для шрифта без глифов ⁻ и ⟨⟩.
var internalReplacer = strings.NewReplacer(
	"⁻³", "^-3", "⁻²", "^-2", "⁻¹", "^-1",
	"⟨", "<", "⟩", ">",
)

// internalLabel раскрывает gnuplot-разметку (S_{355}, δ_l^{meas}); подмена
// отдельных символов нужна только при запасном шрифте.
func internalLabel(s string) string {
	if internalNoGlyphs {
		s = internalReplacer.Replace(s)
	}
	if !strings.ContainsAny(s, "^_") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c == '^' || c == '_') && i+1 < len(s) && s[i+1] == '{' {
			b.WriteByte(c)
			i += 2
			for i < len(s) && s[i] != '}' {
				b.WriteByte(s[i])
				i++
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func internalColor(i int) color.Color { return internalPalette[i%len(internalPalette)] }

func internalGlyph(i int) draw.GlyphDrawer { return internalGlyphs[i%len(internalGlyphs)] }

// internalNewPlot создаёт график с оформлением, близким к gnuplot-скриптам.
func internalNewPlot(po *plotOpts) *plot.Plot {
	initInternalPlotFonts()
	p := plot.New()
	fs := internalFontPt
	if po != nil && po.fontSize > 0 {
		fs = po.fontSize
	}
	small := vg.Points(math.Max(fs-1, 4))
	p.Title.TextStyle.Font.Size = vg.Points(fs + 1)
	p.X.Label.TextStyle.Font.Size = vg.Points(fs)
	p.Y.Label.TextStyle.Font.Size = vg.Points(fs)
	p.X.Tick.Label.Font.Size = small
	p.Y.Tick.Label.Font.Size = small
	p.Legend.TextStyle.Font.Size = small
	p.Legend.Top = true
	p.Legend.Left = false
	p.Add(plotter.NewGrid())
	return p
}

// internalRender рисует PDF; паника внутри рендерера не прерывает расчёт —
// печатается предупреждение, как у gnuplot-ветки.
func internalRender(outPath string, w, h vg.Length, drawFn func(draw.Canvas)) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[warning] failed to build plot %s: %v\n",
				outPath, r)
			ok = false
		}
	}()

	canvas := newPDFCanvas(w, h)
	drawFn(draw.NewCanvas(canvas, w, h))

	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[warning] failed to build plot %s: %v\n",
			outPath, err)
		return false
	}
	if _, err := canvas.WriteTo(f); err != nil {
		f.Close()
		_ = os.Remove(outPath)
		fmt.Fprintf(os.Stderr, "[warning] failed to build plot %s: %v\n",
			outPath, err)
		return false
	}
	if err := f.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "[warning] failed to build plot %s: %v\n",
			outPath, err)
		return false
	}
	return true
}

// internalSegments разбивает серию на непрерывные участки по нефинитным
// значениям (аналог пропусков NaN в gnuplot).
func internalSegments(xs, ys []float64) []plotter.XYs {
	var out []plotter.XYs
	var cur plotter.XYs
	flush := func() {
		if len(cur) > 0 {
			out = append(out, cur)
			cur = nil
		}
	}
	for i := 0; i < len(xs) && i < len(ys); i++ {
		if !isFinite(xs[i]) || !isFinite(ys[i]) {
			flush()
			continue
		}
		cur = append(cur, plotter.XY{X: xs[i], Y: ys[i]})
	}
	flush()
	return out
}

// internalAddSeries добавляет линию (и маркеры), при необходимости — с
// записью в легенду. Каждая серия рисуется своим цветом/маркером.
func internalAddSeries(p *plot.Plot, xs, ys []float64, clr color.Color,
	width vg.Length, markers bool, glyph draw.GlyphDrawer, dashed bool,
	name string, legend bool) {
	segs := internalSegments(xs, ys)
	if len(segs) == 0 {
		return
	}
	ls := draw.LineStyle{Color: clr, Width: width}
	if dashed {
		ls.Dashes = []vg.Length{vg.Points(4), vg.Points(3)}
	}
	var thumb plot.Thumbnailer
	for _, seg := range segs {
		if len(seg) >= 2 {
			l, err := plotter.NewLine(seg)
			if err != nil {
				continue
			}
			l.LineStyle = ls
			p.Add(l)
			if thumb == nil {
				thumb = l
			}
		}
		if markers {
			sc, err := plotter.NewScatter(seg)
			if err != nil {
				continue
			}
			sc.GlyphStyle = draw.GlyphStyle{Color: clr, Radius: vg.Points(2),
				Shape: glyph}
			p.Add(sc)
			if thumb == nil {
				thumb = sc
			}
		}
	}
	if legend && thumb != nil && name != "" {
		p.Legend.Add(name, thumb)
	}
}

// internalAddBars рисует сгруппированные столбцы с подписями по каналам.
// dataW — оценка ширины области данных панели (vg.Length): по ней
// подбираются ширина столбца и поля, чтобы крайние группы не обрезались
// (gonum/plot не учитывает Offset в DataRange).
func internalAddBars(p *plot.Plot, labels []string, cols [][]float64,
	names []string, colors []color.Color, dataW vg.Length) {
	n := len(labels)
	k := len(cols)
	if n == 0 || k == 0 {
		return
	}
	if dataW <= 0 {
		dataW = vg.Points(240)
	}
	unit := dataW / vg.Length(n+2)
	width := unit / vg.Length(k) * 0.8
	if width > vg.Points(16) {
		width = vg.Points(16)
	}
	if width < vg.Points(4) {
		width = vg.Points(4)
	}
	total := vg.Length(k) * width

	lo, hi := 0.0, 0.0
	for _, vals := range cols {
		for _, v := range vals {
			if !isFinite(v) {
				continue
			}
			lo = math.Min(lo, v)
			hi = math.Max(hi, v)
		}
	}
	if hi > lo {
		hi += 0.06 * (hi - lo)
	} else {
		hi = 1
	}

	for j := range cols {
		v := make(plotter.Values, len(cols[j]))
		for i := range cols[j] {
			if isFinite(cols[j][i]) {
				v[i] = cols[j][i]
			}
		}
		b, err := plotter.NewBarChart(v, width)
		if err != nil {
			continue
		}
		b.Color = colors[j%len(colors)]
		b.LineStyle = draw.LineStyle{Color: color.Gray{Y: 0x55},
			Width: vg.Points(0.5)}
		b.Offset = vg.Length(j)*width - total/2 + width/2
		p.Add(b)
		if j < len(names) && names[j] != "" {
			p.Legend.Add(names[j], b)
		}
	}
	p.NominalX(labels...)
	p.X.Min, p.X.Max = -1, float64(n)
	p.Y.Min, p.Y.Max = lo, hi
}

// internalDrawTitle рисует общий заголовок по центру верхней полосы.
func internalDrawTitle(c draw.Canvas, txt string, po *plotOpts) {
	fs := internalFontPt + 1
	if po != nil && po.fontSize > 0 {
		fs = po.fontSize + 1
	}
	fnt := plotfont.From(plot.DefaultFont, vg.Points(fs))
	width, height, _ := plot.DefaultTextHandler.Box(txt, fnt)
	c.SetColor(color.Black)
	c.FillString(plotfont.DefaultCache.Lookup(fnt, fnt.Size), vg.Point{
		X: (c.Min.X + c.Max.X - width) / 2,
		Y: c.Max.Y - height,
	}, txt)
}

// internalRanges вычисляет дополненные отступом диапазоны по набору точек;
// для логарифмической шкалы учитываются только положительные x.
func internalRanges(xs []float64, logX bool, yss ...[]float64) (xLo, xHi,
	yLo, yHi float64, ok bool) {
	xLo, xHi = math.Inf(1), math.Inf(-1)
	yLo, yHi = math.Inf(1), math.Inf(-1)
	for _, x := range xs {
		if !isFinite(x) || (logX && x <= 0) {
			continue
		}
		xLo = math.Min(xLo, x)
		xHi = math.Max(xHi, x)
	}
	for _, ys := range yss {
		for _, y := range ys {
			if !isFinite(y) {
				continue
			}
			yLo = math.Min(yLo, y)
			yHi = math.Max(yHi, y)
		}
	}
	if math.IsInf(xLo, 1) {
		return 0, 0, 0, 0, false
	}
	if math.IsInf(yLo, 1) {
		// Данных нет, но оси должны быть корректными: пустая панель
		// лучше паники в логарифмической шкале.
		yLo, yHi = 0, 1
	}
	switch {
	case logX && xHi > xLo:
		xLo /= 1.05
		xHi *= 1.05
	case logX:
		xLo *= 0.9
		xHi *= 1.1
	case xHi > xLo:
		pad := 0.04 * (xHi - xLo)
		xLo -= pad
		xHi += pad
	default:
		xLo -= 0.5
		xHi += 0.5
	}
	if yHi > yLo {
		pad := 0.06 * (yHi - yLo)
		yLo -= pad
		yHi += pad
	} else {
		yLo -= 0.5
		yHi += 0.5
	}
	return xLo, xHi, yLo, yHi, true
}

func internalTiles(cols int) draw.Tiles {
	return draw.Tiles{
		Rows: 1, Cols: cols,
		PadX: 10 * vg.Millimeter, PadY: 2 * vg.Millimeter,
		PadLeft: 3 * vg.Millimeter, PadRight: 3 * vg.Millimeter,
		PadTop: 2 * vg.Millimeter, PadBottom: 2 * vg.Millimeter,
	}
}

// barLabels повторяет подписи столбцов gnuplot-ветки: b355, a532 и т. п.
func barLabels(ch []channel) []string {
	out := make([]string, len(ch))
	for j := range ch {
		kind := 'a'
		if ch[j].kind == chBeta {
			kind = 'b'
		}
		out[j] = fmt.Sprintf("%c%g", kind, ch[j].wlUm*1e3)
	}
	return out
}

// internalPanelDataWidth — оценка ширины области данных панели (без осей и
// подписей), нужна для подбора ширины столбцов.
func internalPanelDataWidth(w vg.Length, cols int) vg.Length {
	return w/vg.Length(cols) - vg.Points(70)
}

// internalProfile — компактный график одной высоты пакетного режима.
func internalProfile(po *plotOpts, outPath string, rGrid, nAvg []float64,
	ch []channel, g1, gFit []float64, h float64) bool {
	if len(rGrid) == 0 || len(nAvg) == 0 {
		return false
	}
	dv := make([]float64, len(rGrid))
	for i := range rGrid {
		v := nAvg[i] / cm3PerM3
		switch {
		case !isFinite(v):
			dv[i] = math.NaN()
		case v > 1e-30:
			dv[i] = v
		default:
			dv[i] = 1e-30
		}
	}

	return internalRender(outPath, 8.0*vg.Inch, 3.0667*vg.Inch, func(c draw.Canvas) {
		tiles := internalTiles(2)

		p1 := internalNewPlot(po)
		p1.Title.Text = fmt.Sprintf("H = %g m", h)
		p1.X.Label.Text = internalLabel("r, µm")
		p1.Y.Label.Text = internalLabel("dV/dln r, µm³/cm³")
		p1.X.Scale = plot.LogScale{}
		p1.X.Tick.Marker = plot.LogTicks{}
		if xLo, xHi, yLo, yHi, ok := internalRanges(rGrid, true, dv); ok {
			p1.X.Min, p1.X.Max = xLo, xHi
			p1.Y.Min, p1.Y.Max = yLo, yHi
		}
		internalAddSeries(p1, rGrid, dv, internalColor(0), vg.Points(2), false,
			nil, false, "", false)
		p1.Draw(tiles.At(c, 0, 0))

		p2 := internalNewPlot(po)
		p2.Y.Label.Text = internalLabel("coefficient")
		internalAddBars(p2, barLabels(ch), [][]float64{g1, gFit},
			[]string{"measured g1", "computed g"},
			[]color.Color{internalColor(0), internalColor(1)},
			internalPanelDataWidth(8.0*vg.Inch, 2))
		p2.Draw(tiles.At(c, 1, 0))
	})
}

// internalProfiles — сводный вертикальный профиль нескольких серий.
func internalProfiles(po *plotOpts, outPath, title, xlabel string, nSeries int,
	alt []float64, vals [][]float64, names []string, dashed []bool,
	logAuto bool) bool {
	if len(alt) <= 0 || nSeries <= 0 {
		return false
	}
	lo, hi, hLo, hHi, useLog := profileAxes(alt, vals, nSeries, logAuto)

	return internalRender(outPath, 6.0*vg.Inch, 4.8*vg.Inch, func(c draw.Canvas) {
		p := internalNewPlot(po)
		p.Title.Text = internalLabel(title)
		p.X.Label.Text = internalLabel(xlabel)
		p.Y.Label.Text = internalLabel("Altitude H, m")
		p.X.Min, p.X.Max = lo, hi
		p.Y.Min, p.Y.Max = hLo, hHi
		if useLog {
			p.X.Scale = plot.LogScale{}
			p.X.Tick.Marker = plot.LogTicks{}
		}
		for s := 0; s < nSeries; s++ {
			name := ""
			if s < len(names) {
				name = internalLabel(names[s])
			}
			d := s < len(dashed) && dashed[s]
			internalAddSeries(p, vals[s], alt, internalColor(s), vg.Points(1.8),
				true, internalGlyph(s), d, name, true)
		}
		p.Draw(c)
	})
}

// internalRefractiveProfile — профиль Re(m)/Im(m) по высотам: две панели
// (gnuplot-версия использует две оси X, здесь — две панели).
func internalRefractiveProfile(po *plotOpts, outPath string, alt, mRe, mIm []float64) bool {
	n := len(alt)
	if n <= 0 {
		return false
	}
	idx := altitudeOrder(alt)
	ys := make([]float64, n)
	xsRe := make([]float64, n)
	xsIm := make([]float64, n)
	for k, i := range idx {
		ys[k] = alt[i]
		xsRe[k] = mRe[i]
		xsIm[k] = mIm[i]
	}
	reLo, reHi, imLo, imHi, hLo, hHi := refractiveAxes(alt, mRe, mIm)

	const titleBand = 8 * vg.Millimeter
	return internalRender(outPath, 6.0*vg.Inch, 4.8*vg.Inch, func(c draw.Canvas) {
		body := c
		body.Max.Y -= titleBand
		tiles := draw.Tiles{
			Rows: 1, Cols: 2,
			PadX: 8 * vg.Millimeter, PadY: 2 * vg.Millimeter,
			PadLeft: 2 * vg.Millimeter, PadRight: 2 * vg.Millimeter,
			PadTop: 2 * vg.Millimeter, PadBottom: 2 * vg.Millimeter,
		}

		pRe := internalNewPlot(po)
		pRe.X.Label.Text = "Re(m)"
		pRe.Y.Label.Text = internalLabel("Altitude H, m")
		pRe.X.Min, pRe.X.Max = reLo, reHi
		pRe.Y.Min, pRe.Y.Max = hLo, hHi
		internalAddSeries(pRe, xsRe, ys, internalColor(0), vg.Points(1.8), true,
			internalGlyph(0), false, "", false)
		pRe.Draw(tiles.At(body, 0, 0))

		pIm := internalNewPlot(po)
		pIm.X.Label.Text = "Im(m)"
		pIm.Y.Label.Text = internalLabel("Altitude H, m")
		pIm.X.Min, pIm.X.Max = imLo, imHi
		pIm.Y.Min, pIm.Y.Max = hLo, hHi
		internalAddSeries(pIm, xsIm, ys, internalColor(1), vg.Points(1.8), true,
			internalGlyph(1), false, "", false)
		pIm.Draw(tiles.At(body, 1, 0))

		internalDrawTitle(c, internalLabel("Refractive index vs altitude"), po)
	})
}

// internalResults — основной график одиночного режима (2 панели).
func internalResults(po *plotOpts, outPath string, rGrid []float64,
	nTrue, nAvg, nAvgRaw []float64, nBest [][]float64, nBestCount int,
	ch []channel, g1, gFit []float64, mAvgRe, mAvgIm float64) bool {
	if len(rGrid) == 0 {
		return false
	}
	scale := func(v []float64) []float64 {
		out := make([]float64, len(v))
		for i := range v {
			out[i] = v[i] / cm3PerM3
			if !isFinite(out[i]) {
				out[i] = math.NaN()
			}
		}
		return out
	}
	mean := scale(nAvg)
	raw := scale(nAvgRaw)
	var tru []float64
	if nTrue != nil {
		tru = scale(nTrue)
	}
	ns := nBestCount
	if ns > 64 {
		ns = 64
	}
	if ns > len(nBest) {
		ns = len(nBest)
	}
	sols := make([][]float64, ns)
	for t := 0; t < ns; t++ {
		sols[t] = scale(nBest[t])
	}

	all := make([][]float64, 0, len(sols)+3)
	all = append(all, raw, mean)
	if tru != nil {
		all = append(all, tru)
	}
	all = append(all, sols...)

	return internalRender(outPath, 8.6667*vg.Inch, 3.4667*vg.Inch, func(c draw.Canvas) {
		tiles := internalTiles(2)

		p1 := internalNewPlot(po)
		p1.X.Scale = plot.LogScale{}
		p1.X.Tick.Marker = plot.LogTicks{}
		p1.X.Label.Text = internalLabel("r, µm")
		p1.Y.Label.Text = internalLabel("dV/dln r, µm³/cm³")
		p1.Title.Text = internalLabel(fmt.Sprintf("Volume particle size "+
			"distribution, ⟨m⟩ = %.3f%+.3fj", mAvgRe, mAvgIm))
		if xLo, xHi, yLo, yHi, ok := internalRanges(rGrid, true, all...); ok {
			p1.X.Min, p1.X.Max = xLo, xHi
			p1.Y.Min, p1.Y.Max = yLo, yHi
		}
		gray := color.Gray{Y: 0xbb}
		for t := 0; t < ns; t++ {
			internalAddSeries(p1, rGrid, sols[t], gray, vg.Points(1), false,
				nil, false, "", false)
		}
		if tru != nil {
			internalAddSeries(p1, rGrid, tru, color.Black, vg.Points(1.6),
				false, nil, true, "true n(r)", true)
		}
		internalAddSeries(p1, rGrid, raw, color.RGBA{0xff, 0x8c, 0x00, 0xff},
			vg.Points(1), false, nil, true, "mean (unsmoothed)", true)
		internalAddSeries(p1, rGrid, mean, internalColor(0), vg.Points(2.6),
			false, nil, false, "mean of best solutions", true)
		p1.Draw(tiles.At(c, 0, 0))

		p2 := internalNewPlot(po)
		p2.Title.Text = internalLabel("Measured vs computed coefficients")
		p2.Y.Label.Text = internalLabel("coefficient, rel. units")
		internalAddBars(p2, barLabels(ch), [][]float64{g1, gFit},
			[]string{"measured g1", "computed g (mean)"},
			[]color.Color{internalColor(0), internalColor(1)},
			internalPanelDataWidth(8.6667*vg.Inch, 2))
		p2.Draw(tiles.At(c, 1, 0))
	})
}
