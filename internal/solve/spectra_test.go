package solve

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// testX2Table — таблица с Cext = Cback = x², S11 = x, S22 = 1 для проверки
// спектров: при C = x² ядро K(λ, r) не зависит от λ.
func testX2Table(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("# x Cext Cabs Cback Qext Qabs Qback S11 S22 n\n")
	const n = 100
	for i := 0; i < n; i++ {
		x := 0.02 * math.Pow(15000.0, float64(i)/float64(n-1))
		fmt.Fprintf(&sb, "%.17g %.17g 0 %.17g 0 0 0 %.17g 1 1\n", x, x*x, x*x, x)
	}
	return sb.String()
}

// testSpectrumSource — источник таблиц в памяти с одним m = 1.5 - 0.001j.
func testSpectrumSource(t *testing.T, text string) *tableSource {
	t.Helper()
	dir := mTableDirname(1.5, -0.001)
	return &tableSource{
		fsys: fstest.MapFS{
			dir + "/data.txt": &fstest.MapFile{Data: []byte(text)},
		},
		label: "test",
		con:   testConsole(),
		cache: make(map[tableCacheKey]*table),
	}
}

// testBinGeom — окно monotone [r0, 1] с заданным числом бинов.
func testBinGeom(t *testing.T, r0 float64, bins int) *geom {
	t.Helper()
	g, err := geomBuildBins(r0, 1.0, bins, 24, 0.1)
	if err != nil {
		t.Fatalf("geomBuildBins: %v", err)
	}
	return g
}

// TestEnsembleEnvelope — огибающие: NaN вне покрытия, low=high=avg там, где
// точку покрывает одно решение, и low ≤ avg ≤ high внутри общего окна.
func TestEnsembleEnvelope(t *testing.T) {
	g1 := testBinGeom(t, 0.1, 4)
	g2 := testBinGeom(t, 0.3, 3)
	geoms := []*geom{g1, g2}
	sols := &solutions{n: 2, method: methodMonotone, kernelOrder: 3,
		sol: []solution{
			{mRe: 1.5, mIm: -0.001, r0: 0.1, r1: 1.0, geomIndex: 0,
				C: []float64{1e-6, 2e-6, 1e-6, 3e-6}},
			{mRe: 1.5, mIm: -0.001, r0: 0.3, r1: 1.0, geomIndex: 1,
				C: []float64{5e-6, 1e-6, 2e-6}},
		}}

	rGrid := []float64{0.05, 0.1, 0.3, 0.7, 1.0, 1.5}
	ens := ensembleAverage(sols, 2, geoms, rGrid, false)

	for i, r := range rGrid {
		if r < 0.1 || r > 1.0 {
			if !math.IsNaN(ens.avg[i]) || !math.IsNaN(ens.low[i]) ||
				!math.IsNaN(ens.high[i]) {
				t.Errorf("r = %g: expected NaN outside coverage, got avg=%g low=%g high=%g",
					r, ens.avg[i], ens.low[i], ens.high[i])
			}
			continue
		}
		if !isFinite(ens.avg[i]) {
			t.Fatalf("r = %g: mean must be finite", r)
		}
		v0 := distAt(&sols.sol[0], g1, r) * kernelToVolumeRatio(3, r)
		v1 := math.NaN()
		if r >= sols.sol[1].r0 && r <= sols.sol[1].r1 {
			v1 = distAt(&sols.sol[1], g2, r) * kernelToVolumeRatio(3, r)
		}
		wantLow, wantHigh := v0, v0
		if isFinite(v1) {
			wantLow = math.Min(v0, v1)
			wantHigh = math.Max(v0, v1)
		}
		checkClose(t, fmt.Sprintf("low at r = %g", r), ens.low[i], wantLow, 1e-12)
		checkClose(t, fmt.Sprintf("high at r = %g", r), ens.high[i], wantHigh, 1e-12)
		if ens.low[i] > ens.avg[i]+1e-18 || ens.avg[i] > ens.high[i]+1e-18 {
			t.Errorf("r = %g: expected low <= avg <= high, got %g, %g, %g",
				r, ens.low[i], ens.avg[i], ens.high[i])
		}
	}
}

