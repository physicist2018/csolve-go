package solve

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// reportCapture — накопители вывода консоли для документа отчёта.
type reportCapture struct {
	out, warn bytes.Buffer
}

func (r *reportCapture) text() string     { return r.out.String() }
func (r *reportCapture) warnings() string { return filterWarnings(r.warn.String()) }

// reportWriters — потоки консоли: при --report вывод дублируется в накопители.
func reportWriters(a *Args) (io.Writer, io.Writer, *reportCapture) {
	if a.Report == "" {
		return os.Stdout, os.Stderr, nil
	}
	rc := &reportCapture{}
	return io.MultiWriter(os.Stdout, &rc.out),
		io.MultiWriter(os.Stderr, &rc.warn), rc
}

// filterWarnings — оставляет строки предупреждений без хронометража ([time]).
func filterWarnings(s string) string {
	var b strings.Builder
	for _, ln := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "[time]") {
			continue
		}
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// writeReportDoc — сохранение консольного отчёта и (если график включён)
// картинки в документ .docx или .odt (формат по расширению).
func writeReportDoc(a *Args, c *console, rep *reportCapture, prof *profile,
	ch []channel, g1 []float64, nTrueFn func(float64) float64) error {
	path, inDir := resolveOutPath(a.OutDir, a.Report)
	if inDir {
		if err := mkdirP(a.OutDir); err != nil {
			return err
		}
	}

	var img []byte
	if !a.NoPlot {
		po := plotOpts{format: plotFmtPng, dpi: a.PlotDpi, fontSize: a.PlotFontSize,
			backend: plottingInternal, con: c}
		var nTrue []float64
		if nTrueFn != nil {
			nTrue = make([]float64, prof.nr)
			for i := 0; i < prof.nr; i++ {
				nTrue[i] = nTrueFn(prof.rGrid[i])
			}
		}
		if b, ok := internalResultsPNG(&po, prof.rGrid, nTrue, prof.nAvg,
			prof.nAvgRaw, prof.vCum, prof.nBest, prof.nBestCount, ch, g1,
			prof.gFit, prof.mRe, prof.mIm); ok {
			img = b
		}
	}

	src := a.Data
	if src == "" {
		src = "демо-данные"
	}
	title := "csolve-go: результат восстановления"
	meta := fmt.Sprintf("Дата: %s; входные данные: %s",
		time.Now().Format("2006-01-02 15:04:05"), src)

	var err error
	if strings.EqualFold(filepath.Ext(path), ".odt") {
		err = writeOdt(path, title, meta, rep.text(), rep.warnings(), img)
	} else {
		err = writeDocx(path, title, meta, rep.text(), rep.warnings(), img)
	}
	if err != nil {
		return err
	}
	c.printf("report saved: %s\n", path)
	return nil
}

// splitLines — строки без завершающего пустого элемента.
func splitLines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// xmlEscape — экранирование текста для XML.
func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// imageSizeCM — размеры картинки в см при заданной ширине (пропорции).
func imageSizeCM(img []byte, widthCM float64) (w, h float64) {
	cfg, err := png.DecodeConfig(bytes.NewReader(img))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0
	}
	return widthCM, widthCM * float64(cfg.Height) / float64(cfg.Width)
}

// ---------------- DOCX ----------------

func writeDocx(path, title, meta, text, warnings string, img []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create report: %w", err)
	}
	zw := zip.NewWriter(f)

	add := func(name string) (io.Writer, error) { return zw.Create(name) }
	writeStr := func(name, content string) error {
		w, err := add(name)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, content)
		return err
	}

	var body strings.Builder
	body.WriteString(docxPara(title, true, false, 28))
	body.WriteString(docxPara(meta, false, false, 18))
	for _, ln := range splitLines(text) {
		body.WriteString(docxPara(ln, false, true, 16))
	}
	if len(img) > 0 {
		wcm, hcm := imageSizeCM(img, 17.0)
		if wcm > 0 {
			cx := int(wcm / 2.54 * 914400)
			cy := int(hcm / 2.54 * 914400)
			body.WriteString(docxImage(cx, cy))
		}
	}
	if warnings != "" {
		body.WriteString(docxPara("Предупреждения", true, false, 22))
		for _, ln := range splitLines(warnings) {
			body.WriteString(docxPara(ln, false, true, 16))
		}
	}

	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
		` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"` +
		` xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"` +
		` xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"` +
		` xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body>` +
		body.String() +
		`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/>` +
		`<w:pgMar w:top="1134" w:right="1134" w:bottom="1134" w:left="1134"` +
		` w:header="708" w:footer="708" w:gutter="0"/></w:sectPr>` +
		`</w:body></w:document>`

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Default Extension="png" ContentType="image/png"/>` +
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
		`</Types>`

	rootRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
		`</Relationships>`

	if err := writeStr("[Content_Types].xml", contentTypes); err != nil {
		return err
	}
	if err := writeStr("_rels/.rels", rootRels); err != nil {
		return err
	}
	if len(img) > 0 {
		docRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/>` +
			`</Relationships>`
		if err := writeStr("word/_rels/document.xml.rels", docRels); err != nil {
			return err
		}
		w, err := add("word/media/image1.png")
		if err != nil {
			return err
		}
		if _, err := w.Write(img); err != nil {
			return err
		}
	}
	if err := writeStr("word/document.xml", document); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

