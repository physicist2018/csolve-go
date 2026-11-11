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
	zeroBoundary    bool
	order           int
	maxLogSpan      float64
	nBasis          int
}

func designBuild(ch []channel, nch int, kts []*ktable, mRe, mIm []float64,
	degrees []int, r0Grid, r1Grid []float64, minRatio float64,
	nBasisIn int, zeroBoundary bool, order int, maxLogSpan float64) *design {
	_ = ch
	d := &design{
		nch:          nch,
		nBasis:       nBasisIn,
		zeroBoundary: zeroBoundary,
		order:        order,
		maxLogSpan:   maxLogSpan,
	}
	if d.nBasis <= 0 {
		d.nBasis = nch
	}

	deg := []int{}
	r0s := []float64{}
	r1s := []float64{}
	for _, dg := range degrees {
		for _, r0 := range r0Grid {
			for _, r1 := range r1Grid {
				if r1 < minRatio*r0 {
					continue
				}
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
		d.geom[w] = geomBuild(r0s[w], r1s[w], deg[w], d.nBasis,
			zeroBoundary, order, maxLogSpan)
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
					bv := g.values[q*nunk : (q+1)*nunk]
					for i := 0; i < nunk; i++ {
						row[i] += wgt * bv[i]
					}
				}
			}
			d.s0[im*d.nW+w] = svdNorm2(Ab, nch, nunk)
		}
	}
	return d
}
