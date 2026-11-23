package solve

import (
	"math"
	"sort"
)

// maxSpectrumCurves — сколько индивидуальных решений рисуется на графике
// спектра. Огибающие и среднее всегда считаются по всему ансамблю.
const maxSpectrumCurves = 64

// wlPoint — измеренная точка спектральной зависимости.
type wlPoint struct {
	wlNm  float64
	value float64
}

// spectrumSet — кривые одной величины: индивидуальные решения ансамбля
// (curves — не более maxSpectrumCurves для графика), их среднее и огибающие,
// а также измеренные точки входных данных.
type spectrumSet struct {
	curves          [][]float64 // [решение][длина волны]
	mean, low, high []float64   // по всему ансамблю; NaN — нет данных
	meas            []wlPoint
	measName        string
}

// usable — есть ли хоть одно конечное значение средней кривой.
func (s *spectrumSet) usable() bool {
	if s == nil {
		return false
	}
	for _, v := range s.mean {
		if isFinite(v) {
			return true
		}
	}
	return false
}

// spectra — спектральные зависимости α(λ), β(λ), δ_l(λ), посчитанные по
// каждому решению ансамбля на общей сетке длин волн.
type spectra struct {
	wlNm  []float64
	alpha *spectrumSet
	beta  *spectrumSet
	delta *spectrumSet
	g     int // число решений в ансамбле
}

// spectrumPanel — одна панель графика спектра.
type spectrumPanel struct {
	title, ylabel string
	wl            []float64
	set           *spectrumSet
}

// namedPanel — панель спектра вместе с суффиксом имени файла.
type namedPanel struct {
	suffix string
	panel  spectrumPanel
}

// namedPanels — доступные панели с суффиксами alpha/beta/depol.
func (sp *spectra) namedPanels() []namedPanel {
	if sp == nil {
		return nil
	}
	var out []namedPanel
	add := func(suffix, title, ylabel string, set *spectrumSet) {
		if set != nil && set.usable() {
			out = append(out, namedPanel{suffix: suffix, panel: spectrumPanel{
				title: title, ylabel: ylabel, wl: sp.wlNm, set: set}})
		}
	}
	add("alpha", "Extinction coefficient α(λ)", "α, 1/m", sp.alpha)
	add("beta", "Backscatter coefficient β(λ)", "β, 1/(m·sr)", sp.beta)
	add("depol", "Linear depolarization δ_l(λ)", "δ_l, %", sp.delta)
	return out
}

// panels — доступные панели (нет данных — панель пропускается).
func (sp *spectra) panels() []spectrumPanel {
	np := sp.namedPanels()
	out := make([]spectrumPanel, len(np))
	for i := range np {
		out[i] = np[i].panel
	}
	return out
}

func newMatrix(rows, cols int) [][]float64 {
	m := make([][]float64, rows)
	for i := range m {
		m[i] = make([]float64, cols)
	}
	return m
}

