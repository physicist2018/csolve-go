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
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// batchReportImages — PNG-графики отдельных высот, накопленные во время
// расчёта пакета (только при включённом отчёте и не --no-plot).
type batchReportImages struct {
	altitude []altitudeReportImages
}

type altitudeReportImages struct {
	h        float64
	profile  []byte
	spectrum []byte
}

func (r *batchReportImages) add(h float64, profile, spectrum []byte) {
	if r == nil || (len(profile) == 0 && len(spectrum) == 0) {
		return
	}
	r.altitude = append(r.altitude, altitudeReportImages{
		h: h, profile: profile, spectrum: spectrum})
}

// reportNum — компактное число для таблицы; нефинитные значения — прочерк.
func reportNum(v float64) string {
	if !isFinite(v) {
		return "—"
	}
	return strconv.FormatFloat(v, 'g', 4, 64)
}

// reportAt — значение среза или NaN вне диапазона.
func reportAt(v []float64, i int) float64 {
	if i >= 0 && i < len(v) {
		return v[i]
	}
	return math.NaN()
}

// matrixAnyFinite — есть ли хоть одно конечное значение.
func matrixAnyFinite(rows [][]float64) bool {
	for _, r := range rows {
		for _, v := range r {
			if isFinite(v) {
				return true
			}
		}
	}
	return false
}

// batchReportTables — таблицы сводки по высотам.
func batchReportTables(b *batchInput, met *metrics) []docTable {
	micro := docTable{
		Title: "Микрофизика и оптика по высотам " +
			"(V — µm³/cm³; r — µm; N — cm⁻³; Err — %)",
		Headers: []string{"H, м", "V", "Vc", "Vf", "N", "reff", "Re(m)",
			"Im(m)", "Err"},
		Rows: make([][]string, 0, b.nAlt),
	}
	withSfrac := len(met.sfVolMean) == b.nAlt && b.nAlt > 0
	if withSfrac {
		micro.Headers = append(micro.Headers, "sfrac (объём)")
	}
	modes := docTable{
		Title:   "Параметры логнормальных мод",
		Headers: []string{"H, м", "rmf", "rmc", "rm", "σf", "σc", "σ"},
		Rows:    make([][]string, 0, b.nAlt),
	}
	for i := 0; i < b.nAlt; i++ {
		row := []string{
			formatG6(b.alt[i]),
			reportNum(reportAt(met.V, i)),
			reportNum(reportAt(met.Vc, i)),
			reportNum(reportAt(met.Vf, i)),
			reportNum(reportAt(met.Nc, i)),
			reportNum(reportAt(met.reff, i)),
			reportNum(reportAt(met.mRe, i)),
			reportNum(reportAt(met.mIm, i)),
			reportNum(reportAt(met.err, i) * 100.0),
		}
		if withSfrac {
			row = append(row, reportNum(reportAt(met.sfVolMean, i)))
		}
		micro.Rows = append(micro.Rows, row)
		modes.Rows = append(modes.Rows, []string{
			formatG6(b.alt[i]),
			reportNum(reportAt(met.rmf, i)),
			reportNum(reportAt(met.rmc, i)),
			reportNum(reportAt(met.rmo, i)),
			reportNum(reportAt(met.sgf, i)),
			reportNum(reportAt(met.sgc, i)),
			reportNum(reportAt(met.sgo, i)),
		})
	}
	tables := []docTable{micro, modes}

	if !matrixAnyFinite(met.dpol) && !matrixAnyFinite(met.dpolMeas) {
		return tables
	}
	dep := docTable{
		Title: "Линейная деполяризация δ_l, %",
		Rows:  make([][]string, 0, b.nAlt),
	}
	dep.Headers = append(dep.Headers, "H, м")
	for _, wl := range met.dpolWl {
		dep.Headers = append(dep.Headers, fmt.Sprintf("δ_l(%g)", wl*1e3))
	}
	for _, wl := range met.dpolMeasWl {
		dep.Headers = append(dep.Headers, fmt.Sprintf("δ_l^meas(%g)", wl*1e3))
	}
	for i := 0; i < b.nAlt; i++ {
		row := make([]string, 0, len(dep.Headers))
		row = append(row, formatG6(b.alt[i]))
		for k := range met.dpol {
			row = append(row, reportNum(reportAt(met.dpol[k], i)))
		}
		for k := range met.dpolMeas {
			row = append(row, reportNum(reportAt(met.dpolMeas[k], i)))
		}
		dep.Rows = append(dep.Rows, row)
	}
	return append(tables, dep)
}

