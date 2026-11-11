package solve

import "math"

// glNodesWeights — узлы и веса Гаусса–Лежандра на [-1, 1] (по возрастанию).
func glNodesWeights(order int) ([]float64, []float64) {
	x := make([]float64, order)
	w := make([]float64, order)
	n := order
	if n <= 0 {
		return x, w
	}
	for i := 0; i < n; i++ {
		z := math.Cos(csPi * (float64(i) + 0.75) / (float64(n) + 0.5))
		pp := 1.0
		for it := 0; it < 100; it++ {
			p1, p2 := 1.0, 0.0
			for j := 1; j <= n; j++ {
				p3 := p2
				p2 = p1
				p1 = ((2.0*float64(j)-1.0)*z*p2 - (float64(j)-1.0)*p3) / float64(j)
			}
			pp = float64(n) * (z*p1 - p2) / (z*z - 1.0)
			z1 := z
			z = z1 - p1/pp
			if math.Abs(z-z1) <= 1e-16 {
				break
			}
		}
		// пересчёт производной в сошедшейся точке
		p1, p2 := 1.0, 0.0
		for j := 1; j <= n; j++ {
			p3 := p2
			p2 = p1
			p1 = ((2.0*float64(j)-1.0)*z*p2 - (float64(j)-1.0)*p3) / float64(j)
		}
		pp = float64(n) * (z*p1 - p2) / (z*z - 1.0)
		x[n-1-i] = z
		w[n-1-i] = 2.0 / ((1.0 - z*z) * pp * pp)
	}
	return x, w
}

func findSpan(U []float64, nbf, p int, u float64) int {
	n := nbf - 1
	if u <= U[p] {
		return p
	}
	if u >= U[n+1] {
		return n
	}
	low, high := p, n+1
	mid := (low + high) / 2
	for u < U[mid] || u >= U[mid+1] {
		if u < U[mid] {
			high = mid
		} else {
			low = mid
		}
		mid = (low + high) / 2
	}
	return mid
}

// bsplineValues — значения всех B-сплайнов степени p на векторе узлов U.
// Длина результата: len(U) - p - 1.
func bsplineValues(U []float64, p int, t float64) []float64 {
	nbf := len(U) - p - 1
	if nbf <= 0 {
		return []float64{}
	}
	out := make([]float64, nbf)
	u := t
	if u >= U[nbf] {
		u = math.Nextafter(U[nbf], math.Inf(-1))
	}
	if u < U[p] {
		u = U[p]
	}
	span := findSpan(U, nbf, p, u)
	if p+1 > 16 {
		fatal("B-spline degree too large")
	}
	var N, left, right [17]float64
	N[0] = 1.0
	for j := 1; j <= p; j++ {
		left[j] = u - U[span+1-j]
		right[j] = U[span+j] - u
		saved := 0.0
		for r := 0; r < j; r++ {
			denom := right[r+1] + left[j-r]
			temp := 0.0
			if denom != 0.0 {
				temp = N[r] / denom
			}
			N[r] = saved + right[r+1]*temp
			saved = left[j-r] * temp
		}
		N[j] = saved
	}
	for j := 0; j <= p; j++ {
		idx := span - p + j
		if idx >= 0 && idx < nbf {
			out[idx] = N[j]
		}
	}
	return out
}

// localBasis — локальные значения (только p+1 ненулевых) в окне i0..i0+m-1.
func localBasis(U []float64, q int, u float64, i0, m int) []float64 {
	out := make([]float64, m)
	nbfq := len(U) - q - 1
	uu := u
	if uu >= U[nbfq] {
		uu = math.Nextafter(U[nbfq], math.Inf(-1))
	}
	if uu < U[q] {
		uu = U[q]
	}
	sq := findSpan(U, nbfq, q, uu)
	var N, left, right [17]float64
	N[0] = 1.0
	for j := 1; j <= q; j++ {
		left[j] = uu - U[sq+1-j]
		right[j] = U[sq+j] - uu
		saved := 0.0
		for r := 0; r < j; r++ {
			denom := right[r+1] + left[j-r]
			temp := 0.0
			if denom != 0.0 {
				temp = N[r] / denom
			}
			N[r] = saved + right[r+1]*temp
			saved = left[j-r] * temp
		}
		N[j] = saved
	}
	for j := 0; j <= q; j++ {
		idx := sq - q + j
		if idx >= i0 && idx < i0+m {
			out[idx-i0] = N[j]
		}
	}
	return out
}

