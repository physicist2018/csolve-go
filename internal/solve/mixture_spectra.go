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

import "math"

// spectModel — таблицы, по которым считается сечение на произвольной длине
// волны: для смеси — пара «сфера + несфера» и доля сфер; для обычного режима —
// одна таблица (sph == nil, sfrac == 0).
type spectModel struct {
	sph, non *table
	sfrac    float64
}

// volumeFraction пересчитывает долю сфер по концентрации p в объёмную долю.
// Для встроенных наборов r — объёмно-эквивалентный радиус, поэтому средние
// объёмы сфер и несфер совпадают и объёмная доля равна числовой; формула
// оставлена явной на случай иной конвенции радиуса.
func volumeFraction(p float64) float64 {
	if !isFinite(p) {
		return math.NaN()
	}
	vSph, vNon := 1.0, 1.0
	num := p * vSph
	den := num + (1.0-p)*vNon
	if !(den > 0) {
		return math.NaN()
	}
	return num / den
}

// blend2 смешивает значения сфер (vs, вес ws) и несфер (vN, вес wN); NaN
// допустим только для слагаемого с нулевым весом (например, отсутствующей
// таблицы).
func blend2(ws, vs, wN, vN float64) float64 {
	if math.IsNaN(vs) {
		if ws == 0 {
			return vN
		}
		return math.NaN()
	}
	if math.IsNaN(vN) {
		if wN == 0 {
			return vs
		}
		return math.NaN()
	}
	return ws*vs + wN*vN
}

// colVal — значение столбца таблицы с множителями масштаба (sc·cbs); NaN,
// если столбец недоступен.
func colVal(t *table, sp *spline, has bool, x, wlUm, sc, cbs float64,
	con *console) float64 {
	if t == nil || !has || sp == nil {
		return math.NaN()
	}
	return sc * cbs * spectrumColumn(t, sp, x, wlUm, con)
}

// spectrumColumns — столбцы [ext, back, S11, S22] смешанного сечения на сетке
// rq для длины волны wlUm. Для обычной модели (sph == nil) — просто столбцы
// несферной таблицы с конвенцией cbackNon.
func spectrumColumns(m spectModel, cbackSph, cbackNon, wlUm, refWl float64,
	rq []float64, sphCon, nonCon *console) [4][]float64 {
	n := len(rq)
	ext := make([]float64, n)
	back := make([]float64, n)
	s11 := make([]float64, n)
	s22 := make([]float64, n)
	sc := um2ToM2 * math.Pow(wlUm/refWl, 2.0)
	ws := 0.0
	if m.sph != nil {
		ws = m.sfrac
	}
	wN := 1.0 - ws
	for k, r := range rq {
		x := 2.0 * math.Pi * r / wlUm
		var eS, bS, a1S, a2S float64 = math.NaN(), math.NaN(), math.NaN(), math.NaN()
		if m.sph != nil {
			eS = colVal(m.sph, m.sph.spCext, m.sph.hasCext, x, wlUm, sc, 1.0, sphCon)
			bS = colVal(m.sph, m.sph.spCback, m.sph.hasCback, x, wlUm, sc,
				cbackSph, sphCon)
			a1S = colVal(m.sph, m.sph.spS11, m.sph.hasS11, x, wlUm, sc, 1.0, sphCon)
			a2S = colVal(m.sph, m.sph.spS22, m.sph.hasS22, x, wlUm, sc, 1.0, sphCon)
		}
		var eN, bN, a1N, a2N float64 = math.NaN(), math.NaN(), math.NaN(), math.NaN()
		if m.non != nil {
			eN = colVal(m.non, m.non.spCext, m.non.hasCext, x, wlUm, sc, 1.0, nonCon)
			bN = colVal(m.non, m.non.spCback, m.non.hasCback, x, wlUm, sc,
				cbackNon, nonCon)
			a1N = colVal(m.non, m.non.spS11, m.non.hasS11, x, wlUm, sc, 1.0, nonCon)
			a2N = colVal(m.non, m.non.spS22, m.non.hasS22, x, wlUm, sc, 1.0, nonCon)
		}
		ext[k] = blend2(ws, eS, wN, eN)
		back[k] = blend2(ws, bS, wN, bN)
		s11[k] = blend2(ws, a1S, wN, a1N)
		s22[k] = blend2(ws, a2S, wN, a2N)
	}
	return [4][]float64{ext, back, s11, s22}
}

