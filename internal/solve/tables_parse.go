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
	"bufio"
	"bytes"
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"
)

func tokenIndex(toks []string, name string) int {
	for i, t := range toks {
		if t == name {
			return i
		}
	}
	return -1
}

// mTableDirname — имя каталога таблицы для заданного m.
func mTableDirname(re, im float64) string {
	// нормализуем отрицательный ноль
	if re == 0 {
		re = 0
	}
	if im == 0 {
		im = 0
	}
	return fmt.Sprintf("mre_%.2f_mim_%.5f", re, im)
}

// parseMDirname — разбор имени mre_<real>_mim_<imag>.
func parseMDirname(name string) (re, im float64, ok bool) {
	if !strings.HasPrefix(name, "mre_") {
		return 0, 0, false
	}
	rest := name[4:]
	i := strings.Index(rest, "_mim_")
	if i < 0 {
		return 0, 0, false
	}
	r, err := strconv.ParseFloat(strings.TrimSpace(rest[:i]), 64)
	if err != nil {
		return 0, 0, false
	}
	m, err := strconv.ParseFloat(strings.TrimSpace(rest[i+5:]), 64)
	if err != nil {
		return 0, 0, false
	}
	return r, m, true
}

// logLogSpline — кубический сплайн ln y от ln x (все значения должны быть > 0).
func logLogSpline(xs, ys []float64) *spline {
	lx := make([]float64, len(xs))
	ly := make([]float64, len(ys))
	for i := range xs {
		lx[i] = math.Log(xs[i])
		ly[i] = math.Log(ys[i])
	}
	return newSpline(lx, ly)
}

func loadColumnSplines(t *table) {
	if t.hasCext {
		t.spCext = logLogSpline(t.x, t.cext)
	}
	if t.hasCback {
		t.spCback = logLogSpline(t.x, t.cback)
	}
	if t.hasS11 {
		t.spS11 = logLogSpline(t.x, t.s11)
	}
	if t.hasS22 {
		t.spS22 = logLogSpline(t.x, t.s22)
	}
}

// tableLoad — загрузка таблицы для m-каталога mdir из источника src.
func tableLoad(src *tableSource, mdir string, extrapolate bool) (*table, error) {
	data, disp, err := src.readTable(mdir)
	if err != nil {
		return nil, fmt.Errorf("cross-section table file not found: %s (expected %s[.gz] — "+
			"mkbase/mkbase_spheres format)", src.display(path.Join(mdir, tableFileName)),
			tableFileName)
	}
	t, dxMax, err := parseTable(data, disp, extrapolate)
	if err != nil {
		return nil, err
	}
	if dxMax > 1.0 {
		src.warnLargeStep(disp, dxMax)
	}
	return t, nil
}

// tableColumns — найденные в заголовке индексы столбцов и число полей в строке.
type tableColumns struct {
	ics, ixs map[string]int // индекс значения в строке и позиция столбца x
	ix, need int
}

// scanTableHeader — поиск столбцов по строкам-заголовкам "# x ...".
func scanTableHeader(data []byte, disp string) (tableColumns, error) {
	tc := tableColumns{
		ics: make(map[string]int, len(tableColumnNames)),
		ixs: make(map[string]int, len(tableColumnNames)),
	}
	for _, n := range tableColumnNames {
		tc.ics[n], tc.ixs[n] = -1, -1
	}
	scan := bufio.NewScanner(bytes.NewReader(data))
	scan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scan.Scan() {
		s := strings.TrimLeft(scan.Text(), " \t\r\n\v\f")
		if !strings.HasPrefix(s, "#") {
			continue
		}
		toks := strings.Fields(s[1:])
		if len(toks) == 0 || toks[0] != "x" {
			continue
		}
		ix := tokenIndex(toks, "x")
		for _, n := range tableColumnNames {
			if tc.ics[n] >= 0 {
				continue
			}
			if ic := tokenIndex(toks, n); ic >= 0 {
				tc.ixs[n], tc.ics[n] = ix, ic
			}
		}
	}
	if err := scan.Err(); err != nil {
		return tc, fmt.Errorf("%s: failed to read table: %w", disp, err)
	}
	if tc.ics["Cext"] < 0 && tc.ics["Cback"] < 0 {
		return tc, fmt.Errorf("%s: mkbase-format header with columns 'x' and 'Cext' not found "+
			"(expected, for example, '# x Cext Cabs Cback Qext Qabs Qback S11 S22 n')",
			disp)
	}

	tc.ix, tc.need = -1, 0
	for _, n := range tableColumnNames {
		if tc.ics[n] < 0 {
			continue
		}
		if tc.ix < 0 {
			tc.ix = tc.ixs[n]
		} else if tc.ixs[n] != tc.ix {
			return tc, fmt.Errorf("%s: columns Cext/Cback/S11/S22 come from headers with "+
				"different x positions — ambiguous table format", disp)
		}
		if tc.ics[n]+1 > tc.need {
			tc.need = tc.ics[n] + 1
		}
	}
	if tc.ix+1 > tc.need {
		tc.need = tc.ix + 1
	}
	return tc, nil
}

