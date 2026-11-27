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
	"math"
	"testing"
)

func TestSplitMixType(t *testing.T) {
	cases := []struct {
		in  string
		non string
		ok  bool
	}{
		{"sphere+spheroid", "spheroid", true},
		{"sphere+agglomerate", "agglomerate", true},
		{"sphere+agglomerate-cs", "agglomerate-cs", true},
		{"sphere", "", false},
		{"agglomerate", "", false},
		{"sphere+unknown", "", false},
	}
	for _, c := range cases {
		non, ok := splitMixType(c.in)
		if ok != c.ok || non != c.non {
			t.Errorf("splitMixType(%q) = (%q,%v), want (%q,%v)", c.in, non, ok,
				c.non, c.ok)
		}
	}
}

// testMixKernel строит одноканальное ядро K(r) = exp(a + b·ln r).
func testMixKernel(r []float64, a, b float64) *ktable {
	lr := make([]float64, len(r))
	val := make([]float64, len(r))
	for i, rr := range r {
		lr[i] = math.Log(rr)
		val[i] = a + b*lr[i]
	}
	return &ktable{n: len(r), r: r, rMin: r[0], rMax: r[len(r)-1],
		nch: 1, sp: []*spline{newSpline(lr, val)}}
}

func TestMixKtable(t *testing.T) {
	r := make([]float64, 20)
	for i := range r {
		r[i] = 0.1 * math.Pow(10, float64(i)/5.0)
	}
	a := testMixKernel(r, 1.0, 0.5) // K1
	b := testMixKernel(r, 0.2, 0.1) // K2
	for _, sf := range []float64{0, 0.25, 0.5, 1} {
		m := mixKtable(a, b, sf)
		for _, rr := range r {
			k1 := ktableKernel(a, 0, rr)
			k2 := ktableKernel(b, 0, rr)
			km := ktableKernel(m, 0, rr)
			want := sf*k1 + (1-sf)*k2
			if math.Abs(km-want) > 1e-9*want {
				t.Fatalf("sfrac=%g r=%g: K_mix=%g, want %g", sf, rr, km, want)
			}
		}
	}
}

func TestVolumeFractionIdentity(t *testing.T) {
	for _, p := range []float64{0, 0.3, 0.5, 1} {
		if q := volumeFraction(p); math.Abs(q-p) > 1e-12 {
			t.Errorf("volumeFraction(%g) = %g, want %g (equal-volume radii)", p, q, p)
		}
	}
	if !math.IsNaN(volumeFraction(math.NaN())) {
		t.Errorf("volumeFraction(NaN) must be NaN")
	}
}

func TestSfracGridFromArgs(t *testing.T) {
	a, err := ParseArgs([]string{"--type", "sphere+spheroid", "--n-sfrac", "5"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	grid, fixed := sfracGridFromArgs(a)
	if fixed || len(grid) != 5 || grid[0] != 0 || grid[len(grid)-1] != 1 {
		t.Fatalf("grid = %v fixed=%v, want 5 values over [0,1]", grid, fixed)
	}

	a2, err := ParseArgs([]string{"--type", "sphere+agglomerate", "--sphere-frac", "0.35"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	grid2, fixed2 := sfracGridFromArgs(a2)
	if !fixed2 || len(grid2) != 1 || math.Abs(grid2[0]-0.35) > 1e-12 {
		t.Fatalf("fixed sfrac = %v fixed=%v, want [0.35]", grid2, fixed2)
	}
}

func TestArgsMixValidation(t *testing.T) {
	if a, err := ParseArgs([]string{"--type", "sphere+spheroid"}); err != nil {
		t.Fatalf("mixture type must parse: %v", err)
	} else if !a.isMixed() {
		t.Fatalf("isMixed() = false for sphere+spheroid")
	}
	// sfrac-опции без смеси — ошибка
	if _, err := ParseArgs([]string{"--type", "sphere", "--n-sfrac", "3"}); err == nil {
		t.Errorf("expected error: --n-sfrac without mixture type")
	}
	if _, err := ParseArgs([]string{"--type", "sphere", "--sphere-frac", "0.5"}); err == nil {
		t.Errorf("expected error: --sphere-frac without mixture type")
	}
	if _, err := ParseArgs([]string{"--type", "sphere+spheroid", "--sphere-frac", "1.5"}); err == nil {
		t.Errorf("expected error: --sphere-frac out of [0,1]")
	}
	if _, err := ParseArgs([]string{"--type", "sphere+spheroid", "--sfrac-min", "0.8", "--sfrac-max", "0.2"}); err == nil {
		t.Errorf("expected error: --sfrac-max < --sfrac-min")
	}
}
