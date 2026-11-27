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
	"errors"
	"math"
	"slices"

	"gonum.org/v1/gonum/mat"
)

// gammaSelect — правило выбора параметра регуляризации gamma (--gamma-select).
type gammaSelect string

const (
	gammaFixed gammaSelect = "fixed"
	gammaCV    gammaSelect = "cv"
	gammaErr   gammaSelect = "err"
)

// gammaSelects — значения --gamma-select в порядке справки.
var gammaSelects = []gammaSelect{gammaFixed, gammaCV, gammaErr}

type solution struct {
	mRe, mIm  float64
	degree    int // степень B-сплайна (0 в монотонном режиме)
	bins      int // число бинов (0 в режиме spline)
	r0, r1    float64
	gamma     float64
	C         []float64 // коэффициенты (spline) или приращения объёма v (monotone)
	nu        int
	geomIndex int
	mIdx      int     // индекс в развёрнутой сетке m (с учётом sfrac в смеси)
	sfrac     float64 // доля сфер по концентрации; NaN вне смеси
	err       float64
	gFit      []float64
	id        int
}

type solutions struct {
	sol         []solution
	n           int
	scale       float64
	method      methodKind
	bins        int
	regOrder    int
	kernelOrder int
}

// nLrows — число строк штрафа за негладкость для порядка регуляризации order.
func nLrows(nu, order int) int {
	switch order {
	case 0:
		return nu
	case 1:
		return max(nu-1, 0)
	default:
		return max(nu-2, 0)
	}
}

// regRowInto заполняет строку штрафа k для порядка order:
// 0 — γ·e_k (минимальный объём), 1 — γ·(e_k − e_{k+1}),
// 2 — γ·(e_k − 2·e_{k+1} + e_{k+2}).
func regRowInto(row []float64, k, order int, gamma float64) {
	for i := range row {
		row[i] = 0
	}
	switch order {
	case 0:
		row[k] = gamma
	case 1:
		row[k] = gamma
		row[k+1] = -gamma
	default:
		row[k] = gamma
		row[k+1] = -2.0 * gamma
		row[k+2] = gamma
	}
}

// buildAugRows — расширенная система [W A; gamma D; E] c = [W g1n; 0; 0],
// где D — строки штрафа порядка order, E — nExtra дополнительных строк
// ограничений (например, по δ_l), записанных подряд в extra (nExtra × nu).
func buildAugRows(a []float64, nch, nu int, gamma float64, weights, g1n []float64,
	extra []float64, nExtra, order int, aug, rhs []float64) {
	nL := nLrows(nu, order)
	for j := 0; j < nch; j++ {
		for i := 0; i < nu; i++ {
			aug[j*nu+i] = weights[j] * a[j*nu+i]
		}
		rhs[j] = weights[j] * g1n[j]
	}
	for k := 0; k < nL; k++ {
		regRowInto(aug[(nch+k)*nu:(nch+k+1)*nu], k, order, gamma)
		rhs[nch+k] = 0
	}
	for k := 0; k < nExtra; k++ {
		copy(aug[(nch+nL+k)*nu:(nch+nL+k+1)*nu], extra[k*nu:(k+1)*nu])
		rhs[nch+nL+k] = 0
	}
}

func buildAug(a []float64, nch, nu int, gamma float64, weights, g1n []float64,
	order int, aug, rhs []float64) {
	buildAugRows(a, nch, nu, gamma, weights, g1n, nil, 0, order, aug, rhs)
}

// solveGammaRows — то же, что solveGamma, но с nExtra дополнительными
// строками ограничений extra (nExtra × nu, правая часть — нули).
func solveGammaRows(a []float64, nch, nu int, gamma float64, weights, g1n []float64,
	nonneg bool, extra []float64, nExtra, order int) []float64 {
	nL := nLrows(nu, order)
	rows := nch + nL + nExtra
	aug := make([]float64, rows*nu)
	rhs := make([]float64, rows)
	buildAugRows(a, nch, nu, gamma, weights, g1n, extra, nExtra, order, aug, rhs)
	if nonneg {
		return nnls(aug, rows, nu, rhs)
	}
	return lstsq(aug, rows, nu, rhs)
}

func solveGamma(a []float64, nch, nu int, gamma float64, weights, g1n []float64,
	nonneg bool, order int) []float64 {
	return solveGammaRows(a, nch, nu, gamma, weights, g1n, nonneg, nil, 0, order)
}

func residualErr(a []float64, nch, nu int, c, g1n []float64) float64 {
	sum := 0.0
	for j := 0; j < nch; j++ {
		p := 0.0
		for i := 0; i < nu; i++ {
			p += a[j*nu+i] * c[i]
		}
		sum += math.Abs(p-g1n[j]) / g1n[j]
	}
	return sum / float64(nch)
}

// cvBruteforce — LOO-кросс-валидация прямым пересчётом (для NNLS и
// вырожденных leverage).
func cvBruteforce(a []float64, nch, nu int, gamma float64, weights, g1n []float64,
	nonneg bool, order int) float64 {
	nL := nLrows(nu, order)
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
				aug[r*nu+i] = weights[j] * a[j*nu+i]
			}
			rhs[r] = weights[j] * g1n[j]
			r++
		}
		for k := 0; k < nL; k++ {
			regRowInto(aug[(nch-1+k)*nu:(nch-1+k+1)*nu], k, order, gamma)
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
			pred += a[skip*nu+i] * c[i]
		}
		total += math.Abs(pred-g1n[skip]) / g1n[skip]
	}
	return total
}

