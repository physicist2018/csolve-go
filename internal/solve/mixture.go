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
	"fmt"
	"math"
	"strings"
)

// Смесь «сферы + несферы». Включается значением --type вида
// "sphere+spheroid" или "sphere+agglomerate". Прямая модель по каналам:
//
//	K_mix(r) = sfrac·K_sphere(r) + (1−sfrac)·K_non(r)
//
// где sfrac — доля сфер по концентрации (числу) частиц. Сетка сканирования по
// m берётся по несферному набору (сфероиды/агломераты), а ядро сфер — по
// ближайшему m из плотной сетки набора "sphere".
//
// Нормировка Cback: sphere и spheroid хранят Cback = 4π·dσ/dΩ, agglomerate —
// Cback = dσ/dΩ. Перед смешиванием обе приводятся к dσ/dΩ (для β-канала).

// sphereCbackScale — множитель приведения Cback набора sphere к dσ/dΩ.
const sphereCbackScale = 1.0 / (4.0 * math.Pi)

// splitMixType разбирает составной тип "sphere+X"; возвращает X и ok.
func splitMixType(t string) (non string, ok bool) {
	const p = "sphere+"
	if !strings.HasPrefix(t, p) {
		return "", false
	}
	non = strings.TrimPrefix(t, p)
	switch non {
	case "spheroid", "agglomerate", "agglomerate-cs":
		return non, true
	default:
		return "", false
	}
}

// isMixed — включён ли режим смеси по значению --type.
func (a *Args) isMixed() bool {
	_, ok := splitMixType(a.Type)
	return ok
}

// mixtureNonType — тип несферного набора для отчётов ("" вне смеси).
func mixtureNonType(a *Args) string {
	if n, ok := splitMixType(a.Type); ok {
		return n
	}
	return ""
}

// nonCbackScale — конвенция Cback несферного набора: сфероиды хранят 4π,
// агломераты — dσ/dΩ.
func nonCbackScale(nonType string) float64 {
	if nonType == "spheroid" {
		return sphereCbackScale
	}
	return 1.0
}

// mixtureData — всё, что нужно для смешанного прогона: два источника, ядра
// обоих наборов по «якорной» сетке m (по несферному набору) и развёрнутые
// (anchor × sfrac) массивы для сканирования.
type mixtureData struct {
	sphereSrc, nonSrc  *tableSource
	nonType            string
	cbackSph, cbackNon float64

	// якорная сетка (m несферного набора) и ближайшее m сфер для каждого якоря
	aRe, aIm []float64
	sRe, sIm []float64
	aSphTab  []*table // таблица сфер по ближайшему m (на якорь)
	aNonTab  []*table // таблица несфер по m якоря (на якорь)
	ktsSph   []*ktable
	ktsNon   []*ktable

	nSf       int       // число значений sfrac
	sfracGrid []float64 // сетка sfrac по концентрации
	fixed     bool      // sfrac фиксирован (--sphere-frac)

	r1MaxEff float64

	// развёрнутые (combined) массивы длины nAnchor*nSf
	mRe, mIm []float64 // m якоря (повторяется для каждого sfrac)
	sfrac    []float64 // доля сфер по концентрации
	anchor   []int     // индекс якоря для combined-индекса
	kts      []*ktable // смешанное ядро A_mix для (anchor, sfrac)
	sphTab   []*table  // таблица сфер для spectra/depol
	nonTab   []*table  // таблица несфер для spectra/depol
}

// nM — число combined-моделей (якорь × sfrac).
func (md *mixtureData) nM() int { return len(md.kts) }

// demoKernel — смешанное ядро для демо с заданными истинными m и sfrac:
// якорь — ближайший m несферного набора, sfrac — ближайшее значение сетки.
func (md *mixtureData) demoKernel(re, im, sfrac float64) *ktable {
	ai := nearestAnchorIdx(md, re, im)
	si := 0
	bestD := math.Inf(1)
	for k, s := range md.sfracGrid {
		d := math.Abs(s - sfrac)
		if d < bestD {
			bestD = d
			si = k
		}
	}
	ci := ai*md.nSf + si
	if ci < 0 || ci >= len(md.kts) {
		return nil
	}
	return md.kts[ci]
}

// nearestSphereIdx — индекс ближайшего m в сетке сфер (по |ΔRe|+|ΔIm|).
func nearestSphereIdx(sre, sim []float64, re, im float64) int {
	best := 0
	bestD := math.Inf(1)
	for i := range sre {
		d := math.Abs(sre[i]-re) + math.Abs(sim[i]-im)
		if d < bestD {
			bestD = d
			best = i
		}
	}
	return best
}

