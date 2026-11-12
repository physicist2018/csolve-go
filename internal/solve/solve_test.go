package solve

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
)

func checkClose(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	d := math.Abs(got - want)
	scale := math.Abs(want)
	if scale < 1 {
		scale = 1
	}
	if d > tol*scale {
		t.Errorf("%s: got %.17g want %.17g (d=%.3g)", what, got, want, d)
	}
}

func TestSpline(t *testing.T) {
	x := []float64{0.1, 0.5, 1.0, 2.0, 3.0}
	y := []float64{1.0, 2.0, 4.0, 16.0, 9.0}
	s := newSpline(x, y)
	pts := []float64{0.2, 0.7, 1.5, 2.5, 3.5}
	want := []float64{1.3600000000000003, 2.46, 9.8125, 17.4375, -14.4375}
	for i := range pts {
		checkClose(t, "spline not-a-knot", s.eval(pts[i]), want[i], 1e-12)
	}

	x3 := []float64{0.1, 0.5, 1.0}
	y3 := []float64{1.0, 2.0, 4.0}
	s3 := newSpline(x3, y3)
	p3 := []float64{0.2, 0.7, 1.5}
	w3 := []float64{1.2, 2.6999999999999997, 6.833333333333333}
	for i := range p3 {
		checkClose(t, "spline n=3", s3.eval(p3[i]), w3[i], 1e-12)
	}
}

func TestGaussLegendre(t *testing.T) {
	order := 24
	x, w := glNodesWeights(order)
	s8, s2 := 0.0, 0.0
	for i := 0; i < order; i++ {
		s8 += w[i] * math.Pow(x[i], 8)
		s2 += w[i] * x[i] * x[i]
	}
	checkClose(t, "GL int x^8", s8, 2.0/9.0, 1e-14)
	checkClose(t, "GL int x^2", s2, 2.0/3.0, 1e-14)
	for i := 1; i < order; i++ {
		if !(x[i] > x[i-1]) {
			t.Fatalf("GL nodes are not increasing")
		}
	}
}

func TestQuadrature(t *testing.T) {
	nodes, weights := quadratureGrid([]float64{1.0, 2.0}, 24, 0.1)
	s := 0.0
	for i := range nodes {
		s += weights[i] / nodes[i]
	}
	checkClose(t, "quadrature int_1^2 dx/x", s, math.Log(2.0), 1e-12)
}

func TestBSpline(t *testing.T) {
	g := geomBuild(0.1, 10.0, 3, 6, false, 24, 0.1)
	for k := 0; k < 25; k++ {
		tt := 0.1 * math.Pow(100.0, float64(k)/24.0)
		full := bsplineValues(g.knots, g.degree, tt)
		s := 0.0
		for i := range full {
			s += full[i]
			if full[i] < -1e-14 {
				t.Fatalf("negative B-spline value at t=%g", tt)
			}
		}
		if math.Abs(s-1.0) > 1e-12 {
			t.Fatalf("partition of unity broken at t=%g: %g", tt, s)
		}
	}
	for q := 0; q < g.nNodes; q++ {
		s := 0.0
		for i := 0; i < g.nUnknown; i++ {
			s += g.values[q*g.nUnknown+i]
		}
		if math.Abs(s-1.0) > 1e-12 {
			t.Fatalf("partition of unity broken at node %d: %g", q, s)
		}
	}
}

func TestBandChol(t *testing.T) {
	const n, kb = 4, 1
	bw := 2*kb + 1
	M := [4][4]float64{{4, 1, 0, 0}, {1, 4, 1, 0}, {0, 1, 4, 1}, {0, 0, 1, 4}}
	xt := []float64{1, 2, 3, 4}
	b := make([]float64, n)
	band := make([]float64, n*bw)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			b[i] += M[i][j] * xt[j]
			if absInt(i-j) <= kb {
				band[i*bw+(j-i+kb)] = M[i][j]
			}
		}
	}
	x := bandCholSolve(band, n, kb, b)
	for i := 0; i < n; i++ {
		checkClose(t, "banded Cholesky", x[i], xt[i], 1e-12)
	}
}

func TestNNLS(t *testing.T) {
	a := []float64{1, 0, 0, 1, 1, 1}
	b := []float64{1.0, -1.0, 0.0}
	x := nnls(a, 3, 2, b)
	checkClose(t, "NNLS x[1] boundary", x[1], 0.0, 1e-10)
	checkClose(t, "NNLS x[0]", x[0], 0.5, 1e-10)
}

func TestDirname(t *testing.T) {
	if got := mTableDirname(1.45, -0.005); got != "mre_1.45_mim_-0.00500" {
		t.Errorf("directory name: got %q", got)
	}
	negZero := math.Copysign(0, -1)
	if got := mTableDirname(1.45, negZero); got != "mre_1.45_mim_0.00000" {
		t.Errorf("negative zero: got %q", got)
	}
	re, im, ok := parseMDirname("mre_1.45_mim_-0.00500")
	if !ok {
		t.Fatalf("parse m dirname failed")
	}
	checkClose(t, "name Re", re, 1.45, 1e-15)
	checkClose(t, "name Im", im, -0.005, 1e-15)
	if _, _, ok := parseMDirname("average"); ok {
		t.Errorf("unrelated name parsed as m dir")
	}
}

