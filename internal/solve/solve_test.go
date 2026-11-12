package solve

import (
	"bytes"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
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
