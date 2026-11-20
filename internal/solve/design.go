package solve

import "math"

// design — матрицы A для всех (m, окно) за один проход:
// A[j,i] = int K_j(r) q(r) B_i(r) dr, q(r) = 3/(4 pi r^4).
type design struct {
	nM, nW, nch, nu int
	A               []float64 // [nM][nW][nch][nu]
	s0              []float64 // [nM][nW]
	degree          []int
	r0, r1          []float64
	mRe, mIm        []float64
	geom            []*geom
	method          methodKind
	bins            int
	regOrder        int
}

func designBuild(ch []channel, nch int, kts []*ktable, mRe, mIm []float64,
	r0Grid, r1Grid []float64, minRatio float64, spec designSpec) (*design, error) {
	_ = ch
	d := &design{
		nch:      nch,
		method:   spec.method,
		regOrder: spec.regOrder,
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

	for im := 0; im < d.nM; im++ {
		for w := 0; w < d.nW; w++ {
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
					k := math.Exp(kts[im].sp[j].evalHint(logr[q], &hint))
					wgt := k * g.dvol[q] * g.weights[q]
					g.addNodeContribution(row, q, wgt)
				}
			}
			d.s0[im*d.nW+w] = svdNorm2(Ab, nch, nunk)
		}
	}
	return d, nil
}