// sfracGridFromArgs — сетка доли сфер по концентрации.
func sfracGridFromArgs(a *Args) (grid []float64, fixed bool) {
	if a.SphereFracSet {
		v := a.SphereFrac
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		return []float64{v}, true
	}
	lo, hi := a.SFracMin, a.SFracMax
	if lo < 0 {
		lo = 0
	}
	if hi > 1 {
		hi = 1
	}
	if !(hi > lo) {
		return []float64{lo}, true
	}
	n := a.NSfrac
	if n < 1 {
		n = 1
	}
	if n == 1 {
		return []float64{lo}, true
	}
	return linspace(lo, hi, n), false
}

// mixKtable строит смешанное ядро K = sfrac·a + (1−sfrac)·b на той же сетке r
// (a и b должны быть построены на идентичной сетке).
func mixKtable(a, b *ktable, sfrac float64) *ktable {
	n := a.n
	kt := &ktable{
		n:    n,
		r:    a.r,
		rMin: a.rMin,
		rMax: a.rMax,
		nch:  a.nch,
		sp:   make([]*spline, a.nch),
	}
	lr := make([]float64, n)
	for i := 0; i < n; i++ {
		lr[i] = math.Log(a.r[i])
	}
	val := make([]float64, n)
	for j := 0; j < a.nch; j++ {
		for i := 0; i < n; i++ {
			k1 := math.Exp(a.sp[j].eval(lr[i]))
			k2 := math.Exp(b.sp[j].eval(lr[i]))
			km := sfrac*k1 + (1.0-sfrac)*k2
			if !(km > 0) {
				km = 1e-300
			}
			val[i] = math.Log(km)
		}
		kt.sp[j] = newSpline(lr, val)
	}
	return kt
}

// blendDepolKt — смешанное ядро S11/S22: K = sfrac·a + (1−sfrac)·b.
func blendDepolKt(a, b *ktable, sfrac float64, r []float64) *ktable {
	n := len(r)
	kt := &ktable{
		n:    n,
		r:    r,
		rMin: r[0],
		rMax: r[n-1],
		nch:  2,
		sp:   make([]*spline, 2),
	}
	lr := make([]float64, n)
	for i := 0; i < n; i++ {
		lr[i] = math.Log(r[i])
	}
	val := make([]float64, n)
	for j := 0; j < 2; j++ {
		for i := 0; i < n; i++ {
			k1 := math.Exp(a.sp[j].eval(lr[i]))
			k2 := math.Exp(b.sp[j].eval(lr[i]))
			km := sfrac*k1 + (1.0-sfrac)*k2
			if !(km > 0) {
				km = 1e-300
			}
			val[i] = math.Log(km)
		}
		kt.sp[j] = newSpline(lr, val)
	}
	return kt
}