func TestTableParse(t *testing.T) {
	content := "# Units: x (dimless), C (um^2)\n" +
		"# x Cback Cext Qext n\n" +
		"0.1 7.0 1.0 0.5 5\n" +
		"0.5 8.0 2.0 0.6 5\n" +
		"1.0 9.0 4.0 0.7 5\n" +
		"2.0 10.0 16.0 0.8 5\n"
	tb, _ := parseTable([]byte(content), "table.txt", false)
	if !tb.hasCext || !tb.hasCback {
		t.Fatalf("both columns must be found")
	}
	if tb.hasS11 || tb.hasS22 {
		t.Errorf("S11/S22 must be absent in the old table format")
	}
	checkClose(t, "table x_min", tb.xMin, 0.1, 1e-15)
	checkClose(t, "table x_max", tb.xMax, 2.0, 1e-15)
	checkClose(t, "table Cext at node", tableCrossSection(tb, false, 0.5), 2.0, 1e-12)
	checkClose(t, "table Cback at node", tableCrossSection(tb, true, 1.0), 9.0, 1e-12)
	checkClose(t, "table interpolation", tableCrossSection(tb, false, 0.25), 1.5, 0.2)
}

func TestTableParseS11S22(t *testing.T) {
	content := "# x Cext Cabs Cback Qext Qabs Qback S11 S22 n\n" +
		"0.1 1.0 0.1 7.0 0.5 0.1 3.5 7.0 3.5 5\n" +
		"0.5 2.0 0.1 8.0 0.6 0.1 3.8 8.0 2.0 5\n" +
		"1.0 4.0 0.1 9.0 0.7 0.1 4.0 9.0 1.0 5\n" +
		"2.0 16.0 0.1 10.0 0.8 0.1 4.2 10.0 0.5 5\n"
	tb, _ := parseTable([]byte(content), "data.txt", false)
	if !tb.hasS11 || !tb.hasS22 {
		t.Fatalf("S11/S22 columns must be parsed (new table format)")
	}
	checkClose(t, "table S11 at node", math.Exp(tb.spS11.eval(math.Log(1.0))),
		9.0, 1e-12)
	checkClose(t, "table S22 at node", math.Exp(tb.spS22.eval(math.Log(1.0))),
		1.0, 1e-12)

	// nan в S22 (mkbase пишет nan, если столбца s22 нет) — колонка отбрасывается
	bad := strings.Replace(content, "7.0 3.5 5", "7.0 nan 5", 1)
	tbNaN, _ := parseTable([]byte(bad), "data.txt", false)
	if tbNaN.hasS22 || !tbNaN.hasS11 {
		t.Errorf("nan in S22 must disable the S22 column only")
	}
}

// TestDepol: S11 = x, S22 = 1 ⇒ δ_l = (x-1)/(x+1)*100 при x = 2πr/λ.
func TestDepol(t *testing.T) {
	xs := make([]float64, 60)
	for i := range xs {
		xs[i] = 0.05 * math.Pow(1200.0, float64(i)/float64(len(xs)-1))
	}
	var sb strings.Builder
	sb.WriteString("# x Cext Cabs Cback Qext Qabs Qback S11 S22\n")
	for _, x := range xs {
		fmt.Fprintf(&sb, "%.17g %.6g 0 1 0 0 0 %.17g 1\n", x, x, x)
	}
	tb, _ := parseTable([]byte(sb.String()), "data.txt", false)
	if !tb.hasS11 || !tb.hasS22 {
		t.Fatalf("S11/S22 must be available")
	}

	r0 := 1.0
	rq := []float64{r0}
	wq := []float64{1.0}
	nq := []float64{1.0}
	ds := &depolSet{wl: append([]float64(nil), depolWlsUm...),
		kt: make([][2]*ktable, 1)}
	for iw, wl := range ds.wl {
		ds.kt[0][iw] = depolKtableBuild(tb, wl, tableRefWlUm, rq)
	}
	ds.nOK = 1
	if !depolSetAvailable(ds) {
		t.Fatalf("depol set must be available")
	}
	d11, d22, delta := depolFromProfile(ds, []int{0}, rq, wq, nq)
	for iw, wl := range ds.wl {
		x := 2.0 * math.Pi * r0 / wl
		want := (x - 1.0) / (x + 1.0) * 100.0
		checkClose(t, fmt.Sprintf("delta_l(%g nm)", wl*1e3), delta[iw], want, 1e-9)
		if !(d11[iw] > d22[iw]) {
			t.Errorf("S11 must exceed S22 for x > 1")
		}
	}
	if !(delta[0] > delta[1]) {
		t.Errorf("delta_l at 355 nm must exceed delta_l at 532 nm for "+
			"the S11=x, S22=1 table: %g vs %g", delta[0], delta[1])
	}

	// таблица без S11/S22 — δ_l не определена
	dsEmpty := &depolSet{wl: append([]float64(nil), depolWlsUm...),
		kt: make([][2]*ktable, 1)}
	_, _, deltaEmpty := depolFromProfile(dsEmpty, []int{0}, rq, wq, nq)
	for iw := range deltaEmpty {
		if !math.IsNaN(deltaEmpty[iw]) {
			t.Errorf("delta_l must be NaN without S11/S22")
		}
	}
}

