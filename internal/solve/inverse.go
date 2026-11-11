package solve

import (
	"fmt"
	"math"
	"os"
	"sort"

	"gonum.org/v1/gonum/mat"
)

const (
	gammaFixed = 0
	gammaCV    = 1
	gammaErr   = 2
)

type solution struct {
	mRe, mIm  float64
	degree    int
	r0, r1    float64
	gamma     float64
	C         []float64
	nu        int
	geomIndex int
	err       float64
	gFit      []float64
	id        int
}

type solutions struct {
	sol   []solution
	n     int
	scale float64
}

func nLrows(nu int) int {
	if nu > 2 {
		return nu - 2
	}
	return 0
}

// buildAug — расширенная система [W A; gamma L] c = [W g1n; 0].
func buildAug(A []float64, nch, nu int, gamma float64, weights, g1n []float64,
	aug, rhs []float64) {
	nL := nLrows(nu)
	for j := 0; j < nch; j++ {
		for i := 0; i < nu; i++ {
			aug[j*nu+i] = weights[j] * A[j*nu+i]
		}
		rhs[j] = weights[j] * g1n[j]
	}
	for k := 0; k < nL; k++ {
		row := aug[(nch+k)*nu : (nch+k+1)*nu]
		for i := range row {
			row[i] = 0
		}
		row[k] = gamma
		row[k+1] = -2.0 * gamma
		row[k+2] = gamma
		rhs[nch+k] = 0
	}
}

func solveGamma(A []float64, nch, nu int, gamma float64, weights, g1n []float64,
	nonneg bool) []float64 {
	nL := nLrows(nu)
	rows := nch + nL
	aug := make([]float64, rows*nu)
	rhs := make([]float64, rows)
	buildAug(A, nch, nu, gamma, weights, g1n, aug, rhs)
	if nonneg {
		return nnls(aug, rows, nu, rhs)
	}
	return lstsq(aug, rows, nu, rhs)
}

func residualErr(A []float64, nch, nu int, c, g1n []float64) float64 {
	sum := 0.0
	for j := 0; j < nch; j++ {
		p := 0.0
		for i := 0; i < nu; i++ {
			p += A[j*nu+i] * c[i]
		}
		sum += math.Abs(p-g1n[j]) / g1n[j]
	}
	return sum / float64(nch)
}

// cvBruteforce — LOO-кросс-валидация прямым пересчётом (для NNLS и
// вырожденных leverage).
func cvBruteforce(A []float64, nch, nu int, gamma float64, weights, g1n []float64,
	nonneg bool) float64 {
	nL := nLrows(nu)
	rows := (nch - 1) + nL
	aug := make([]float64, max(rows, 0)*nu)
	rhs := make([]float64, max(rows, 0))
	total := 0.0
	for skip := 0; skip < nch; skip++ {
		r := 0
		for j := 0; j < nch; j++ {
			if j == skip {
				continue
			}
			for i := 0; i < nu; i++ {
				aug[r*nu+i] = weights[j] * A[j*nu+i]
			}
			rhs[r] = weights[j] * g1n[j]
			r++
		}
		for k := 0; k < nL; k++ {
			row := aug[(nch-1+k)*nu : (nch-1+k+1)*nu]
			for i := range row {
				row[i] = 0
			}
			row[k] = gamma
			row[k+1] = -2.0 * gamma
			row[k+2] = gamma
			rhs[nch-1+k] = 0
		}
		var c []float64
		if nonneg {
			c = nnls(aug, rows, nu, rhs)
		} else {
			c = lstsq(aug, rows, nu, rhs)
		}
		pred := 0.0
		for i := 0; i < nu; i++ {
			pred += A[skip*nu+i] * c[i]
		}
		total += math.Abs(pred-g1n[skip]) / g1n[skip]
	}
	return total
}

// cvClosed — LOO-оценка закрытой формулой через рычаги; ok=false — нужен
// брутфорс.
func cvClosed(A []float64, nch, nu int, gamma float64, weights, g1n []float64) (
	score float64, ok bool) {
	nL := nLrows(nu)
	rows := nch + nL
	if nu <= 0 || rows <= 0 {
		return 0, true
	}
	aug := make([]float64, rows*nu)
	rhs := make([]float64, rows)
	buildAug(A, nch, nu, gamma, weights, g1n, aug, rhs)

	data := append([]float64(nil), aug...)
	var svd mat.SVD
	if !svd.Factorize(mat.NewDense(rows, nu, data), mat.SVDThin) {
		return 0, false
	}
	vals := svd.Values(nil)
	smax := 0.0
	if len(vals) > 0 {
		smax = vals[0]
	}
	cutoff := csEps * float64(max(rows, nu)) * smax
	keep := make([]bool, len(vals))
	rank := 0
	for k, v := range vals {
		keep[k] = v > cutoff && v != 0
		if keep[k] {
			rank++
		}
	}

	c := make([]float64, nu)
	if rank > 0 {
		bb := mat.NewVecDense(rows, append([]float64(nil), rhs...))
		dst := mat.NewVecDense(nu, nil)
		svd.SolveVecTo(dst, bb, rank)
		copy(c, dst.RawVector().Data)
	}

	var vmat mat.Dense
	svd.VTo(&vmat)

	score = 0.0
	ok = true
	for j := 0; j < nch; j++ {
		lev := 0.0
		for p := 0; p < len(vals); p++ {
			if !keep[p] {
				continue
			}
			q := 0.0
			for i := 0; i < nu; i++ {
				q += vmat.At(i, p) * A[j*nu+i]
			}
			lev += (q / vals[p]) * (q / vals[p])
		}
		denom := 1.0 - weights[j]*weights[j]*lev
		pred := 0.0
		for i := 0; i < nu; i++ {
			pred += A[j*nu+i] * c[i]
		}
		if !(denom > 0) {
			ok = false
			break
		}
		score += math.Abs((g1n[j]-pred)/denom) / g1n[j]
	}
	return score, ok
}