// cvClosed — LOO-оценка закрытой формулой через рычаги; ok=false — нужен
// брутфорс.
func cvClosed(a []float64, nch, nu int, gamma float64, weights, g1n []float64,
	order int) (score float64, ok bool) {
	nL := nLrows(nu, order)
	rows := nch + nL
	if nu <= 0 || rows <= 0 {
		return 0, true
	}
	aug := make([]float64, rows*nu)
	rhs := make([]float64, rows)
	buildAug(a, nch, nu, gamma, weights, g1n, order, aug, rhs)

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
	cutoff := machineEps * float64(max(rows, nu)) * smax
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
				q += vmat.At(i, p) * a[j*nu+i]
			}
			lev += (q / vals[p]) * (q / vals[p])
		}
		denom := 1.0 - weights[j]*weights[j]*lev
		pred := 0.0
		for i := 0; i < nu; i++ {
			pred += a[j*nu+i] * c[i]
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
	slices.SortStableFunc(sol, func(a, b solution) int { return compareFloats(a.err, b.err) })
}

// scanVariants — перебор (m, окно) с регуляризацией. Если dc задано, в систему
// добавляется строка ограничения по измеренному δ_l. В монотонном режиме
// решение всегда ищется при условии v ≥ 0 (V(r) не убывает).
func scanVariants(d *design, g1 []float64, nch int, gammaSel gammaSelect,
	gammaRelFixed, gammaExpMin, gammaExpMax float64, nGamma int,
	nonneg, relative bool, dc *depolCons, c *console) (*solutions, error) {
	out := &solutions{method: d.method, bins: d.bins, regOrder: d.regOrder,
		kernelOrder: d.kernelOrder}
	if d.nW == 0 || d.nM == 0 {
		out.scale = 1.0
		return out, nil
	}
	for j := 0; j < nch; j++ {
		if !(g1[j] > 0) {
			return nil, errors.New("all measured coefficients g1 must be positive")
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
	// монотонность V требует неотрицательности приращений при любых опциях
	nonneg = nonneg || d.method == methodMonotone
	if d.nu > nch && d.method == methodSpline {
		c.warnf("[warning] unknowns per window (%d) exceed "+
			"measurements (%d): the solution relies mainly on regularization "+
			"(the system is underdetermined)\n", d.nu, nch)
	}

	nu := d.nu
	nm, nw := d.nM, d.nW
	total := nm * nw

	var grel []float64
	if gammaSel == gammaFixed {
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

	// Каждый индекс t = im*nw+iw пишет только в out.sol[t]; матрицы A — только
	// читаются, поэтому перебор можно распараллелить без изменения порядка.
	parallelFor(total, d.workers, func(t int) {
		im, iw := t/nw, t%nw
		off := (im*nw + iw) * nch * nu
		A := d.A[off : off+nch*nu]
		s0 := d.s0[im*nw+iw]
		gamma := 0.0
		var cNorm []float64

		if gammaSel == gammaFixed {
			gamma = grel[0] * s0
			cNorm = solveGamma(A, nch, nu, gamma, weights, g1n, nonneg,
				d.regOrder)
		} else {
			best := math.Inf(1)
			for k := 0; k < len(grel); k++ {
				gk := grel[k] * s0
				var score float64
				if gammaSel == gammaCV {
					if nonneg {
						score = cvBruteforce(A, nch, nu, gk, weights, g1n, true,
							d.regOrder)
					} else {
						sc, ok := cvClosed(A, nch, nu, gk, weights, g1n,
							d.regOrder)
						if !ok {
							score = cvBruteforce(A, nch, nu, gk, weights, g1n,
								false, d.regOrder)
						} else {
							score = sc
						}
					}
				} else {
					cv := solveGamma(A, nch, nu, gk, weights, g1n, nonneg,
						d.regOrder)
					score = residualErr(A, nch, nu, cv, g1n)
				}
				if score < best {
					best = score
					gamma = gk
				}
			}
			cNorm = solveGamma(A, nch, nu, gamma, weights, g1n, nonneg,
				d.regOrder)
		}

		cAbs := make([]float64, nu)
		for i := 0; i < nu; i++ {
			cAbs[i] = cNorm[i] * scale
		}
		// второй проход: то же γ, но с строкой ограничения δ_l
		if extra, nExtra := dc.rows(im, iw, cAbs); nExtra > 0 {
			c2 := solveGammaRows(A, nch, nu, gamma, weights, g1n,
				nonneg, extra, nExtra, d.regOrder)
			for i := 0; i < nu; i++ {
				cAbs[i] = c2[i] * scale
			}
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
		so.mIdx = im
		so.sfrac = math.NaN()
		if d.sfrac != nil {
			so.sfrac = d.sfrac[im]
		}
		so.degree = d.degree[iw]
		so.bins = d.bins
		so.r0 = d.r0[iw]
		so.r1 = d.r1[iw]
		so.gamma = gamma
		so.C = append([]float64(nil), cAbs...)
		so.nu = nu
		so.geomIndex = iw
		so.err = err
		so.gFit = append([]float64(nil), gfit...)
		so.id = t
	})

	sortSolutions(out.sol)
	return out, nil
}
