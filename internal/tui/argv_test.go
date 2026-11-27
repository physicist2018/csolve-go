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

package tui

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// newTestState — состояние с параметрами по умолчанию для тестов.
func newTestState(t *testing.T) *state {
	t.Helper()
	s, err := newState()
	if err != nil {
		t.Fatalf("newState: %v", err)
	}
	return s
}

func TestBuildArgvDefaults(t *testing.T) {
	s := newTestState(t)
	argv, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("default state: %v", err)
	}
	if len(argv) != 2 || argv[0] != "--type" || argv[1] != "agglomerate" {
		t.Fatalf("unexpected argv: %v", argv)
	}
	if a.Type != "agglomerate" {
		t.Errorf("type = %q, want agglomerate", a.Type)
	}
	if a.Data != "" || a.Batch != "" {
		t.Errorf("expected demo input, got data=%q batch=%q", a.Data, a.Batch)
	}
	if !a.Nonneg || !a.DepolRatioCons {
		t.Errorf("default constraints: nonneg=%v depol=%v",
			a.Nonneg, a.DepolRatioCons)
	}
	if a.Modes != "stat" {
		t.Errorf("default modes = %q, want stat", a.Modes)
	}
	if a.NBasis != 7 || a.NrOut != 25 {
		t.Errorf("default n-basis/nr-out = %d/%d, want 7/25",
			a.NBasis, a.NrOut)
	}
	if a.Best != 0.05 {
		t.Errorf("default best = %g, want 0.05", a.Best)
	}
	if a.R0Min != 0.05 || a.R0Max != 0.3 || a.NR0 != 7 {
		t.Errorf("default r0 grid = %g..%g x%d", a.R0Min, a.R0Max, a.NR0)
	}
	if a.R1Min != 0.45 || a.R1Max != 3.0 || a.NR1 != 7 {
		t.Errorf("default r1 grid = %g..%g x%d", a.R1Min, a.R1Max, a.NR1)
	}
	if a.PlotFormat != "pdf" || a.PlotFontSize != 12 {
		t.Errorf("default plot format/font = %s/%g", a.PlotFormat, a.PlotFontSize)
	}
	if a.LidarRatioW != 1.0 || a.LidarRatioIter != 5 {
		t.Errorf("default lidar ratio w/iter = %g/%d",
			a.LidarRatioW, a.LidarRatioIter)
	}
	if s.modes != "stat" || !s.nonneg || !s.depolCons || s.plotFormat != "pdf" {
		t.Errorf("TUI defaults: modes=%q nonneg=%v depol=%v format=%q",
			s.modes, s.nonneg, s.depolCons, s.plotFormat)
	}
}

func TestDefaultConstraintsCanBeDisabled(t *testing.T) {
	s := newTestState(t)
	// in the monotone method non-negativity is implicit; the flags exist for
	// --method spline only
	s.method = "spline"
	s.nonneg = false
	s.depolCons = false

	argv, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("disabled constraints: %v", err)
	}
	if a.Nonneg || a.DepolRatioCons {
		t.Errorf("constraints must be off: nonneg=%v depol=%v",
			a.Nonneg, a.DepolRatioCons)
	}
	if !contains(argv, "--no-nonneg") || !contains(argv, "--no-depol-ratio-cons") {
		t.Errorf("argv misses disable flags: %v", argv)
	}
}

func TestChangedDefaultsEmitFlags(t *testing.T) {
	s := newTestState(t)
	s.modes = "auto"
	s.plotFontSize = "10"
	s.nrOut = "50"

	argv, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("changed defaults: %v", err)
	}
	if a.Modes != "auto" || a.PlotFontSize != 10 || a.NrOut != 50 {
		t.Errorf("modes/font/nr-out = %s/%g/%d", a.Modes, a.PlotFontSize, a.NrOut)
	}
	if !contains(argv, "--modes") || !contains(argv, "--plot-font-size") ||
		!contains(argv, "--nr-out") {
		t.Errorf("argv misses changed values: %v", argv)
	}
}

func TestPresetDemoSphere(t *testing.T) {
	s := newTestState(t)
	presetByKey("demo-spheres").apply(s)

	_, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("demo-spheres: %v", err)
	}
	if a.Type != "sphere" || !a.TableExtrapolate {
		t.Errorf("type=%q extrapolate=%v", a.Type, a.TableExtrapolate)
	}
	if a.R0Min != 0.05 || a.R0Max != 0.3 || a.R1Min != 0.5 || a.R1Max != 1.9 {
		t.Errorf("windows = %g..%g, %g..%g", a.R0Min, a.R0Max, a.R1Min, a.R1Max)
	}
	if a.OutDir != "out" {
		t.Errorf("out dir = %q", a.OutDir)
	}
	if !s.readyToRun() {
		t.Error("quick preset should be ready to run")
	}
}

func TestInputFileRequired(t *testing.T) {
	s := newTestState(t)
	s.mode = modeData
	if _, _, err := buildArgv(s); err == nil {
		t.Fatal("expected an error for a missing JSON file")
	}

	s.dataPath = "input.json"
	_, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("with JSON file: %v", err)
	}
	if a.Data != "input.json" {
		t.Errorf("data = %q", a.Data)
	}
}

func TestBatchLidarConstraint(t *testing.T) {
	s := newTestState(t)
	s.mode = modeBatch
	s.batchPath = "measurements.txt"
	s.lidarCons = true
	s.lidarW = "0.7"

	argv, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if !a.LidarRatioCons || a.LidarRatioW != 0.7 {
		t.Errorf("lidar cons=%v w=%g", a.LidarRatioCons, a.LidarRatioW)
	}
	if !contains(argv, "--lidar-ratio-cons") {
		t.Errorf("argv misses --lidar-ratio-cons: %v", argv)
	}
}