// buildMixtureKernels собирает ядра смеси по якорной сетке несферного набора.
// baseCh — каналы решения (β/α и длины волн). Возвращает заполненный
// mixtureData с развёрнутыми массивами.
func buildMixtureKernels(a *Args, c *console, baseCh []channel) (*mixtureData, error) {
	nonType, ok := splitMixType(a.Type)
	if !ok {
		return nil, fmt.Errorf("not a mixture type %q", a.Type)
	}
	sphereSrc, err := builtinTableSource("sphere", c)
	if err != nil {
		return nil, err
	}
	nonSrc, err := builtinTableSource(nonType, c)
	if err != nil {
		return nil, err
	}
	md := &mixtureData{
		sphereSrc: sphereSrc,
		nonSrc:    nonSrc,
		nonType:   nonType,
		cbackSph:  sphereCbackScale,
		cbackNon:  nonCbackScale(nonType),
	}

	sReGrid, sImGrid, err := tableMGrid(sphereSrc)
	if err != nil {
		return nil, err
	}
	aRe, aIm, err := tableMGrid(nonSrc)
	if err != nil {
		return nil, err
	}
	if len(aRe) == 0 || len(sReGrid) == 0 {
		return nil, fmt.Errorf("mixture %s: empty refractive index grid", a.Type)
	}
	md.aRe, md.aIm = aRe, aIm
	na := len(aRe)
	md.sRe = make([]float64, na)
	md.sIm = make([]float64, na)
	for i := 0; i < na; i++ {
		j := nearestSphereIdx(sReGrid, sImGrid, aRe[i], aIm[i])
		md.sRe[i], md.sIm[i] = sReGrid[j], sImGrid[j]
	}

	c.printf("mixture mode: %s (sphere + %s)\n", a.Type, nonType)
	c.printf("  Cback convention: sphere/spheroid = 4pi, agglomerate = dsigma\n")
	c.printf("  mixture: sfrac·sphere + (1−sfrac)·%s, sfrac = sphere fraction "+
		"by concentration\n", nonType)

	// карта сопоставления m (якорь несфер → ближайшее m сфер)
	c.printf("  m mapping (anchor %s -> nearest sphere), %d points:\n", nonType, na)
	shown := 0
	for i := 0; i < na; i++ {
		dRe := math.Abs(md.sRe[i] - aRe[i])
		relIm := 0.0
		if aIm[i] > 0 {
			relIm = math.Abs(md.sIm[i]-aIm[i]) / aIm[i]
		}
		warn := dRe > 0.02 || relIm > 0.2
		if shown < 64 {
			c.printf("    %g%+gj -> %g%+gj%s\n", aRe[i], aIm[i], md.sRe[i],
				md.sIm[i], mapNote(warn))
			shown++
		}
		if warn && shown <= 64 {
			c.printf("      [warning] m offset is large; sphere cross section " +
				"is taken at a different m (approximation)\n")
		}
	}
	if na > 64 {
		c.printf("    ... (%d more)\n", na-64)
	}

	// покрытие по x: min по обоим наборам и всем m
	capmin := math.Inf(1)
	for i := 0; i < na; i++ {
		chs, err := loadTableChannels(nonSrc, aRe[i], aIm[i], baseCh,
			a.TableExtrapolate, a.TableRefWl, md.cbackNon)
		if err != nil {
			return nil, err
		}
		if chs.cap < capmin {
			capmin = chs.cap
		}
		chsS, err := loadTableChannels(sphereSrc, md.sRe[i], md.sIm[i], baseCh,
			a.TableExtrapolate, a.TableRefWl, md.cbackSph)
		if err != nil {
			return nil, err
		}
		if chsS.cap < capmin {
			capmin = chsS.cap
		}
	}
	md.r1MaxEff = math.Min(a.R1Max, capmin)
	if md.r1MaxEff < a.R1Min {
		return nil, fmt.Errorf("mixture %s: table coverage in x limits r1_max = %g µm < "+
			"r1_min = %g — decrease --r1-min or extend the tables",
			a.Type, md.r1MaxEff, a.R1Min)
	}
	c.printf("  effective r1_max = %g µm (requested: %g)\n", md.r1MaxEff, a.R1Max)

	// ядра наборов по якорной сетке (общая сетка r = r1MaxEff)
	md.aSphTab = make([]*table, na)
	md.aNonTab = make([]*table, na)
	md.ktsSph = make([]*ktable, na)
	md.ktsNon = make([]*ktable, na)
	for i := 0; i < na; i++ {
		chsN, err := loadTableChannels(nonSrc, aRe[i], aIm[i], baseCh,
			a.TableExtrapolate, a.TableRefWl, md.cbackNon)
		if err != nil {
			return nil, err
		}
		ktN, err := ktableBuild(chsN, a.R0Min, md.r1MaxEff, a.DxMax)
		if err != nil {
			return nil, err
		}
		chsS, err := loadTableChannels(sphereSrc, md.sRe[i], md.sIm[i], baseCh,
			a.TableExtrapolate, a.TableRefWl, md.cbackSph)
		if err != nil {
			return nil, err
		}
		ktS, err := ktableBuild(chsS, a.R0Min, md.r1MaxEff, a.DxMax)
		if err != nil {
			return nil, err
		}
		md.ktsNon[i] = ktN
		md.ktsSph[i] = ktS
		md.aNonTab[i] = chsN.ch[0].tab
		md.aSphTab[i] = chsS.ch[0].tab
	}
	c.printf("  table: %d r points per m\n", md.ktsNon[0].n)

	// сетка sfrac и развёрнутые массивы
	md.sfracGrid, md.fixed = sfracGridFromArgs(a)
	md.nSf = len(md.sfracGrid)
	ns := md.nSf
	total := na * ns
	md.mRe = make([]float64, total)
	md.mIm = make([]float64, total)
	md.sfrac = make([]float64, total)
	md.anchor = make([]int, total)
	md.kts = make([]*ktable, total)
	md.sphTab = make([]*table, total)
	md.nonTab = make([]*table, total)
	cidx := 0
	for i := 0; i < na; i++ {
		for si := 0; si < ns; si++ {
			s := md.sfracGrid[si]
			md.mRe[cidx] = aRe[i]
			md.mIm[cidx] = aIm[i]
			md.sfrac[cidx] = s
			md.anchor[cidx] = i
			md.kts[cidx] = mixKtable(md.ktsSph[i], md.ktsNon[i], s)
			md.sphTab[cidx] = md.aSphTab[i]
			md.nonTab[cidx] = md.aNonTab[i]
			cidx++
		}
	}
	if md.fixed {
		c.printf("  sphere fraction sfrac: fixed at %g\n", md.sfracGrid[0])
	} else {
		c.printf("  sphere fraction sfrac: scanned over [%g, %g] (%d values)\n",
			md.sfracGrid[0], md.sfracGrid[ns-1], ns)
	}
	return md, nil
}

