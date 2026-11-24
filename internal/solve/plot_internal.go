package solve

// Встроенный рендерер графиков: PDF средствами Go (gonum.org/v1/plot +
// собственный UTF-8 canvas), без внешнего gnuplot и без cgo. Шрифт —
// DejaVu Sans: латиница, греческий, кириллица, µ, ³, ⁻, ⟨⟩.

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"codeberg.org/go-fonts/dejavu/dejavusans"
	"golang.org/x/image/font/opentype"

	"gonum.org/v1/plot"
	plotfont "gonum.org/v1/plot/font"
	"gonum.org/v1/plot/plotter"
	plottext "gonum.org/v1/plot/text"
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

// internalPalette — палитра, повторяющая gnuplot-оформление.
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

// sciParts разбирает число на мантиссу (один знак после точки) и
// десятичный порядок; ok = false для нуля и нечисловых значений.
func sciParts(v float64) (string, int, bool) {
	if !isFinite(v) || v == 0 {
		return "", 0, false
	}
	exp := int(math.Floor(math.Log10(math.Abs(v))))
	mant := v / math.Pow(10, float64(exp))
	if math.Abs(mant) >= 9.95 {
		mant /= 10
		exp++
	}
	return strconv.FormatFloat(mant, 'f', 1, 64), exp, true
}

var superscriptDigits = []rune("⁰¹²³⁴⁵⁶⁷⁸⁹")

func superscriptInt(n int) string {
	s := strconv.Itoa(n)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '-' {
			b.WriteRune('⁻')
			continue
		}
		b.WriteRune(superscriptDigits[int(r-'0')])
	}
	return b.String()
}

// sciTickLabelInternal — подпись тика вида 4.5×10⁻⁵.
func sciTickLabelInternal(v float64) string {
	mant, exp, ok := sciParts(v)
	if !ok {
		return "0"
	}
	if internalNoGlyphs {
		return mant + "×10^" + strconv.Itoa(exp)
	}
	return mant + "×10" + superscriptInt(exp)
}

// sciTicks — тики линейной оси с научными подписями (для α и β).
type sciTicks struct{}

