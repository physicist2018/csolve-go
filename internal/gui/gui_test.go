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

package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	solverOnce sync.Once
	solverPath string
	solverErr  error
)

// realSolver собирает настоящий бинарник csolve и возвращает его путь.
func realSolver() (string, error) {
	solverOnce.Do(func() {
		dir, err := os.MkdirTemp("", "csolve-solver-*")
		if err != nil {
			solverErr = err
			return
		}
		out := filepath.Join(dir, "csolve")
		if runtime.GOOS == "windows" {
			out += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", out, "./cmd/csolve")
		cmd.Dir = filepath.Join("..", "..") // из internal/gui в корень модуля
		if b, err := cmd.CombinedOutput(); err != nil {
			solverErr = fmt.Errorf("build solver: %v\n%s", err, b)
			return
		}
		solverPath = out
	})
	return solverPath, solverErr
}

func newTestApp(t *testing.T) *app {
	a, err := newApp()
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}
	t.Cleanup(a.Close)
	return a
}

func doGet(_ *testing.T, h http.Handler, token, url string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-CSolve-Token", token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doPost(_ *testing.T, h http.Handler, token, url string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSolve-Token", token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuth(t *testing.T) {
	a := newTestApp(t)
	h := a.Handler()
	rec := doGet(t, h, "wrong-token", "/api/spec")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong token: got %d, want 403", rec.Code)
	}
	rec2 := doGet(t, h, "", "/")
	if rec2.Code != http.StatusOK {
		t.Fatalf("index: got %d, want 200", rec2.Code)
	}
}

