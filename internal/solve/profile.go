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
)

type profile struct {
	nr         int
	rGrid      []float64
	nAvg       []float64
	nAvgRaw    []float64
	nLow       []float64 // нижняя огибающая по лучшим решениям
	nHigh      []float64 // верхняя огибающая по лучшим решениям
	vCum       []float64 // кумулятивный объём V(r) = ∫ n dln r, µm³/cm³
	nBest      [][]float64
	nBestCount int
	mRe, mIm   float64
	gFit       []float64
	moments    moments
	modes      modes
	count      []float64
	deltaL     []float64 // δ_l = (S11-S22)/(S11+S22)*100, %
	deltaLwl   []float64 // длины волн δ_l, µm
	spectra    *spectra  // α(λ), β(λ), δ_l(λ) по решениям ансамбля

	// Доля сфер в режиме смеси (NaN вне смеси): сначала по концентрации, затем
	// пересчёт в объёмную долю (для объёмно-эквивалентных радиусов совпадает).
	sfracMix     bool
	sfracNum     float64
	sfracNumLow  float64
	sfracNumHigh float64
	sfracVol     float64
	sfracVolLow  float64
	sfracVolHigh float64
}

// mEps — допуск сравнения m: значения приходят из имён каталогов таблиц,
// поэтому точное == ненадёжно, а шаг сетки m на порядки больше.
const mEps = 1e-6

// mIndex — индекс пары (re, im) в сетке m; -1, если пары нет.
func mIndex(mre, mim []float64, re, im float64) int {
	for i := range mre {
		if math.Abs(mre[i]-re) <= mEps && math.Abs(mim[i]-im) <= mEps {
			return i
		}
	}
	return -1
}