func (sciTicks) Ticks(min, max float64) []plot.Tick {
	ticks := plot.DefaultTicks{}.Ticks(min, max)
	for i := range ticks {
		if ticks[i].Label == "" {
			continue
		}
		ticks[i].Label = sciTickLabelInternal(ticks[i].Value)
	}
	return ticks
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
func internalRender(po *plotOpts, outPath string, w, h vg.Length, drawFn func(draw.Canvas)) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			po.warnf("[warning] failed to build plot %s: %v\n",
				outPath, r)
			ok = false
		}
	}()

	canvas := newPDFCanvas(w, h)
	drawFn(draw.NewCanvas(canvas, w, h))

	f, err := os.Create(outPath)
	if err != nil {
		po.warnf("[warning] failed to build plot %s: %v\n",
			outPath, err)
		return false
	}
	if _, err := canvas.WriteTo(f); err != nil {
		f.Close()
		_ = os.Remove(outPath)
		po.warnf("[warning] failed to build plot %s: %v\n",
			outPath, err)
		return false
	}
	if err := f.Close(); err != nil {
		po.warnf("[warning] failed to build plot %s: %v\n",
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

// internalAddPoints рисует только маркеры (без соединяющей линии) —
// измеренные точки на графиках спектров.
func internalAddPoints(p *plot.Plot, xs, ys []float64, clr color.Color,
	glyph draw.GlyphDrawer, name string) {
	var thumb plot.Thumbnailer
	for i := 0; i < len(xs) && i < len(ys); i++ {
		if !isFinite(xs[i]) || !isFinite(ys[i]) {
			continue
		}
		sc, err := plotter.NewScatter(plotter.XYs{{X: xs[i], Y: ys[i]}})
		if err != nil {
			continue
		}
		sc.GlyphStyle = draw.GlyphStyle{Color: clr, Radius: vg.Points(3),
			Shape: glyph}
		p.Add(sc)
		if thumb == nil {
			thumb = sc
		}
	}
	if thumb != nil && name != "" {
		p.Legend.Add(name, thumb)
	}
}

// internalAddBars рисует сгруппированные столбцы с подписями по каналам.
// dataW — оценка ширины области данных панели: по ней подбираются ширина
// столбца и поля, чтобы крайние группы не обрезались (gonum/plot не
// учитывает Offset в DataRange).
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
	return internalTilesGrid(1, cols)
}

func internalTilesGrid(rows, cols int) draw.Tiles {
	return draw.Tiles{
		Rows: rows, Cols: cols,
		PadX: 10 * vg.Millimeter, PadY: 2 * vg.Millimeter,
		PadLeft: 3 * vg.Millimeter, PadRight: 3 * vg.Millimeter,
		PadTop: 2 * vg.Millimeter, PadBottom: 2 * vg.Millimeter,
	}
}

// internalSpectrumTiles — увеличенные отступы для сетки спектров 2×2:
// место под научные подписи оси Y, межколоночный зазор и заголовки нижнего
// ряда, которые иначе налезают на подписи оси X верхнего ряда.
func internalSpectrumTiles(rows, cols int, scale float64) draw.Tiles {
	k := vg.Length(scale)
	return draw.Tiles{
		Rows: rows, Cols: cols,
		PadX: 24 * vg.Millimeter * k, PadY: 20 * vg.Millimeter * k,
		PadLeft: 23 * vg.Millimeter * k, PadRight: 10 * vg.Millimeter * k,
		PadTop: 2 * vg.Millimeter * k, PadBottom: 3 * vg.Millimeter * k,
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
func internalProfile(po *plotOpts, outPath string, rGrid, nAvg, nLow, nHigh,
	vCum []float64, ch []channel, g1, gFit []float64, h float64,
	gBest int) bool {
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
	// огибающие — минимум/максимум по G лучшим решениям
	lo := make([]float64, len(rGrid))
	hi := make([]float64, len(rGrid))
	for i := range rGrid {
		lo[i], hi[i] = math.NaN(), math.NaN()
		if i < len(nLow) && isFinite(nLow[i]) {
			lo[i] = nLow[i] / cm3PerM3
		}
		if i < len(nHigh) && isFinite(nHigh[i]) {
			hi[i] = nHigh[i] / cm3PerM3
		}
	}
	cum := make([]float64, len(rGrid))
	for i := range rGrid {
		v := math.NaN()
		if i < len(vCum) {
			v = vCum[i] / cm3PerM3
		}
		if !isFinite(v) {
			v = math.NaN()
		}
		cum[i] = v
	}

	return internalRender(po, outPath, 10.0*vg.Inch, 3.0667*vg.Inch, func(c draw.Canvas) {
		tiles := internalTiles(3)

		p1 := internalNewPlot(po)
		p1.Title.Text = fmt.Sprintf("H = %g m", h)
		p1.X.Label.Text = internalLabel("r, µm")
		p1.Y.Label.Text = internalLabel("dV/dln r, µm³/cm³")
		p1.X.Scale = plot.LogScale{}
		p1.X.Tick.Marker = plot.LogTicks{}
		if xLo, xHi, yLo, yHi, ok := internalRanges(rGrid, true, dv, lo, hi); ok {
			p1.X.Min, p1.X.Max = xLo, xHi
			p1.Y.Min, p1.Y.Max = yLo, yHi
		}
		loLabel, hiLabel := envelopeLegend(gBest)
		internalAddSeries(p1, rGrid, lo, internalColor(1), vg.Points(1.4),
			false, nil, true, loLabel, true)
		internalAddSeries(p1, rGrid, hi, internalColor(1), vg.Points(1.4),
			false, nil, true, hiLabel, true)
		internalAddSeries(p1, rGrid, dv, internalColor(0), vg.Points(2), false,
			nil, false, "mean", true)
		p1.Draw(tiles.At(c, 0, 0))

		p2 := internalNewPlot(po)
		p2.Y.Label.Text = internalLabel("coefficient")
		internalAddBars(p2, barLabels(ch), [][]float64{g1, gFit},
			[]string{"measured g1", "computed g"},
			[]color.Color{internalColor(0), internalColor(1)},
			internalPanelDataWidth(10.0*vg.Inch, 3))
		p2.Draw(tiles.At(c, 1, 0))

		p3 := internalNewPlot(po)
		p3.X.Label.Text = internalLabel("r, µm")
		p3.Y.Label.Text = internalLabel("V, µm³/cm³")
		p3.X.Scale = plot.LogScale{}
		p3.X.Tick.Marker = plot.LogTicks{}
		if xLo, xHi, yLo, yHi, ok := internalRanges(rGrid, true, cum); ok {
			p3.X.Min, p3.X.Max = xLo, xHi
			p3.Y.Min, p3.Y.Max = yLo, yHi
		}
		internalAddSeries(p3, rGrid, cum, internalColor(2), vg.Points(2), false,
			nil, false, "", false)
		p3.Draw(tiles.At(c, 2, 0))
	})
}

// internalProfiles — сводный вертикальный профиль нескольких серий.
func internalProfiles(po *plotOpts, outPath, title, xlabel string, nSeries int,
	alt []float64, vals [][]float64, names []string, dashed []bool,
	logAuto bool) bool {
	if len(alt) == 0 || nSeries <= 0 {
		return false
	}
	lo, hi, hLo, hHi, useLog := profileAxes(alt, vals, nSeries, logAuto)

	return internalRender(po, outPath, 6.0*vg.Inch, 4.8*vg.Inch, func(c draw.Canvas) {
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
	return internalRender(po, outPath, 6.0*vg.Inch, 4.8*vg.Inch, func(c draw.Canvas) {
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

// internalResultsDraw — основной график одиночного режима (3 панели).
func internalResultsDraw(po *plotOpts, rGrid []float64,
	nTrue, nAvg, nAvgRaw, nLow, nHigh, vCum []float64, nBest [][]float64,
	nBestCount int, ch []channel, g1, gFit []float64, mAvgRe,
	mAvgIm float64) (func(draw.Canvas), bool) {
	if len(rGrid) == 0 {
		return nil, false
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
	low := scale(nLow)
	high := scale(nHigh)
	cum := scale(vCum)
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

	all := make([][]float64, 0, len(sols)+5)
	all = append(all, raw, mean, low, high)
	if tru != nil {
		all = append(all, tru)
	}
	all = append(all, sols...)

	return func(c draw.Canvas) {
		tiles := internalTiles(3)

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
		envCol := internalColor(1)
		loLabel, hiLabel := envelopeLegend(nBestCount)
		internalAddSeries(p1, rGrid, low, envCol, vg.Points(1.6), false,
			nil, true, loLabel, true)
		internalAddSeries(p1, rGrid, high, envCol, vg.Points(1.6), false,
			nil, true, hiLabel, true)
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
			internalPanelDataWidth(8.6667*vg.Inch, 3))
		p2.Draw(tiles.At(c, 1, 0))

		p3 := internalNewPlot(po)
		p3.X.Scale = plot.LogScale{}
		p3.X.Tick.Marker = plot.LogTicks{}
		p3.X.Label.Text = internalLabel("r, µm")
		p3.Y.Label.Text = internalLabel("V, µm³/cm³")
		p3.Title.Text = internalLabel("Cumulative volume V(r)")
		if xLo, xHi, yLo, yHi, ok := internalRanges(rGrid, true, cum); ok {
			p3.X.Min, p3.X.Max = xLo, xHi
			p3.Y.Min, p3.Y.Max = yLo, yHi
		}
		internalAddSeries(p3, rGrid, cum, internalColor(2), vg.Points(2.4),
			false, nil, false, "V(r)", true)
		p3.Draw(tiles.At(c, 2, 0))
	}, true
}

// internalSpectrumPlot — панель «величина vs длина волны»: тонкие серые
// кривые индивидуальных решений, огибающие (штриховые), среднее и измеренные
// точки. withLegend управляет повтором легенды внутри панели.
func internalSpectrumPlot(po *plotOpts, pn spectrumPanel, withLegend bool) *plot.Plot {
	p := internalNewPlot(po)
	p.Title.Text = internalLabel(pn.title)
	p.X.Label.Text = internalLabel("λ, nm")
	p.Y.Label.Text = internalLabel(pn.ylabel)
	if pn.sciY {
		p.Y.Tick.Marker = sciTicks{}
	}

	all := make([][]float64, 0, 4+len(pn.set.curves))
	all = append(all, pn.set.mean, pn.set.low, pn.set.high)
	all = append(all, pn.set.curves...)
	if len(pn.set.meas) > 0 {
		ys := make([]float64, len(pn.set.meas))
		for i, m := range pn.set.meas {
			ys[i] = m.value
		}
		all = append(all, ys)
	}
	if xLo, xHi, yLo, yHi, ok := internalRanges(pn.wl, false, all...); ok {
		p.X.Min, p.X.Max = xLo, xHi
		p.Y.Min, p.Y.Max = yLo, yHi
	}

	gray := color.Gray{Y: 0xbb}
	for _, cur := range pn.set.curves {
		internalAddSeries(p, pn.wl, cur, gray, vg.Points(0.8), false, nil,
			false, "", false)
	}
	envCol := internalColor(1)
	internalAddSeries(p, pn.wl, pn.set.low, envCol, vg.Points(1.6), false,
		nil, true, "lower envelope", withLegend)
	internalAddSeries(p, pn.wl, pn.set.high, envCol, vg.Points(1.6), false,
		nil, true, "upper envelope", withLegend)
	internalAddSeries(p, pn.wl, pn.set.mean, internalColor(0), vg.Points(2.6),
		false, nil, false, "mean of best solutions", withLegend)

	if len(pn.set.meas) > 0 {
		xs := make([]float64, len(pn.set.meas))
		ys := make([]float64, len(pn.set.meas))
		for i, m := range pn.set.meas {
			xs[i], ys[i] = m.wlNm, m.value
		}
		measName := ""
		if withLegend {
			measName = pn.set.measName
		}
		internalAddPoints(p, xs, ys, color.Black, draw.RingGlyph{}, measName)
	}
	return p
}

// internalDrawSpectrumLegend — одна общая горизонтальная легенда для сетки
// панелей. hasMeas управляет строкой «measured».
func internalDrawSpectrumLegend(c draw.Canvas, po *plotOpts, hasMeas bool) {
	fs := internalFontPt - 1
	if po != nil && po.fontSize > 0 {
		fs = math.Max(po.fontSize-1, 4)
	}
	sty := plottext.Style{
		Color:   color.Black,
		Font:    plotfont.From(plot.DefaultFont, vg.Points(fs)),
		XAlign:  plottext.XLeft,
		YAlign:  plottext.YCenter,
		Handler: plot.DefaultTextHandler,
	}
	type entry struct {
		label  string
		clr    color.Color
		width  vg.Length
		dashed bool
		glyph  bool
	}
	entries := []entry{
		{"individual solutions", color.Gray{Y: 0xbb}, vg.Points(0.8), false, false},
		{"lower envelope", internalColor(1), vg.Points(1.6), true, false},
		{"upper envelope", internalColor(1), vg.Points(1.6), true, false},
		{"mean of best solutions", internalColor(0), vg.Points(2.4), false, false},
	}
	if hasMeas {
		entries = append(entries, entry{"measured", color.Black, 0, false, true})
	}
	sampleW, gap, space := vg.Points(18), vg.Points(4), vg.Points(10)
	widths := make([]vg.Length, len(entries))
	total := vg.Length(0)
	for i, e := range entries {
		widths[i] = sty.Width(e.label)
		total += sampleW + gap + widths[i]
		if i > 0 {
			total += space
		}
	}
	x := c.Min.X + (c.Max.X-c.Min.X-total)/2
	y := (c.Min.Y + c.Max.Y) / 2
	for i, e := range entries {
		if e.glyph {
			c.DrawGlyph(draw.GlyphStyle{Color: e.clr, Radius: vg.Points(3),
				Shape: draw.RingGlyph{}}, vg.Point{X: x + sampleW/2, Y: y})
		} else {
			ls := draw.LineStyle{Color: e.clr, Width: e.width}
			if e.dashed {
				ls.Dashes = []vg.Length{vg.Points(4), vg.Points(3)}
			}
			c.StrokeLine2(ls, x, y, x+sampleW, y)
		}
		c.FillText(sty, vg.Point{X: x + sampleW + gap, Y: y}, e.label)
		x += sampleW + gap + widths[i] + space
	}
}

// internalSpectrumFigure — одна или несколько панелей «величина vs λ» в PDF.
// Несколько панелей раскладываются в сетку 2×2 по полям slot; под ними
// рисуется одна общая легенда.
func internalSpectrumFigure(po *plotOpts, outPath string, panels []spectrumPanel,
	wIn, hIn float64) bool {
	if len(panels) == 0 {
		return false
	}
	multi := len(panels) > 1
	rows, cols := 1, 1
	scale := plotFontScale(po)
	legendH := vg.Length(0)
	if multi {
		cols = 2
		for _, pn := range panels {
			if pn.slot >= 2 {
				rows = 2
				break
			}
		}
		hIn *= float64(rows)
		legendH = vg.Inch * vg.Length(spectrumLegendHIn*scale)
	}
	hasMeas := false
	for _, pn := range panels {
		if len(pn.set.meas) > 0 {
			hasMeas = true
			break
		}
	}
	w := vg.Length(wIn * float64(vg.Inch))
	h := vg.Length(hIn*float64(vg.Inch)) + legendH
	return internalRender(po, outPath, w, h, func(c draw.Canvas) {
		panelRect, legendRect := c.Rectangle, c.Rectangle
		if multi {
			panelRect.Max.Y -= legendH
			legendRect.Min.Y = legendRect.Max.Y - legendH
		}
		panelCanvas := draw.Canvas{Canvas: c.Canvas, Rectangle: panelRect}
		tiles := internalTiles(1)
		if multi {
			tiles = internalSpectrumTiles(rows, cols, scale)
		}
		for _, pn := range panels {
			col, row := 0, 0
			if multi {
				col, row = pn.slot%2, pn.slot/2
			}
			internalSpectrumPlot(po, pn, !multi).Draw(tiles.At(panelCanvas, col, row))
		}
		if multi {
			internalDrawSpectrumLegend(draw.Canvas{Canvas: c.Canvas,
				Rectangle: legendRect}, po, hasMeas)
		}
	})
}

// internalSpectrumPNG — та же панель в PNG (для встраивания в отчёт).
func internalSpectrumPNG(po *plotOpts, pn spectrumPanel) (out []byte, ok bool) {
	w := vg.Length(6.0 * float64(vg.Inch))
	h := vg.Length(4.0 * float64(vg.Inch))
	return internalDrawToPNG(po, w, h, func(c draw.Canvas) {
		internalSpectrumPlot(po, pn, true).Draw(c)
	})
}