// buildSpectra — спектры α(λ), β(λ), δ_l(λ) по первым G решениям ансамбля.
//
// Для каждого решения t и каждой длины волны λ:
//
//	α_t(λ) = Σ_q w_q K_ext(λ, r_q) q(r_q) n_t(r_q),   q(r) = 3/(4πr⁴)
//	β_t(λ) = то же с Cback (с учётом --table-cback)
//	δ_t(λ) = (D11-D22)/(D11+D22)·100,  D = Σ_q w_q S11/22(λ, r_q) q n_t
//
// где n_t — dV/dln r решения t (ровно, без интерполяции по выходной сетке),
// K = µm²→m² · (λ/λ_ref)² · C(2πr/λ) — та же формула, что у ядер каналов.
// Возвращает nil, если спектры отключены или нет длин волн.
func buildSpectra(a *Args, sols *solutions, geom []*geom,
	rq, wq []float64, ds *depolSet, src *tableSource, ch []channel,
	g1, depWl, depVal []float64) *spectra {
	if a == nil || !a.Spectra || src == nil || sols == nil || sols.n == 0 ||
		len(rq) == 0 || len(geom) == 0 {
		return nil
	}
	wlNm, ok := spectrumWavelengths(a, ch, depWl, ds)
	if !ok {
		return nil
	}
	G := a.bestCount(sols.n)
	if G > sols.n {
		G = sols.n
	}
	if G < 1 {
		return nil
	}

	ko := normalizeKernelOrder(a.KernelOrder)
	cbackScale := 1.0
	if a.TableCback4pi {
		cbackScale = 1.0 / (4.0 * math.Pi)
	}

	// распределение каждого решения на узлах квадратуры (dV/dln r)
	vol := make([][]float64, G)
	tabs := make([]*table, G)
	for t := 0; t < G; t++ {
		s := &sols.sol[t]
		gi := s.geomIndex
		if gi < 0 || gi >= len(geom) || geom[gi] == nil {
			return nil
		}
		vol[t] = make([]float64, len(rq))
		for k, r := range rq {
			vol[t][k] = distAt(s, geom[gi], r) * kernelToVolumeRatio(ko, r)
		}
		tab, err := tableCacheGet(src, s.mRe, s.mIm, a.TableExtrapolate)
		if err != nil {
			return nil
		}
		tabs[t] = tab
	}

	// ядра зависят только от таблицы (m), поэтому считаем их один раз на m
	uniqTabs := make([]*table, 0, G)
	uniqIdx := make(map[*table]int, G)
	tabOf := make([]int, G)
	for t, tab := range tabs {
		ui, ok := uniqIdx[tab]
		if !ok {
			ui = len(uniqTabs)
			uniqIdx[tab] = ui
			uniqTabs = append(uniqTabs, tab)
		}
		tabOf[t] = ui
	}
	nu := len(uniqTabs)
	ext := newMatrix(nu, len(rq))
	back := newMatrix(nu, len(rq))
	s11 := newMatrix(nu, len(rq))
	s22 := newMatrix(nu, len(rq))

	nw := len(wlNm)
	alphaV := newMatrix(G, nw)
	betaV := newMatrix(G, nw)
	deltaV := newMatrix(G, nw)
	useDepol := depolSetAvailable(ds)
	refWl := a.TableRefWl

	for iw := 0; iw < nw; iw++ {
		wlUm := wlNm[iw] * 1e-3
		sc := um2ToM2 * math.Pow(wlUm/refWl, 2.0)
		for ui, tab := range uniqTabs {
			canExt := tab.hasCext && tab.spCext != nil
			canBack := tab.hasCback && tab.spCback != nil
			canS := useDepol && tab.hasS11 && tab.hasS22 &&
				tab.spS11 != nil && tab.spS22 != nil
			for k, r := range rq {
				x := 2.0 * math.Pi * r / wlUm
				ext[ui][k] = math.NaN()
				back[ui][k] = math.NaN()
				s11[ui][k] = math.NaN()
				s22[ui][k] = math.NaN()
				if canExt {
					ext[ui][k] = sc * spectrumColumn(tab, tab.spCext, x,
						wlUm, src.con)
				}
				if canBack {
					back[ui][k] = sc * cbackScale *
						spectrumColumn(tab, tab.spCback, x, wlUm, src.con)
				}
				if canS {
					s11[ui][k] = sc * spectrumColumn(tab, tab.spS11, x,
						wlUm, src.con)
					s22[ui][k] = sc * spectrumColumn(tab, tab.spS22, x,
						wlUm, src.con)
				}
			}
		}
		for t := 0; t < G; t++ {
			ui := tabOf[t]
			sumE, sumB, sum11, sum22 := 0.0, 0.0, 0.0, 0.0
			for k := range rq {
				w := wq[k] * qDvol(rq[k]) * vol[t][k]
				if w == 0 {
					continue
				}
				sumE += w * ext[ui][k]
				sumB += w * back[ui][k]
				sum11 += w * s11[ui][k]
				sum22 += w * s22[ui][k]
			}
			alphaV[t][iw] = sumE
			betaV[t][iw] = sumB
			if isFinite(sum11) && isFinite(sum22) && sum11+sum22 > 0 {
				deltaV[t][iw] = (sum11 - sum22) / (sum11 + sum22) * 100.0
			} else {
				deltaV[t][iw] = math.NaN()
			}
		}
	}

	alpha := aggregateSpectrum(alphaV, wlNm, maxSpectrumCurves)
	beta := aggregateSpectrum(betaV, wlNm, maxSpectrumCurves)
	delta := aggregateSpectrum(deltaV, wlNm, maxSpectrumCurves)
	alpha.meas, beta.meas, delta.meas = measuredSpectrumPoints(ch, g1, depWl,
		depVal)
	alpha.measName = "measured α"
	beta.measName = "measured β"
	delta.measName = "measured δ_l"

	return &spectra{wlNm: wlNm, alpha: alpha, beta: beta, delta: delta, g: G}
}

// spectrumColumn — значение табличного столбца в точке x. В отличие от ядер
// каналов, здесь x зажимается в диапазон таблицы: спектры диагностические и
// на восстановление не влияют. Предупреждение — один раз на таблицу.
func spectrumColumn(t *table, sp *spline, x, wlUm float64, c *console) float64 {
	xc := x
	if xc < t.xMin {
		xc = t.xMin
	}
	if xc > t.xMax {
		xc = t.xMax
	}
	if xc != x && !t.specClampWarned {
		t.specClampWarned = true
		if c != nil {
			c.warnf("[warning] %s: spectrum requested at x = %g outside "+
				"[%g, %g] for λ = %g nm — x is clamped; the spectrum may be "+
				"biased (a wider table is needed)\n", t.path, x, t.xMin,
				t.xMax, wlUm*1e3)
		}
	}
	return math.Exp(sp.eval(math.Log(xc)))
}

