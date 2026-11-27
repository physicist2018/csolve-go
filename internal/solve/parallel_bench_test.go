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
	"testing"
)

// benchWorkers — набор для сравнения: строго последовательно и auto.
func benchWorkers() []int {
	out := []int{1}
	if au := resolveJobs(0); au > 1 {
		out = append(out, au)
	}
	return out
}

// benchKtable — таблица ядра с заданным числом каналов (для бенчмарков).
func benchKtable(rMin, rMax float64, n, nch int,
	kf func(j int, r float64) float64) *ktable {
	r := geomspace(rMin, rMax, n)
	kt := &ktable{n: n, r: r, rMin: rMin, rMax: rMax, nch: nch,
		sp: make([]*spline, nch)}
	lr := make([]float64, n)
	for i := range r {
		lr[i] = math.Log(r[i])
	}
	for j := 0; j < nch; j++ {
		val := make([]float64, n)
		for i := range r {
			val[i] = math.Log(kf(j, r[i]))
		}
		kt.sp[j] = newSpline(lr, val)
	}
	return kt
}

// benchDesignInputs — вход для designBuild/scanVariants: 8 значений m,
// 5 каналов, окна 7×7.
func benchDesignInputs() (kts []*ktable, mRe, mIm, r0Grid, r1Grid []float64) {
	const nm, nch, nNodes = 8, 5, 300
	kts = make([]*ktable, nm)
	mRe = make([]float64, nm)
	mIm = make([]float64, nm)
	for i := 0; i < nm; i++ {
		mRe[i] = 1.3 + 0.05*float64(i)
		mIm[i] = 0.001 * float64(i)
		si := float64(i)
		kts[i] = benchKtable(0.05, 2.0, nNodes, nch, func(j int, r float64) float64 {
			return 1.0 + 0.3*float64(j) + 0.05*si*r
		})
	}
	r0Grid = geomspace(0.05, 0.3, 7)
	r1Grid = geomspace(0.45, 2.0, 7)
	return kts, mRe, mIm, r0Grid, r1Grid
}

func benchDesignSpec(workers int) designSpec {
	return designSpec{
		method:      methodMonotone,
		bins:        10,
		quadOrder:   24,
		maxLogSpan:  0.1,
		regOrder:    2,
		kernelOrder: 3,
		workers:     workers,
	}
}

// BenchmarkDesignBuild — сборка матриц A при разном числе воркеров.
func BenchmarkDesignBuild(b *testing.B) {
	const nch = 5
	kts, mRe, mIm, r0, r1 := benchDesignInputs()

	for _, w := range benchWorkers() {
		workers := w
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			spec := benchDesignSpec(workers)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := designBuild(nil, nch, kts, mRe, mIm, r0, r1, 0,
					spec); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkScanVariants — перебор решений (правило err, 21 γ) при разном
// числе воркеров; матрицы A собираются один раз до замера.
func BenchmarkScanVariants(b *testing.B) {
	const nch, nGamma = 5, 21
	kts, mRe, mIm, r0, r1 := benchDesignInputs()
	d, err := designBuild(nil, nch, kts, mRe, mIm, r0, r1, 0,
		benchDesignSpec(1))
	if err != nil {
		b.Fatalf("designBuild: %v", err)
	}
	g1 := make([]float64, nch)
	for i := range g1 {
		g1[i] = 1.0 + 0.1*float64(i)
	}
	c := testConsole()

	for _, w := range benchWorkers() {
		workers := w
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			d.workers = workers
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := scanVariants(d, g1, nch, gammaErr, 0.1, -7, 5,
					nGamma, true, true, nil, c); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
