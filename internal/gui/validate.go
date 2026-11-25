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
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"csolve/internal/solve"
)

// Request — запрос на запуск/проверку от браузера.
type Request struct {
	Mode        string         `json:"mode"` // demo | json | batch
	DataPath    string         `json:"data_path"`
	BatchPath   string         `json:"batch_path"`
	TablesKind  string         `json:"tables_kind"` // builtin | external
	BuiltinType string         `json:"builtin_type"`
	TablesDir   string         `json:"tables_dir"`
	Options     map[string]any `json:"options"`
	Output      Output         `json:"output"`
}

// Output — параметры вывода (управляются GUI, а не опциями формы).
type Output struct {
	PlotFormat   string  `json:"plot_format"` // none | png | pdf | svg
	PlotEngine   string  `json:"plot_engine"` // internal | gnuplot
	PlotDPI      float64 `json:"plot_dpi"`
	PlotFontSize float64 `json:"plot_font_size"`
	Spectra      bool    `json:"spectra"`
	Report       bool    `json:"report"`
	ReportFormat string  `json:"report_format"` // docx | odt
}

// argBuilder — накопитель аргументов командной строки.
type argBuilder struct{ argv []string }

func (b *argBuilder) add(flag, value string) { b.argv = append(b.argv, flag, value) }
func (b *argBuilder) flag(flag string)       { b.argv = append(b.argv, flag) }

// buildArgv — перевод формы в CLI-аргументы и их проверка. outDir — каталог,
// в который решатель запишет выход; для предпросмотра может быть любым.
func buildArgv(req *Request, outDir string) ([]string, *solve.Args, error) {
	if req == nil {
		return nil, nil, errors.New("empty request")
	}
	b := &argBuilder{}
	if err := b.input(req); err != nil {
		return nil, nil, err
	}
	if err := b.tables(req); err != nil {
		return nil, nil, err
	}
	if err := b.options(req); err != nil {
		return nil, nil, err
	}
	if err := b.output(req, outDir); err != nil {
		return nil, nil, err
	}
	a, err := solve.ParseArgs(b.argv)
	if err != nil {
		return nil, nil, err
	}
	return b.argv, a, nil
}

// input — режим входа: демо, JSON или пакетный TXT.
func (b *argBuilder) input(req *Request) error {
	switch req.Mode {
	case "", "demo":
		// демо-данные
	case "json":
		p := strings.TrimSpace(req.DataPath)
		if p == "" {
			return errors.New("a JSON file with measurements is required")
		}
		b.add("--data", absPath(p))
	case "batch":
		p := strings.TrimSpace(req.BatchPath)
		if p == "" {
			return errors.New("a batch TXT file is required")
		}
		b.add("--batch", absPath(p))
	default:
		return fmt.Errorf("unknown input mode %q", req.Mode)
	}
	return nil
}

// tables — источник таблиц сечений (встроенный набор или внешний каталог).
func (b *argBuilder) tables(req *Request) error {
	if req.TablesKind == "external" {
		dir := strings.TrimSpace(req.TablesDir)
		if dir == "" {
			return errors.New("an external tables directory is required")
		}
		b.add("--tables", absPath(dir))
		return nil
	}
	t := strings.TrimSpace(req.BuiltinType)
	if t == "" {
		t = defaultTableType()
	}
	if !slices.Contains(tableTypes(), t) {
		return fmt.Errorf("unknown built-in table type %q", t)
	}
	b.add("--type", t)
	return nil
}

// options — опции формы, отличающиеся от значений по умолчанию.
func (b *argBuilder) options(req *Request) error {
	defs := defaultValues()
	for key, raw := range req.Options {
		if raw == nil {
			continue
		}
		f := fieldByKey(key)
		if f == nil {
			return fmt.Errorf("unknown option %q", key)
		}
		def, hasDef := defs[key]
		if !hasDef {
			return fmt.Errorf("option %q has no known default", key)
		}

		switch f.Type {
		case "bool":
			v, err := toBool(raw)
			if err != nil {
				return fmt.Errorf("option --%s: %w", key, err)
			}
			d, ok := toBoolStrict(def)
			if !ok || v == d {
				continue
			}
			if v {
				b.flag("--" + key)
			} else if f.FalseKey != "" {
				b.flag("--" + f.FalseKey)
			} else {
				return fmt.Errorf("option --%s has no "+
					"negative form", key)
			}

		default:
			s, err := rawString(raw)
			if err != nil {
				return fmt.Errorf("option --%s: %w", key, err)
			}
			if s == "" {
				continue
			}
			if f.Type == "enum" {
				if ds, ok2 := def.(string); ok2 && ds == s {
					continue
				}
			} else if f.Type == "optional" {
				// всегда передаём явно заданное значение
			} else { // int / float
				if numEqual(s, def) {
					continue
				}
			}
			b.add("--"+key, s)
		}
	}
	return nil
}

// numEqual — равно ли строковое значение числу по умолчанию.
func numEqual(s string, def any) bool {
	v := parseFloatQuiet(s)
	switch d := def.(type) {
	case float64:
		return v == d
	case int:
		return v == float64(d)
	}
	return false
}

// toBoolStrict возвращает bool-значение по умолчанию, если оно задано.
func toBoolStrict(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	}
	return false, false
}

func parseFloatQuiet(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// output — выходные флаги (GUI управляет ими отдельно от опций формы).
func (b *argBuilder) output(req *Request, outDir string) error {
	out := req.Output
	if outDir == "" {
		outDir = "."
	}

	// спектры
	if !out.Spectra {
		b.flag("--no-spectra")
	}

	// графики
	format := ""
	if out.PlotFormat != "none" {
		engine := out.PlotEngine
		if engine != "gnuplot" {
			engine = "internal"
		}
		if engine == "gnuplot" {
			switch out.PlotFormat {
			case "png", "pdf", "svg":
				format = out.PlotFormat
			default:
				format = "pdf"
			}
		} else {
			switch out.PlotFormat {
			case "png", "pdf":
				format = out.PlotFormat
			default:
				format = "png"
			}
		}
		b.add("--plotting", engine)
		b.add("--plot-format", format)
		if out.PlotDPI > 0 {
			b.add("--plot-dpi", strconv.FormatFloat(out.PlotDPI, 'g', -1, 64))
		}
		if out.PlotFontSize > 0 {
			b.add("--plot-font-size", strconv.FormatFloat(out.PlotFontSize, 'g', -1, 64))
		}
	} else {
		b.flag("--no-plot")
	}

	var outDirFlag string
	if req.Mode == "batch" {
		outDirFlag = "--batch-out-dir"
	} else {
		outDirFlag = "--out-dir"
		b.add("--json", "result.json")
		if format != "" {
			b.add("--out", "result."+format)
		}
	}
	b.add(outDirFlag, outDir)

	// отчёт (одиночный и пакетный режимы)
	if out.Report {
		format := "docx"
		if out.ReportFormat == "odt" {
			format = "odt"
		}
		b.add("--report", "report."+format)
	}
	return nil
}

// absPath — абсолютный путь относительно текущего каталога.
func absPath(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(baseDir(), p)
}

func toBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	}
	return false, fmt.Errorf("expected a boolean, got %v", v)
}

func rawString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x), nil
	case float64:
		// целочисленные значения без экспоненты и точки
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10), nil
		}
		return strconv.FormatFloat(x, 'g', -1, 64), nil
	}
	return "", fmt.Errorf("expected a number or string, got %v", v)
}
