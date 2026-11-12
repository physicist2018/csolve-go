package solve

import (
	"fmt"
	"math"
	"os"
	"strings"
)

// depolWlsUm — длины волн, на которые пересчитываются столбцы S11/S22 при
// расчёте линейной деполяризации обратного рассеяния.
var depolWlsUm = []float64{0.355, 0.532}

// depolSet — таблицы ядер S11(r), S22(r) для всех m сетки и обеих длин волн.
// kt[m][iw] == nil означает, что у таблицы для этого m нет валидных столбцов
// S11/S22 (например, старые базы `base/average` или `miedir`).
type depolSet struct {
	wl  []float64
	kt  [][2]*ktable
	nOK int
}

func depolSetAvailable(ds *depolSet) bool { return ds != nil && ds.nOK > 0 }

// depolSetBuild — построение ядер деполяризации по всем m из таблиц. Сетка r
// совпадает с сеткой основных ядер (dxMax, минимальная длина волны).
func depolSetBuild(src *tableSource, mre, mim []float64, refWl, rMin, rMax,
	dxMax float64, extrapolate bool) *depolSet {
	ds := &depolSet{
		wl: append([]float64(nil), depolWlsUm...),
		kt: make([][2]*ktable, len(mre)),
	}
	if !(rMax > rMin) || len(mre) == 0 {
		return ds
	}
	lamMin := ds.wl[0]
	for _, wl := range ds.wl {
		if wl < lamMin {
			lamMin = wl
		}
	}
	r := rGridFor(rMin, rMax, dxMax, lamMin)
	for i := range mre {
		tab := tableCacheGet(src, mre[i], mim[i], extrapolate)
		if !tab.hasS11 || !tab.hasS22 {
			continue
		}
		for iw, wl := range ds.wl {
			ds.kt[i][iw] = depolKtableBuild(tab, wl, refWl, r)
		}
		ds.nOK++
	}
	return ds
}

// depolKtableBuild — ядра S11/S22 на сетке r для длины волны wl.
func depolKtableBuild(tab *table, wl, refWl float64, r []float64) *ktable {
	kt := &ktable{
		n:    len(r),
		r:    r,
		rMin: r[0],
		rMax: r[len(r)-1],
		nch:  2,
		sp:   make([]*spline, 2),
	}
	lr := make([]float64, len(r))
	for i := range r {
		lr[i] = math.Log(r[i])
	}
	scale := um2ToM2 * math.Pow(wl/refWl, 2.0)
	cols := [2]*spline{tab.spS11, tab.spS22}
	val := make([]float64, len(r))
	for j := 0; j < 2; j++ {
		for i := range r {
			x := 2.0 * csPi * r[i] / wl
			K := scale * tableSColumn(tab, cols[j], x, wl)
			if !(K > 0) || !isFinite(K) {
				fatal("%s: depolarization kernel S(r) must be positive and "+
					"finite (x = %g, λ = %g nm)", tab.path, x, wl*1e3)
			}
			val[i] = math.Log(K)
		}
		kt.sp[j] = newSpline(lr, val)
	}
	return kt
}

// tableSColumn — значение S11/S22 при заданном x. В отличие от Cext/Cback
// (там выход за диапазон — ошибка) здесь x зажимается в [xMin, xMax]:
// δ_l — диагностическая величина, и пересчёт на 355 нм требует больших x,
// чем покрывает таблица, поэтому расчёт не прерывается. Предупреждение
// печатается один раз на таблицу.
func tableSColumn(t *table, sp *spline, x, wl float64) float64 {
	xc := x
	if xc < t.xMin {
		xc = t.xMin
	}
	if xc > t.xMax {
		xc = t.xMax
	}
	if xc != x && !t.sClampWarned {
		t.sClampWarned = true
		fmt.Fprintf(os.Stderr, "[warning] %s: S11/S22 requested at x = %g "+
			"outside [%g, %g] for λ = %g nm — x is clamped; δ_l may be "+
			"biased (a wider table is needed)\n", t.path, x, t.xMin, t.xMax,
			wl*1e3)
	}
	return math.Exp(sp.eval(math.Log(xc)))
}

// depolWlIndex — индекс длины волны target (µm) в списке wl, иначе -1.
func depolWlIndex(wl []float64, target float64) int {
	for i, w := range wl {
		if w > 0 && math.Abs(w-target) <= 1e-6*target {
			return i
		}
	}
	return -1
}