// TestDepolProjection: проекции S11/S22 на базис дают то же δ_l, что и
// прямая свёртка с распределением на узлах окна.
func TestDepolProjection(t *testing.T) {
	xs := make([]float64, 60)
	for i := range xs {
		xs[i] = 0.05 * math.Pow(1200.0, float64(i)/float64(len(xs)-1))
	}
	var sb strings.Builder
	sb.WriteString("# x Cext Cabs Cback Qext Qabs Qback S11 S22\n")
	for _, x := range xs {
		fmt.Fprintf(&sb, "%.17g %.6g 0 1 0 0 0 %.17g 1\n", x, x, x)
	}
	tb, _ := parseTable([]byte(sb.String()), "data.txt", false)

	g := geomBuild(1.0, 2.0, 1, 4, false, 24, 0.1)
	d := &design{nM: 1, nW: 1, nu: g.nUnknown, geom: []*geom{g}}
	r := rGridFor(1.0, 2.0, 0.1, 0.355)
	ds := &depolSet{wl: append([]float64(nil), depolWlsUm...),
		kt: make([][2]*ktable, 1)}
	for iw, wl := range ds.wl {
		ds.kt[0][iw] = depolKtableBuild(tb, wl, tableRefWlUm, r)
	}
	ds.nOK = 1

	p := depolProjBuild(ds, d, []int{1})
	if p == nil {
		t.Fatalf("projection must be built")
	}
	C := []float64{1.3, 0.4, 0.2, 0.7}
	nq := make([]float64, g.nNodes)
	for q := 0; q < g.nNodes; q++ {
		for i := 0; i < g.nUnknown; i++ {
			nq[q] += C[i] * g.values[q*g.nUnknown+i]
		}
	}
	_, _, delta := depolFromProfile(ds, []int{0}, g.nodes, g.weights, nq)
	got := p.depolModel(0, 0, 0, C)
	checkClose(t, "delta_l from projections vs convolution", got,
		delta[1], 1e-10)
}

func TestRankByDepol(t *testing.T) {
	// a11=[4,1], a22=[1,1]: C=[1,0] ⇒ δ=60 %, C=[0,1] ⇒ δ=0 %.
	p := &depolProj{wl: []float64{0.532}, iw: []int{1}, nM: 1, nW: 1, nu: 2,
		a11: [][]float64{{4, 1}}, a22: [][]float64{{1, 1}}}
	mk := func() *solutions {
		return &solutions{n: 2, sol: []solution{
			{mRe: 1.45, mIm: -0.005, geomIndex: 0, err: 0.1, id: 0,
				C: []float64{0, 1}},
			{mRe: 1.45, mIm: -0.005, geomIndex: 0, err: 0.1, id: 1,
				C: []float64{1, 0}},
		}}
	}
	mre := []float64{1.45}
	mim := []float64{-0.005}

	sols := mk()
	rankByDepol(sols, p, mre, mim, []float64{50.0}, 1.0)
	if sols.sol[0].id != 1 {
		t.Errorf("solution closest to δ=50%% must rank first, got id %d",
			sols.sol[0].id)
	}

	// нулевой вес — порядок по err не меняется
	sols = mk()
	rankByDepol(sols, p, mre, mim, []float64{50.0}, 0.0)
	if sols.sol[0].id != 0 || sols.sol[1].id != 1 {
		t.Errorf("zero weight must keep the order by err")
	}

	// невалидное измерение — штраф не применяется
	sols = mk()
	rankByDepol(sols, p, mre, mim, []float64{math.NaN()}, 1.0)
	if sols.sol[0].id != 0 {
		t.Errorf("NaN target must not reorder solutions")
	}
}

