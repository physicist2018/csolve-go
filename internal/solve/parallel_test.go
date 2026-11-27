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
	"testing"
)

// TestParallelForRunsAllIndices — каждый индекс выполняется ровно один раз.
func TestParallelForRunsAllIndices(t *testing.T) {
	const n = 37
	for _, workers := range []int{1, 2, 5, 64} {
		seen := make([]int, n)
		parallelFor(n, workers, func(i int) { seen[i]++ })
		for i, c := range seen {
			if c != 1 {
				t.Fatalf("workers=%d: index %d visited %d times", workers, i, c)
			}
		}
	}
}

// TestJobsFlag — разбор --jobs/-j и значение по умолчанию (NumCPU-1).
func TestJobsFlag(t *testing.T) {
	def := JobsDefault()
	if def < 1 || def > jobsMax {
		t.Fatalf("JobsDefault = %d, want 1..%d", def, jobsMax)
	}

	cases := []struct {
		name    string
		argv    []string
		raw     int
		workers int
	}{
		{"default", nil, def, def},
		{"one", []string{"--jobs", "1"}, 1, 1},
		{"three", []string{"--jobs", "3"}, 3, 3},
		{"short", []string{"-j", "4"}, 4, 4},
		{"auto", []string{"--jobs", "0"}, 0, def},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := ParseArgs(tc.argv)
			if err != nil {
				t.Fatalf("ParseArgs(%v): %v", tc.argv, err)
			}
			if a.Jobs != tc.raw {
				t.Errorf("Jobs = %d, want %d", a.Jobs, tc.raw)
			}
			if got := a.Workers(); got != tc.workers {
				t.Errorf("Workers = %d, want %d", got, tc.workers)
			}
		})
	}

	for _, argv := range [][]string{
		{"--jobs=-1"},
		{"--jobs", "100000"},
		{"--jobs", "2", "-j", "3"},
	} {
		if _, err := ParseArgs(argv); err == nil {
			t.Errorf("ParseArgs(%v) must fail", argv)
		}
	}
}

func parallelTestKernels() []*ktable {
	kt1 := testKtable(0.05, 2.0, 200, func(j int, r float64) float64 {
		return 1.0 + 0.5*float64(j) + 0.1*r
	})
	kt2 := testKtable(0.05, 2.0, 200, func(j int, r float64) float64 {
		return 2.0 + 0.3*float64(j) + 0.05*r
	})
	return []*ktable{kt1, kt2}
}

func parallelTestSpec(workers int) designSpec {
	return designSpec{
		method:      methodMonotone,
		bins:        5,
		quadOrder:   16,
		maxLogSpan:  0.1,
		regOrder:    2,
		kernelOrder: 3,
		workers:     workers,
	}
}

// TestDesignParallelMatchesSerial — матрицы A, s0 и cond при 1 и N воркерах
// бит-в-бит совпадают.
func TestDesignParallelMatchesSerial(t *testing.T) {
	kts := parallelTestKernels()
	mRe := []float64{1.5, 1.4}
	mIm := []float64{0.0, -0.01}
	r0Grid := []float64{0.06, 0.1, 0.15}
	r1Grid := []float64{0.5, 1.2}

	d1, err := designBuild(nil, 2, kts, mRe, mIm, r0Grid, r1Grid, 0,
		parallelTestSpec(1))
	if err != nil {
		t.Fatalf("designBuild(serial): %v", err)
	}
	dN, err := designBuild(nil, 2, kts, mRe, mIm, r0Grid, r1Grid, 0,
		parallelTestSpec(4))
	if err != nil {
		t.Fatalf("designBuild(parallel): %v", err)
	}

	if d1.nW != dN.nW || d1.nM != dN.nM || d1.nu != dN.nu {
		t.Fatalf("shape differs: serial (%d,%d,%d) parallel (%d,%d,%d)",
			d1.nW, d1.nM, d1.nu, dN.nW, dN.nM, dN.nu)
	}
	if len(d1.A) != len(dN.A) {
		t.Fatalf("len(A): serial %d, parallel %d", len(d1.A), len(dN.A))
	}
	for i := range d1.A {
		if d1.A[i] != dN.A[i] {
			t.Fatalf("A[%d]: serial %v, parallel %v", i, d1.A[i], dN.A[i])
		}
	}
	for i := range d1.s0 {
		if d1.s0[i] != dN.s0[i] || d1.cond[i] != dN.cond[i] {
			t.Fatalf("s0/cond[%d]: serial (%v,%v), parallel (%v,%v)",
				i, d1.s0[i], d1.cond[i], dN.s0[i], dN.cond[i])
		}
	}
}

// TestScanVariantsParallelMatchesSerial — перебор решений при 1 и N воркерах
// даёт одинаковые решения в одинаковом порядке.
func TestScanVariantsParallelMatchesSerial(t *testing.T) {
	kts := parallelTestKernels()
	mRe := []float64{1.5, 1.4}
	mIm := []float64{0.0, -0.01}
	r0Grid := []float64{0.06, 0.1, 0.15}
	r1Grid := []float64{0.5, 1.2}

	d, err := designBuild(nil, 2, kts, mRe, mIm, r0Grid, r1Grid, 0,
		parallelTestSpec(1))
	if err != nil {
		t.Fatalf("designBuild: %v", err)
	}
	g1 := []float64{1.0, 0.8}
	c := testConsole()

	d.workers = 1
	s1, err := scanVariants(d, g1, 2, gammaFixed, 0.1, -7, 5, 41, true, true,
		nil, c)
	if err != nil {
		t.Fatalf("scanVariants(serial): %v", err)
	}
	d.workers = 4
	sN, err := scanVariants(d, g1, 2, gammaFixed, 0.1, -7, 5, 41, true, true,
		nil, c)
	if err != nil {
		t.Fatalf("scanVariants(parallel): %v", err)
	}

	if s1.n != sN.n {
		t.Fatalf("n: serial %d, parallel %d", s1.n, sN.n)
	}
	for i := 0; i < s1.n; i++ {
		a, b := &s1.sol[i], &sN.sol[i]
		if a.id != b.id || a.err != b.err || a.gamma != b.gamma ||
			a.mRe != b.mRe || a.mIm != b.mIm || a.r0 != b.r0 || a.r1 != b.r1 ||
			a.geomIndex != b.geomIndex {
			t.Fatalf("solution %d differs: serial %+v, parallel %+v", i, a, b)
		}
		if len(a.C) != len(b.C) || len(a.gFit) != len(b.gFit) {
			t.Fatalf("solution %d: size differs", i)
		}
		for k := range a.C {
			if a.C[k] != b.C[k] {
				t.Fatalf("solution %d C[%d]: %v vs %v", i, k, a.C[k], b.C[k])
			}
		}
		for k := range a.gFit {
			if a.gFit[k] != b.gFit[k] {
				t.Fatalf("solution %d gFit[%d]: %v vs %v", i, k, a.gFit[k],
					b.gFit[k])
			}
		}
	}
}