// depolColumnMatch — сопоставление измеренных длин волн δ_l (µm) с длинами
// волн набора ядер ds. Возвращает индексы длин волн ds (iw) и индексы
// соответствующих столбцов измерения (col). Несовпадающие длины волн
// пропускаются с предупреждением.
func depolColumnMatch(ds *depolSet, wl []float64, label string) (iw, col []int) {
	if ds == nil || len(ds.wl) == 0 {
		return nil, nil
	}
	for k, w := range wl {
		j := -1
		for i, wds := range ds.wl {
			if w > 0 && math.Abs(wds-w) <= 1e-6*w {
				j = i
				break
			}
		}
		if j < 0 {
			var sb strings.Builder
			for i, wds := range ds.wl {
				if i > 0 {
					sb.WriteString(", ")
				}
				fmt.Fprintf(&sb, "%.6g nm", wds*1e3)
			}
			fmt.Fprintf(os.Stderr, "[warning] %s: measured depolarization at "+
				"%.6g nm does not match S11/S22 wavelengths (%s) — ignored\n",
				label, w*1e3, sb.String())
			continue
		}
		iw = append(iw, j)
		col = append(col, k)
	}
	return iw, col
}

// depolProj — предвычисленные проекции ядер S11/S22 на базис окна:
//
//	a11[i] = ∫ S11(r) q(r) B_i(r) dr,   a22[i] = ∫ S22(r) q(r) B_i(r) dr
//
// По ним δ_l любого решения считается через два скалярных произведения.
// Раскладка: [iw][(im*nW + w)*nu + i], iw — индекс в iwDep.
type depolProj struct {
	wl  []float64 // длины волн, для которых построены проекции, µm
	iw  []int     // индексы этих длин волн в depolSet.wl
	nM  int
	nW  int
	nu  int
	a11 [][]float64
	a22 [][]float64
}

func depolProjBuild(ds *depolSet, d *design, iwDep []int) *depolProj {
	if ds == nil || d == nil || d.nW <= 0 || d.nu <= 0 || len(iwDep) == 0 {
		return nil
	}
	p := &depolProj{
		iw: append([]int(nil), iwDep...),
		nM: d.nM,
		nW: d.nW,
		nu: d.nu,
	}
	for _, iw := range iwDep {
		p.wl = append(p.wl, ds.wl[iw])
	}
	sz := d.nM * d.nW * d.nu
	p.a11 = make([][]float64, len(iwDep))
	p.a22 = make([][]float64, len(iwDep))
	for k := range iwDep {
		p.a11[k] = make([]float64, sz)
		p.a22[k] = make([]float64, sz)
	}
	for im := 0; im < d.nM; im++ {
		if im >= len(ds.kt) {
			break
		}
		for w := 0; w < d.nW; w++ {
			g := d.geom[w]
			base := (im*d.nW + w) * d.nu
			for k, iw := range iwDep {
				kt := ds.kt[im][iw]
				if kt == nil {
					continue
				}
				a11 := p.a11[k][base : base+d.nu]
				a22 := p.a22[k][base : base+d.nu]
				for q := 0; q < g.nNodes; q++ {
					r := g.nodes[q]
					wgt := g.weights[q] * g.dvol[q]
					if wgt == 0 {
						continue
					}
					s11 := ktableKernel(kt, 0, r)
					s22 := ktableKernel(kt, 1, r)
					bv := g.values[q*d.nu : (q+1)*d.nu]
					for i := 0; i < d.nu; i++ {
						a11[i] += wgt * s11 * bv[i]
						a22[i] += wgt * s22 * bv[i]
					}
				}
			}
		}
	}
	return p
}

// depolModel — модельное δ_l, %, для решения с коэффициентами C в окне
// iwGeom при показателе m с индексом im для проекции k.
func (p *depolProj) depolModel(im, iwGeom, k int, C []float64) float64 {
	if p == nil || k < 0 || k >= len(p.a11) {
		return math.NaN()
	}
	if im < 0 || im >= p.nM || iwGeom < 0 || iwGeom >= p.nW {
		return math.NaN()
	}
	base := (im*p.nW + iwGeom) * p.nu
	a11 := p.a11[k][base : base+p.nu]
	a22 := p.a22[k][base : base+p.nu]
	s11, s22 := 0.0, 0.0
	for i := 0; i < p.nu && i < len(C); i++ {
		s11 += C[i] * a11[i]
		s22 += C[i] * a22[i]
	}
	den := s11 + s22
	if !(den > 0) || !isFinite(den) || !isFinite(s11-s22) {
		return math.NaN()
	}
	return (s11 - s22) / den * 100.0
}

// depolDeviation — среднее относительное отклонение модельного δ_l от
// измеренных значений (targets выровнен с p.wl). Пары с нефинитными или
// неположительными измерениями пропускаются; при отсутствии пар — NaN.
func depolDeviation(p *depolProj, im, iwGeom int, C, targets []float64) float64 {
	if p == nil {
		return math.NaN()
	}
	sum, cnt := 0.0, 0
	n := len(targets)
	if n > len(p.wl) {
		n = len(p.wl)
	}
	for k := 0; k < n; k++ {
		t := targets[k]
		if !isFinite(t) || !(t > 0) {
			continue
		}
		v := p.depolModel(im, iwGeom, k, C)
		if !isFinite(v) {
			continue
		}
		sum += math.Abs(v-t) / t
		cnt++
	}
	if cnt == 0 {
		return math.NaN()
	}
	return sum / float64(cnt)
}