// TestDepolFunctional: строка ограничения δ_l в функционале приближает
// модельное δ_l к измеренному; w=0 — ограничение не добавляется.
func TestDepolFunctional(t *testing.T) {
	p := &depolProj{wl: []float64{0.532}, iw: []int{1}, nM: 1, nW: 1, nu: 2,
		a11: [][]float64{{4, 1}}, a22: [][]float64{{1, 1}}}
	A := []float64{1, 0, 0, 1}
	weights := []float64{1, 1}
	g1n := []float64{1, 0.5}
	const nch = 2

	c0 := solveGamma(A, nch, p.nu, 0, weights, g1n, false)
	d0 := p.depolModel(0, 0, 0, c0)
	checkClose(t, "channel-only delta_l", d0, 50.0, 1e-9)

	// без дополнительных строк результат тот же
	cEq := solveGammaRows(A, nch, p.nu, 0, weights, g1n, false, nil, 0)
	for i := range cEq {
		checkClose(t, "solveGammaRows without extra rows", cEq[i], c0[i], 1e-12)
	}

	dc := &depolCons{proj: p, w: 1, targets: []float64{60}}
	extra, nExtra := dc.rows(0, 0, c0)
	if nExtra != 1 || len(extra) != p.nu {
		t.Fatalf("expected 1 row of length %d, got %d rows / %d values",
			p.nu, nExtra, len(extra))
	}
	c1 := solveGammaRows(A, nch, p.nu, 0, weights, g1n, false, extra, nExtra)
	d1 := p.depolModel(0, 0, 0, c1)
	if !(d1 > d0 && d1 < 60) {
		t.Errorf("delta_l must move towards the measurement: %g -> %g", d0, d1)
	}

	// большой вес — ограничение выполняется почти точно
	dc.w = 1e4
	extra, nExtra = dc.rows(0, 0, c0)
	c2 := solveGammaRows(A, nch, p.nu, 0, weights, g1n, false, extra, nExtra)
	checkClose(t, "large weight delta_l", p.depolModel(0, 0, 0, c2), 60.0, 1e-3)

	// w = 0 — строки нет
	zero := &depolCons{proj: p, w: 0, targets: []float64{60}}
	if _, n := zero.rows(0, 0, c0); n != 0 {
		t.Errorf("zero weight must add no rows, got %d", n)
	}
	// невалидные цели — строк нет
	for _, target := range []float64{math.NaN(), 100.0, -100.0} {
		bad := &depolCons{proj: p, w: 1, targets: []float64{target}}
		if _, n := bad.rows(0, 0, c0); n != 0 {
			t.Errorf("target %g must be skipped, got %d rows", target, n)
		}
	}
	// nil-получатель безопасен (ограничение выключено)
	var nilDC *depolCons
	if _, n := nilDC.rows(0, 0, c0); n != 0 {
		t.Errorf("nil constraint must add no rows")
	}
}

// TestWriteProfileDepolColumns: в dist_<H>.txt добавляются колонки
// depol_calc_532 (всегда) и depol_meas_532 (если есть dep532).
func TestWriteProfileDepolColumns(t *testing.T) {
	dir := t.TempDir()
	b := &batchInput{nch: 2, ndep: 1, nAlt: 1,
		ch:    []batchChannel{{chBeta, 0.532, "b532"}, {chAlpha, 0.532, "a532"}},
		depWl: []float64{0.532}, dep: []float64{7.5}}
	g1 := []float64{1e-4, 3e-4}
	prof := &profile{nr: 2, rGrid: []float64{0.1, 0.2},
		nAvg: []float64{1, 2}, gFit: append([]float64(nil), g1...)}
	writeProfile(dir, 500, b, g1, prof,
		distDepol{calc: 6.25, meas: 7.5, hasMeas: true})

	data, err := os.ReadFile(filepath.Join(dir, "dist_500.txt"))
	if err != nil {
		t.Fatalf("read dist file: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "depol_calc_532") ||
		!strings.Contains(text, "depol_meas_532") {
		t.Errorf("dist header must declare depol columns:\n%s", text)
	}
	rows := 0
	for _, ln := range strings.Split(text, "\n") {
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		f := strings.Fields(ln)
		if len(f) != 4 {
			t.Fatalf("expected 4 columns, got %d: %q", len(f), ln)
		}
		checkClose(t, "dist depol_calc", mustFloat(t, f[2]), 6.25, 1e-12)
		checkClose(t, "dist depol_meas", mustFloat(t, f[3]), 7.5, 1e-12)
		rows++
	}
	if rows != 2 {
		t.Errorf("expected 2 data rows, got %d", rows)
	}

	// без измерения — только колонка расчёта (nan при отсутствии S11/S22)
	b2 := &batchInput{nch: 2, nAlt: 1,
		ch: []batchChannel{{chBeta, 0.532, "b532"}, {chAlpha, 0.532, "a532"}}}
	writeProfile(dir, 700, b2, g1, prof, distDepol{calc: math.NaN()})
	data, err = os.ReadFile(filepath.Join(dir, "dist_700.txt"))
	if err != nil {
		t.Fatalf("read dist file: %v", err)
	}
	if strings.Contains(string(data), "depol_meas_532") {
		t.Errorf("dist must not declare measured depol without dep column")
	}
	if !strings.Contains(string(data), "depol_calc_532") {
		t.Errorf("dist must always declare depol_calc_532")
	}
	for _, ln := range strings.Split(string(data), "\n") {
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		f := strings.Fields(ln)
		if len(f) != 3 || f[2] != "nan" {
			t.Errorf("expected 3 columns with nan depol, got %q", ln)
		}
	}
}

func mustFloat(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("parse float %q: %v", s, err)
	}
	return v
}

func TestBatchParseDepol(t *testing.T) {
	write := func(t *testing.T, content string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "batch.txt")
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatalf("write temp batch: %v", err)
		}
		return p
	}

	b := parseBatchTxt(write(t, "Altitude b532 a532 dep532\n"+
		"500 1e-4 3e-4 12.5\n700 1.1e-4 3.1e-4 nan\n"))
	if b.nch != 2 || b.ndep != 1 {
		t.Fatalf("nch/ndep = %d/%d, want 2/1", b.nch, b.ndep)
	}
	if b.ch[0].name != "b532" || b.ch[0].kind != chBeta ||
		b.ch[1].name != "a532" || b.ch[1].kind != chAlpha {
		t.Errorf("unexpected channels: %+v", b.ch)
	}
	checkClose(t, "dep wavelength", b.depWl[0], 0.532, 1e-12)
	checkClose(t, "dep[0]", b.dep[0], 12.5, 0)
	if !math.IsNaN(b.dep[1]) {
		t.Errorf("nan in dep column must be preserved, got %g", b.dep[1])
	}
	checkClose(t, "alt[1]", b.alt[1], 700.0, 0)
	checkClose(t, "data[2]", b.data[2], 1.1e-4, 0)

	// произвольный порядок столбцов
	b = parseBatchTxt(write(t, "Altitude dep355 a532 b532\n"+
		"500 8.0 3e-4 1e-4\n"))
	if b.nch != 2 || b.ndep != 1 {
		t.Fatalf("nch/ndep = %d/%d, want 2/1", b.nch, b.ndep)
	}
	checkClose(t, "dep wavelength (dep355)", b.depWl[0], 0.355, 1e-12)
	checkClose(t, "dep value", b.dep[0], 8.0, 0)
	checkClose(t, "a532 value", b.data[0], 3e-4, 0)
	checkClose(t, "b532 value", b.data[1], 1e-4, 0)
	if b.ch[0].name != "a532" || b.ch[1].name != "b532" {
		t.Errorf("channel order must follow the header: %+v", b.ch)
	}
}