// nearestAnchorIdx — индекс ближайшего якорного m (сетка несферного набора).
func nearestAnchorIdx(md *mixtureData, re, im float64) int {
	best := 0
	bestD := math.Inf(1)
	for i := range md.aRe {
		d := math.Abs(md.aRe[i]-re) + math.Abs(md.aIm[i]-im)
		if d < bestD {
			bestD = d
			best = i
		}
	}
	return best
}

// convolveColumns — свёртка столбцов сечения с распределением на rq:
// sum_k wq·q(rq)·dist·col.
func convolveColumns(cols [4][]float64, rq, wq, dist []float64) (e, b, a1, a2 float64) {
	for k := range rq {
		w := wq[k] * qDvol(rq[k]) * dist[k]
		if w == 0 {
			continue
		}
		e += w * cols[0][k]
		b += w * cols[1][k]
		a1 += w * cols[2][k]
		a2 += w * cols[3][k]
	}
	return e, b, a1, a2
}

// buildSpectraMix — спектры α(λ), β(λ), S(λ), δ_l(λ) для режима смеси. Ядро
// каждого решения — смешанное сечения (по sfrac решения и её таблицам).
func buildSpectraMix(a *Args, sols *solutions, geom []*geom, rq, wq []float64,
	ds *depolSet, mix *mixtureData, ch []channel, g1, depWl, depVal []float64) *spectra {
	if a == nil || !a.Spectra || mix == nil || sols == nil || sols.n == 0 ||
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
	refWl := a.TableRefWl
	cbackSph, cbackNon := mix.cbackSph, mix.cbackNon

	vol := make([][]float64, G)
	models := make([]spectModel, G)
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
		ci := s.mIdx
		if ci < 0 || ci >= len(mix.sphTab) {
			return nil
		}
		models[t] = spectModel{sph: mix.sphTab[ci], non: mix.nonTab[ci],
			sfrac: mix.sfrac[ci]}
	}

	nw := len(wlNm)
	alphaV := newMatrix(G, nw)
	betaV := newMatrix(G, nw)
	lidarV := newMatrix(G, nw)
	deltaV := newMatrix(G, nw)

	for iw := 0; iw < nw; iw++ {
		wlUm := wlNm[iw] * 1e-3
		cols := make([][4][]float64, G)
		for t := 0; t < G; t++ {
			cols[t] = spectrumColumns(models[t], cbackSph, cbackNon, wlUm,
				refWl, rq, mix.sphereSrc.con, mix.nonSrc.con)
		}
		for t := 0; t < G; t++ {
			e, b, a1, a2 := convolveColumns(cols[t], rq, wq, vol[t])
			alphaV[t][iw] = e
			betaV[t][iw] = b
			if isFinite(a1) && isFinite(a2) && a1+a2 > 0 {
				deltaV[t][iw] = (a1 - a2) / (a1 + a2) * 100.0
			} else {
				deltaV[t][iw] = math.NaN()
			}
		}
	}
	for t := 0; t < G; t++ {
		for iw := 0; iw < nw; iw++ {
			x, y := alphaV[t][iw], betaV[t][iw]
			if isFinite(x) && isFinite(y) && y > 0 {
				v := x / y
				if isFinite(v) {
					lidarV[t][iw] = v
					continue
				}
			}
			lidarV[t][iw] = math.NaN()
		}
	}

	alpha := aggregateSpectrum(alphaV, wlNm, a.PlotBestLimit)
	beta := aggregateSpectrum(betaV, wlNm, a.PlotBestLimit)
	lidar := aggregateSpectrum(lidarV, wlNm, a.PlotBestLimit)
	delta := aggregateSpectrum(deltaV, wlNm, a.PlotBestLimit)
	alpha.meas, beta.meas, delta.meas = measuredSpectrumPoints(ch, g1, depWl, depVal)
	alpha.measName = "measured α"
	beta.measName = "measured β"
	lidar.meas = measuredLidarPoints(alpha.meas, beta.meas)
	lidar.measName = "measured S"
	delta.measName = "measured δ_l"
	return &spectra{wlNm: wlNm, alpha: alpha, beta: beta, lidar: lidar,
		delta: delta, g: G}
}

// weightedModel — модель сечения с весом (кратностью) в ансамбле.
type weightedModel struct {
	model spectModel
	w     float64
}

