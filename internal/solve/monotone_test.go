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
	"testing"
)

// testKtable — таблица ядер с аналитическим K(r) на сетке ln r.
func testKtable(rMin, rMax float64, n int, kf func(j int, r float64) float64) *ktable {
	r := geomspace(rMin, rMax, n)
	kt := &ktable{n: n, r: r, rMin: rMin, rMax: rMax, nch: 2}
	lr := make([]float64, n)
	for i := range r {
		lr[i] = math.Log(r[i])
	}
	for j := 0; j < kt.nch; j++ {
		val := make([]float64, n)
		for i := range r {
			val[i] = math.Log(kf(j, r[i]))
		}
		kt.sp = append(kt.sp, newSpline(lr, val))
	}
	return kt
}

func TestGeomBuildBins(t *testing.T) {
	const r0, r1 = 0.1, 2.0
	const bins = 5
	g, err := geomBuildBins(r0, r1, bins, 16, 0.1)
	if err != nil {
		t.Fatalf("geomBuildBins: %v", err)
	}
	if g.nUnknown != bins || len(g.invBinW) != bins {
		t.Fatalf("bins = %d/%d, want %d", g.nUnknown, len(g.invBinW), bins)
	}
	if len(g.binEdges) != bins+1 {
		t.Fatalf("edges = %d, want %d", len(g.binEdges), bins+1)
	}
	checkClose(t, "r0 edge", g.binEdges[0], r0, 1e-15)
	checkClose(t, "r1 edge", g.binEdges[bins], r1, 1e-15)

	// ширины в ln r одинаковы, r-ширины геометрические
	wantW := math.Log(r1/r0) / bins
	seen := make([]bool, bins)
	for k := 0; k < bins; k++ {
		dln := math.Log(g.binEdges[k+1] / g.binEdges[k])
		checkClose(t, "bin width in ln r", dln, wantW, 1e-12)
		checkClose(t, "invBinW", g.invBinW[k], 1.0/dln, 1e-12)
	}
	// каждый узел относится к своему бину, все бины покрыты
	for q := 0; q < g.nNodes; q++ {
		b := g.nodeBin[q]
		if b < 0 || b >= bins {
			t.Fatalf("node %d has bin %d", q, b)
		}
		seen[b] = true
		if g.nodes[q] < g.binEdges[b] || g.nodes[q] > g.binEdges[b+1] {
			t.Fatalf("node %g outside bin %d", g.nodes[q], b)
		}
	}
	for k, ok := range seen {
		if !ok {
			t.Errorf("bin %d has no quadrature nodes", k)
		}
	}
	// веса квадратуры внутри бина дают его ширину в r
	for k := 0; k < bins; k++ {
		s := 0.0
		for q := 0; q < g.nNodes; q++ {
			if g.nodeBin[q] == k {
				s += g.weights[q]
			}
		}
		checkClose(t, "bin quadrature sum", s, g.binEdges[k+1]-g.binEdges[k], 1e-12)
	}
}

// TestBinsDesignConstantKernel проверяет, что A[j,k] совпадает с
// (1/Δln r_k)·∫ K q dr по бину для постоянного ядра.
func TestBinsDesignConstantKernel(t *testing.T) {
	const r0, r1 = 0.1, 1.5
	const bins = 4
	const K = 2.5
	kt := testKtable(r0*0.9, r1*1.1, 400, func(_ int, _ float64) float64 { return K })
	d, err := designBuild(nil, 2, []*ktable{kt}, []float64{1.5}, []float64{0},
		[]float64{r0}, []float64{r1}, 0, designSpec{
			method:     methodMonotone,
			bins:       bins,
			quadOrder:  24,
			maxLogSpan: 0.1,
			regOrder:   2,
		})
	if err != nil {
		t.Fatalf("designBuild: %v", err)
	}
	if d.nW != 1 || d.nu != bins {
		t.Fatalf("nW/nu = %d/%d, want 1/%d", d.nW, d.nu, bins)
	}
	g := d.geom[0]
	for k := 0; k < bins; k++ {
		a, b := g.binEdges[k], g.binEdges[k+1]
		nodes, wts := quadratureGrid([]float64{a, b}, 64, 0.02)
		want := 0.0
		for q := range nodes {
			want += wts[q] * K * qDvol(nodes[q])
		}
		want /= math.Log(b / a)
		for j := 0; j < 2; j++ {
			got := d.A[(0*d.nW+0)*2*bins+j*bins+k]
			if !(got > 0) {
				t.Fatalf("A[%d][%d] = %g must be positive", j, k, got)
			}
			checkClose(t, "A from bins", got, want, 1e-8*want)
		}
	}
}