func TestSmoothing(t *testing.T) {
	r := []float64{0.05, 0.08469069900482264, 0.14345028995850928,
		0.24297810658061286, 0.41155971378360795, 0.6971055968511699,
		1.1807672055499936, 2.0}
	y := []float64{1.0, 2.0, 1.5, 3.0, 2.2, 2.8, 1.7, 2.5}
	w := []float64{1, 1, 1, 1, 1, 1, 1, 1}
	want := []float64{1.0000605683247177, 1.7972784813389382,
		1.8136974164873019, 2.684401088600474, 2.465905496820976,
		2.5324755824187664, 1.9235917468040047, 2.4999220818628123}
	nAvg := append([]float64(nil), y...)
	smoothDistribution(r, nAvg, w, 1e-4)
	for i := range want {
		checkClose(t, "smoothing vs scipy", nAvg[i], want[i], 1e-9)
	}
}

func TestSmoothingLam1(t *testing.T) {
	x := []float64{0.0, 1.0 / 6.0, 2.0 / 6.0, 0.5, 4.0 / 6.0, 5.0 / 6.0, 1.0}
	y := []float64{30.0, 30.4, 29.7, 30.2, 30.9, 30.1, 30.5}
	want := []float64{30.032290318114548, 30.106986571862308,
		30.181759424004692, 30.257218107418254, 30.332666884010926,
		30.407299445676983, 30.481779248902395}
	out := smoothingSplineFit(x, y, nil, 1.0, x)
	for i := range want {
		checkClose(t, "smoothing lam=1", out[i], want[i], 1e-9)
	}
}

