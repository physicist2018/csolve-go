package solve

import (
	"fmt"
	"math"
)

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

	// Смесь сфер с несферической компонентой: A(f) = (1-f)A + f*A2.
	A2    []float64 // матрица второй (сферической) компоненты
	fracs []float64 // доли сфер f_k, k = 0..nf-1
	nf    int
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

	deg, r0s, r1s := windowsBuild(degrees, r0Grid, r1Grid, minRatio)
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

// mixWith — включает смесь: вторая матрица d2 (сферическая компонента) и
// доли сфер fracs. Обе матрицы должны быть построены на одинаковых окнах и
// сетке m, т.е. одним и тем же вызовом designBuild.
func (d *design) mixWith(d2 *design, fracs []float64) {
	if d2 == nil || len(fracs) == 0 {
		return
	}
	if d2.nM != d.nM || d2.nW != d.nW || d2.nch != d.nch || d2.nu != d.nu {
		fatal("internal error: mixture designs have different shapes")
	}
	d.A2 = d2.A
	d.fracs = append([]float64(nil), fracs...)
	d.nf = len(d.fracs)
}

// windowsBuild — окна перебора: degree × r0 × r1 с отбраковкой r1 < minRatio·r0.
func windowsBuild(degrees []int, r0Grid, r1Grid []float64, minRatio float64) ([]int, []float64, []float64) {
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
	return deg, r0s, r1s
}

// countWindows — число окон (degree × r0 × r1 после фильтра --min-ratio).
func countWindows(degrees []int, r0Grid, r1Grid []float64, minRatio float64) int {
	deg, _, _ := windowsBuild(degrees, r0Grid, r1Grid, minRatio)
	return len(deg)
}

// variantTotal — общее число вариантов nM · nW · nf (nf < 1 трактуется как 1).
func variantTotal(nM, nW, nf int) int {
	if nM <= 0 || nW <= 0 {
		return 0
	}
	if nf < 1 {
		nf = 1
	}
	return nM * nW * nf
}

// variantReport — "N (M m × W windows[ × F f])" для консольного отчёта.
func variantReport(nM, nW, nf int) string {
	if nf < 1 {
		nf = 1
	}
	s := fmt.Sprintf("%d m × %d windows", nM, nW)
	if nf > 1 {
		s += fmt.Sprintf(" × %d f", nf)
	}
	return fmt.Sprintf("%d (%s)", variantTotal(nM, nW, nf), s)
}

// variantCount — общее число вариантов перебора (m × окна × f).
func (d *design) variantCount() int { return variantTotal(d.nM, d.nW, d.nf) }

// variantReport — расшифровка числа вариантов для консольного отчёта.
func (d *design) variantReport() string { return variantReport(d.nM, d.nW, d.nf) }

// fracAt — доля сфер для индекса kf (0 вне смеси).
func (d *design) fracAt(kf int) float64 {
	if d == nil || d.A2 == nil || kf < 0 || kf >= len(d.fracs) {
		return 0
	}
	return d.fracs[kf]
}

// block — матрица отклика A(f) для (m, окно, доля сфер) и её спектральная
// норма. Вне смеси (A2 == nil) возвращается готовый блок без копирования.
func (d *design) block(im, iw, kf int, scratch []float64) ([]float64, float64) {
	off := (im*d.nW + iw) * d.nch * d.nu
	size := d.nch * d.nu
	if d.A2 == nil {
		return d.A[off : off+size], d.s0[im*d.nW+iw]
	}
	f := d.fracAt(kf)
	a1 := d.A[off : off+size]
	a2 := d.A2[off : off+size]
	buf := scratch
	if len(buf) < size {
		buf = make([]float64, size)
	}
	for i := 0; i < size; i++ {
		buf[i] = (1.0-f)*a1[i] + f*a2[i]
	}
	return buf, svdNorm2(buf, d.nch, d.nu)
}