func TestLidarConstraintOnlyInBatch(t *testing.T) {
	s := newTestState(t)
	s.lidarCons = true

	argv, _, err := buildArgv(s)
	if err != nil {
		t.Fatalf("demo: %v", err)
	}
	if contains(argv, "--lidar-ratio-cons") {
		t.Errorf("lidar constraint must not be passed outside batch mode: %v", argv)
	}
}

func TestBatchThinning(t *testing.T) {
	s := newTestState(t)
	s.mode = modeBatch
	s.batchPath = "measurements.txt"
	s.batchThin = "100"

	argv, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if a.BatchThin != 100 {
		t.Errorf("batch thin = %g, want 100", a.BatchThin)
	}
	if !contains(argv, "--batch-thin") {
		t.Errorf("argv misses --batch-thin: %v", argv)
	}
}

func TestBatchThinningOnlyInBatch(t *testing.T) {
	s := newTestState(t)
	s.batchThin = "100"

	argv, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("demo: %v", err)
	}
	if a.BatchThin != 0 {
		t.Errorf("batch thin = %g outside batch mode, want 0", a.BatchThin)
	}
	if contains(argv, "--batch-thin") {
		t.Errorf("altitude thinning must not be passed outside batch mode: %v", argv)
	}
}

func TestExternalTablesRequireDirectory(t *testing.T) {
	s := newTestState(t)
	s.tablesKind = tablesExternal
	if _, _, err := buildArgv(s); err == nil {
		t.Fatal("expected an error for a missing tables directory")
	}

	s.tablesDir = "bases/large_aggl_100/shape02"
	argv, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("external tables: %v", err)
	}
	if a.Tables != s.tablesDir {
		t.Errorf("tables = %q", a.Tables)
	}
	if contains(argv, "--type") {
		t.Errorf("--type must not be passed with --tables: %v", argv)
	}
}

func TestInternalRendererRejectsNonPDF(t *testing.T) {
	s := newTestState(t)
	s.out = "result.png"
	if _, _, err := buildArgv(s); err == nil {
		t.Fatal("expected an error for a PNG output with the internal renderer")
	}

	s.out = "result.pdf"
	if _, _, err := buildArgv(s); err != nil {
		t.Fatalf("PDF output: %v", err)
	}
}

func TestBestFraction(t *testing.T) {
	s := newTestState(t)
	s.best = "0.1"
	_, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("best fraction: %v", err)
	}
	if a.Best != 0.1 {
		t.Errorf("best = %g", a.Best)
	}
}

func TestPlotBestLimit(t *testing.T) {
	s := newTestState(t)
	s.plotBestLim = "128"
	argv, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("plot-best-limit: %v", err)
	}
	if a.PlotBestLimit != 128 {
		t.Errorf("PlotBestLimit = %d, want 128", a.PlotBestLimit)
	}
	if !slices.Contains(argv, "--plot-best-limit") {
		t.Errorf("argv is missing --plot-best-limit: %v", argv)
	}

	// по умолчанию (0 = все) флаг не добавляется
	argv, _, err = buildArgv(newTestState(t))
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	if slices.Contains(argv, "--plot-best-limit") {
		t.Errorf("default argv must not contain --plot-best-limit: %v", argv)
	}
}

func TestDescribeOutputs(t *testing.T) {
	s := newTestState(t)
	s.outDir = "out"
	s.jsonOut = "res.json"

	_, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("build argv: %v", err)
	}
	text := describeOutputs(s, a)
	for _, want := range []string{"out/result.pdf", "out/res.json"} {
		if !strings.Contains(text, want) {
			t.Errorf("output paths %q miss %q", text, want)
		}
	}

	s.mode = modeBatch
	s.batchPath = "measurements.txt"
	s.outDir = "batch_out"
	_, a, err = buildArgv(s)
	if err != nil {
		t.Fatalf("build argv (batch): %v", err)
	}
	if text := describeOutputs(s, a); !strings.Contains(text, "batch_out") {
		t.Errorf("batch output text = %q", text)
	}
}

func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"--out", "my file.pdf", "--json", "a'b.json"})
	want := "--out 'my file.pdf' --json 'a'\\''b.json'"
	if got != want {
		t.Errorf("shellJoin = %q, want %q", got, want)
	}
}

func TestJobsFlag(t *testing.T) {
	s := newTestState(t)

	// по умолчанию (как в дефолтах решателя) флаг не добавляется
	argv, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	if contains(argv, "--jobs") {
		t.Errorf("default argv must not contain --jobs: %v", argv)
	}
	if a.Jobs != s.def.Jobs {
		t.Errorf("Jobs = %d, want default %d", a.Jobs, s.def.Jobs)
	}

	// изменённое значение передаётся; выбираем число, заведомо отличное от
	// дефолта (он зависит от числа CPU на машине)
	want := 3
	if s.def.Jobs == want {
		want = 2
	}
	s.jobs = strconv.Itoa(want)
	argv, a, err = buildArgv(s)
	if err != nil {
		t.Fatalf("jobs=%d: %v", want, err)
	}
	if a.Jobs != want {
		t.Errorf("Jobs = %d, want %d", a.Jobs, want)
	}
	if !contains(argv, "--jobs") {
		t.Errorf("argv is missing --jobs: %v", argv)
	}

	// 0 = auto разрешается в дефолт
	s.jobs = "0"
	_, a, err = buildArgv(s)
	if err != nil {
		t.Fatalf("jobs=0: %v", err)
	}
	if a.Jobs != 0 {
		t.Errorf("Jobs = %d, want 0 (auto)", a.Jobs)
	}
	if a.Workers() != s.def.Jobs {
		t.Errorf("Workers = %d, want default %d", a.Workers(), s.def.Jobs)
	}
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