func TestArgs(t *testing.T) {
	{
		a, err := ParseArgs([]string{"--tables", "dir"})
		if err != nil {
			t.Fatalf("defaults: %v", err)
		}
		if len(a.Degrees) != 1 || a.Degrees[0] != 1 {
			t.Errorf("default degree must be 1, got %v", a.Degrees)
		}
		if a.Type != "agglomerate" {
			t.Errorf("default --type must be agglomerate, got %q", a.Type)
		}
		checkClose(t, "plot-dpi default", a.PlotDpi, 150.0, 0)
		checkClose(t, "plot-font-size default", a.PlotFontSize, 0.0, 0)
		if a.Plotting != plottingInternal {
			t.Errorf("default --plotting must be internal, got %d", a.Plotting)
		}
		if a.PlotFormat != plotFmtPdf {
			t.Errorf("internal backend must force pdf, got %d", a.PlotFormat)
		}
	}
	{
		// без --tables таблицы встроенные
		a, err := ParseArgs(nil)
		if err != nil {
			t.Fatalf("builtin tables must not require --tables: %v", err)
		}
		if a.Tables != "" || a.Type != "agglomerate" {
			t.Errorf("builtin defaults: tables=%q type=%q", a.Tables, a.Type)
		}
	}
	{
		a, err := ParseArgs([]string{"--type", "sphere"})
		if err != nil {
			t.Fatalf("--type sphere: %v", err)
		}
		if a.Type != "sphere" {
			t.Errorf("--type sphere not parsed: %q", a.Type)
		}
	}
	{
		if _, err := ParseArgs([]string{"--type", "bad"}); err == nil {
			t.Errorf("unknown --type must be rejected")
		}
		_, err := ParseArgs([]string{"--type", "sphere", "--tables", "dir"})
		if err == nil {
			t.Errorf("--type and --tables must be mutually exclusive")
		} else if !strings.Contains(err.Error(), "mutually exclusive") {
			t.Errorf("unexpected error for --type with --tables: %v", err)
		}
	}
	{
		a, err := ParseArgs([]string{"--tables", "dir", "--degree", "2",
			"--plot-font-size=14", "--r1-min=0.5", "--nonneg"})
		if err != nil {
			t.Fatalf("single degree: %v", err)
		}
		if len(a.Degrees) != 1 || a.Degrees[0] != 2 {
			t.Errorf("--degree 2 must give exactly one degree, got %v", a.Degrees)
		}
		checkClose(t, "--plot-font-size=14", a.PlotFontSize, 14.0, 0)
		checkClose(t, "--r1-min=0.5", a.R1Min, 0.5, 0)
		if !a.Nonneg {
			t.Errorf("--nonneg not parsed")
		}
	}
	{
		a, err := ParseArgs([]string{"--tables", "dir", "--depol-ratio-cons"})
		if err != nil {
			t.Fatalf("--depol-ratio-cons: %v", err)
		}
		if !a.DepolRatioCons {
			t.Errorf("--depol-ratio-cons not parsed")
		}
		checkClose(t, "--depol-ratio-w default", a.DepolRatioW, 1.0, 0)
		a, err = ParseArgs([]string{"--tables", "dir", "--depol-ratio-w", "2.5"})
		if err != nil {
			t.Fatalf("--depol-ratio-w: %v", err)
		}
		checkClose(t, "--depol-ratio-w", a.DepolRatioW, 2.5, 0)
		if _, err := ParseArgs([]string{"--tables", "dir", "--depol-ratio-w=-1"}); err == nil {
			t.Errorf("negative --depol-ratio-w must be rejected")
		}
		if a.DepolRatioMode != depolModeFunc {
			t.Errorf("default --depol-ratio-mode must be func")
		}
		a, err = ParseArgs([]string{"--tables", "dir", "--depol-ratio-mode", "rank"})
		if err != nil {
			t.Fatalf("--depol-ratio-mode rank: %v", err)
		}
		if a.DepolRatioMode != depolModeRank {
			t.Errorf("--depol-ratio-mode rank not parsed")
		}
		if _, err := ParseArgs([]string{"--tables", "dir", "--depol-ratio-mode", "bad"}); err == nil {
			t.Errorf("invalid --depol-ratio-mode must be rejected")
		}
	}
	{
		// --degree принимает ровно одно значение
		if _, err := ParseArgs([]string{"--tables", "dir", "--degree", "1", "2"}); err == nil {
			t.Errorf("--degree 1 2 must be rejected")
		} else if !strings.Contains(err.Error(), "unexpected argument") {
			t.Errorf("unexpected error for extra degree: %v", err)
		}
		if _, err := ParseArgs([]string{"--tables", "dir", "--degree", "1",
			"--degree", "2"}); err == nil {
			t.Errorf("repeated --degree must be rejected")
		}
		if _, err := ParseArgs([]string{"--tables", "dir", "--degree", "5"}); err == nil {
			t.Errorf("--degree 5 must be rejected")
		}
	}
	{
		a, err := ParseArgs([]string{"--tables", "dir", "--table-cback", "4pi",
			"--plot-format", "pdf", "--modes", "stat"})
		if err != nil {
			t.Fatalf("enums: %v", err)
		}
		if !a.TableCback4pi {
			t.Errorf("--table-cback 4pi not parsed")
		}
		if a.PlotFormat != plotFmtPdf {
			t.Errorf("--plot-format pdf not parsed")
		}
		if a.Modes != 2 {
			t.Errorf("--modes stat not parsed")
		}
	}
	{
		if _, err := ParseArgs([]string{"--tables", "dir", "--table-cback", "bad"}); err == nil {
			t.Errorf("invalid enum must be rejected")
		}
	}
	{
		if _, err := ParseArgs([]string{"--tables", "dir", "--nr0", "0"}); err == nil {
			t.Errorf("--nr0 0 must be rejected")
		}
	}
}

