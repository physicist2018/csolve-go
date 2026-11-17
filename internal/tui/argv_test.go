package tui

import (
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

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
