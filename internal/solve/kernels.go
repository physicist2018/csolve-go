package solve

import (
	"math"
)

// ktable — таблица ядер на сетке r и её интерполяция (ln K от ln r).
type ktable struct {
	n    int
	r    []float64
	rMin float64
	rMax float64
	nch  int
	sp   []*spline
}

// rGridFor — сетка радиусов с шагом dr = dxMax*lamMin/(2*pi) (шаг в x не
// больше dxMax) и последним узлом ровно в rMax.
func rGridFor(rMin, rMax, dxMax, lamMin float64) []float64 {
	dr := dxMax * lamMin / (2.0 * csPi)
	if !(dr > 0) {
		fatal("invalid grid step r (dx-max = %g)", dxMax)
	}

	n := int(math.Ceil((rMax - rMin) / dr))
	if n < 0 {
		n = 0
	}
	r := make([]float64, n+1)
	for i := 0; i < n; i++ {
		r[i] = rMin + float64(i)*dr
	}
	nr := n
	if nr == 0 || r[nr-1] < rMax-0.5*dr {
		r[nr] = rMax
		nr++
	}
	return r[:nr]
}

func ktableBuild(chs *channels, rMin, rMax, dxMax float64) *ktable {
	nch := chs.n
	if nch <= 0 {
		fatal("no channels")
	}
	lamMin := chs.ch[0].wlUm
	for j := 1; j < nch; j++ {
		if chs.ch[j].wlUm < lamMin {
			lamMin = chs.ch[j].wlUm
		}
	}
	r := rGridFor(rMin, rMax, dxMax, lamMin)
	nr := len(r)

	kt := &ktable{
		n:    nr,
		r:    r,
		rMin: rMin,
		rMax: rMax,
		nch:  nch,
		sp:   make([]*spline, nch),
	}
	lr := make([]float64, nr)
	val := make([]float64, nr)
	for i := 0; i < nr; i++ {
		lr[i] = math.Log(r[i])
	}
	for j := 0; j < nch; j++ {
		ch := &chs.ch[j]
		if ch.tab == nil {
			fatal("Mie mode is not supported by the C version — specify --tables")
		}
		scale := um2ToM2 * math.Pow(ch.wlUm/ch.refWlUm, 2.0)
		for i := 0; i < nr; i++ {
			x := 2.0 * csPi * r[i] / ch.wlUm
			C := tableCrossSection(ch.tab, ch.kind == chBeta, x)
			K := scale * C
			if ch.kind == chBeta {
				K *= ch.cbackScale
			}
			if !(K > 0) {
				fatal("kernel K(r) must be strictly positive on the grid "+
					"(channel %d, r = %g)", j, r[i])
			}
			val[i] = math.Log(K)
		}
		kt.sp[j] = newSpline(lr, val)
	}
	return kt
}

func ktableKernel(kt *ktable, j int, r float64) float64 {
	rc := r
	if rc < kt.rMin {
		rc = kt.rMin
	}
	if rc > kt.rMax {
		rc = kt.rMax
	}
	return math.Exp(kt.sp[j].eval(math.Log(rc)))
}
