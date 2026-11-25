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
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
)

type batchChannel struct {
	kind chKind
	wlUm float64
	name string
}

type batchInput struct {
	ch   []batchChannel
	nch  int
	alt  []float64
	data []float64 // nAlt × nch, по строкам
	nAlt int

	depWl []float64 // длины волн измеренных δ_l, µm (столбцы dep<nm>)
	dep   []float64 // nAlt x ndep, row-major; δ_l в процентах
	ndep  int

	// thinWindow — ширина окна усреднения по высоте, м (0 — без прореживания).
	// thinCounts — число исходных строк в каждом усреднённом профиле.
	thinWindow float64
	thinCounts []int
}

// batchColSpec — описание одного столбца таблицы (кроме высоты).
type batchColSpec struct {
	dep  bool
	kind chKind
	wlUm float64
	name string
}

func parseBatchTxt(path string) (*batchInput, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	b := &batchInput{}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	haveHeader, haveRow := false, false
	var specs []batchColSpec

	for scan.Scan() {
		s := strings.TrimSpace(scan.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		toks := strings.Fields(s)
		if len(toks) == 0 {
			continue
		}
		if !haveHeader {
			low := strings.ToLower(toks[0])
			if low != "altitude" && low != "alt" && low != "h" {
				return nil, fmt.Errorf("first header field must be 'Altitude', got '%s'", toks[0])
			}
			specs = make([]batchColSpec, 0, len(toks)-1)
			for i := 1; i < len(toks); i++ {
				spec, ok := parseBatchColumnName(toks[i])
				if !ok {
					return nil, fmt.Errorf("unknown column name '%s' (expected b<nm>, a<nm> or dep<nm>)",
						toks[i])
				}
				if spec.dep {
					for _, prev := range specs {
						if prev.dep && prev.wlUm == spec.wlUm {
							return nil, fmt.Errorf("duplicate depolarization column '%s'", toks[i])
						}
					}
					b.depWl = append(b.depWl, spec.wlUm)
				} else {
					b.ch = append(b.ch, batchChannel{spec.kind, spec.wlUm, spec.name})
				}
				specs = append(specs, spec)
			}
			b.nch = len(b.ch)
			b.ndep = len(b.depWl)
			if b.nch <= 0 {
				return nil, errors.New("no channels in header (expected b/a<nm>)")
			}
			haveHeader = true
			continue
		}

		if len(toks) != len(specs)+1 {
			return nil, fmt.Errorf("expected %d values per row, got %d: '%s'",
				len(specs), len(toks)-1, s)
		}
		hv, err := strconv.ParseFloat(toks[0], 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse line '%s'", s)
		}
		b.alt = append(b.alt, hv)
		for i, spec := range specs {
			v, err := strconv.ParseFloat(toks[i+1], 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse line '%s'", s)
			}
			if spec.dep {
				b.dep = append(b.dep, v)
			} else {
				b.data = append(b.data, v)
			}
		}
		haveRow = true
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	if !haveHeader {
		return nil, fmt.Errorf("file %s is empty", path)
	}
	if !haveRow {
		return nil, fmt.Errorf("no altitude rows in file %s", path)
	}
	b.nAlt = len(b.alt)
	return b, nil
}

// thinBatchAltitudes — прореживание пакета по высоте: строки, попавшие в
// окно ширины w, заменяются средними значениями. Высота результата — центр
// фактического окна; окна без валидных β/α пропускаются с предупреждением.
func thinBatchAltitudes(b *batchInput, w float64, c *console) (*batchInput, error) {
	if !isFinite(w) || !(w > 0) {
		return nil, fmt.Errorf("altitude thinning window must be a finite number > 0")
	}
	if b.nAlt == 0 {
		return nil, errors.New("no altitude rows to thin")
	}
	hmin, hmax := b.alt[0], b.alt[0]
	for _, h := range b.alt {
		if !isFinite(h) {
			return nil, errors.New("altitude values must be finite")
		}
		if h < hmin {
			hmin = h
		}
		if h > hmax {
			hmax = h
		}
	}
	span := hmax - hmin
	nWin := 1
	if span > 0 {
		nf := math.Ceil(span / w)
		if nf > 1e7 {
			return nil, fmt.Errorf("too many altitude windows (%g); "+
				"increase --batch-thin", nf)
		}
		nWin = int(nf)
	}

	type acc struct {
		rows   int
		sum    []float64
		cnt    []int
		depSum []float64
		depCnt []int
	}
	bins := make(map[int]*acc)
	for i, h := range b.alt {
		k := 0
		if span > 0 {
			k = int(math.Floor((h - hmin) / w))
		}
		if k < 0 {
			k = 0
		}
		if k >= nWin {
			k = nWin - 1
		}
		a := bins[k]
		if a == nil {
			a = &acc{
				sum:    make([]float64, b.nch),
				cnt:    make([]int, b.nch),
				depSum: make([]float64, b.ndep),
				depCnt: make([]int, b.ndep),
			}
			bins[k] = a
		}
		a.rows++
		row := b.data[i*b.nch : (i+1)*b.nch]
		for j, v := range row {
			if isFinite(v) {
				a.sum[j] += v
				a.cnt[j]++
			}
		}
		if b.ndep > 0 {
			drow := b.dep[i*b.ndep : (i+1)*b.ndep]
			for j, v := range drow {
				if isFinite(v) {
					a.depSum[j] += v
					a.depCnt[j]++
				}
			}
		}
	}

	keys := make([]int, 0, len(bins))
	for k := range bins {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	out := &batchInput{
		ch:         append([]batchChannel(nil), b.ch...),
		nch:        b.nch,
		depWl:      append([]float64(nil), b.depWl...),
		ndep:       b.ndep,
		thinWindow: w,
	}
	var skipped [][2]float64
	for _, k := range keys {
		a := bins[k]
		lo := hmin + float64(k)*w
		hi := lo + w
		if hi > hmax {
			hi = hmax
		}
		if !(hi > lo) {
			hi = hmax
		}
		vals := make([]float64, b.nch)
		valid := true
		for j := range vals {
			if a.cnt[j] == 0 {
				valid = false
				break
			}
			vals[j] = a.sum[j] / float64(a.cnt[j])
			if !(vals[j] > 0) {
				valid = false
				break
			}
		}
		if !valid {
			skipped = append(skipped, [2]float64{lo, hi})
			continue
		}
		out.alt = append(out.alt, (lo+hi)/2)
		out.data = append(out.data, vals...)
		for j := 0; j < b.ndep; j++ {
			v := math.NaN()
			if a.depCnt[j] > 0 {
				v = a.depSum[j] / float64(a.depCnt[j])
			}
			out.dep = append(out.dep, v)
		}
		out.thinCounts = append(out.thinCounts, a.rows)
	}
	if len(out.alt) == 0 {
		return nil, errors.New("altitude thinning left no valid windows")
	}
	out.nAlt = len(out.alt)

	c.printf("altitude thinning: window %g m, H = %g..%g m\n", w, hmin, hmax)
	c.printf("  %d input rows -> %d averaged profiles; skipped %d "+
		"empty/invalid windows\n", b.nAlt, out.nAlt, len(skipped))
	for i, r := range skipped {
		if i == 5 {
			c.warnf("  [warning] altitude thinning: %d more windows skipped\n",
				len(skipped)-i)
			break
		}
		c.warnf("  [warning] altitude thinning: skipped window [%g, %g] m "+
			"(no valid β/α values)\n", r[0], r[1])
	}
	return out, nil
}

// parseBatchColumnName — разбор имени столбца: b<nm> (β), a<nm> (α) или
// dep<nm> (измеренное δ_l, %).
func parseBatchColumnName(nm string) (batchColSpec, bool) {
	low := strings.ToLower(nm)
	if strings.HasPrefix(low, "dep") {
		wl, err := strconv.Atoi(nm[3:])
		if err != nil || wl <= 0 {
			return batchColSpec{}, false
		}
		return batchColSpec{dep: true, wlUm: float64(wl) * 1e-3, name: nm}, true
	}
	if len(nm) < 2 {
		return batchColSpec{}, false
	}
	digits := true
	for k := 1; k < len(nm); k++ {
		if nm[k] < '0' || nm[k] > '9' {
			digits = false
		}
	}
	if !digits || (low[0] != 'b' && low[0] != 'a') {
		return batchColSpec{}, false
	}
	wl, err := strconv.Atoi(nm[1:])
	if err != nil || wl <= 0 {
		return batchColSpec{}, false
	}
	kind := chAlpha
	if low[0] == 'b' {
		kind = chBeta
	}
	return batchColSpec{kind: kind, wlUm: float64(wl) * 1e-3, name: nm}, true
}

type commonWl struct {
	wl float64
	ib int // индекс канала β
	ia int // индекс канала α
}

func commonWavelengths(b *batchInput) []commonWl {
	wls := []float64{}
	ibs := []int{}
	ias := []int{}
	for j := 0; j < b.nch; j++ {
		wl := b.ch[j].wlUm
		k := -1
		for i := range wls {
			if wls[i] == wl {
				k = i
				break
			}
		}
		if k < 0 {
			k = len(wls)
			wls = append(wls, wl)
			ibs = append(ibs, -1)
			ias = append(ias, -1)
		}
		// как словарь в Python: для одинаковых wl побеждает последний
		if b.ch[j].kind == chBeta {
			ibs[k] = j
		} else {
			ias[k] = j
		}
	}
	out := []commonWl{}
	for i := range wls {
		if ibs[i] >= 0 && ias[i] >= 0 {
			out = append(out, commonWl{wls[i], ibs[i], ias[i]})
		}
	}
	slices.SortStableFunc(out, func(a, b commonWl) int { return compareFloats(a.wl, b.wl) })
	return out
}

func lidarRatioValue(gFit []float64, common []commonWl, k int) float64 {
	b := gFit[common[k].ib]
	a := gFit[common[k].ia]
	if b > 0 && a > 0 {
		return a / b
	}
	return math.NaN()
}