func localDeriv(U []float64, p, deriv int, u float64, i0, m int) []float64 {
	if deriv <= 0 {
		return localBasis(U, p, u, i0, m)
	}
	out := make([]float64, m)
	if deriv > p {
		return out
	}
	v := localDeriv(U, p-1, deriv-1, u, i0, m+1)
	for k := 0; k < m; k++ {
		i := i0 + k
		den1 := U[i+p] - U[i]
		den2 := U[i+p+1] - U[i+1]
		d1, d2 := 0.0, 0.0
		if den1 > 0 {
			d1 = v[k] / den1
		}
		if den2 > 0 {
			d2 = v[k+1] / den2
		}
		out[k] = float64(p) * (d1 - d2)
	}
	return out
}

// bsplineDerivWindow — производная порядка deriv: p+1 локальных значений.
func bsplineDerivWindow(U []float64, p, deriv int, t float64) ([]float64, int) {
	nbf := len(U) - p - 1
	u := t
	if u >= U[nbf] {
		u = math.Nextafter(U[nbf], math.Inf(-1))
	}
	if u < U[p] {
		u = U[p]
	}
	s := findSpan(U, nbf, p, u)
	return localDeriv(U, p, deriv, u, s-p, p+1), s
}

// quadratureGrid — составная квадратура Гаусса–Лежандра по [a, b] с
// разбиением так, чтобы ширина подотрезка в ln r не превосходила maxLogSpan.
func quadratureGrid(breaks []float64, order int, maxLogSpan float64) ([]float64, []float64) {
	nodes := []float64{}
	weights := []float64{}
	gx, gw := glNodesWeights(order)
	for i := 0; i < len(breaks)-1; i++ {
		a, b := breaks[i], breaks[i+1]
		if !(b > a) {
			continue
		}
		nSeg := int(math.Ceil(math.Log(b/a) / maxLogSpan))
		if nSeg < 1 {
			nSeg = 1
		}
		bounds := linspace(math.Log(a), math.Log(b), nSeg+1)
		for k := 0; k < nSeg; k++ {
			aa := math.Exp(bounds[k])
			bb := math.Exp(bounds[k+1])
			mid := 0.5 * (bb + aa)
			half := 0.5 * (bb - aa)
			for q := 0; q < order; q++ {
				nodes = append(nodes, half*gx[q]+mid)
				weights = append(weights, half*gw[q])
			}
		}
	}
	return nodes, weights
}

// geom — базис, квадратура и геометрия окна [r0, r1].
type geom struct {
	degree, nBasis, nUnknown int
	interiorOnly             bool
	r0, r1                   float64
	nKnots                   int
	knots                    []float64
	nNodes                   int
	nodes                    []float64
	weights                  []float64
	values                   []float64 // nNodes x nUnknown
	dvol                     []float64 // nNodes
}

func geomBuild(r0, r1 float64, degree, nBasis int, interiorOnly bool, order int, maxLogSpan float64) *geom {
	pInternal := nBasis - degree - 1
	if pInternal < 0 {
		fatal("expected n_basis >= degree + 1")
	}
	if interiorOnly && nBasis < 3 {
		fatal("boundary conditions n(r0)=n(r1)=0 require n_basis >= 3")
	}

	nk := nBasis + degree + 1
	knots := make([]float64, nk)
	for i := 0; i <= degree; i++ {
		knots[i] = r0
		knots[nk-1-i] = r1
	}
	if pInternal > 0 {
		inter := geomspace(r0, r1, pInternal+2)
		for i := 0; i < pInternal; i++ {
			knots[degree+1+i] = inter[i+1]
		}
	}

	// уникальные узлы (breaks)
	breaks := make([]float64, 0, nk)
	for i := 0; i < nk; i++ {
		if len(breaks) == 0 || knots[i] != breaks[len(breaks)-1] {
			breaks = append(breaks, knots[i])
		}
	}

	nodes, weights := quadratureGrid(breaks, order, maxLogSpan)

	nu := nBasis
	if interiorOnly {
		nu -= 2
	}
	values := make([]float64, len(nodes)*nu)
	off := 0
	if interiorOnly {
		off = 1
	}
	for q := range nodes {
		full := bsplineValues(knots, degree, nodes[q])
		for i := 0; i < nu; i++ {
			values[q*nu+i] = full[off+i]
		}
	}
	dvol := make([]float64, len(nodes))
	for q, r := range nodes {
		dvol[q] = 3.0 / (4.0 * csPi * r * r * r * r)
	}

	return &geom{
		degree:       degree,
		nBasis:       nBasis,
		nUnknown:     nu,
		interiorOnly: interiorOnly,
		r0:           r0,
		r1:           r1,
		nKnots:       nk,
		knots:        knots,
		nNodes:       len(nodes),
		nodes:        nodes,
		weights:      weights,
		values:       values,
		dvol:         dvol,
	}
}