// mapNote — пометка к строке карты m.
func mapNote(warn bool) string {
	if warn {
		return "  (large offset)"
	}
	return ""
}

// depolSetBuildMixed — ядра S11/S22 для смеси: для каждого combined-индекса
// ядра сфер и несфер смешиваются по sfrac. Сетка r — как у основных ядер.
func depolSetBuildMixed(md *mixtureData, wl []float64, refWl, rMin, rMax,
	dxMax float64, extrapolate bool, c *console) (*depolSet, error) {
	ds := &depolSet{
		wl: depolWavelengths(wl),
		kt: make([][]*ktable, md.nM()),
	}
	for i := range ds.kt {
		ds.kt[i] = make([]*ktable, len(ds.wl))
	}
	if !(rMax > rMin) || md.nM() == 0 {
		return ds, nil
	}
	lamMin := ds.wl[0]
	for _, w := range ds.wl {
		if w < lamMin {
			lamMin = w
		}
	}
	r, err := rGridFor(rMin, rMax, dxMax, lamMin)
	if err != nil {
		return nil, err
	}
	na := len(md.aRe)
	ns := md.nSf
	for i := 0; i < na; i++ {
		sphTab := md.aSphTab[i]
		nonTab := md.aNonTab[i]
		if sphTab == nil || nonTab == nil || !sphTab.hasS11 || !sphTab.hasS22 ||
			!nonTab.hasS11 || !nonTab.hasS22 {
			continue
		}
		// ядра S11/S22 по длинам волн для обоих наборов
		sphKt := make([]*ktable, len(ds.wl))
		nonKt := make([]*ktable, len(ds.wl))
		okAny := false
		for iw, w := range ds.wl {
			a, errA := depolKtableBuild(sphTab, w, refWl, r, c)
			b, errB := depolKtableBuild(nonTab, w, refWl, r, c)
			if errA != nil || errB != nil {
				continue
			}
			sphKt[iw] = a
			nonKt[iw] = b
			okAny = true
		}
		if !okAny {
			continue
		}
		for si := 0; si < ns; si++ {
			cidx := i*ns + si
			s := md.sfracGrid[si]
			for iw := range ds.wl {
				if sphKt[iw] == nil || nonKt[iw] == nil {
					continue
				}
				ds.kt[cidx][iw] = blendDepolKt(sphKt[iw], nonKt[iw], s, r)
			}
		}
		ds.nOK++
	}
	return ds, nil
}

// depolSetReportMixed — краткая диагностика δ_l для смеси.
func depolSetReportMixed(ds *depolSet, md *mixtureData, c *console) {
	if ds == nil || len(ds.wl) == 0 {
		return
	}
	if ds.nOK == 0 {
		c.warnf("[warning] mixture %s: tables have no valid S11/S22 columns "+
			"(or they contain nan) — linear depolarization is not computed\n",
			md.nonType)
		return
	}
	c.printf("  linear depolarization δ_l: from mixed S11/S22 (%d of %d m values)\n",
		ds.nOK, len(md.aRe))
}

// meanSfracOf — средняя доля сфер (по концентрации) по первым G решениям.
func meanSfracOf(sols *solutions, G int) float64 {
	sum, cnt := 0.0, 0
	for t := 0; t < sols.n && t < G; t++ {
		v := sols.sol[t].sfrac
		if isFinite(v) {
			sum += v
			cnt++
		}
	}
	if cnt == 0 {
		return math.NaN()
	}
	return sum / float64(cnt)
}

// sfracRange — минимум (lo=true) или максимум доли сфер по первым G решениям.
func sfracRange(sols *solutions, G int, lo bool) float64 {
	best := math.Inf(1)
	if !lo {
		best = math.Inf(-1)
	}
	cnt := 0
	for t := 0; t < sols.n && t < G; t++ {
		v := sols.sol[t].sfrac
		if !isFinite(v) {
			continue
		}
		cnt++
		if lo {
			if v < best {
				best = v
			}
		} else if v > best {
			best = v
		}
	}
	if cnt == 0 {
		return math.NaN()
	}
	return best
}