func TestHelpEndpoint(t *testing.T) {
	a := newTestApp(t)
	h := a.Handler()

	rec := doGet(t, h, a.token, "/api/help")
	if rec.Code != http.StatusOK {
		t.Fatalf("help: got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("content type = %q, want text/markdown", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "# csolve-go") {
		t.Errorf("help body must contain the README (%d bytes)", len(body))
	}

	// кнопка и диалог подключены на странице
	idx := doGet(t, h, "", "/")
	if !strings.Contains(idx.Body.String(), `id="help-btn"`) {
		t.Error("index page must contain the help button")
	}
	js := doGet(t, h, "", "/assets/app.js")
	if !strings.Contains(js.Body.String(), "openHelpModal") {
		t.Error("app.js must wire the help dialog")
	}

	// без токена доступ запрещён
	req := httptest.NewRequest(http.MethodGet, "/api/help", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("help without token: got %d, want 403", rr.Code)
	}
}

func TestMetaEndpoint(t *testing.T) {
	a := newTestApp(t)
	h := a.Handler()

	rec := doGet(t, h, a.token, "/api/meta")
	if rec.Code != http.StatusOK {
		t.Fatalf("meta: got %d", rec.Code)
	}
	var m struct {
		Version string `json:"version"`
		OS      string `json:"os"`
		Arch    string `json:"arch"`
		Issues  string `json:"issues"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	if m.Version == "" {
		t.Error("meta.version is empty")
	}
	if m.OS != runtime.GOOS || m.Arch != runtime.GOARCH {
		t.Errorf("meta os/arch = %s/%s, want %s/%s", m.OS, m.Arch, runtime.GOOS, runtime.GOARCH)
	}
	if !strings.HasSuffix(m.Issues, "/issues/new") {
		t.Errorf("meta.issues = %q, want .../issues/new", m.Issues)
	}

	// кнопка и диалог подключены на странице
	idx := doGet(t, h, "", "/")
	if !strings.Contains(idx.Body.String(), `id="issue-btn"`) {
		t.Error("index page must contain the issue button")
	}
	js := doGet(t, h, "", "/assets/app.js")
	if !strings.Contains(js.Body.String(), "openIssueModal") {
		t.Error("app.js must wire the issue dialog")
	}
}

func TestPlotEngineArgv(t *testing.T) {
	flagVal := func(argv []string, flag string) string {
		for i := 0; i+1 < len(argv); i++ {
			if argv[i] == flag {
				return argv[i+1]
			}
		}
		return ""
	}
	hasFlag := func(argv []string, flag string) bool {
		for _, a := range argv {
			if a == flag {
				return true
			}
		}
		return false
	}

	cases := []struct {
		name    string
		engine  string
		format  string
		backend string
		fmt     string
		out     string
		noPlot  bool
	}{
		{"internal-png", "internal", "png", "internal", "png", "result.png", false},
		{"default-engine", "", "png", "internal", "png", "result.png", false},
		{"gnuplot-svg", "gnuplot", "svg", "gnuplot", "svg", "result.svg", false},
		{"gnuplot-pdf", "gnuplot", "pdf", "gnuplot", "pdf", "result.pdf", false},
		{"internal-svg-fallback", "internal", "svg", "internal", "png", "result.png", false},
		{"no-plot", "gnuplot", "none", "", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &Request{Mode: "demo", TablesKind: "builtin",
				BuiltinType: "agglomerate", Options: map[string]any{},
				Output: Output{PlotFormat: tc.format, PlotEngine: tc.engine, Spectra: true}}
			argv, _, err := buildArgv(req, "/tmp/x")
			if err != nil {
				t.Fatalf("buildArgv: %v", err)
			}
			if tc.noPlot {
				if !hasFlag(argv, "--no-plot") {
					t.Errorf("--no-plot missing: %v", argv)
				}
				return
			}
			if got := flagVal(argv, "--plotting"); got != tc.backend {
				t.Errorf("--plotting = %q, want %q", got, tc.backend)
			}
			if got := flagVal(argv, "--plot-format"); got != tc.fmt {
				t.Errorf("--plot-format = %q, want %q", got, tc.fmt)
			}
			if got := flagVal(argv, "--out"); got != tc.out {
				t.Errorf("--out = %q, want %q", got, tc.out)
			}
		})
	}
}

func TestParseBatchSummaryColumns(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("moments.txt", "# H  Vf  Vc  V  rmf  rmc  rm  reff  N  sigma_f  sigma_c  sigma\n"+
		"# V — µm³/cm³; rm/reff — µm\n"+
		"1  0.1 0.2 0.3 0.4 0.5 0.6 0.7 0.8 0.9 1.0 1.1\n"+
		"2  0.11 0.21 0.31 0.41 0.51 0.61 0.71 0.81 0.91 1.01 1.11\n")
	write("refractive.txt", "# H  Re(m)  Im(m)\n1 1.5 0.01\n2 1.6 0.02\n")
	write("errors.txt", "# H  355  532  Err\n# relative errors\n"+
		"1  0.1 0.2 0.15\n2  0.11 0.21 0.16\n")
	write("delpol.txt", "# H  delta_l_355  delta_l_meas_355\n# linear depolarization\n"+
		"1  5 6\n2  5.5 6.5\n")
	write("lidar_ratios.txt", "# H  S_355  S_532\n# lidar ratio\n"+
		"1  50 60\n2  51 61\n")

	bs := parseBatchSummary(dir)
	if len(bs.Alt) != 2 {
		t.Fatalf("alt = %v, want 2 rows", bs.Alt)
	}
	checkCols := func(name string, want []string) {
		t.Helper()
		got := bs.Columns[name]
		if len(got) != len(want) {
			t.Fatalf("%s columns = %v, want %v", name, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s columns = %v, want %v", name, got, want)
			}
		}
	}
	checkCols("moments", []string{"Vf", "Vc", "V", "rmf", "rmc", "rm", "reff", "N", "sigma_f", "sigma_c", "sigma"})
	checkCols("errors", []string{"355", "532", "Err"})
	checkCols("delpol", []string{"delta_l_355", "delta_l_meas_355"})
	checkCols("lidar_ratios", []string{"S_355", "S_532"})

	// число значений в каждом файле должно совпадать с числом столбцов×высот
	for name, cols := range bs.Columns {
		if got, want := len(bs.Tables[name]), len(cols)*len(bs.Alt); got != want {
			t.Errorf("%s has %d values, want %d", name, got, want)
		}
	}
	if bs.Notes["moments"] == "" {
		t.Error("moments note (units) not captured")
	}
}

func TestSpecAndValidate(t *testing.T) {
	a := newTestApp(t)
	h := a.Handler()

	rec := doGet(t, h, a.token, "/api/spec")
	if rec.Code != http.StatusOK {
		t.Fatalf("spec: got %d", rec.Code)
	}
	var spec struct {
		Fields []*Field `json:"fields"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	if len(spec.Fields) == 0 {
		t.Fatal("spec has no fields")
	}

	req := &Request{Mode: "demo", TablesKind: "builtin", BuiltinType: "agglomerate",
		Options: map[string]any{}, Output: Output{PlotFormat: "png", Spectra: true}}
	rec2 := doPost(t, h, a.token, "/api/validate", req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("validate: got %d", rec2.Code)
	}
	var vr validateResp
	if err := json.Unmarshal(rec2.Body.Bytes(), &vr); err != nil {
		t.Fatalf("decode validate: %v", err)
	}
	if !vr.Ok || vr.Error != "" {
		t.Fatalf("validate not ok: %s", vr.Error)
	}
}

func TestRunDemo(t *testing.T) {
	exe, err := realSolver()
	if err != nil {
		t.Skipf("solver build skipped: %v", err)
	}
	req := &Request{
		Mode:        "demo",
		TablesKind:  "builtin",
		BuiltinType: "agglomerate",
		Options:     map[string]any{"nr0": 1, "nr1": 1, "best": 1},
		Output:      Output{PlotFormat: "png", PlotDPI: 110, Spectra: false},
	}
	a := newTestApp(t)
	a.manager.solverPath = exe
	h := a.Handler()

	rec := doPost(t, h, a.token, "/api/jobs", req)
	if rec.Code != http.StatusOK {
		t.Fatalf("jobs: got %d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("no job id; body=%s", rec.Body.String())
	}
	j := a.manager.get(id)
	if j == nil {
		t.Fatalf("job %s missing", id)
	}

	t.Logf("waiting for job %s (ptr=%p)", j.ID, j)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		j.mu.Lock()
		st := j.Status
		done := st == statusDone || st == statusFailed || st == statusCancelled
		j.mu.Unlock()
		if done {
			t.Logf("job finished with status %s", st)
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	j.mu.Lock()
	st, errText := j.Status, j.ErrText
	files := append([]fileItem(nil), j.Files...)
	logs := append([]logLine(nil), j.Logs...)
	j.mu.Unlock()
	if st != statusDone {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("demo job status=%s err=%s logs=%d\n", st, errText, len(logs)))
		n := len(logs)
		if n > 40 {
			n = 40
		}
		for _, l := range logs[max(0, len(logs)-n):] {
			sb.WriteString("[" + l.Stream + "] " + l.Text + "\n")
		}
		t.Fatalf("%s", sb.String())
	}
	foundJSON, foundPNG := false, false
	for _, f := range files {
		if f.Name == "result.json" {
			foundJSON = true
		}
		if filepath.Ext(f.Name) == ".png" {
			foundPNG = true
		}
	}
	if !foundJSON {
		t.Error("result.json not produced")
	}
	if !foundPNG {
		t.Error("no PNG plot produced")
	}
	png := filepath.Join(j.Dir, "result.png")
	data, err := os.ReadFile(png)
	if err != nil || len(data) < 8 || string(data[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Errorf("result.png is not a valid PNG (err=%v)", err)
	}
}

func TestPathTraversal(t *testing.T) {
	a := newTestApp(t)
	h := a.Handler()
	dir := filepath.Join(a.tempRoot, "job-x")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(a.tempRoot, "secret.txt")
	if err := os.WriteFile(secret, []byte("s3cr3t"), 0644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet,
		"/api/jobs/job-x/files/..%2Fsecret.txt", nil)
	req.Header.Set("X-CSolve-Token", a.token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "s3cr3t") {
		t.Fatal("path traversal leaked a file")
	}
	_ = a.manager
}

func TestJobsField(t *testing.T) {
	f := fieldByKey("jobs")
	if f == nil {
		t.Fatal("jobs field is missing from the form spec")
	}
	if f.Group != "numerics" || f.Type != "int" {
		t.Errorf("jobs field = group %q type %q, want numerics/int", f.Group, f.Type)
	}
	v, ok := defaultValues()["jobs"]
	if !ok {
		t.Fatal("jobs default is missing")
	}
	if _, ok := v.(int); !ok {
		t.Fatalf("jobs default type = %T, want int", v)
	}
	def := v.(int)
	// значение должно отличаться от дефолта (он зависит от числа CPU)
	want := 2
	if def == want {
		want = 3
	}

	req := &Request{
		Mode:        "demo",
		TablesKind:  "builtin",
		BuiltinType: defaultTableType(),
		Options:     map[string]any{"jobs": want},
	}
	argv, a, err := buildArgv(req, t.TempDir())
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if a.Jobs != want {
		t.Errorf("Jobs = %d, want %d", a.Jobs, want)
	}
	found := false
	for _, arg := range argv {
		if arg == "--jobs" {
			found = true
		}
	}
	if !found {
		t.Errorf("argv is missing --jobs: %v", argv)
	}
}
