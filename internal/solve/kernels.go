package solve

import (
	"errors"
	"fmt"
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
func rGridFor(rMin, rMax, dxMax, lamMin float64) ([]float64, error) {
	dr := dxMax * lamMin / (2.0 * math.Pi)
	if !(dr > 0) {
		return nil, fmt.Errorf("invalid grid step r (dx-max = %g)", dxMax)
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
	return r[:nr], nil
}

func ktableBuild(chs *channels, rMin, rMax, dxMax float64) (*ktable, error) {
	nch := len(chs.ch)
	if nch <= 0 {
		return nil, errors.New("no channels")
	}
	lamMin := chs.ch[0].wlUm
	for j := 1; j < nch; j++ {
		if chs.ch[j].wlUm < lamMin {
			lamMin = chs.ch[j].wlUm
		}
	}
	r, err := rGridFor(rMin, rMax, dxMax, lamMin)
	if err != nil {
		return nil, err
	}
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
			// Инвариант: loadTableChannels привязывает таблицу к каждому каналу.
			panic("solve: channel has no cross-section table")
		}
		scale := um2ToM2 * math.Pow(ch.wlUm/ch.refWlUm, 2.0)
		for i := 0; i < nr; i++ {
			x := 2.0 * math.Pi * r[i] / ch.wlUm
			if !ch.tab.extrapolate && (x < ch.tab.xMin || x > ch.tab.xMax) {
				return nil, tableRangeError(ch.tab)
			}
			C := tableCrossSection(ch.tab, ch.kind == chBeta, x)
			K := scale * C
			if ch.kind == chBeta {
				K *= ch.cbackScale
			}
			if !(K > 0) {
				return nil, fmt.Errorf("kernel K(r) must be strictly positive on the grid "+
					"(channel %d, r = %g)", j, r[i])
			}
			val[i] = math.Log(K)
		}
		kt.sp[j] = newSpline(lr, val)
	}
	return kt, nil
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
