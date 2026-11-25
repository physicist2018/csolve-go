package solve

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testPNG — маленькая валидная PNG-картинка для тестов отчёта.
func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 0xff, A: 0xff})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func readZip(t *testing.T, path string) (*zip.ReadCloser, func(string) string) {
	t.Helper()
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %s: %v", path, err)
	}
	read := func(name string) string {
		for _, f := range z.File {
			if f.Name != name {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			defer rc.Close()
			var b bytes.Buffer
			if _, err := b.ReadFrom(rc); err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			return b.String()
		}
		return ""
	}
	return z, read
}

func xmlValid(s string) error {
	dec := xml.NewDecoder(strings.NewReader(s))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func TestWriteDocx(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.docx")
	text := "=== Channel agreement ===\n  beta(355 nm)  g = 1 < 2 & 3\n"
	warn := "[warning] something happened"
	imgs := []docImage{
		{Data: testPNG(t, 8, 4)},
		{Data: testPNG(t, 6, 4)},
	}
	sections := []docImageSection{{Images: imgs}}
	params := []string{"Повторить: csolve --data x.json", "--data x.json"}
	if err := writeDocx(path, "Title", "meta", params, text, nil, sections, warn); err != nil {
		t.Fatalf("writeDocx: %v", err)
	}

	z, read := readZip(t, path)
	defer z.Close()

	for _, part := range []string{"[Content_Types].xml", "_rels/.rels",
		"word/document.xml", "word/_rels/document.xml.rels"} {
		body := read(part)
		if body == "" {
			t.Errorf("missing part %q", part)
			continue
		}
		if err := xmlValid(body); err != nil {
			t.Errorf("invalid XML in %s: %v", part, err)
		}
	}
	if read("word/media/image1.png") == "" ||
		read("word/media/image2.png") == "" {
		t.Errorf("missing embedded image")
	}
	doc := read("word/document.xml")
	if !strings.Contains(doc, "w:drawing") ||
		!strings.Contains(doc, `r:embed="rId2"`) {
		t.Errorf("document.xml must embed both images")
	}
	if !strings.Contains(doc, "g = 1 &lt; 2 &amp; 3") {
		t.Errorf("document text must be XML-escaped:\n%s", doc)
	}
	if !strings.Contains(doc, "Предупреждения") {
		t.Errorf("warnings section is missing")
	}
	if !strings.Contains(doc, "Применённые опции") ||
		!strings.Contains(doc, "--data x.json") {
		t.Errorf("applied options section is missing")
	}
	if strings.Index(doc, "Применённые опции") >
		strings.Index(doc, "Channel agreement") {
		t.Errorf("applied options must be at the very beginning")
	}
}

func TestWriteOdt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.odt")
	text := "line one\n  indented text"
	imgs := []docImage{
		{Data: testPNG(t, 8, 4)},
		{Data: testPNG(t, 6, 4)},
	}
	sections := []docImageSection{{Images: imgs}}
	if err := writeOdt(path, "Title", "meta", nil, text, nil, sections, ""); err != nil {
		t.Fatalf("writeOdt: %v", err)
	}

	z, read := readZip(t, path)
	defer z.Close()

	if z.File[0].Name != "mimetype" {
		t.Fatalf("mimetype must be the first entry, got %q", z.File[0].Name)
	}
	if z.File[0].Method != zip.Store {
		t.Errorf("mimetype must be stored (uncompressed)")
	}
	if got := read("mimetype"); got != odtMime {
		t.Errorf("mimetype = %q, want %q", got, odtMime)
	}
	for _, part := range []string{"content.xml", "styles.xml",
		"META-INF/manifest.xml"} {
		if err := xmlValid(read(part)); err != nil {
			t.Errorf("invalid XML in %s: %v", part, err)
		}
	}
	content := read("content.xml")
	if !strings.Contains(content, "draw:image") ||
		!strings.Contains(content, "Pictures/image1.png") ||
		!strings.Contains(content, "Pictures/image2.png") {
		t.Errorf("content.xml must reference both images")
	}
	if !strings.Contains(content, "indented<text:s/>text") {
		t.Errorf("spaces must be preserved via text:s:\n%s", content)
	}
}

func TestWriteDocxTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.docx")
	tables := []docTable{{
		Title:   "Microphysics",
		Headers: []string{"H, m", "V", "N & reff"},
		Rows: [][]string{
			{"100", "1.5", "nan"},
			{"200", "2.5", "3"},
		},
	}}
	sections := []docImageSection{{
		Title:  "Plots",
		Images: []docImage{{Data: testPNG(t, 8, 4), Caption: "caption"}},
	}}
	if err := writeDocx(path, "Title", "meta", nil, "log", tables, sections, ""); err != nil {
		t.Fatalf("writeDocx: %v", err)
	}
	z, read := readZip(t, path)
	defer z.Close()
	doc := read("word/document.xml")
	if err := xmlValid(doc); err != nil {
		t.Fatalf("invalid document.xml: %v", err)
	}
	for _, want := range []string{"<w:tbl>", "H, m", "N &amp; reff",
		"caption", "Plots", "<w:tblHeader/>",
		`w:br w:type="page"`} {
		if !strings.Contains(doc, want) {
			t.Errorf("document must contain %q", want)
		}
	}
	if strings.Index(doc, "<w:tbl>") > strings.Index(doc, "Microphysics") {
		t.Errorf("table caption must follow the table")
	}
	if got := strings.Count(doc, docxPageBreak()); got != 1 {
		t.Errorf("page breaks = %d, want 1 (last object does not break)", got)
	}
}

func TestWriteOdtTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.odt")
	tables := []docTable{{
		Title:   "Microphysics",
		Headers: []string{"H, m", "V"},
		Rows:    [][]string{{"100", "1.5"}},
	}}
	sections := []docImageSection{{
		Title:  "Plots",
		Images: []docImage{{Data: testPNG(t, 8, 4), Caption: "caption"}},
	}}
	if err := writeOdt(path, "Title", "meta", nil, "log", tables, sections, ""); err != nil {
		t.Fatalf("writeOdt: %v", err)
	}
	z, read := readZip(t, path)
	defer z.Close()
	content := read("content.xml")
	if err := xmlValid(content); err != nil {
		t.Fatalf("invalid content.xml: %v", err)
	}
	for _, want := range []string{"table:table", "table:table-header-rows",
		"Microphysics", "caption", "H,<text:s/>m",
		`text:style-name="PageBreak"`} {
		if !strings.Contains(content, want) {
			t.Errorf("content must contain %q", want)
		}
	}
	if strings.Index(content, "<table:table") > strings.Index(content, "Microphysics") {
		t.Errorf("table caption must follow the table")
	}
	if got := strings.Count(content, odtPageBreak()); got != 1 {
		t.Errorf("page breaks = %d, want 1 (last object does not break)", got)
	}
}

func TestReportPNG(t *testing.T) {
	c := newConsole(io.Discard, io.Discard, false)
	dir := t.TempDir()
	existingPNG := testPNG(t, 4, 3)
	existing := filepath.Join(dir, "plot.png")
	if err := os.WriteFile(existing, existingPNG, 0644); err != nil {
		t.Fatalf("write existing png: %v", err)
	}

	t.Run("gnuplot png reuses file", func(t *testing.T) {
		a := &Args{Plotting: plottingGnuplot, PlotFormat: plotFmtPng}
		got, ok := reportPNG(a, c, existing,
			func(*plotOpts) ([]byte, bool) {
				t.Fatal("internal renderer must not be used")
				return nil, false
			},
			func(*plotOpts, string) bool {
				t.Fatal("gnuplot render must not be used")
				return false
			})
		if !ok || !bytes.Equal(got, existingPNG) {
			t.Fatalf("reportPNG must return the existing PNG")
		}
	})

	t.Run("gnuplot pdf renders temp png", func(t *testing.T) {
		a := &Args{Plotting: plottingGnuplot, PlotFormat: plotFmtPdf}
		tmpPNG := testPNG(t, 5, 3)
		got, ok := reportPNG(a, c, "",
			func(*plotOpts) ([]byte, bool) {
				t.Fatal("internal renderer must not be used")
				return nil, false
			},
			func(po *plotOpts, path string) bool {
				if po.format != plotFmtPng || po.backend != plottingGnuplot {
					t.Errorf("unexpected temp plot opts: %+v", po)
				}
				if !po.quiet {
					t.Errorf("temporary report plot must be quiet")
				}
				return os.WriteFile(path, tmpPNG, 0644) == nil
			})
		if !ok || !bytes.Equal(got, tmpPNG) {
			t.Fatalf("reportPNG must return the temporary PNG")
		}
	})

	t.Run("internal uses in-memory renderer", func(t *testing.T) {
		a := &Args{Plotting: plottingInternal, PlotFormat: plotFmtPdf}
		inMem := testPNG(t, 6, 3)
		got, ok := reportPNG(a, c, existing,
			func(po *plotOpts) ([]byte, bool) {
				if po.backend != plottingInternal {
					t.Errorf("unexpected backend: %q", po.backend)
				}
				return inMem, true
			},
			func(*plotOpts, string) bool {
				t.Fatal("gnuplot render must not be used")
				return false
			})
		if !ok || !bytes.Equal(got, inMem) {
			t.Fatalf("reportPNG must return internal bytes")
		}
	})
}

func TestFilterWarnings(t *testing.T) {
	in := "[warning] keep me\n[time] scan 0.1 s\nother\n"
	got := filterWarnings(in)
	if strings.Contains(got, "[time]") {
		t.Errorf("timing lines must be dropped: %q", got)
	}
	if !strings.Contains(got, "[warning] keep me") ||
		!strings.Contains(got, "other") {
		t.Errorf("warnings must be kept: %q", got)
	}
}

func TestImageSizeCM(t *testing.T) {
	img := testPNG(t, 10, 5)
	w, h := imageSizeCM(img, 17)
	if w != 17 {
		t.Errorf("width = %g, want 17", w)
	}
	if h < 8.49 || h > 8.51 {
		t.Errorf("height = %g, want 8.5", h)
	}
}

func TestAppliedOptionLines(t *testing.T) {
	a, err := ParseArgs([]string{"--data", "in.json", "--type", "agglomerate",
		"--gamma-min", "-7", "--report", "report.docx"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	got := appliedOptionLines(a)
	want := []string{
		"Повторить: csolve --data in.json --type agglomerate " +
			"--gamma-min -7 --report report.docx",
		"",
		"--data in.json",
		"--type agglomerate",
		"--gamma-min -7",
		"--report report.docx",
	}
	if len(got) != len(want) {
		t.Fatalf("lines = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("lines[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if got := appliedOptionLines(&Args{}); got != nil {
		t.Errorf("no argv must give no lines: %#v", got)
	}
	if got := appliedOptionLines(nil); got != nil {
		t.Errorf("nil args must give no lines: %#v", got)
	}

	if q := quoteArg("a b"); q != "'a b'" {
		t.Errorf("quoteArg = %q", q)
	}
}