func TestCumulativeVolume(t *testing.T) {
	r := []float64{0.1, 0.2, 0.4, 0.8}
	n := []float64{1, 3, 2, 0}
	v := cumulativeVolume(r, n)
	if len(v) != len(r) {
		t.Fatalf("len = %d", len(v))
	}
	checkClose(t, "V(r0)", v[0], 0, 0)
	for i := 1; i < len(v); i++ {
		if !(v[i] >= v[i-1]) {
			t.Fatalf("V must not decrease: %v", v)
		}
	}
	// V(r1) = ∫ n dln r по трапециям
	want := 0.5*math.Log(2)*(1+3) + 0.5*math.Log(2)*(3+2) + 0.5*math.Log(2)*(2+0)
	checkClose(t, "V total", v[3], want, 1e-12)
	if total := volumeTotal(v); !isFinite(total) || total != v[3] {
		t.Errorf("volumeTotal = %g, want %g", total, v[3])
	}

	// разрыв покрытия: NaN до первого значения и после разрыва
	n2 := []float64{math.NaN(), 1, math.NaN(), 2}
	v2 := cumulativeVolume(r, n2)
	if !math.IsNaN(v2[0]) || !math.IsNaN(v2[2]) || !math.IsNaN(v2[3]) {
		t.Errorf("gaps must give NaN: %v", v2)
	}
	checkClose(t, "V at first covered point", v2[1], 0, 0)

	if !math.IsNaN(volumeTotal([]float64{math.NaN(), math.NaN()})) {
		t.Errorf("empty coverage must give NaN total")
	}
}

func TestMonotoneCheck(t *testing.T) {
	ok := []float64{0, 1, 2, 3}
	if step, viol := monotoneCheck(ok); viol != 0 || step != 0 {
		t.Errorf("monotone V flagged: step=%g viol=%d", step, viol)
	}
	bad := []float64{0, 1, 0.5, 2}
	step, viol := monotoneCheck(bad)
	if viol != 1 {
		t.Fatalf("violations = %d, want 1", viol)
	}
	checkClose(t, "worst step", step, -0.5/2, 1e-12)
}

func TestRegRowInto(t *testing.T) {
	row := make([]float64, 4)
	const g = 2.0
	cases := []struct {
		order int
		row   int
		want  []float64
	}{
		{0, 1, []float64{0, g, 0, 0}},
		{1, 1, []float64{0, g, -g, 0}},
		{2, 1, []float64{0, g, -2 * g, g}},
	}
	for _, c := range cases {
		for i := range row {
			row[i] = math.NaN()
		}
		regRowInto(row, c.row, c.order, g)
		for i := range c.want {
			checkClose(t, "reg row", row[i], c.want[i], 0)
		}
	}
	if n := nLrows(5, 0); n != 5 {
		t.Errorf("nLrows(5,0) = %d", n)
	}
	if n := nLrows(5, 1); n != 4 {
		t.Errorf("nLrows(5,1) = %d", n)
	}
	if n := nLrows(5, 2); n != 3 {
		t.Errorf("nLrows(5,2) = %d", n)
	}
}