// sortSolutions — устойчивая сортировка по Err.
func sortSolutions(sol []solution) {
	sort.SliceStable(sol, func(i, j int) bool { return sol[i].err < sol[j].err })
}

// scanVariants — перебор (m, окно) с регуляризацией по кривизне.
func scanVariants(d *design, g1 []float64, nch, gammaSelect int,
	gammaRelFixed, gammaExpMin, gammaExpMax float64, nGamma int,
	nonneg, relative bool) *solutions {
	out := &solutions{}
	if d.nW == 0 || d.nM == 0 {
		out.scale = 1.0
		return out
	}
	for j := 0; j < nch; j++ {
		if !(g1[j] > 0) {
			fatal("all measured coefficients g1 must be positive")
		}
	}
	scale := g1[0]
	for j := 1; j < nch; j++ {
		if g1[j] > scale {
			scale = g1[j]
		}
	}
	g1n := make([]float64, nch)
	weights := make([]float64, nch)
	for j := 0; j < nch; j++ {
		g1n[j] = g1[j] / scale
		if relative {
			weights[j] = 1.0 / g1n[j]
		} else {
			weights[j] = 1.0
		}
	}
	if d.nu > nch {
		fmt.Fprintf(os.Stderr, "[warning] unknowns per window (%d) exceed "+
			"measurements (%d): the solution relies mainly on regularization "+
			"(the system is underdetermined)\n", d.nu, nch)
	}

	nu := d.nu
	nm, nw := d.nM, d.nW
	total := nm * nw

	var grel []float64
	if gammaSelect == gammaFixed {
		grel = []float64{gammaRelFixed}
	} else {
		ng := nGamma
		if ng <= 0 {
			ng = 1
		}
		lin := linspace(gammaExpMin, gammaExpMax, ng)
		grel = make([]float64, ng)
		for k := 0; k < ng; k++ {
			grel[k] = math.Pow(10.0, lin[k])
		}
	}

	out.sol = make([]solution, total)
	out.n = total
	out.scale = scale

	t := 0
	for im := 0; im < nm; im++ {
		for iw := 0; iw < nw; iw++ {
			off := (im*nw + iw) * nch * nu
			A := d.A[off : off+nch*nu]
			s0 := d.s0[im*nw+iw]
			gamma := 0.0
			var cNorm []float64

			if gammaSelect == gammaFixed {
				gamma = grel[0] * s0
				cNorm = solveGamma(A, nch, nu, gamma, weights, g1n, nonneg)
			} else {
				best := math.Inf(1)
				for k := 0; k < len(grel); k++ {
					gk := grel[k] * s0
					var score float64
					if gammaSelect == gammaCV {
						if nonneg {
							score = cvBruteforce(A, nch, nu, gk, weights, g1n, true)
						} else {
							sc, ok := cvClosed(A, nch, nu, gk, weights, g1n)
							if !ok {
								score = cvBruteforce(A, nch, nu, gk, weights, g1n, false)
							} else {
								score = sc
							}
						}
					} else {
						c := solveGamma(A, nch, nu, gk, weights, g1n, nonneg)
						score = residualErr(A, nch, nu, c, g1n)
					}
					if score < best {
						best = score
						gamma = gk
					}
				}
				cNorm = solveGamma(A, nch, nu, gamma, weights, g1n, nonneg)
			}

			cAbs := make([]float64, nu)
			for i := 0; i < nu; i++ {
				cAbs[i] = cNorm[i] * scale
			}
			gfit := make([]float64, nch)
			for j := 0; j < nch; j++ {
				p := 0.0
				for i := 0; i < nu; i++ {
					p += A[j*nu+i] * cAbs[i]
				}
				gfit[j] = p
			}
			err := 0.0
			for j := 0; j < nch; j++ {
				err += math.Abs(gfit[j]-g1[j]) / g1[j]
			}
			err /= float64(nch)

			so := &out.sol[t]
			so.mRe = d.mRe[im]
			so.mIm = d.mIm[im]
			so.degree = d.degree[iw]
			so.r0 = d.r0[iw]
			so.r1 = d.r1[iw]
			so.gamma = gamma
			so.C = append([]float64(nil), cAbs...)
			so.nu = nu
			so.geomIndex = iw
			so.err = err
			so.gFit = append([]float64(nil), gfit...)
			so.id = t
			t++
		}
	}

	sortSolutions(out.sol)
	return out
}