// depolPenaltyVector — штраф w·|δ_l − δ_l^meas| / δ_l^meas для каждого
// решения, индексированный по sol.id (nil, если штраф не нужен).
func depolPenaltyVector(sols *solutions, p *depolProj, mre, mim,
	targets []float64, w float64) []float64 {
	if sols == nil || sols.n == 0 || p == nil || !(w > 0) {
		return nil
	}
	pen := make([]float64, sols.n)
	for t := 0; t < sols.n; t++ {
		s := &sols.sol[t]
		if s.id < 0 || s.id >= len(pen) {
			continue
		}
		im := mIndex(mre, mim, s.mRe, s.mIm)
		dev := depolDeviation(p, im, s.geomIndex, s.C, targets)
		if isFinite(dev) {
			pen[s.id] = w * dev
		}
	}
	return pen
}

// rankByDepol — ранжирование решений с штрафом за относительное отклонение
// δ_l от измеренных значений: score = err + w * dev. Возвращает вектор
// штрафов (по sol.id), чтобы его можно было сложить с другими штрафами.
func rankByDepol(sols *solutions, p *depolProj, mre, mim, targets []float64,
	w float64) []float64 {
	pen := depolPenaltyVector(sols, p, mre, mim, targets, w)
	if pen == nil {
		return nil
	}
	score := make([]float64, sols.n)
	for t := 0; t < sols.n; t++ {
		score[t] = sols.sol[t].err + pen[sols.sol[t].id]
	}
	reorderByScore(sols, score)
	return pen
}

// depolSetReport — диагностика доступности столбцов S11/S22.
func depolSetReport(ds *depolSet, src *tableSource, nm int) {
	if ds == nil || len(ds.wl) == 0 {
		return
	}
	label := src.label
	if ds.nOK == 0 {
		fmt.Fprintf(os.Stderr, "[warning] tables in %s have no valid S11/S22 "+
			"columns (or they contain nan) — linear depolarization is not "+
			"computed\n", label)
		return
	}
	var sb strings.Builder
	sb.WriteString("linear depolarization δ_l at ")
	for i, wl := range ds.wl {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "%g nm", wl*1e3)
	}
	fmt.Printf("  %s: from tables S11/S22 (%d of %d m values)\n", sb.String(),
		ds.nOK, nm)
	if ds.nOK < nm {
		fmt.Fprintf(os.Stderr, "[warning] tables in %s: S11/S22 available "+
			"only for %d of %d m values — δ_l is averaged over the available "+
			"ones\n", label, ds.nOK, nm)
	}
}

// depolFromProfile — свёртка S11(r) и S22(r) с распределением n(r) на узлах
// квадратуры (rq, wq) и δ_l = (D11-D22)/(D11+D22)*100 %.
//
// Усреднение по m то же, что и для gFit: ядра складываются по всем решениям
// ансамбля, у которых есть столбцы S11/S22.
func depolFromProfile(ds *depolSet, mIdx []int, rq, wq, nq []float64) (
	d11, d22, delta []float64) {
	nw := 0
	if ds != nil {
		nw = len(ds.wl)
	}
	d11 = make([]float64, nw)
	d22 = make([]float64, nw)
	delta = make([]float64, nw)
	if nw == 0 {
		return d11, d22, delta
	}

	dvol := make([]float64, len(rq))
	for k, r := range rq {
		dvol[k] = 3.0 / (4.0 * csPi * r * r * r * r)
	}

	for iw := 0; iw < nw; iw++ {
		s11, s22, cnt := 0.0, 0.0, 0
		for _, im := range mIdx {
			if im < 0 || im >= len(ds.kt) {
				continue
			}
			kt := ds.kt[im][iw]
			if kt == nil {
				continue
			}
			a, b := 0.0, 0.0
			for k := range rq {
				w := wq[k] * dvol[k] * nq[k]
				if w == 0 {
					continue
				}
				a += w * ktableKernel(kt, 0, rq[k])
				b += w * ktableKernel(kt, 1, rq[k])
			}
			s11 += a
			s22 += b
			cnt++
		}
		if cnt == 0 {
			d11[iw], d22[iw], delta[iw] = math.NaN(), math.NaN(), math.NaN()
			continue
		}
		s11 /= float64(cnt)
		s22 /= float64(cnt)
		d11[iw], d22[iw] = s11, s22
		if s11+s22 > 0 {
			delta[iw] = (s11 - s22) / (s11 + s22) * 100.0
		} else {
			delta[iw] = math.NaN()
		}
	}
	return d11, d22, delta
}
