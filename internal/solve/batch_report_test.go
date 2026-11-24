package solve

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBatchReportDoc(t *testing.T) {
	dir := t.TempDir()
	a := &Args{Report: "report.docx", NoPlot: true}
	c := newConsole(io.Discard, io.Discard, false)

	b := &batchInput{alt: []float64{100, 200}, nAlt: 2, nch: 1}
	met := &metrics{
		nAlt: 2,
		err:  []float64{0.1, 0.2},
		V:    []float64{1.5, 2.5},
		Vc:   []float64{0.5, 0.6},
		Vf:   []float64{1.0, 1.9},
		rmf:  []float64{0.1, 0.2},
		rmc:  []float64{0.8, 0.9},
		rmo:  []float64{0.3, 0.4},
		reff: []float64{0.2, 0.3},
		Nc:   []float64{10, 20},
		sgf:  []float64{1.5, 1.6},
		sgc:  []float64{1.3, 1.4},
		sgo:  []float64{2.5, 2.6},
		mRe:  []float64{1.5, 1.51},
		mIm:  []float64{0.01, 0.02},
	}
	st := &batchSetup{b: b}
	rep := &reportCapture{}
	rep.out.WriteString("=== log ===\nscan altitude 100 m\n")

	if err := writeBatchReportDoc(a, c, rep, dir, st, met, nil, 0, nil); err != nil {
		t.Fatalf("writeBatchReportDoc: %v", err)
	}

	z, read := readZip(t, filepath.Join(dir, "report.docx"))
	defer z.Close()
	doc := read("word/document.xml")
	if err := xmlValid(doc); err != nil {
		t.Fatalf("invalid document.xml: %v", err)
	}
	for _, want := range []string{
		"Микрофизика и оптика по высотам",
		"Параметры логнормальных мод",
		"scan altitude 100 m",
		"<w:tbl>",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("document must contain %q", want)
		}
	}
	if strings.Contains(doc, "Линейная деполяризация") {
		t.Errorf("depolarization table must be omitted without data")
	}
}

func TestParseArgsBatchReport(t *testing.T) {
	if _, err := ParseArgs([]string{"--batch", "m.txt", "--report",
		"report.docx"}); err != nil {
		t.Fatalf("--report must be allowed in batch mode: %v", err)
	}
	if _, err := ParseArgs([]string{"--batch", "m.txt", "--report",
		"report.odt"}); err != nil {
		t.Fatalf("--report .odt must be allowed in batch mode: %v", err)
	}
	if _, err := ParseArgs([]string{"--batch", "m.txt", "--report",
		"report.pdf"}); err == nil {
		t.Errorf("--report must reject unsupported extensions")
	}
}