// summaryReportImages — сводные профили пакета в PNG для отчёта. При
// --plotting=gnuplot используются готовые PNG-файлы (--plot-format png) или
// те же графики перерисовываются gnuplot в PNG.
func summaryReportImages(a *Args, c *console, outDir string, b *batchInput,
	met *metrics, common []commonWl, nl int) []docImage {
	ext := plotFormatExt(a.PlotFormat)
	var out []docImage
	if bts, ok := reportPNG(a, c, filepath.Join(outDir, "refractive."+ext),
		func(po *plotOpts) ([]byte, bool) {
			return internalRefractiveProfilePNG(po, b.alt, met.mRe, met.mIm)
		},
		func(po *plotOpts, path string) bool {
			return plotRefractiveProfile(po, path, b.alt, met.mRe, met.mIm)
		}); ok {
		out = append(out, docImage{Data: bts,
			Caption: "Показатель преломления по высотам"})
	}
	for _, sp := range batchProfilePlotSpecs(b, met, common, nl) {
		spec := sp
		existing := filepath.Join(outDir, spec.name+"."+ext)
		if bts, ok := reportPNG(a, c, existing,
			func(po *plotOpts) ([]byte, bool) {
				return internalProfilesPNG(po, spec.title, spec.xlabel,
					len(spec.vals), b.alt, spec.vals, spec.names,
					spec.dashed, spec.logAuto)
			},
			func(po *plotOpts, path string) bool {
				return plotProfilesStyled(po, path, spec.title,
					spec.xlabel, len(spec.vals), b.alt, spec.vals,
					spec.names, spec.dashed, spec.logAuto)
			}); ok {
			out = append(out, docImage{Data: bts, Caption: spec.caption})
		}
	}
	return out
}

// writeBatchReportDoc — сохранение пакетного отчёта: лог расчёта, сводка по
// высотам (таблицы) и графики PNG (сводные и по каждой высоте).
func writeBatchReportDoc(a *Args, c *console, rep *reportCapture, outDir string,
	st *batchSetup, met *metrics, common []commonWl, nl int,
	rimg *batchReportImages) error {
	path, inDir := resolveOutPath(outDir, a.Report)
	if inDir {
		if err := mkdirP(outDir); err != nil {
			return err
		}
	}

	b := st.b
	srcLabel := "встроенные таблицы"
	if st.src != nil && st.src.label != "" {
		srcLabel = st.src.label
	}
	meta := fmt.Sprintf("Дата: %s; входные данные: %s; высот: %d; таблицы: %s",
		time.Now().Format("2006-01-02 15:04:05"), a.Batch, b.nAlt, srcLabel)
	if b.thinWindow > 0 {
		meta += fmt.Sprintf("; усреднение по высоте: окно %g м", b.thinWindow)
	}
	if a.Method != "" {
		meta += fmt.Sprintf("; метод: %s", a.Method)
	}

	tables := batchReportTables(b, met)
	var sections []docImageSection
	if !a.NoPlot {
		if imgs := summaryReportImages(a, c, outDir, b, met, common, nl); len(imgs) > 0 {
			sections = append(sections, docImageSection{
				Title: "Сводные графики по высотам", Images: imgs})
		}
		if rimg != nil {
			imgs := make([]docImage, 0, 2*len(rimg.altitude))
			for _, ar := range rimg.altitude {
				if len(ar.profile) > 0 {
					imgs = append(imgs, docImage{Data: ar.profile,
						Caption: fmt.Sprintf("H = %s м — распределение "+
							"dV/dln r, V(r) и коэффициенты", formatG6(ar.h))})
				}
				if len(ar.spectrum) > 0 {
					imgs = append(imgs, docImage{Data: ar.spectrum,
						Caption: fmt.Sprintf("H = %s м — спектры α(λ), β(λ), "+
							"S(λ), δ_l(λ)", formatG6(ar.h))})
				}
			}
			if len(imgs) > 0 {
				sections = append(sections, docImageSection{
					Title: "Графики по высотам", Images: imgs})
			}
		}
	}

	title := "csolve-go: пакетный отчёт по высотам"
	params := appliedOptionLines(a)
	var err error
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