// docxPara — параграф; sizeHalf — размер в полупунктах, mono — моноширинный.
func docxPara(text string, bold, mono bool, sizeHalf int) string {
	var rpr strings.Builder
	rpr.WriteString("<w:rPr>")
	if mono {
		rpr.WriteString(`<w:rFonts w:ascii="Courier New" w:hAnsi="Courier New" w:cs="Courier New"/>`)
	}
	if bold {
		rpr.WriteString("<w:b/>")
	}
	fmt.Fprintf(&rpr, `<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, sizeHalf, sizeHalf)
	rpr.WriteString("</w:rPr>")
	return `<w:p><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/>` +
		`</w:pPr><w:r>` + rpr.String() + `<w:t xml:space="preserve">` +
		xmlEscape(text) + `</w:t></w:r></w:p>`
}

// docxImage — параграф с inline-картинкой размером cx × cy (EMU).
func docxImage(cx, cy int) string {
	return fmt.Sprintf(`<w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:drawing>`+
		`<wp:inline distT="0" distB="0" distL="0" distR="0">`+
		`<wp:extent cx="%d" cy="%d"/><wp:docPr id="1" name="Picture 1"/>`+
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
		`<pic:pic><pic:nvPicPr><pic:cNvPr id="1" name="image1.png"/><pic:cNvPicPr/></pic:nvPicPr>`+
		`<pic:blipFill><a:blip r:embed="rId1"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic>`+
		`</a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`,
		cx, cy, cx, cy)
}

// ---------------- ODT ----------------

const odtMime = "application/vnd.oasis.opendocument.text"

func writeOdt(path, title, meta, text, warnings string, img []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create report: %w", err)
	}
	zw := zip.NewWriter(f)

	// mimetype — первым, без сжатия (требование ODF).
	hdr := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(mw, odtMime); err != nil {
		return err
	}

	var body strings.Builder
	body.WriteString(odtPara(title, "ReportTitle"))
	body.WriteString(odtPara(meta, "ReportMeta"))
	for _, ln := range splitLines(text) {
		body.WriteString(odtPara(ln, "Preformatted_20_Text"))
	}
	if len(img) > 0 {
		wcm, hcm := imageSizeCM(img, 17.0)
		if wcm > 0 {
			body.WriteString(odtImage(wcm, hcm))
		}
	}
	if warnings != "" {
		body.WriteString(odtPara("Предупреждения", "ReportHeading"))
		for _, ln := range splitLines(warnings) {
			body.WriteString(odtPara(ln, "Preformatted_20_Text"))
		}
	}

	content := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<office:document-content` +
		` xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"` +
		` xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"` +
		` xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0"` +
		` xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0"` +
		` xmlns:xlink="http://www.w3.org/1999/xlink"` +
		` office:version="1.2"><office:body><office:text>` +
		body.String() + `</office:text></office:body></office:document-content>`

	styles := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<office:document-styles` +
		` xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"` +
		` xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0"` +
		` xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"` +
		` xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0"` +
		` office:version="1.2"><office:styles>` +
		`<style:style style:name="Standard" style:family="paragraph"/>` +
		`<style:style style:name="ReportTitle" style:family="paragraph" style:parent-style-name="Standard">` +
		`<style:text-properties fo:font-size="14pt" fo:font-weight="bold"/></style:style>` +
		`<style:style style:name="ReportMeta" style:family="paragraph" style:parent-style-name="Standard">` +
		`<style:text-properties fo:font-size="9pt"/></style:style>` +
		`<style:style style:name="ReportHeading" style:family="paragraph" style:parent-style-name="Standard">` +
		`<style:text-properties fo:font-size="11pt" fo:font-weight="bold"/></style:style>` +
		`<style:style style:name="Preformatted_20_Text" style:family="paragraph" style:parent-style-name="Standard">` +
		`<style:text-properties fo:font-family="monospace" fo:font-size="8pt"/></style:style>` +
		`</office:styles></office:document-styles>`

	manifest := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"` +
		` manifest:version="1.2">` +
		`<manifest:file-entry manifest:full-path="/" manifest:media-type="` + odtMime + `"/>` +
		`<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>` +
		`<manifest:file-entry manifest:full-path="styles.xml" manifest:media-type="text/xml"/>`
	if len(img) > 0 {
		manifest += `<manifest:file-entry manifest:full-path="Pictures/image1.png" manifest:media-type="image/png"/>`
	}
	manifest += `</manifest:manifest>`

	if err := zipWriteStr(zw, "content.xml", content); err != nil {
		return err
	}
	if err := zipWriteStr(zw, "styles.xml", styles); err != nil {
		return err
	}
	if err := zipWriteStr(zw, "META-INF/manifest.xml", manifest); err != nil {
		return err
	}
	if len(img) > 0 {
		w, err := zw.Create("Pictures/image1.png")
		if err != nil {
			return err
		}
		if _, err := w.Write(img); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

func zipWriteStr(zw *zip.Writer, name, content string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, content)
	return err
}

// odtPara — параграф ODT; пробелы/табы сохраняются через text:s/text:tab.
func odtPara(text, style string) string {
	return `<text:p text:style-name="` + style + `">` + odtText(text) +
		`</text:p>`
}

// odtText — экранирование с сохранением пробелов и табуляций.
func odtText(s string) string {
	s = xmlEscape(s)
	s = strings.ReplaceAll(s, "\t", "<text:tab/>")
	s = strings.ReplaceAll(s, " ", "<text:s/>")
	return s
}

// odtImage — параграф с картинкой заданного размера (см).
func odtImage(wcm, hcm float64) string {
	return fmt.Sprintf(`<text:p><draw:frame draw:name="image1" text:anchor-type="as-char"`+
		` svg:width="%.2fcm" svg:height="%.2fcm">`+
		`<draw:image xlink:href="Pictures/image1.png" xlink:type="simple"`+
		` xlink:show="embed" xlink:actuate="onLoad"/></draw:frame></text:p>`,
		wcm, hcm)
}