// TestSpectraInvariants — при Cext = Cback = x² ядра не зависят от λ, поэтому
// средние α(λ) и β(λ) постоянны; огибающие охватывают среднее.
func TestSpectraInvariants(t *testing.T) {
	src := testSpectrumSource(t, testX2Table(t))
	g := testBinGeom(t, 0.1, 4)
	geoms := []*geom{g}
	sols := &solutions{n: 2, method: methodMonotone, kernelOrder: 3,
		sol: []solution{
			{mRe: 1.5, mIm: -0.001, r0: 0.1, r1: 1.0, geomIndex: 0,
				C: []float64{1e-6, 2e-6, 1e-6, 3e-6}},
			{mRe: 1.5, mIm: -0.001, r0: 0.1, r1: 1.0, geomIndex: 0,
				C: []float64{2e-6, 1e-6, 3e-6, 1e-6}},
		}}
	ch := []channel{{kind: chBeta, wlUm: 0.355}, {kind: chAlpha, wlUm: 0.532}}
	g1 := []float64{1e-6, 1e-5}
	a := &Args{Spectra: true, SpectrumPoints: 6, Best: 2,
		KernelOrder: 3, TableRefWl: tableRefWlUm}
	rq, wq := quadratureGrid([]float64{0.1, 1.0}, 24, 0.1)

	sp := buildSpectra(a, sols, geoms, rq, wq, nil, src, ch, g1, nil, nil)
	if sp == nil {
		t.Fatal("spectra must be built")
	}
	if len(sp.wlNm) < 4 {
		t.Fatalf("expected a wavelength grid, got %d points", len(sp.wlNm))
	}
	if !sp.alpha.usable() || !sp.beta.usable() {
		t.Fatal("alpha and beta spectra must be available")
	}
	if sp.delta.usable() {
		t.Error("delta_l must be unavailable without S11/S22 in the depol set")
	}
	for _, set := range []*spectrumSet{sp.alpha, sp.beta} {
		ref := set.mean[0]
		if !(ref > 0) {
			t.Fatalf("expected positive spectrum, got %g", ref)
		}
		for i := range sp.wlNm {
			checkClose(t, fmt.Sprintf("invariant spectrum at λ = %g nm",
				sp.wlNm[i]), set.mean[i], ref, 1e-6)
			if set.low[i] > set.mean[i]+1e-18 ||
				set.mean[i] > set.high[i]+1e-18 {
				t.Errorf("λ = %g nm: expected low <= mean <= high", sp.wlNm[i])
			}
		}
	}
	if len(sp.alpha.meas) != 1 || sp.alpha.meas[0].wlNm != 532 {
		t.Errorf("measured alpha marker must be at 532 nm: %+v", sp.alpha.meas)
	}
	if len(sp.beta.meas) != 1 || sp.beta.meas[0].wlNm != 355 {
		t.Errorf("measured beta marker must be at 355 nm: %+v", sp.beta.meas)
	}
}

