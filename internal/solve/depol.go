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
