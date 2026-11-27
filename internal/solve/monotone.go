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
	"fmt"
	"math"
)

// methodKind — способ решения обратной задачи (--method).
type methodKind string

const (
	// methodMonotone — метод монотонных функций: восстанавливается кумулятивный
	// объём V(r) по неотрицательным приращениям на бинах сетки ln r.
	methodMonotone methodKind = "monotone"
	// methodSpline — прежний алгоритм: B-сплайны на окне с перебором степени.
	methodSpline methodKind = "spline"
)

// methodKinds — значения --method в порядке справки.
var methodKinds = []methodKind{methodMonotone, methodSpline}

// designSpec — параметры построения матриц A и геометрии окон.
type designSpec struct {
	method       methodKind
	degrees      []int
	nBasis       int
	bins         int
	zeroBoundary bool
	quadOrder    int
	maxLogSpan   float64
	regOrder     int
	kernelOrder  int
	// workers — число воркеров для сборки матриц A; 0 = auto (NumCPU-1).
	workers int
	// sfrac — доля сфер (по концентрации) для каждого значения m сканирования;
	// nil вне режима смеси.
	sfrac []float64
}

// binsFor — число бинов монотонной сетки при заданном --v-bins (0 = auto).
// Автозначение: max(2·nch, 8), но не больше vBinsMax.
func binsFor(vBins, nch int) int {
	if vBins > 0 {
		return vBins
	}
	n := 2 * nch
	if n < 8 {
		n = 8
	}
	if n > vBinsMax {
		n = vBinsMax
	}
	return n
}

// vBinsMax — верхняя граница числа бинов (совпадает с пределом --v-bins).
const vBinsMax = 64

// geomBuildBins — геометрия монотонного метода: сетка бинов по ln r на окне
// [r0, r1], квадратура Гаусса–Лежандра внутри каждого бина.
//
// Неизвестные — приращения объёма v_k ≥ 0 на бине k; связь с распределением
// n(r) = dV/dln r: n = v_k/Δln r_k на бине k. Поэтому вклад узла q в матрицу A
// равен весу, умноженному на 1/Δln r_k, а не на значение базисной функции.
func geomBuildBins(r0, r1 float64, bins, order int, maxLogSpan float64) (*geom, error) {
	if bins < 1 {
		return nil, errors.New("the monotone method needs at least one bin (--v-bins >= 1)")
	}
	if !(r1 > r0) || !(r0 > 0) {
		return nil, fmt.Errorf("invalid window for the monotone method: r0 = %g, r1 = %g", r0, r1)
	}
	edges := geomspace(r0, r1, bins+1)
	g := &geom{
		kind:     basisBins,
		r0:       r0,
		r1:       r1,
		nBasis:   bins,
		nUnknown: bins,
		binEdges: edges,
		invBinW:  make([]float64, bins),
	}
	for k := 0; k < bins; k++ {
		dln := math.Log(edges[k+1] / edges[k])
		if !(dln > 0) || !isFinite(dln) {
			return nil, fmt.Errorf("degenerate size bin %d on [%g, %g]", k+1, r0, r1)
		}
		g.invBinW[k] = 1.0 / dln
	}
	for k := 0; k < bins; k++ {
		nodes, weights := quadratureGrid([]float64{edges[k], edges[k+1]}, order,
			maxLogSpan)
		for q := range nodes {
			g.nodes = append(g.nodes, nodes[q])
			g.weights = append(g.weights, weights[q])
			g.nodeBin = append(g.nodeBin, k)
		}
	}
	g.nNodes = len(g.nodes)
	return g, nil
}

// binIndexAt — индекс бина, содержащего r (0..len(edges)-2); -1 вне сетки.
func binIndexAt(edges []float64, r float64) int {
	n := len(edges)
	if n < 2 || r < edges[0] || r > edges[n-1] {
		return -1
	}
	lo, hi := 0, n-1
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if r < edges[mid] {
			hi = mid
		} else {
			lo = mid
		}
	}
	if lo > n-2 {
		lo = n - 2
	}
	return lo
}

// cumulativeVolume — кумулятивный объём V(r) = ∫ n(r′) dln r′ по среднему
// распределению n на сетке rGrid (трапеции по ln r). V(r) не убывает там, где
// n ≥ 0; в точках без покрытия и после разрыва покрытия — NaN.
func cumulativeVolume(rGrid, n []float64) []float64 {
	out := make([]float64, len(rGrid))
	v := 0.0
	started := false
	broken := false
	for i := range rGrid {
		if !isFinite(n[i]) {
			out[i] = math.NaN()
			if started {
				broken = true
			}
			continue
		}
		switch {
		case !started:
			out[i] = 0
			started = true
		case broken:
			out[i] = math.NaN()
		default:
			dl := math.Log(rGrid[i]) - math.Log(rGrid[i-1])
			v += 0.5 * dl * (n[i-1] + n[i])
			out[i] = v
		}
	}
	return out
}

// volumeTotal — последнее конечное значение кумулятивного объёма (NaN, если
// покрытия нет).
func volumeTotal(vCum []float64) float64 {
	for i := len(vCum) - 1; i >= 0; i-- {
		if isFinite(vCum[i]) {
			return vCum[i]
		}
	}
	return math.NaN()
}

// clampNonNegative обнуляет небольшие отрицательные значения n(r), которые
// может дать сглаживание среднего: монотонный метод восстанавливает n ≥ 0, и
// отрицательные выбросы — артефакт сглаживающего сплайна.
func clampNonNegative(v []float64) {
	for i := range v {
		if isFinite(v[i]) && v[i] < 0 {
			v[i] = 0
		}
	}
}

// monotoneCheck — контроль монотонности V на сетке: наибольший отрицательный
// шаг (в единицах полного объёма) и число нарушений. Используется для
// диагностики в консоли.
func monotoneCheck(vCum []float64) (minStep float64, violations int) {
	minStep = 0
	total := volumeTotal(vCum)
	scale := math.Abs(total)
	if !(scale > 0) {
		scale = 1
	}
	for i := 1; i < len(vCum); i++ {
		if !isFinite(vCum[i]) || !isFinite(vCum[i-1]) {
			continue
		}
		d := vCum[i] - vCum[i-1]
		if d < 0 {
			violations++
			if d < minStep {
				minStep = d
			}
		}
	}
	return minStep / scale, violations
}
