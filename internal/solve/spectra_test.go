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
	if !sp.lidar.usable() {
		t.Fatal("lidar ratio spectrum must be available")
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
	// Cext = Cback = x², поэтому S = α/β = 1 на всей сетке.
	for i := range sp.wlNm {
		checkClose(t, fmt.Sprintf("lidar ratio at λ = %g nm", sp.wlNm[i]),
			sp.lidar.mean[i], 1.0, 1e-6)
		if sp.lidar.low[i] > sp.lidar.mean[i]+1e-18 ||
			sp.lidar.mean[i] > sp.lidar.high[i]+1e-18 {
			t.Errorf("λ = %g nm: lidar ratio low <= mean <= high violated",
				sp.wlNm[i])
		}
	}
	if len(sp.alpha.meas) != 1 || sp.alpha.meas[0].wlNm != 532 {
		t.Errorf("measured alpha marker must be at 532 nm: %+v", sp.alpha.meas)
	}
	if len(sp.beta.meas) != 1 || sp.beta.meas[0].wlNm != 355 {
		t.Errorf("measured beta marker must be at 355 nm: %+v", sp.beta.meas)
	}
	if len(sp.lidar.meas) != 0 {
		t.Errorf("no common measured wavelengths, got S markers: %+v",
			sp.lidar.meas)
	}
}

// TestSpectrumPanelSlots — панели привязаны к фиксированным ячейкам 2×2:
// α (0,0), β (0,1), S (1,0), δ_l (1,1), даже если часть панелей отсутствует.
func TestSpectrumPanelSlots(t *testing.T) {
	set := func() *spectrumSet { return &spectrumSet{mean: []float64{1}} }
	sp := &spectra{
		wlNm: []float64{532}, alpha: set(), beta: set(), lidar: set(),
		delta: set(),
	}
	want := []struct {
		suffix string
		slot   int
	}{
		{"alpha", spectrumSlotAlpha},
		{"beta", spectrumSlotBeta},
		{"lidar", spectrumSlotLidar},
		{"depol", spectrumSlotDepol},
	}
	np := sp.namedPanels()
	if len(np) != len(want) {
		t.Fatalf("panels = %d, want %d", len(np), len(want))
	}
	for i := range want {
		if np[i].suffix != want[i].suffix || np[i].panel.slot != want[i].slot {
			t.Errorf("panel %d = %q slot %d, want %q slot %d", i,
				np[i].suffix, np[i].panel.slot, want[i].suffix, want[i].slot)
		}
	}

	sp.alpha = nil
	np = sp.namedPanels()
	if len(np) != 3 || np[0].suffix != "beta" || np[0].panel.slot != 1 ||
		np[1].suffix != "lidar" || np[1].panel.slot != 2 ||
		np[2].suffix != "depol" || np[2].panel.slot != 3 {
		t.Errorf("sparse panels lost fixed slots: %+v", np)
	}
}