// TestBinsNNLSRecovery — при квадратной хорошо обусловленной A и v ≥ 0 решатель
// восстанавливает приращения, а значит и монотонную V.
func TestBinsNNLSRecovery(t *testing.T) {
	const nu = 4
	r := []float64{0.1, 0.2, 0.4, 0.8}
	vTrue := []float64{1, 2, 0.5, 1.5}
	a := make([]float64, nu*nu)
	for j := 0; j < nu; j++ {
		for k := 0; k < nu; k++ {
			// гладкие убывающие ядра с перекрытием
			a[j*nu+k] = math.Exp(-float64(absInt(j-k))) + 0.1*float64(k+1)
		}
	}
	g := make([]float64, nu)
	for j := 0; j < nu; j++ {
		for k := 0; k < nu; k++ {
			g[j] += a[j*nu+k] * vTrue[k]
		}
	}
	weights := make([]float64, nu)
	for j := range weights {
		weights[j] = 1
	}
	c := solveGamma(a, nu, nu, 1e-12, weights, g, true, 2)
	for k := 0; k < nu; k++ {
		checkClose(t, "recovered bin volume", c[k], vTrue[k], 1e-6*vTrue[k])
	}
	// V не убывает по построению
	prev := 0.0
	for k := 0; k < nu; k++ {
		prev += c[k]
		if prev <= 0 {
			t.Fatalf("cumulative volume must grow: %g", prev)
		}
	}
	// distAt отдаёт v_k/Δln r_k
	g2, err := geomBuildBins(r[0], r[3], nu, 16, 0.1)
	if err != nil {
		t.Fatalf("geomBuildBins: %v", err)
	}
	s := &solution{r0: r[0], r1: r[3], C: c}
	for k := 0; k < nu; k++ {
		mid := math.Sqrt(g2.binEdges[k] * g2.binEdges[k+1])
		checkClose(t, "distAt", distAt(s, g2, mid), c[k]*g2.invBinW[k], 1e-12)
	}
	if got := distAt(s, g2, r[3]*2); got != 0 {
		t.Errorf("distAt outside the window = %g, want 0", got)
	}
}

func TestBinsFor(t *testing.T) {
	if got := binsFor(0, 5); got != 10 {
		t.Errorf("binsFor(0,5) = %d, want 10", got)
	}
	if got := binsFor(0, 2); got != 8 {
		t.Errorf("binsFor(0,2) = %d, want 8", got)
	}
	if got := binsFor(0, 40); got != vBinsMax {
		t.Errorf("binsFor(0,40) = %d, want %d", got, vBinsMax)
	}
	if got := binsFor(12, 5); got != 12 {
		t.Errorf("binsFor(12,5) = %d, want 12", got)
	}
}

func TestArgsMethodBins(t *testing.T) {
	a, err := ParseArgs([]string{"--tables", "dir"})
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}
	if a.Method != methodMonotone {
		t.Errorf("default method = %q, want monotone", a.Method)
	}
	if a.RegOrder != 2 || a.VBins != 0 {
		t.Errorf("defaults reg-order/v-bins = %d/%d", a.RegOrder, a.VBins)
	}

	a, err = ParseArgs([]string{"--tables", "dir", "--method", "spline",
		"--degree", "2", "--v-bins", "12", "--reg-order", "1"})
	if err != nil {
		t.Fatalf("explicit options: %v", err)
	}
	if a.Method != methodSpline || a.Degrees[0] != 2 || a.VBins != 12 ||
		a.RegOrder != 1 {
		t.Errorf("parsed method/degree/v-bins/reg-order = %q/%v/%d/%d",
			a.Method, a.Degrees, a.VBins, a.RegOrder)
	}
	if _, err := ParseArgs([]string{"--tables", "dir", "--method", "bad"}); err == nil {
		t.Errorf("invalid --method must be rejected")
	}
	if _, err := ParseArgs([]string{"--tables", "dir", "--v-bins", "1000"}); err == nil {
		t.Errorf("--v-bins above the limit must be rejected")
	}
	if _, err := ParseArgs([]string{"--tables", "dir", "--reg-order", "3"}); err == nil {
		t.Errorf("--reg-order above 2 must be rejected")
	}
}