func TestBuiltinTables(t *testing.T) {
	cases := []struct {
		kind    string
		dir     string
		nM      int
		nPoints int
		xMinMax [2]float64 // грубые границы: [ниже xMin, выше xMax]
	}{
		{"sphere", "base_sph_25/average", 286, 41, [2]float64{0.02, 600.0}},
		{"spheroid", "base_sphrd_25/average", 286, 41, [2]float64{0.02, 600.0}},
		{"agglomerate", "large_aggl_100/average", 56, 100, [2]float64{0.05, 80.0}},
	}
	for _, c := range cases {
		src := builtinTableSource(c.kind)
		if src.origin != c.dir {
			t.Errorf("%s: origin = %q, want %q", c.kind, src.origin, c.dir)
		}
		mre, mim := tableMGrid(src)
		if len(mre) != c.nM || len(mim) != c.nM {
			t.Fatalf("%s: %d/%d m values, want %d", c.kind, len(mre),
				len(mim), c.nM)
		}
		tab := tableCacheGet(src, mre[0], mim[0], false)
		if !tab.hasCext || !tab.hasCback || !tab.hasS11 || !tab.hasS22 {
			t.Errorf("%s: table %s must have Cext/Cback/S11/S22", c.kind,
				tab.path)
		}
		if tab.n != c.nPoints {
			t.Errorf("%s: %d x points, want %d", c.kind, tab.n, c.nPoints)
		}
		if !(tab.xMin < c.xMinMax[0]) || !(tab.xMax > c.xMinMax[1]) {
			t.Errorf("%s: x range [%g, %g] outside expected bounds", c.kind,
				tab.xMin, tab.xMax)
		}
		if tab.xMin <= 0 || !isFinite(tab.xMax) {
			t.Errorf("%s: invalid x range [%g, %g]", c.kind, tab.xMin, tab.xMax)
		}
	}
}

