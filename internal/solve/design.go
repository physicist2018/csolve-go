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

// design — матрицы A для всех (m, окно) за один проход:
// A[j,i] = int K_j(r) q(r) B_i(r) dr, q(r) = 3/(4 pi r^4).
type design struct {
	nM, nW, nch, nu int
	A               []float64 // [nM][nW][nch][nu]
	s0              []float64 // [nM][nW]
	cond            []float64 // [nM][nW] число обусловленности A
	degree          []int
	r0, r1          []float64
	mRe, mIm        []float64
	sfrac           []float64 // доля сфер (по концентрации) на значение m; nil вне смеси
	geom            []*geom
	method          methodKind
	bins            int
	regOrder        int
	kernelOrder     int
	workers         int // число воркеров при сборке A; 1 = последовательно
}

func designBuild(ch []channel, nch int, kts []*ktable, mRe, mIm []float64,
	r0Grid, r1Grid []float64, minRatio float64, spec designSpec) (*design, error) {
	_ = ch
	d := &design{
		nch:         nch,
		method:      spec.method,
		regOrder:    spec.regOrder,
		kernelOrder: normalizeKernelOrder(spec.kernelOrder),
		workers:     resolveJobs(spec.workers),
	}
	if d.method == methodMonotone {
		d.bins = binsFor(spec.bins, nch)
		d.regOrder = spec.regOrder
		if d.regOrder < 0 || d.regOrder > 2 {
			d.regOrder = 2
		}
	} else {
		d.regOrder = 2
	}

	deg := []int{}
	r0s := []float64{}
	r1s := []float64{}
	for _, r0 := range r0Grid {
		for _, r1 := range r1Grid {
			if r1 < minRatio*r0 {
				continue
			}
			if d.method == methodMonotone {
				deg = append(deg, 0)
				r0s = append(r0s, r0)
				r1s = append(r1s, r1)
				continue
			}
			for _, dg := range spec.degrees {
				deg = append(deg, dg)
				r0s = append(r0s, r0)
				r1s = append(r1s, r1)
			}
		}
	}
	d.nW = len(deg)
	d.nM = len(mRe)
	d.degree = deg
	d.r0 = r0s
	d.r1 = r1s
	d.mRe = append([]float64(nil), mRe...)
	d.mIm = append([]float64(nil), mIm...)
	if len(spec.sfrac) == d.nM {
		d.sfrac = append([]float64(nil), spec.sfrac...)
	}

	d.geom = make([]*geom, d.nW)
	for w := 0; w < d.nW; w++ {
		var (
			g   *geom
			err error
		)
		if d.method == methodMonotone {
			g, err = geomBuildBins(r0s[w], r1s[w], d.bins, spec.quadOrder,
				spec.maxLogSpan)
		} else {
			g, err = geomBuild(r0s[w], r1s[w], deg[w], spec.nBasis,
				spec.zeroBoundary, spec.quadOrder, spec.maxLogSpan)
		}
		if err != nil {
			return nil, err
		}
		d.geom[w] = g
	}

	nu := 0
	if d.nW > 0 {
		nu = d.geom[0].nUnknown
	}
	d.nu = nu

	total := d.nM * d.nW * nch * nu
	d.A = make([]float64, total)
	d.s0 = make([]float64, d.nM*d.nW)
	d.cond = make([]float64, d.nM*d.nW)

	// Каждая пара (im, w) пишет в собственные блоки d.A, d.s0 и d.cond,
	// поэтому сборку можно распараллелить без изменения результата.
	fill := func(k int) {
		im, w := k/d.nW, k%d.nW
		g := d.geom[w]
		nn := g.nNodes
		nunk := g.nUnknown
		off := ((im*d.nW + w) * nch * nunk)
		Ab := d.A[off : off+nch*nunk]

		logr := make([]float64, nn)
		for q := 0; q < nn; q++ {
			rc := g.nodes[q]
			if rc < kts[im].rMin {
				rc = kts[im].rMin
			}
			if rc > kts[im].rMax {
				rc = kts[im].rMax
			}
			logr[q] = math.Log(rc)
		}
		for j := 0; j < nch; j++ {
			hint := -1
			row := Ab[j*nunk : (j+1)*nunk]
			for q := 0; q < nn; q++ {
				kk := math.Exp(kts[im].sp[j].evalHint(logr[q], &hint))
				wgt := kk * kernelWeight(d.kernelOrder, g.nodes[q]) * g.weights[q]
				g.addNodeContribution(row, q, wgt)
			}
		}
		d.s0[im*d.nW+w], d.cond[im*d.nW+w] = svdNormCond(Ab, nch, nunk)
	}
	parallelFor(d.nM*d.nW, d.workers, fill)
	return d, nil
}

// reportCondition — отдельная строка диагностики: число обусловленности A по
// окнам. Для каждого окна берётся максимум по m (худшая обусловленность), затем
// выводятся min/median/max по окнам и худшее окно.
func reportCondition(d *design, c *console) {
	if d == nil || d.nW <= 0 || d.nM <= 0 {
		return
	}
	worst := make([]float64, d.nW)
	for w := 0; w < d.nW; w++ {
		v := math.Inf(-1)
		for im := 0; im < d.nM; im++ {
			cv := d.cond[im*d.nW+w]
			if math.IsNaN(cv) {
				cv = math.Inf(1)
			}
			if cv > v {
				v = cv
			}
		}
		worst[w] = v
	}
	vals := append([]float64(nil), worst...)
	sort.Float64s(vals)
	median := vals[len(vals)/2]
	wi := 0
	for w := 1; w < d.nW; w++ {
		if worst[w] > worst[wi] {
			wi = w
		}
	}
	if d.method == methodMonotone {
		c.printf("A condition number (max over m, by window): min %.3g, "+
			"median %.3g, max %.3g; worst window r0 = %g µm, r1 = %g µm\n",
			vals[0], median, vals[len(vals)-1], d.r0[wi], d.r1[wi])
		return
	}
	c.printf("A condition number (max over m, by window): min %.3g, "+
		"median %.3g, max %.3g; worst window r0 = %g µm, r1 = %g µm, "+
		"degree %d\n", vals[0], median, vals[len(vals)-1], d.r0[wi],
		d.r1[wi], d.degree[wi])
}