// profileFinalize — итоговый профиль одной высоты (общий код одиночного и
// пакетного режимов). ds — ядра S11/S22 для δ_l (nil — не считаются);
// src — источник таблиц для спектров α(λ)/β(λ); g1 — измеренные
// коэффициенты каналов ch; depWl/depVal — измеренная δ_l для маркеров;
// mix — данные смеси (nil вне режима смеси).
func profileFinalize(a *Args, ch []channel, kts []*ktable, mre, mim []float64,
	sols *solutions, geom []*geom, rMin, rMax float64, ds *depolSet,
	src *tableSource, g1, depWl, depVal []float64, mix *mixtureData) (*profile, error) {
	_ = ch
	p := &profile{}
	nr := a.NrOut
	p.nr = nr
	p.rGrid = geomspace(rMin, rMax, nr)

	ens := ensembleAverage(sols, a.bestCount(sols.n), geom, p.rGrid, true)
	// Решение получено для выбранного порядка --kernel-order (y_k);
	// пересчитываем усреднённое распределение и все лучшие решения обратно
	// в dV/dln r до всякой обработки (сглаживание, моменты, графики).
	ko := normalizeKernelOrder(a.KernelOrder)
	applyKernelToVolume(ko, p.rGrid, ens.avg)
	applyKernelToVolume(ko, p.rGrid, ens.low)
	applyKernelToVolume(ko, p.rGrid, ens.high)
	for t := range ens.mat {
		applyKernelToVolume(ko, p.rGrid, ens.mat[t])
	}
	p.nBest = ens.mat
	p.nAvgRaw = ens.avg
	p.nAvg = append([]float64(nil), ens.avg...)
	p.nLow = ens.low
	p.nHigh = ens.high
	p.mRe = ens.mRe
	p.mIm = ens.mIm
	if ens.mix {
		p.sfracMix = true
		p.sfracNum = ens.sfracMean
		p.sfracNumLow = ens.sfracLow
		p.sfracNumHigh = ens.sfracHigh
		// Пересчёт доли по концентрации в объёмную долю. Для встроенных
		// наборов r — объёмно-эквивалентный радиус, поэтому средние объёмы
		// сфер и несфер одинаковы и объёмная доля совпадает с числовой.
		p.sfracVol = volumeFraction(ens.sfracMean)
		p.sfracVolLow = volumeFraction(ens.sfracLow)
		p.sfracVolHigh = volumeFraction(ens.sfracHigh)
	}
	if !a.NoSmooth {
		smoothDistribution(p.rGrid, p.nAvg, ens.count, a.SmoothLam)
		// монотонный метод по построению даёт n ≥ 0; сглаживающий сплайн
		// может дать небольшой отрицательный выброс у нуля — ограничиваем
		if sols.method == methodMonotone {
			clampNonNegative(p.nAvg)
		}
	}
	// огибающие не сглаживаются, но для монотонного метода отрицательные
	// значения — только ошибка округления NNLS
	if sols.method == methodMonotone {
		clampNonNegative(p.nLow)
		clampNonNegative(p.nHigh)
	}
	// кумулятивный объём строится по итоговому среднему n ≥ 0, поэтому V(r)
	// монотонна и V(r_end) совпадает с C_v
	p.vCum = cumulativeVolume(p.rGrid, p.nAvg)

	G := a.bestCount(sols.n)
	p.nBestCount = G

	bestIdx := make([]int, G)
	for t := 0; t < G; t++ {
		bestIdx[t] = sols.sol[t].mIdx
		if bestIdx[t] < 0 || bestIdx[t] >= len(kts) {
			return nil, fmt.Errorf("internal error: solution kernel index %d is out of range",
				bestIdx[t])
		}
	}

	rq, wq := quadratureGrid([]float64{rMin, rMax}, a.QuadOrder, a.LogSpan)
	navg0 := make([]float64, nr)
	for i := 0; i < nr; i++ {
		if isFinite(p.nAvg[i]) {
			navg0[i] = p.nAvg[i]
		}
	}
	nqVals := make([]float64, len(rq))
	for k := range rq {
		nqVals[k] = interpClamped(rq[k], p.rGrid, navg0)
	}

	nch := len(sols.sol[0].gFit)
	p.gFit = make([]float64, nch)
	for j := 0; j < nch; j++ {
		s := 0.0
		for k := range rq {
			ksum := 0.0
			for t := 0; t < G; t++ {
				ksum += ktableKernel(kts[bestIdx[t]], j, rq[k])
			}
			kavg := ksum / float64(G)
			r := rq[k]
			q := 3.0 / (4.0 * math.Pi * r * r * r * r)
			s += wq[k] * kavg * q * nqVals[k]
		}
		p.gFit[j] = s
	}

	// δ_l — свёртка S11/S22 с тем же средним распределением
	if depolSetAvailable(ds) {
		p.deltaLwl = append([]float64(nil), ds.wl...)
		_, _, p.deltaL = depolFromProfile(ds, bestIdx, rq, wq, nqVals)
	}

	// спектры α(λ), β(λ), δ_l(λ) по каждому решению ансамбля
	if mix != nil {
		p.spectra = buildSpectraMix(a, sols, geom, rq, wq, ds, mix, ch, g1,
			depWl, depVal)
	} else {
		p.spectra = buildSpectra(a, sols, geom, rq, wq, ds, src, ch, g1, depWl,
			depVal)
	}
	// «Полые» расчётные точки — по средней функции распределения n_avg,
	// посчитанной с усреднённым по лучшим G решениям ядром (так же, как gFit):
	// g(λ) = ∫ [avg_t K_t(λ,r)] q(r) n_avg(r) dln r.
	meanModels := make([]spectModel, 0, G)
	for t := 0; t < G; t++ {
		if mix != nil {
			ci := bestIdx[t]
			meanModels = append(meanModels, spectModel{sph: mix.sphTab[ci],
				non: mix.nonTab[ci], sfrac: mix.sfrac[ci]})
			continue
		}
		b := &sols.sol[t]
		tab, terr := tableCacheGet(src, b.mRe, b.mIm, a.TableExtrapolate)
		if terr != nil {
			continue
		}
		meanModels = append(meanModels, spectModel{non: tab})
	}
	if len(meanModels) > 0 {
		applyMeanDistributionComputed(p.spectra, a, src, mix, ch, depWl, rq, wq,
			nqVals, meanModels)
	}

	p.moments = sizeMoments(p.nAvg, p.rGrid)
	p.count = make([]float64, nr)
	for t := 0; t < G; t++ {
		s := &sols.sol[t]
		for i := 0; i < nr; i++ {
			if p.rGrid[i] >= s.r0 && p.rGrid[i] <= s.r1 {
				p.count[i] += 1.0
			}
		}
	}
	p.modes = modeAnalysis(p.rGrid, p.nAvg, a.RSplit, p.count, a.Modes)
	return p, nil
}