// TestSpectraDepolMatchesProfile — δ_l(λ) одиночного решения совпадает с
// depolFromProfile при той же свёртке S11/S22.
func TestSpectraDepolMatchesProfile(t *testing.T) {
	text := testX2Table(t)
	tb, _, err := parseTable([]byte(text), "data.txt", false)
	if err != nil {
		t.Fatalf("parseTable: %v", err)
	}
	src := testSpectrumSource(t, text)
	g := testBinGeom(t, 0.1, 4)
	sols := &solutions{n: 1, method: methodMonotone, kernelOrder: 3,
		sol: []solution{
			{mRe: 1.5, mIm: -0.001, r0: 0.1, r1: 1.0, geomIndex: 0,
				C: []float64{1e-6, 2e-6, 1e-6, 3e-6}},
		}}
	rq, wq := quadratureGrid([]float64{0.1, 1.0}, 24, 0.1)

	ds := &depolSet{wl: []float64{0.355}, kt: make([][]*ktable, 1)}
	ds.kt[0] = make([]*ktable, 1)
	kt, err := depolKtableBuild(tb, 0.355, tableRefWlUm, rq, testConsole())
	if err != nil {
		t.Fatalf("depolKtableBuild: %v", err)
	}
	ds.kt[0][0] = kt
	ds.nOK = 1

	nvol := make([]float64, len(rq))
	for k, r := range rq {
		nvol[k] = distAt(&sols.sol[0], g, r)
	}
	_, _, want := depolFromProfile(ds, []int{0}, rq, wq, nvol)

	a := &Args{Spectra: true, SpectrumPoints: 2, Best: 1,
		KernelOrder: 3, TableRefWl: tableRefWlUm,
		SpectrumWlMin: 355, SpectrumWlMax: 532}
	sp := buildSpectra(a, sols, []*geom{g}, rq, wq, ds, src, nil, nil, nil, nil)
	if sp == nil || !sp.delta.usable() {
		t.Fatal("delta_l spectrum must be available")
	}
	idx := -1
	for i, wl := range sp.wlNm {
		if math.Abs(wl-355) < 1e-9 {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("355 nm must be on the grid: %v", sp.wlNm)
	}
	checkClose(t, "delta_l spectrum vs depolFromProfile", sp.delta.mean[idx],
		want[0], 1e-9)
}

// TestSpectrumWavelengths — сетка включает входные длины волн, учитывает
// переопределение границ и вырожденный случай одной длины волны.
func TestSpectrumWavelengths(t *testing.T) {
	ch := []channel{{kind: chBeta, wlUm: 0.355}, {kind: chAlpha, wlUm: 0.532}}
	a := &Args{Spectra: true, SpectrumPoints: 5, TableRefWl: tableRefWlUm}
	wl, ok := spectrumWavelengths(a, ch, nil, nil)
	if !ok || len(wl) != 5 {
		t.Fatalf("expected 5 wavelengths, got %v", wl)
	}
	if wl[0] != 355 || wl[len(wl)-1] != 532 {
		t.Errorf("default range must be [355, 532], got %v", wl)
	}

	a.SpectrumWlMin = 300
	a.SpectrumWlMax = 1000
	wl, ok = spectrumWavelengths(a, ch, []float64{0.700}, nil)
	if !ok || wl[0] != 300 || wl[len(wl)-1] != 1000 {
		t.Fatalf("range overrides must be applied, got %v", wl)
	}
	has := func(v float64) bool {
		for _, w := range wl {
			if math.Abs(w-v) < 1e-9 {
				return true
			}
		}
		return false
	}
	for _, v := range []float64{355, 532, 700} {
		if !has(v) {
			t.Errorf("input wavelength %g must be on the grid: %v", v, wl)
		}
	}

	// одна длина волны: диапазон расширяется
	a2 := &Args{Spectra: true, SpectrumPoints: 3, TableRefWl: tableRefWlUm}
	wl, ok = spectrumWavelengths(a2, []channel{{wlUm: 0.532}}, nil, nil)
	if !ok || len(wl) < 2 || wl[0] >= 532 || wl[len(wl)-1] <= 532 {
		t.Errorf("single-wavelength range must be expanded around 532: %v", wl)
	}
}

// TestWriteSpectrum — файл пакетного режима: 10 столбцов, измеренные значения
// в комментарии, nan для отсутствующих величин.
func TestWriteSpectrum(t *testing.T) {
	dir := t.TempDir()
	sp := &spectra{
		wlNm: []float64{355, 532},
		alpha: &spectrumSet{
			mean: []float64{1e-4, 7e-5}, low: []float64{9e-5, 6e-5},
			high:     []float64{1.1e-4, 8e-5},
			meas:     []wlPoint{{wlNm: 355, value: 1.05e-4}},
			measName: "measured α",
		},
	}
	if err := writeSpectrum(dir, 500, sp); err != nil {
		t.Fatalf("writeSpectrum: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "spectrum_500.txt"))
	if err != nil {
		t.Fatalf("read spectrum file: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "lambda_nm alpha_mean") {
		t.Errorf("header is missing:\n%s", text)
	}
	if !strings.Contains(text, "a355=0.000105") {
		t.Errorf("measured marker is missing:\n%s", text)
	}
	rows := 0
	for _, ln := range strings.Split(text, "\n") {
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		f := strings.Fields(ln)
		if len(f) != 10 {
			t.Fatalf("expected 10 columns, got %d: %q", len(f), ln)
		}
		for i := 4; i < len(f); i++ {
			if f[i] != "nan" {
				t.Errorf("missing beta/delta must be nan, got %q", f[i])
			}
		}
		rows++
	}
	if rows != 2 {
		t.Errorf("expected 2 data rows, got %d", rows)
	}
}

// TestOutSpectraJSON — JSON-секция спектров: среднее/огибающие и измеренные
// точки; nil при отключённых спектрах.
func TestOutSpectraJSON(t *testing.T) {
	if outSpectraFrom(nil) != nil {
		t.Error("nil spectra must produce a nil JSON section")
	}
	sp := &spectra{
		wlNm: []float64{355, 532},
		alpha: &spectrumSet{
			mean: []float64{1e-4, 7e-5}, low: []float64{9e-5, math.NaN()},
			high: []float64{1.1e-4, 8e-5},
			meas: []wlPoint{{wlNm: 355, value: 1.05e-4}},
		},
	}
	o := outSpectraFrom(sp)
	if o == nil {
		t.Fatal("expected a JSON section")
	}
	data, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	for _, key := range []string{`"wl_nm"`, `"alpha"`, `"mean"`, `"low"`,
		`"high"`, `"measured"`, `"beta"`, `"depolarization"`} {
		if !strings.Contains(text, key) {
			t.Errorf("JSON is missing %s: %s", key, text)
		}
	}
}