// meanColumns — усреднённые по ансамблю столбцы [ext, back, S11, S22] на сетке
// rq для длины волны wlUm. Усреднение — по лучшим решениям (с кратностями),
// так же как в gFit. Для каждого элемента считается среднее только по
// конечным значениям.
func meanColumns(models []weightedModel, cbackSph, cbackNon, wlUm, refWl float64,
	rq []float64, sphCon, nonCon *console) [4][]float64 {
	n := len(rq)
	sum := [4][]float64{make([]float64, n), make([]float64, n),
		make([]float64, n), make([]float64, n)}
	cnt := [4][]float64{make([]float64, n), make([]float64, n),
		make([]float64, n), make([]float64, n)}
	for _, wm := range models {
		if wm.model.sph == nil && wm.model.non == nil {
			continue
		}
		c := spectrumColumns(wm.model, cbackSph, cbackNon, wlUm, refWl, rq,
			sphCon, nonCon)
		for q := 0; q < 4; q++ {
			for k := 0; k < n; k++ {
				v := c[q][k]
				if isFinite(v) {
					sum[q][k] += wm.w * v
					cnt[q][k] += wm.w
				}
			}
		}
	}
	out := [4][]float64{make([]float64, n), make([]float64, n),
		make([]float64, n), make([]float64, n)}
	for q := 0; q < 4; q++ {
		for k := 0; k < n; k++ {
			if cnt[q][k] > 0 {
				out[q][k] = sum[q][k] / cnt[q][k]
			} else {
				out[q][k] = math.NaN()
			}
		}
	}
	return out
}

// applyMeanDistributionComputed пересчитывает «полые» расчётные точки по
// средней функции распределения meanNq и усреднённому по лучшим решениям
// ядру (models). Так же, как gFit: g(λ) = ∫ [avg K(λ,r)] q n_avg dln r.
// Работает в обоих режимах (mix может быть nil).
func applyMeanDistributionComputed(sp *spectra, a *Args, src *tableSource,
	mix *mixtureData, ch []channel, depWl []float64, rq, wq, meanNq []float64,
	models []spectModel) {
	if sp == nil || a == nil || len(sp.wlNm) == 0 || len(meanNq) == 0 ||
		len(models) == 0 {
		return
	}

	// свёртка весов моделей: одинаковые модели (таблицы+sfrac) объединяем с
	// кратностью, чтобы среднее по ансамблю было точным, но без лишних работ
	type mkey struct {
		sph, non *table
		sfrac    float64
	}
	cnt := map[mkey]int{}
	order := make([]mkey, 0, len(models))
	for _, m := range models {
		k := mkey{m.sph, m.non, m.sfrac}
		if _, ok := cnt[k]; !ok {
			order = append(order, k)
		}
		cnt[k]++
	}
	wm := make([]weightedModel, len(order))
	for i, k := range order {
		wm[i] = weightedModel{model: spectModel{sph: k.sph, non: k.non,
			sfrac: k.sfrac}, w: float64(cnt[k])}
	}

	refWl := a.TableRefWl
	cbackSph, cbackNon := 0.0, 1.0
	if a.TableCback4pi {
		cbackNon = sphereCbackScale
	}
	if mix != nil {
		cbackSph, cbackNon = mix.cbackSph, mix.cbackNon
	}
	sphCon, nonCon := src.con, src.con
	if mix != nil {
		sphCon, nonCon = mix.sphereSrc.con, mix.nonSrc.con
	}

	nw := len(sp.wlNm)
	meanA := make([]float64, nw)
	meanB := make([]float64, nw)
	meanL := make([]float64, nw)
	meanD := make([]float64, nw)
	for iw := 0; iw < nw; iw++ {
		cols := meanColumns(wm, cbackSph, cbackNon, sp.wlNm[iw]*1e-3, refWl,
			rq, sphCon, nonCon)
		e, b, a1, a2 := convolveColumns(cols, rq, wq, meanNq)
		meanA[iw] = e
		meanB[iw] = b
		if isFinite(e) && isFinite(b) && b > 0 {
			meanL[iw] = e / b
		} else {
			meanL[iw] = math.NaN()
		}
		if isFinite(a1) && isFinite(a2) && a1+a2 > 0 {
			meanD[iw] = (a1 - a2) / (a1 + a2) * 100.0
		} else {
			meanD[iw] = math.NaN()
		}
	}

	gapWl := inputWavelengthsNm(ch, depWl)
	sets := []struct {
		set   *spectrumSet
		curve []float64
	}{
		{sp.alpha, meanA},
		{sp.beta, meanB},
		{sp.lidar, meanL},
		{sp.delta, meanD},
	}
	for _, s := range sets {
		if s.set == nil {
			continue
		}
		s.set.meanDist = s.curve
		s.set.computed = missingSpectrumPoints(gapWl, sp.wlNm, s.set)
	}
}