// spectrumWavelengths — сетка длин волн (нм) для спектров: диапазон всех
// входных длин волн (каналы, измеренная δ_l, длины волн ядер δ_l) с
// добавлением самих входных точек. --spectrum-wl-min/--spectrum-wl-max
// переопределяют границы, --spectrum-points — число точек.
func spectrumWavelengths(a *Args, ch []channel, depWl []float64,
	ds *depolSet) ([]float64, bool) {
	lo, hi := math.Inf(1), math.Inf(-1)
	var pts []float64
	add := func(wlUm float64) {
		if !(wlUm > 0) || !isFinite(wlUm) {
			return
		}
		nm := wlUm * 1e3
		pts = append(pts, nm)
		lo = math.Min(lo, nm)
		hi = math.Max(hi, nm)
	}
	for j := range ch {
		add(ch[j].wlUm)
	}
	for _, wl := range depWl {
		add(wl)
	}
	if ds != nil {
		for _, wl := range ds.wl {
			add(wl)
		}
	}
	if a.SpectrumWlMin > 0 {
		lo = a.SpectrumWlMin
	}
	if a.SpectrumWlMax > 0 {
		hi = a.SpectrumWlMax
	}
	if math.IsInf(lo, 1) || math.IsInf(hi, -1) {
		return nil, false
	}
	if !(hi > lo) {
		// одна длина волны: расширяем диапазон, чтобы кривая была видна
		lo *= 0.8
		hi *= 1.25
	}
	if !(hi > lo) {
		return nil, false
	}
	n := a.SpectrumPoints
	if n < 2 {
		n = 2
	}
	grid := linspace(lo, hi, n)
	grid = append(grid, pts...)
	sort.Float64s(grid)
	return dedupeSpectrumGrid(grid), true
}

// dedupeSpectrumGrid — удаление дублей из отсортированной сетки длин волн
// (сравнение с относительным допуском).
func dedupeSpectrumGrid(v []float64) []float64 {
	out := v[:0]
	for _, x := range v {
		if !isFinite(x) {
			continue
		}
		if len(out) > 0 &&
			math.Abs(x-out[len(out)-1]) <= 1e-6*math.Max(1.0, math.Abs(x)) {
			continue
		}
		out = append(out, x)
	}
	return out
}

// aggregateSpectrum — среднее и огибающие по решениям (только конечные
// значения); curves обрезаются до maxSpectrumCurves для графика.
func aggregateSpectrum(vals [][]float64, wlNm []float64, maxCurves int) *spectrumSet {
	nw := len(wlNm)
	s := &spectrumSet{
		mean: make([]float64, nw),
		low:  make([]float64, nw),
		high: make([]float64, nw),
	}
	nc := len(vals)
	if nc > maxCurves {
		nc = maxCurves
	}
	s.curves = make([][]float64, nc)
	for t := 0; t < nc; t++ {
		s.curves[t] = vals[t]
	}
	for i := 0; i < nw; i++ {
		mn, mx := math.Inf(1), math.Inf(-1)
		sum, cnt := 0.0, 0
		for _, v := range vals {
			x := v[i]
			if !isFinite(x) {
				continue
			}
			sum += x
			cnt++
			if x < mn {
				mn = x
			}
			if x > mx {
				mx = x
			}
		}
		if cnt == 0 {
			s.mean[i], s.low[i], s.high[i] = math.NaN(), math.NaN(), math.NaN()
			continue
		}
		s.mean[i] = sum / float64(cnt)
		s.low[i], s.high[i] = mn, mx
	}
	return s
}

// measuredSpectrumPoints — измеренные точки входных данных по каналам и
// измеренной δ_l (нефинитные значения пропускаются).
func measuredSpectrumPoints(ch []channel, g1, depWl, depVal []float64) (
	alpha, beta, delta []wlPoint) {
	for j := range ch {
		if j >= len(g1) || !isFinite(g1[j]) {
			continue
		}
		p := wlPoint{wlNm: ch[j].wlUm * 1e3, value: g1[j]}
		if ch[j].kind == chBeta {
			beta = append(beta, p)
		} else {
			alpha = append(alpha, p)
		}
	}
	for k, wl := range depWl {
		if k >= len(depVal) || !isFinite(depVal[k]) || !(wl > 0) {
			continue
		}
		delta = append(delta, wlPoint{wlNm: wl * 1e3, value: depVal[k]})
	}
	return alpha, beta, delta
}