// scanTableRows — чтение числовых строк по найденным столбцам.
func scanTableRows(data []byte, disp string, tc tableColumns) ([]float64,
	map[string][]float64, error) {
	xs := []float64{}
	cols := make(map[string][]float64, len(tableColumnNames))
	scan := bufio.NewScanner(bytes.NewReader(data))
	scan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scan.Scan() {
		s := strings.TrimLeft(scan.Text(), " \t\r\n\v\f")
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		s = strings.TrimRight(s, " \t\r\n\v\f")
		toks := strings.Fields(s)
		if len(toks) < tc.need {
			return nil, nil, fmt.Errorf("%s: no required column in row: '%s'", disp, s)
		}
		xv, err := strconv.ParseFloat(toks[tc.ix], 64)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: failed to parse column x in row: '%s'", disp, s)
		}
		xs = append(xs, xv)
		for _, n := range tableColumnNames {
			if tc.ics[n] < 0 {
				continue
			}
			v, err := strconv.ParseFloat(toks[tc.ics[n]], 64)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: failed to parse %s in row: '%s'", disp, n, s)
			}
			cols[n] = append(cols[n], v)
		}
	}
	if err := scan.Err(); err != nil {
		return nil, nil, fmt.Errorf("%s: failed to read table: %w", disp, err)
	}
	return xs, cols, nil
}

// validateTableData — проверки, без которых сплайны по таблице не построить.
func validateTableData(xs []float64, cols map[string][]float64,
	ics map[string]int, disp string) error {
	n := len(xs)
	if n < 3 {
		return fmt.Errorf("%s: insufficient points (>= 3), found %d", disp, n)
	}
	for i := 1; i < n; i++ {
		if !(xs[i] > xs[i-1]) {
			return fmt.Errorf("%s: x must be strictly increasing", disp)
		}
	}
	for _, nm := range []string{"Cext", "Cback"} {
		if ics[nm] < 0 {
			continue
		}
		for _, v := range cols[nm] {
			if !(v > 0) || !isFinite(v) {
				return fmt.Errorf("%s: %s cross sections must be > 0 and finite", disp, nm)
			}
		}
	}
	return nil
}

// parseTable — разбор содержимого data.txt. Возвращает таблицу и максимальный
// шаг по x (для диагностики разрешения Mie-ряби).
func parseTable(data []byte, disp string, extrapolate bool) (*table, float64, error) {
	tc, err := scanTableHeader(data, disp)
	if err != nil {
		return nil, 0, err
	}
	xs, cols, err := scanTableRows(data, disp, tc)
	if err != nil {
		return nil, 0, err
	}
	if err := validateTableData(xs, cols, tc.ics, disp); err != nil {
		return nil, 0, err
	}

	n := len(xs)
	// S11/S22 необязательны и могут содержать nan (mkbase пишет nan, если
	// столбца s22 нет в mueller): тогда δ_l по этой таблице не считается.
	validS := func(nm string) bool {
		if tc.ics[nm] < 0 {
			return false
		}
		for _, v := range cols[nm] {
			if !(v > 0) || !isFinite(v) {
				return false
			}
		}
		return true
	}
	dxMax := 0.0
	for i := 1; i < n; i++ {
		if d := xs[i] - xs[i-1]; d > dxMax {
			dxMax = d
		}
	}

	t := &table{
		x:           xs,
		xMin:        xs[0],
		xMax:        xs[n-1],
		extrapolate: extrapolate,
		path:        disp,
	}
	if tc.ics["Cext"] >= 0 {
		t.hasCext = true
		t.cext = cols["Cext"]
	}
	if tc.ics["Cback"] >= 0 {
		t.hasCback = true
		t.cback = cols["Cback"]
	}
	if validS("S11") {
		t.hasS11 = true
		t.s11 = cols["S11"]
	}
	if validS("S22") {
		t.hasS22 = true
		t.s22 = cols["S22"]
	}
	loadColumnSplines(t)
	return t, dxMax, nil
}

// tableRangeError — ошибка выхода x за диапазон таблицы (при отключённой
// экстраполяции).
func tableRangeError(t *table) error {
	return fmt.Errorf("size parameter x outside table range [%g, %g] — extend the table "+
		"range or enable --table-extrapolate (%s)", t.xMin, t.xMax, t.path)
}

// tableCrossSection — сечение при заданном x. Диапазон проверяет вызывающий
// (ktableBuild); сюда попадает только корретное x.
func tableCrossSection(t *table, useCback bool, x float64) float64 {
	sp := t.spCext
	if useCback {
		sp = t.spCback
	}
	if sp == nil {
		panic(fmt.Sprintf("%s: required cross-section column is missing", t.path))
	}
	return math.Exp(sp.eval(math.Log(x)))
}
