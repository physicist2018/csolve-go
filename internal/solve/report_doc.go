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

// quoteArg — аргумент в виде, пригодном для копирования в оболочку.
func quoteArg(s string) string {
	if s == "" || strings.ContainsAny(s, " \t\n\"'\\$`") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

// joinArgs — аргументы через пробел с квотированием.
func joinArgs(args []string) string {
	q := make([]string, len(args))
	for i, s := range args {
		q[i] = quoteArg(s)
	}
	return strings.Join(q, " ")
}

// appliedOptionLines — применённые опции для повторения расчёта: строка с
// полной командой и по одной опции в строке. Пусто, если исходные аргументы
// неизвестны (Args собраны программно, например в тестах).
func appliedOptionLines(a *Args) []string {
	if a == nil || len(a.Argv) == 0 {
		return nil
	}
	lines := []string{"Повторить: csolve " + joinArgs(a.Argv), ""}
	cur := make([]string, 0, 4)
	flush := func() {
		if len(cur) == 0 {
			return
		}
		lines = append(lines, joinArgs(cur))
		cur = nil
	}
	for _, tok := range a.Argv {
		// новая опция — токен вида --flag (или -h); значения, включая
		// отрицательные числа вида -7, остаются в текущей опции
		if len(cur) > 0 && (strings.HasPrefix(tok, "--") || tok == "-h") {
			flush()
		}
		cur = append(cur, tok)
	}
	flush()
	return lines
}

// docTable — таблица отчёта: заголовок, шапка и строки.
type docTable struct {
	Title   string
	Headers []string
	Rows    [][]string
}

// docImage — PNG-картинка и её подпись.
type docImage struct {
	Data    []byte
	Caption string
}

// docImageSection — группа картинок с общим заголовком.
type docImageSection struct {
	Title  string
	Images []docImage
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

	var figures []docImage
	if !a.NoPlot {
		var nTrue []float64
		if nTrueFn != nil {
			nTrue = make([]float64, prof.nr)
			for i := 0; i < prof.nr; i++ {
				nTrue[i] = nTrueFn(prof.rGrid[i])
			}
		}
		plotName := a.Out
		if plotName == "" {
			plotName = "result." + plotFormatExt(a.PlotFormat)
		}
		plotPath, _ := resolveOutPath(a.OutDir, plotName)
		if bts, ok := reportPNG(a, c, plotPath,
			func(po *plotOpts) ([]byte, bool) {
				return internalResultsPNG(po, prof.rGrid, nTrue, prof.nAvg,
					prof.nAvgRaw, prof.nLow, prof.nHigh, prof.vCum,
					prof.nBest, prof.nBestCount, ch, g1, prof.gFit,
					prof.mRe, prof.mIm)
			},
			func(po *plotOpts, path string) bool {
				return plotResults(po, path, prof.rGrid, nTrue, prof.nAvg,
					prof.nAvgRaw, prof.nLow, prof.nHigh, prof.vCum,
					prof.nBest, prof.nBestCount, ch, g1, prof.gFit,
					prof.mRe, prof.mIm)
			}); ok {
			figures = append(figures, docImage{Data: bts,
				Caption: "Результат восстановления: dV/dln r, V(r) и " +
					"сравнение коэффициентов"})
		}
		for _, np := range prof.spectra.namedPanels() {
			pn := np.panel
			existing := spectrumPlotPath(plotPath, np.suffix, a.PlotFormat)
			if bts, ok := reportPNG(a, c, existing,
				func(po *plotOpts) ([]byte, bool) {
					return internalSpectrumPNG(po, pn)
				},
				func(po *plotOpts, path string) bool {
					return plotSpectrum(po, path, pn)
				}); ok {
				figures = append(figures, docImage{Data: bts,
					Caption: pn.title})
			}
		}
	}

	src := a.Data
	if src == "" {
		src = "демо-данные"
	}
	title := "csolve-go: результат восстановления"
	meta := fmt.Sprintf("Дата: %s; входные данные: %s",
		time.Now().Format("2006-01-02 15:04:05"), src)
	params := appliedOptionLines(a)

	var tables []docTable
	if prof.sfracMix {
		tables = append(tables, docTable{
			Title:   "Доля сфер (смесь sphere + " + mixtureNonType(a) + ")",
			Headers: []string{"величина", "значение"},
			Rows: [][]string{
				{"доля сфер по концентрации (среднее)", reportNum(prof.sfracNum)},
				{"доля сфер по концентрации (диапазон)", reportNum(prof.sfracNumLow) +
					" … " + reportNum(prof.sfracNumHigh)},
				{"доля сфер по объёму (среднее)", reportNum(prof.sfracVol)},
				{"доля сфер по объёму (диапазон)", reportNum(prof.sfracVolLow) +
					" … " + reportNum(prof.sfracVolHigh)},
			},
		})
	}

	var err error
	sections := []docImageSection{{Images: figures}}
	if strings.EqualFold(filepath.Ext(path), ".odt") {
		err = writeOdt(path, title, meta, params, rep.text(), tables, sections,
			rep.warnings())
	} else {
		err = writeDocx(path, title, meta, params, rep.text(), tables, sections,
			rep.warnings())
	}
	if err != nil {
		return err
	}
	c.printf("report saved: %s\n", path)
	return nil
}

// reportPlotOpts — параметры встроенного рендера картинок отчёта.
func reportPlotOpts(a *Args, c *console) plotOpts {
	return plotOpts{format: plotFmtPng, dpi: internalPNGDpi,
		fontSize: a.PlotFontSize, backend: plottingInternal, con: c,
		bestLimit: a.PlotBestLimit}
}

// reportPNG — PNG-байты графика выбранным backend'ом. Для internal используется
// рендер в память; для gnuplot — готовый PNG-файл (при --plot-format png) или
// перерисовка того же графика в PNG во временный файл.
func reportPNG(a *Args, c *console, existing string,
	internal func(*plotOpts) ([]byte, bool),
	renderFile func(*plotOpts, string) bool) ([]byte, bool) {
	if a.Plotting != plottingGnuplot {
		po := reportPlotOpts(a, c)
		return internal(&po)
	}
	if a.PlotFormat == plotFmtPng && existing != "" {
		if b, err := os.ReadFile(existing); err == nil && len(b) > 0 {
			return b, true
		}
	}
	dir, err := os.MkdirTemp("", "csolve_report_")
	if err != nil {
		c.warnf("[warning] failed to create temporary directory for " +
			"report plots\n")
		return nil, false
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "plot.png")
	po := plotOpts{format: plotFmtPng, dpi: a.PlotDpi,
		fontSize: a.PlotFontSize, backend: plottingGnuplot, con: c,
		quiet: true, bestLimit: a.PlotBestLimit}
	if !renderFile(&po, path) {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return b, true
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

func writeDocx(path, title, meta string, params []string, text string,
	tables []docTable, sections []docImageSection, warnings string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create report: %w", err)
	}
	zw := zip.NewWriter(f)
	imgs := sectionImages(sections)

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
	if len(params) > 0 {
		body.WriteString(docxPara("Применённые опции", true, false, 22))
		for _, ln := range params {
			body.WriteString(docxPara(ln, false, true, 16))
		}
	}
	for _, ln := range splitLines(text) {
		body.WriteString(docxPara(ln, false, true, 16))
	}
	for _, t := range tables {
		body.WriteString(docxTable(t))
	}
	imgIdx := 0
	for _, s := range sections {
		if s.Title != "" {
			body.WriteString(docxPara(s.Title, true, false, 22))
		}
		for _, img := range s.Images {
			imgIdx++
			body.WriteString(docxFigure(img, imgIdx))
		}
	}
	if warnings != "" {
		body.WriteString(docxPara("Предупреждения", true, false, 22))
		for _, ln := range splitLines(warnings) {
			body.WriteString(docxPara(ln, false, true, 16))
		}
	}

	bodyText := body.String()
	if warnings == "" {
		bodyText = strings.TrimSuffix(bodyText, docxPageBreak())
	}

	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
		` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"` +
		` xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"` +
		` xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"` +
		` xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body>` +
		bodyText +
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
	if len(imgs) > 0 {
		var rels strings.Builder
		rels.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
		for i := range imgs {
			fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image%d.png"/>`, i+1, i+1)
		}
		rels.WriteString(`</Relationships>`)
		if err := writeStr("word/_rels/document.xml.rels", rels.String()); err != nil {
			return err
		}
		for i, img := range imgs {
			w, err := add(fmt.Sprintf("word/media/image%d.png", i+1))
			if err != nil {
				return err
			}
			if _, err := w.Write(img.Data); err != nil {
				return err
			}
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

// docxImage — параграф с inline-картинкой размером cx × cy (EMU); idx —
// номер изображения (1..N), связывается с rId<idx>.
func docxImage(cx, cy, idx int) string {
	return fmt.Sprintf(`<w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:drawing>`+
		`<wp:inline distT="0" distB="0" distL="0" distR="0">`+
		`<wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="Picture %d"/>`+
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
		`<pic:pic><pic:nvPicPr><pic:cNvPr id="%d" name="image%d.png"/><pic:cNvPicPr/></pic:nvPicPr>`+
		`<pic:blipFill><a:blip r:embed="rId%d"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic>`+
		`</a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`,
		cx, cy, idx, idx, idx, idx, idx, cx, cy)
}

// sectionImages — плоский список картинок из секций (для media/rels).
func sectionImages(sections []docImageSection) []docImage {
	var out []docImage
	for _, s := range sections {
		out = append(out, s.Images...)
	}
	return out
}

// docxFigure — картинка, подпись на новой строке и разрыв страницы.
func docxFigure(img docImage, idx int) string {
	wcm, hcm := imageSizeCM(img.Data, 17.0)
	if wcm <= 0 {
		return ""
	}
	var b strings.Builder
	cx := int(wcm / 2.54 * 914400)
	cy := int(hcm / 2.54 * 914400)
	b.WriteString(docxImage(cx, cy, idx))
	if img.Caption != "" {
		b.WriteString(docxCaption(img.Caption))
	}
	b.WriteString(docxPageBreak())
	return b.String()
}

// docxPageBreak — жёсткий разрыв страницы.
func docxPageBreak() string {
	return `<w:p><w:r><w:br w:type="page"/></w:r></w:p>`
}

// docxCaption — подпись под/над картинкой (курсив, 8 pt).
func docxCaption(text string) string {
	return `<w:p><w:pPr><w:spacing w:before="0" w:after="0" w:line="240"` +
		` w:lineRule="auto"/></w:pPr><w:r><w:rPr><w:i/><w:sz w:val="16"/>` +
		`<w:szCs w:val="16"/></w:rPr><w:t xml:space="preserve">` +
		xmlEscape(text) + `</w:t></w:r></w:p>`
}

// docxTable — таблица, подпись на новой строке и разрыв страницы.
func docxTable(t docTable) string {
	if len(t.Headers) == 0 {
		return ""
	}
	var b strings.Builder
	const totalW = 9638 // ширина текста A4 в twips при полях 2 см
	colW := totalW / len(t.Headers)
	fmt.Fprintf(&b, `<w:tbl><w:tblPr><w:tblW w:w="%d" w:type="dxa"/>`,
		totalW)
	b.WriteString(`<w:tblBorders>`)
	for _, side := range []string{"top", "left", "bottom", "right",
		"insideH", "insideV"} {
		fmt.Fprintf(&b, `<w:%s w:val="single" w:sz="4" w:space="0"`+
			` w:color="000000"/>`, side)
	}
	b.WriteString(`</w:tblBorders><w:tblLayout w:type="fixed"/></w:tblPr>` +
		`<w:tblGrid>`)
	for range t.Headers {
		fmt.Fprintf(&b, `<w:gridCol w:w="%d"/>`, colW)
	}
	b.WriteString(`</w:tblGrid><w:tr><w:trPr><w:tblHeader/>` +
		`</w:trPr>`)
	for _, h := range t.Headers {
		b.WriteString(docxCell(h, true, colW))
	}
	b.WriteString(`</w:tr>`)
	for _, row := range t.Rows {
		b.WriteString(`<w:tr>`)
		for i := range t.Headers {
			v := ""
			if i < len(row) {
				v = row[i]
			}
			b.WriteString(docxCell(v, false, colW))
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl>`)
	if t.Title != "" {
		b.WriteString(docxCaption(t.Title))
	}
	b.WriteString(docxPageBreak())
	return b.String()
}

// docxCell — ячейка таблицы (Courier 7 pt), width в twips.
func docxCell(text string, bold bool, width int) string {
	var rpr strings.Builder
	rpr.WriteString(`<w:rPr><w:rFonts w:ascii="Courier New"` +
		` w:hAnsi="Courier New" w:cs="Courier New"/>`)
	if bold {
		rpr.WriteString(`<w:b/>`)
	}
	rpr.WriteString(`<w:sz w:val="14"/><w:szCs w:val="14"/></w:rPr>`)
	return fmt.Sprintf(`<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>`+
		`</w:tcPr>`, width) +
		`<w:p><w:pPr><w:spacing w:before="0" w:after="0" w:line="200"` +
		` w:lineRule="auto"/></w:pPr><w:r>` + rpr.String() +
		`<w:t xml:space="preserve">` + xmlEscape(text) +
		`</w:t></w:r></w:p></w:tc>`
}

// ---------------- ODT ----------------

const odtMime = "application/vnd.oasis.opendocument.text"

func writeOdt(path, title, meta string, params []string, text string,
	tables []docTable, sections []docImageSection, warnings string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create report: %w", err)
	}
	zw := zip.NewWriter(f)
	imgs := sectionImages(sections)

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
	if len(params) > 0 {
		body.WriteString(odtPara("Применённые опции", "ReportHeading"))
		for _, ln := range params {
			body.WriteString(odtPara(ln, "Preformatted_20_Text"))
		}
	}
	for _, ln := range splitLines(text) {
		body.WriteString(odtPara(ln, "Preformatted_20_Text"))
	}
	for _, t := range tables {
		body.WriteString(odtTable(t))
	}
	imgIdx := 0
	for _, s := range sections {
		if s.Title != "" {
			body.WriteString(odtPara(s.Title, "ReportHeading"))
		}
		for _, img := range s.Images {
			imgIdx++
			wcm, hcm := imageSizeCM(img.Data, 17.0)
			if wcm <= 0 {
				continue
			}
			body.WriteString(odtImage(wcm, hcm, imgIdx))
			if img.Caption != "" {
				body.WriteString(odtPara(img.Caption, "ReportCaption"))
			}
			body.WriteString(odtPageBreak())
		}
	}
	if warnings != "" {
		body.WriteString(odtPara("Предупреждения", "ReportHeading"))
		for _, ln := range splitLines(warnings) {
			body.WriteString(odtPara(ln, "Preformatted_20_Text"))
		}
	}

	bodyText := body.String()
	if warnings == "" {
		bodyText = strings.TrimSuffix(bodyText, odtPageBreak())
	}

	content := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<office:document-content` +
		` xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"` +
		` xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"` +
		` xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0"` +
		` xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"` +
		` xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0"` +
		` xmlns:xlink="http://www.w3.org/1999/xlink"` +
		` office:version="1.2"><office:body><office:text>` +
		bodyText + `</office:text></office:body></office:document-content>`

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
		`<style:style style:name="ReportCaption" style:family="paragraph" style:parent-style-name="Standard">` +
		`<style:text-properties fo:font-size="8pt" fo:font-style="italic"/></style:style>` +
		`<style:style style:name="PageBreak" style:family="paragraph" style:parent-style-name="Standard">` +
		`<style:paragraph-properties fo:break-before="page"/></style:style>` +
		`<style:style style:name="ReportTable" style:family="table">` +
		`<style:table-properties style:width="17cm"/></style:style>` +
		`<style:style style:name="TableCells" style:family="table-cell">` +
		`<style:table-cell-properties fo:padding="0.02in" fo:border="0.5pt solid #000000"/></style:style>` +
		`<style:style style:name="TableHeader" style:family="paragraph" style:parent-style-name="Standard">` +
		`<style:text-properties fo:font-family="monospace" fo:font-size="7pt" fo:font-weight="bold"/></style:style>` +
		`<style:style style:name="TableText" style:family="paragraph" style:parent-style-name="Standard">` +
		`<style:text-properties fo:font-family="monospace" fo:font-size="7pt"/></style:style>` +
		`<style:style style:name="Preformatted_20_Text" style:family="paragraph" style:parent-style-name="Standard">` +
		`<style:text-properties fo:font-family="monospace" fo:font-size="8pt"/></style:style>` +
		`</office:styles></office:document-styles>`

	manifest := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"` +
		` manifest:version="1.2">` +
		`<manifest:file-entry manifest:full-path="/" manifest:media-type="` + odtMime + `"/>` +
		`<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>` +
		`<manifest:file-entry manifest:full-path="styles.xml" manifest:media-type="text/xml"/>`
	for i := range imgs {
		manifest += fmt.Sprintf(`<manifest:file-entry manifest:full-path="Pictures/image%d.png" manifest:media-type="image/png"/>`, i+1)
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
	for i, img := range imgs {
		w, err := zw.Create(fmt.Sprintf("Pictures/image%d.png", i+1))
		if err != nil {
			return err
		}
		if _, err := w.Write(img.Data); err != nil {
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

// odtImage — параграф с картинкой заданного размера (см); idx — номер
// изображения, ссылка на Pictures/image<idx>.png.
func odtImage(wcm, hcm float64, idx int) string {
	return fmt.Sprintf(`<text:p><draw:frame draw:name="image%d" text:anchor-type="as-char"`+
		` svg:width="%.2fcm" svg:height="%.2fcm">`+
		`<draw:image xlink:href="Pictures/image%d.png" xlink:type="simple"`+
		` xlink:show="embed" xlink:actuate="onLoad"/></draw:frame></text:p>`,
		idx, wcm, hcm, idx)
}

// odtTable — таблица, подпись на новой строке и разрыв страницы.
func odtTable(t docTable) string {
	if len(t.Headers) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<table:table table:name="ReportTable"` +
		` table:style-name="ReportTable">`)
	fmt.Fprintf(&b, `<table:table-column table:number-columns-repeated="%d"/>`,
		len(t.Headers))
	b.WriteString(`<table:table-header-rows><table:table-row>`)
	for _, h := range t.Headers {
		b.WriteString(odtCell(h, "TableHeader"))
	}
	b.WriteString(`</table:table-row></table:table-header-rows>`)
	for _, row := range t.Rows {
		b.WriteString(`<table:table-row>`)
		for i := range t.Headers {
			v := ""
			if i < len(row) {
				v = row[i]
			}
			b.WriteString(odtCell(v, "TableText"))
		}
		b.WriteString(`</table:table-row>`)
	}
	b.WriteString(`</table:table>`)
	if t.Title != "" {
		b.WriteString(odtPara(t.Title, "ReportCaption"))
	}
	b.WriteString(odtPageBreak())
	return b.String()
}

// odtPageBreak — разрыв страницы перед пустым абзацем (стиль PageBreak).
func odtPageBreak() string {
	return odtPara("", "PageBreak")
}

// odtCell — ячейка таблицы с общим стилем границ.
func odtCell(text, style string) string {
	return `<table:table-cell table:style-name="TableCells"` +
		` office:value-type="string"><text:p text:style-name="` + style +
		`">` + odtText(text) + `</text:p></table:table-cell>`
}
