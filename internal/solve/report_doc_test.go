package solve

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"io"
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
	imgs := [][]byte{testPNG(t, 8, 4), testPNG(t, 6, 4)}
	if err := writeDocx(path, "Title", "meta", text, warn, imgs); err != nil {
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
}

func TestWriteOdt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.odt")
	text := "line one\n  indented text"
	imgs := [][]byte{testPNG(t, 8, 4), testPNG(t, 6, 4)}
	if err := writeOdt(path, "Title", "meta", text, "", imgs); err != nil {
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
