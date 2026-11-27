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
	"sort"
)

// spectrumLegendHIn — высота полосы общей легенды под сеткой спектров, дюймы.
const spectrumLegendHIn = 1.0

// spectrumComputedLegend — подпись расчётных точек в длинах волн без
// измерений (полые маркеры).
const spectrumComputedLegend = "computed (no measured data)"

// wlPoint — измеренная точка спектральной зависимости.
type wlPoint struct {
	wlNm  float64
	value float64
}

// spectrumSet — кривые одной величины: индивидуальные решения ансамбля
// (curves — до limit для графика, 0 — все), их среднее и огибающие, а также
// измеренные точки входных данных и расчётные значения в тех длинах волн,
// где для этой величины измерений нет.
type spectrumSet struct {
	curves          [][]float64 // [решение][длина волны]
	mean, low, high []float64   // по всему ансамблю; NaN — нет данных
	meas            []wlPoint   // измеренные точки (заполненные маркеры)
	measName        string
	computed        []wlPoint // расчёт в λ без измерения (полые маркеры)
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

// spectra — спектральные зависимости α(λ), β(λ), S(λ) = α/β и δ_l(λ),
// посчитанные по каждому решению ансамбля на общей сетке длин волн.
type spectra struct {
	wlNm  []float64
	alpha *spectrumSet
	beta  *spectrumSet
	lidar *spectrumSet
	delta *spectrumSet
	g     int // число решений в ансамбле
}

// spectrumPanel — одна панель графика спектра.
type spectrumPanel struct {
	title, ylabel string
	sciY          bool // научные подписи по оси ординат (α, β)
	slot          int  // фиксированная ячейка сетки 2×2
	wl            []float64
	set           *spectrumSet
}

// Ячейки сетки 2×2: первый ряд — α и β, второй — S и δ_l.
const (
	spectrumSlotAlpha = iota
	spectrumSlotBeta
	spectrumSlotLidar
	spectrumSlotDepol
)

// namedPanel — панель спектра вместе с суффиксом имени файла.
type namedPanel struct {
	suffix string
	panel  spectrumPanel
}

// namedPanels — доступные панели с суффиксами alpha/beta/lidar/depol.
func (sp *spectra) namedPanels() []namedPanel {
	if sp == nil {
		return nil
	}
	var out []namedPanel
	add := func(slot int, suffix, title, ylabel string, sciY bool, set *spectrumSet) {
		if set != nil && set.usable() {
			out = append(out, namedPanel{suffix: suffix, panel: spectrumPanel{
				title: title, ylabel: ylabel, sciY: sciY, slot: slot,
				wl: sp.wlNm, set: set}})
		}
	}
	add(spectrumSlotAlpha, "alpha", "Extinction coefficient α(λ)", "α, 1/m", true,
		sp.alpha)
	add(spectrumSlotBeta, "beta", "Backscatter coefficient β(λ)", "β, 1/(m·sr)",
		true, sp.beta)
	add(spectrumSlotLidar, "lidar", "Lidar ratio S(λ)", "S = α/β, sr", false,
		sp.lidar)
	add(spectrumSlotDepol, "depol", "Linear depolarization δ_l(λ)", "δ_l, %",
		false, sp.delta)
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

// buildSpectra — спектры α(λ), β(λ), S(λ) = α/β и δ_l(λ) по первым G
// решениям ансамбля.
//
// Для каждого решения t и каждой длины волны λ:
//
//	α_t(λ) = Σ_q w_q K_ext(λ, r_q) q(r_q) n_t(r_q),   q(r) = 3/(4πr⁴)
//	β_t(λ) = то же с Cback (с учётом --table-cback)
//	S_t(λ) = α_t(λ)/β_t(λ) (только где β > 0)
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
	lidarV := newMatrix(G, nw)
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

	alpha := aggregateSpectrum(alphaV, wlNm, a.PlotBestLimit)
	beta := aggregateSpectrum(betaV, wlNm, a.PlotBestLimit)
	for t := 0; t < G; t++ {
		for iw := 0; iw < nw; iw++ {
			a, b := alphaV[t][iw], betaV[t][iw]
			if isFinite(a) && isFinite(b) && b > 0 {
				v := a / b
				if isFinite(v) {
					lidarV[t][iw] = v
					continue
				}
			}
			lidarV[t][iw] = math.NaN()
		}
	}
	lidar := aggregateSpectrum(lidarV, wlNm, a.PlotBestLimit)
	delta := aggregateSpectrum(deltaV, wlNm, a.PlotBestLimit)
	alpha.meas, beta.meas, delta.meas = measuredSpectrumPoints(ch, g1, depWl,
		depVal)
	alpha.measName = "measured α"
	beta.measName = "measured β"
	lidar.meas = measuredLidarPoints(alpha.meas, beta.meas)
	lidar.measName = "measured S"
	delta.measName = "measured δ_l"

	// расчётные значения в длинах волн входных данных, где для величины нет
	// измерения (среднее по лучшим решениям); изображаются полыми маркерами
	gapWl := inputWavelengthsNm(ch, depWl)
	alpha.computed = missingSpectrumPoints(gapWl, wlNm, alpha)
	beta.computed = missingSpectrumPoints(gapWl, wlNm, beta)
	lidar.computed = missingSpectrumPoints(gapWl, wlNm, lidar)
	delta.computed = missingSpectrumPoints(gapWl, wlNm, delta)

	return &spectra{wlNm: wlNm, alpha: alpha, beta: beta, lidar: lidar,
		delta: delta, g: G}
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
// значения); curves — не более limit (0 — все). Среднее и огибающие всегда
// считаются по всему ансамблю.
func aggregateSpectrum(vals [][]float64, wlNm []float64, limit int) *spectrumSet {
	nw := len(wlNm)
	s := &spectrumSet{
		mean: make([]float64, nw),
		low:  make([]float64, nw),
		high: make([]float64, nw),
	}
	nc := len(vals)
	if limit > 0 && nc > limit {
		nc = limit
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

// measuredLidarPoints — измеренное S = α/β на общих длинах волн, где есть
// обе измеренные величины и β > 0. Значения отсортированы по λ.
func measuredLidarPoints(alpha, beta []wlPoint) []wlPoint {
	var out []wlPoint
	for _, pa := range alpha {
		if !isFinite(pa.value) {
			continue
		}
		var pb wlPoint
		found := false
		for _, b := range beta {
			if isFinite(b.value) && b.value > 0 &&
				sameWavelength(pa.wlNm, b.wlNm) {
				pb, found = b, true
				break
			}
		}
		if !found {
			continue
		}
		dup := false
		for _, p := range out {
			if sameWavelength(p.wlNm, pa.wlNm) {
				dup = true
				break
			}
		}
		if !dup {
			v := pa.value / pb.value
			if isFinite(v) {
				out = append(out, wlPoint{wlNm: pa.wlNm, value: v})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].wlNm < out[j].wlNm })
	return out
}

// sameWavelength — совпадение длин волн с относительным допуском (как в
// сетке спектров).
func sameWavelength(a, b float64) bool {
	return math.Abs(a-b) <= 1e-6*math.Max(1.0, math.Max(math.Abs(a), math.Abs(b)))
}

// inputWavelengthsNm — отсортированный уникальный набор длин волн (нм)
// входных данных: каналы (α и β) и измеренная δ_l. Это те длины волн, для
// которых вычисленные коэффициенты имеют смысл, даже если конкретная
// величина в этой точке не измерена.
func inputWavelengthsNm(ch []channel, depWl []float64) []float64 {
	var out []float64
	add := func(wlUm float64) {
		if !(wlUm > 0) || !isFinite(wlUm) {
			return
		}
		nm := wlUm * 1e3
		for _, v := range out {
			if sameWavelength(v, nm) {
				return
			}
		}
		out = append(out, nm)
	}
	for j := range ch {
		add(ch[j].wlUm)
	}
	for _, wl := range depWl {
		add(wl)
	}
	sort.Float64s(out)
	return out
}

// missingSpectrumPoints — значения средней кривой в длинах волн wls, где у
// набора нет измеренной точки. Нефинитные средние (нет данных/δ_l без
// S11-S22) пропускаются. Результат отсортирован по длине волны.
func missingSpectrumPoints(wls, grid []float64, set *spectrumSet) []wlPoint {
	if set == nil {
		return nil
	}
	var out []wlPoint
	for _, wl := range wls {
		measured := false
		for _, m := range set.meas {
			if sameWavelength(m.wlNm, wl) && isFinite(m.value) {
				measured = true
				break
			}
		}
		if measured {
			continue
		}
		v, ok := interpAtWavelength(grid, set.mean, wl)
		if !ok {
			continue
		}
		out = append(out, wlPoint{wlNm: wl, value: v})
	}
	return out
}

// interpAtWavelength — линейная интерполяция ys(xs) в точке x по конечным
// значениям. Точное (в допуске sameWavelength) совпадение с узлом сетки
// возвращается как есть; вне диапазона конечных данных — ok = false.
func interpAtWavelength(xs, ys []float64, x float64) (float64, bool) {
	n := len(xs)
	if n == 0 || len(ys) < n || !isFinite(x) {
		return 0, false
	}
	i := sort.SearchFloat64s(xs, x)
	for _, j := range []int{i - 1, i} {
		if j >= 0 && j < n && sameWavelength(xs[j], x) && isFinite(ys[j]) {
			return ys[j], true
		}
	}
	jl, jr := -1, -1
	for j := i - 1; j >= 0; j-- {
		if isFinite(ys[j]) {
			jl = j
			break
		}
	}
	for j := i; j < n; j++ {
		if isFinite(xs[j]) && isFinite(ys[j]) {
			jr = j
			break
		}
	}
	if jl < 0 || jr < 0 {
		return 0, false
	}
	x0, x1 := xs[jl], xs[jr]
	if !(x1 > x0) {
		return ys[jl], true
	}
	t := (x - x0) / (x1 - x0)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return ys[jl] + t*(ys[jr]-ys[jl]), true
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