// TestEmbeddedGzipMatchesSource проверяет, что встроенные .gz не устарели
// относительно текстовых исходников bases/<set>/average/mre_*/data.txt.
func TestEmbeddedGzipMatchesSource(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", "bases")); err != nil {
		t.Skip("bases/ is not available (run 'go generate ./bases' first)")
	}
	for kind, dir := range builtinTableDirs {
		src := builtinTableSource(kind)
		err := fs.WalkDir(src.fsys, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".gz") {
				return nil
			}
			data, _, err := src.readTable(path.Dir(p))
			if err != nil {
				return err
			}
			disk := filepath.Join("..", "..", "bases", dir,
				filepath.FromSlash(path.Dir(p)), "data.txt")
			want, err := os.ReadFile(disk)
			if err != nil {
				return err
			}
			if !bytes.Equal(data, want) {
				t.Errorf("%s: embedded %s differs from %s — run 'go generate ./bases'",
					kind, p, disk)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
}

func TestArgsPlotting(t *testing.T) {
	{
		a, err := ParseArgs([]string{"--tables", "dir", "--plotting", "gnuplot"})
		if err != nil {
			t.Fatalf("--plotting gnuplot: %v", err)
		}
		if a.Plotting != plottingGnuplot {
			t.Errorf("--plotting gnuplot not parsed")
		}
		if a.PlotFormat != plotFmtPng {
			t.Errorf("gnuplot backend must keep png default, got %d",
				a.PlotFormat)
		}
	}
	{
		if _, err := ParseArgs([]string{"--tables", "dir",
			"--plot-format", "png"}); err == nil {
			t.Errorf("--plot-format png with internal backend must fail")
		}
		if _, err := ParseArgs([]string{"--tables", "dir",
			"--plot-format", "svg"}); err == nil {
			t.Errorf("--plot-format svg with internal backend must fail")
		}
	}
	{
		a, err := ParseArgs([]string{"--tables", "dir",
			"--plot-format", "pdf", "--plotting", "internal"})
		if err != nil {
			t.Fatalf("--plot-format pdf: %v", err)
		}
		if a.PlotFormat != plotFmtPdf {
			t.Errorf("--plot-format pdf not parsed with internal backend")
		}
	}
	{
		if _, err := ParseArgs([]string{"--tables", "dir",
			"--out", "result.png"}); err == nil {
			t.Errorf("--out with a non-pdf extension must fail")
		}
		if _, err := ParseArgs([]string{"--tables", "dir",
			"--out", "result.pdf"}); err != nil {
			t.Errorf("--out result.pdf: %v", err)
		}
	}
	{
		// с --no-plot опции графиков не проверяются
		if _, err := ParseArgs([]string{"--tables", "dir", "--no-plot",
			"--plot-format", "png", "--out", "result.png"}); err != nil {
			t.Errorf("--no-plot must ignore plot options: %v", err)
		}
	}
	{
		if _, err := ParseArgs([]string{"--tables", "dir",
			"--plotting", "cairo"}); err == nil {
			t.Errorf("unknown --plotting value must fail")
		}
	}
}

func TestInternalLabel(t *testing.T) {
	cases := map[string]string{
		"r, µm":               "r, µm",
		"dV/dln r, µm³/cm³":   "dV/dln r, µm³/cm³",
		"N, cm⁻³":             "N, cm⁻³",
		"⟨m⟩ = 1.450-0.005j":  "⟨m⟩ = 1.450-0.005j",
		"S_{355}":             "S_355",
		"δ_l^{meas}(532 nm)":  "δ_l^meas(532 nm)",
		"Concentration, см⁻³": "Concentration, см⁻³",
	}
	for in, want := range cases {
		if got := internalLabel(in); got != want {
			t.Errorf("internalLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func checkPDF(t *testing.T, p string) {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("%s is not a PDF", p)
	}
	if len(data) < 1024 {
		t.Fatalf("%s is too small (%d bytes)", p, len(data))
	}
}

func TestInternalPlotSmoke(t *testing.T) {
	dir := t.TempDir()
	po := &plotOpts{fmt: plotFmtPdf, dpi: 150, backend: plottingInternal}
	alt := []float64{0, 500, 1000}
	vals := [][]float64{{1, 2, 3}, {3, math.NaN(), 1}}
	p := filepath.Join(dir, "profiles.pdf")
	if !plotProfilesStyled(po, p, "Профиль", "N, cm⁻³", 2, alt, vals,
		[]string{"calc", "meas"}, []bool{false, true}, true) {
		t.Fatalf("internal profiles plot failed")
	}
	checkPDF(t, p)

	rGrid := []float64{0.05, 0.1, 0.2, 0.4, 0.8}
	nTrue := []float64{0, 1, 2, 1, 0}
	nAvg := []float64{0, 1.1, 2.1, 0.9, 0.1}
	nRaw := []float64{0, 1.2, 2.0, 1.0, 0.2}
	nBest := [][]float64{nAvg, nAvg}
	ch := []channel{{kind: chBeta, wlUm: 0.355}, {kind: chBeta, wlUm: 0.532},
		{kind: chAlpha, wlUm: 0.532}}
	p = filepath.Join(dir, "results.pdf")
	if !plotResults(po, p, rGrid, nTrue, nAvg, nRaw, nBest, len(nBest), ch,
		[]float64{0.9, 0.8, 0.7}, []float64{0.85, 0.82, 0.72}, 1.45, -0.005) {
		t.Fatalf("internal results plot failed")
	}
	checkPDF(t, p)

	p = filepath.Join(dir, "refractive.pdf")
	if !plotRefractiveProfile(po, p, []float64{0, 1000, 2000},
		[]float64{1.45, 1.5, 1.42}, []float64{-0.005, 0, -0.01}) {
		t.Fatalf("internal refractive plot failed")
	}
	checkPDF(t, p)

	p = filepath.Join(dir, "profile.pdf")
	if !plotProfile(po, p, rGrid, []float64{1, 2, 3, 2, 1}, ch,
		[]float64{0.9, 0.8, 0.7}, []float64{0.85, 0.82, 0.72}, 500) {
		t.Fatalf("internal single profile plot failed")
	}
	checkPDF(t, p)
}

func TestInternalPlotEmpty(t *testing.T) {
	dir := t.TempDir()
	po := &plotOpts{fmt: plotFmtPdf, backend: plottingInternal}
	if plotProfilesStyled(po, filepath.Join(dir, "a.pdf"), "t", "x", 0,
		nil, nil, nil, nil, true) {
		t.Errorf("empty series must not produce a plot")
	}
	if plotRefractiveProfile(po, filepath.Join(dir, "b.pdf"), nil, nil, nil) {
		t.Errorf("empty refractive profile must not produce a plot")
	}
}

// utf16be кодирует строку так, как её пишет fpdf для UTF-8 шрифтов.
func utf16be(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 0, len(u)*2)
	for _, r := range u {
		b = append(b, byte(r>>8), byte(r))
	}
	return b
}

// pdfPayloads возвращает распакованные потоки PDF (содержимое сжато zlib).
func pdfPayloads(data []byte) [][]byte {
	var out [][]byte
	for {
		i := bytes.Index(data, []byte("stream"))
		if i < 0 {
			return out
		}
		data = data[i+len("stream"):]
		data = bytes.TrimPrefix(data, []byte("\r"))
		data = bytes.TrimPrefix(data, []byte("\n"))
		j := bytes.Index(data, []byte("endstream"))
		if j < 0 {
			return out
		}
		raw := data[:j]
		data = data[j+len("endstream"):]
		if zr, err := zlib.NewReader(bytes.NewReader(raw)); err == nil {
			if buf, err := io.ReadAll(zr); err == nil {
				out = append(out, buf)
				zr.Close()
				continue
			}
			zr.Close()
		}
		out = append(out, raw)
	}
}

func TestInternalPDFUnicodeText(t *testing.T) {
	dir := t.TempDir()
	po := &plotOpts{fmt: plotFmtPdf, backend: plottingInternal}
	p := filepath.Join(dir, "unicode.pdf")
	alt := []float64{0, 500}
	vals := [][]float64{{1, 2}}
	if !plotProfilesStyled(po, p, "Профиль концентрации",
		"r, µm; N, cm⁻³", 1, alt, vals, []string{"⟨m⟩, δ_l"}, nil, true) {
		t.Fatalf("internal plot failed")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read pdf: %v", err)
	}
	// Шрифт подключается как UTF-8 (Identity-H), текст пишется в UTF-16BE.
	// Раньше vgpdf с cp1252 давал мойзинг вида "Âµ" и "Ð...".
	all := bytes.Join(pdfPayloads(data), []byte("\n"))
	for _, s := range []string{"Профиль", "µm", "⟨m⟩", "cm⁻³"} {
		if !bytes.Contains(all, utf16be(s)) {
			t.Errorf("PDF does not contain %q as UTF-16BE text", s)
		}
	}
	if bytes.Contains(all, []byte("µm")) {
		t.Errorf("PDF contains raw UTF-8 bytes instead of UTF-16BE text")
	}
}