// TestMeasuredLidarPoints — измеренное S считается только на общих длинах
// волн, где есть и α, и β, причём β > 0.
func TestMeasuredLidarPoints(t *testing.T) {
	alpha := []wlPoint{{355, 4e-4}, {532, 3e-4}, {700, math.NaN()}}
	beta := []wlPoint{{355, 1e-4}, {532, 2e-4}, {700, 1e-4}}
	got := measuredLidarPoints(alpha, beta)
	if len(got) != 2 {
		t.Fatalf("measured S points = %+v, want 2", got)
	}
	checkClose(t, "S(355)", got[0].value, 4, 1e-12)
	checkClose(t, "S(532)", got[1].value, 1.5, 1e-12)
	if got[0].wlNm != 355 || got[1].wlNm != 532 {
		t.Errorf("unexpected wavelengths: %+v", got)
	}

	// нет общих длин волн или β = 0 — точек нет
	if pts := measuredLidarPoints(alpha, []wlPoint{{400, 1e-4}}); len(pts) != 0 {
		t.Errorf("expected no common-wavelength points, got %+v", pts)
	}
	if pts := measuredLidarPoints([]wlPoint{{355, 1e-4}},
		[]wlPoint{{355, 0}}); len(pts) != 0 {
		t.Errorf("β = 0 must be skipped, got %+v", pts)
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

// TestEnvelopeUsesBestSolutions — огибающие строятся только по первым G
// (лучшим) решениям: решения за пределами G не влияют на low/high.
func TestEnvelopeUsesBestSolutions(t *testing.T) {
	g := testBinGeom(t, 0.1, 4)
	geoms := []*geom{g}
	mk := func(n int, c []float64) *solutions {
		s := &solutions{n: n, method: methodMonotone, kernelOrder: 3}
		for i := 0; i < n; i++ {
			s.sol = append(s.sol, solution{mRe: 1.5, mIm: -0.001, r0: 0.1,
				r1: 1.0, geomIndex: 0, C: c})
		}
		return s
	}
	weak := []float64{1e-6, 1e-6, 1e-6, 1e-6}
	strong := []float64{3e-6, 3e-6, 3e-6, 3e-6}
	extreme := []float64{1e-2, 1e-2, 1e-2, 1e-2}

	rGrid := []float64{0.2, 0.5, 0.9}
	// G = 2: третье (экстремальное) решение в огибающие попасть не должно
	sols := mk(3, weak)
	sols.sol[1].C = strong
	sols.sol[2].C = extreme
	ens := ensembleAverage(sols, 2, geoms, rGrid, false)
	for i, r := range rGrid {
		v0 := distAt(&sols.sol[0], g, r)
		v1 := distAt(&sols.sol[1], g, r)
		checkClose(t, fmt.Sprintf("low at r=%g", r), ens.low[i],
			math.Min(v0, v1), 1e-15)
		checkClose(t, fmt.Sprintf("high at r=%g", r), ens.high[i],
			math.Max(v0, v1), 1e-15)
		checkClose(t, fmt.Sprintf("avg at r=%g", r), ens.avg[i],
			0.5*(v0+v1), 1e-15)
		if ens.high[i] >= distAt(&sols.sol[2], g, r) {
			t.Errorf("r=%g: extreme solution beyond G must not affect the envelope", r)
		}
	}

	// G = 1: огибающие совпадают со средним
	sols1 := mk(2, weak)
	sols1.sol[1].C = extreme
	ens1 := ensembleAverage(sols1, 1, geoms, rGrid, false)
	for i := range rGrid {
		if ens1.low[i] != ens1.avg[i] || ens1.high[i] != ens1.avg[i] {
			t.Errorf("G=1: expected low=high=avg, got %g, %g, %g",
				ens1.low[i], ens1.avg[i], ens1.high[i])
		}
	}
}

// TestSpectraUseBestSolutions — спектры тоже агрегируются только по G лучшим
// решениям (спектр линеен по распределению, поэтому среднее двух решений
// равно полусумме их спектров).
func TestSpectraUseBestSolutions(t *testing.T) {
	src := testSpectrumSource(t, testX2Table(t))
	g := testBinGeom(t, 0.1, 4)
	weak := []float64{1e-6, 1e-6, 1e-6, 1e-6}
	strong := []float64{3e-6, 3e-6, 3e-6, 3e-6}
	extreme := []float64{1e-2, 1e-2, 1e-2, 1e-2}
	sols := &solutions{n: 3, method: methodMonotone, kernelOrder: 3,
		sol: []solution{
			{mRe: 1.5, mIm: -0.001, r0: 0.1, r1: 1.0, geomIndex: 0, C: weak},
			{mRe: 1.5, mIm: -0.001, r0: 0.1, r1: 1.0, geomIndex: 0, C: strong},
			{mRe: 1.5, mIm: -0.001, r0: 0.1, r1: 1.0, geomIndex: 0, C: extreme},
		}}
	rq, wq := quadratureGrid([]float64{0.1, 1.0}, 24, 0.1)
	ch := []channel{{kind: chAlpha, wlUm: 0.532}}
	base := Args{Spectra: true, SpectrumPoints: 4, KernelOrder: 3,
		TableRefWl: tableRefWlUm}

	sp1 := buildSpectra(&Args{Spectra: true, SpectrumPoints: 4, Best: 1,
		KernelOrder: 3, TableRefWl: tableRefWlUm}, sols, []*geom{g}, rq, wq,
		nil, src, ch, nil, nil, nil)
	if sp1 == nil || sp1.g != 1 {
		t.Fatalf("expected G=1 spectra, got %+v", sp1)
	}
	for i := range sp1.wlNm {
		if sp1.alpha.low[i] != sp1.alpha.mean[i] ||
			sp1.alpha.high[i] != sp1.alpha.mean[i] {
			t.Errorf("G=1: expected low=high=mean for α, got %g, %g, %g",
				sp1.alpha.low[i], sp1.alpha.mean[i], sp1.alpha.high[i])
		}
	}

	// G=2: экстремальное третье решение не входит ни в среднее, ни в огибающие
	a2 := base
	a2.Best = 2
	sp2 := buildSpectra(&a2, sols, []*geom{g}, rq, wq, nil, src, ch, nil,
		nil, nil)
	if sp2 == nil || sp2.g != 2 {
		t.Fatalf("expected G=2 spectra, got %+v", sp2)
	}
	// спектр одного решения (weak или strong) для той же сетки
	one := func(c []float64) *spectra {
		s := &solutions{n: 1, method: methodMonotone, kernelOrder: 3,
			sol: []solution{{mRe: 1.5, mIm: -0.001, r0: 0.1, r1: 1.0,
				geomIndex: 0, C: c}}}
		return buildSpectra(&Args{Spectra: true, SpectrumPoints: 4, Best: 1,
			KernelOrder: 3, TableRefWl: tableRefWlUm}, s, []*geom{g}, rq, wq,
			nil, src, ch, nil, nil, nil)
	}
	sw, ss := one(weak), one(strong)
	for i := range sp2.wlNm {
		checkClose(t, "spectra mean = average of best two", sp2.alpha.mean[i],
			0.5*(sw.alpha.mean[i]+ss.alpha.mean[i]), 1e-12)
		if sp2.alpha.low[i] > sw.alpha.mean[i]+1e-18 ||
			sp2.alpha.high[i] < ss.alpha.mean[i]-1e-18 {
			t.Errorf("λ=%g: envelope must span the best two solutions", sp2.wlNm[i])
		}
	}
}

// TestWriteSpectrum — файл пакетного режима: 13 столбцов (alpha/beta/delta
// и добавленные в конец lidar), измеренные значения в комментарии, nan для
// отсутствующих величин.
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
		lidar: &spectrumSet{
			mean: []float64{70, 60}, low: []float64{68, 58},
			high: []float64{72, 62},
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
	if !strings.Contains(text, "lidar_mean lidar_low lidar_high") {
		t.Errorf("lidar columns are missing:\n%s", text)
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
		if len(f) != 13 {
			t.Fatalf("expected 13 columns, got %d: %q", len(f), ln)
		}
		for i := 4; i < 10; i++ {
			if f[i] != "nan" {
				t.Errorf("missing beta/delta must be nan, got %q", f[i])
			}
		}
		for i := 10; i < 13; i++ {
			if f[i] == "nan" {
				t.Errorf("lidar columns must be filled, got %q", f[i])
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
		`"high"`, `"measured"`, `"beta"`, `"lidar_ratio"`,
		`"depolarization"`} {
		if !strings.Contains(text, key) {
			t.Errorf("JSON is missing %s: %s", key, text)
		}
	}
}

// TestAggregateSpectrumAllCurves — на графике спектра должны рисоваться все
// решения ансамбля, а не первые 64.
func TestAggregateSpectrumAllCurves(t *testing.T) {
	const n = 100
	vals := make([][]float64, n)
	for i := range vals {
		vals[i] = []float64{float64(i), float64(i) + 1}
	}
	s := aggregateSpectrum(vals, []float64{400, 500}, 0)
	if got := len(s.curves); got != n {
		t.Fatalf("curves = %d, want %d", got, n)
	}
	if got := s.curves[n-1][0]; got != float64(n-1) {
		t.Errorf("last curve = %g, want the last solution", got)
	}
	if want := float64(n-1) / 2; math.Abs(s.mean[0]-want) > 1e-12 {
		t.Errorf("mean[0] = %g, want %g", s.mean[0], want)
	}
	if s.low[0] != 0 || s.high[0] != float64(n-1) {
		t.Errorf("envelope = [%g, %g], want [0, %d]", s.low[0], s.high[0], n-1)
	}

	// с явным пределом curves обрезаются, а среднее — по всему ансамблю
	lim := aggregateSpectrum(vals, []float64{400, 500}, 10)
	if got := len(lim.curves); got != 10 {
		t.Errorf("limited curves = %d, want 10", got)
	}
	if want := float64(n-1) / 2; math.Abs(lim.mean[0]-want) > 1e-12 {
		t.Errorf("limited mean[0] = %g, want %g", lim.mean[0], want)
	}
	if lim.low[0] != 0 || lim.high[0] != float64(n-1) {
		t.Errorf("limited envelope = [%g, %g], want [0, %d]",
			lim.low[0], lim.high[0], n-1)
	}
}
