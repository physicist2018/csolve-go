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
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
)

// jfloat — число JSON; NaN/Inf кодируются как null (как в C-версии).
type jfloat float64

func (f jfloat) MarshalJSON() ([]byte, error) {
	v := float64(f)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatFloat(v, 'g', -1, 64)), nil
}

type outMAvg struct {
	Real jfloat `json:"real"`
	Imag jfloat `json:"imag"`
}

type outMoments struct {
	NCm3             jfloat `json:"N_cm-3"`
	VolMeanUm3       jfloat `json:"vol_mean_um3"`
	VolConcUm3PerCm3 jfloat `json:"vol_conc_um3_per_cm3"`
	REffUm           jfloat `json:"r_eff_um"`
	RCovMinUm        jfloat `json:"r_cov_min_um"`
	RCovMaxUm        jfloat `json:"r_cov_max_um"`
	CovFrac          jfloat `json:"cov_frac"`
}

type outUnits struct {
	Beta  string `json:"beta"`
	Alpha string `json:"alpha"`
}

type outChannel struct {
	Kind string `json:"kind"`
	WlNm jfloat `json:"wl_nm"`
	G1   jfloat `json:"g1"`
	GFit jfloat `json:"g_fit"`
}

type outSolution struct {
	Rank   int      `json:"rank"`
	MReal  jfloat   `json:"m_real"`
	MImag  jfloat   `json:"m_imag"`
	Degree int      `json:"degree"`
	Bins   int      `json:"bins,omitempty"`
	R0     jfloat   `json:"r0"`
	R1     jfloat   `json:"r1"`
	Gamma  jfloat   `json:"gamma"`
	Err    jfloat   `json:"err"`
	C      []jfloat `json:"C,omitempty"`
	VBins  []jfloat `json:"v_bins,omitempty"`
}

// outModel — параметры метода восстановления.
type outModel struct {
	Name     string `json:"name"`
	Bins     int    `json:"bins,omitempty"`
	RegOrder int    `json:"reg_order,omitempty"`
	// KernelOrder — порядок --kernel-order; распределение, относительно
	// которого решалась обратная задача (1=dN/dln r, 2=dS/dln r,
	// 3=dV/dln r, 4=d(r^4 N)/dln r).
	KernelOrder int `json:"kernel_order"`
	// KernelDistribution — обозначение этого распределения.
	KernelDistribution string `json:"kernel_distribution"`
	// KernelVolumeConversion — что означают коэффициенты решения при данном
	// порядке и как пересчитать приращения по бинам (v_bins) в приращения
	// объёма. n_avg и v_cum уже пересчитаны в dV/dln r.
	KernelVolumeConversion string `json:"kernel_volume_conversion"`
}

type outMode struct {
	V      jfloat `json:"V_um3_per_cm3"`
	Rm     jfloat `json:"rm_um"`
	Sigma  jfloat `json:"sigma"`
	OK     bool   `json:"ok"`
	Method string `json:"method"`
}

type outModes struct {
	Fine    outMode `json:"fine"`
	Coarse  outMode `json:"coarse"`
	Overall outMode `json:"overall"`
}

// outDepol — линейная деполяризация обратного рассеяния:
// δ_l = (S11-S22)/(S11+S22)*100 % при θ = 180°.
type outDepol struct {
	WlNm   []jfloat `json:"wl_nm"`
	DeltaL []jfloat `json:"delta_l_percent"`
}

// outSpectrumCurves — среднее и огибающие одной спектральной величины.
type outSpectrumCurves struct {
	Mean []jfloat `json:"mean"`
	Low  []jfloat `json:"low"`
	High []jfloat `json:"high"`
}

// outSpectrumPoint — измеренная точка спектральной зависимости.
type outSpectrumPoint struct {
	WlNm  jfloat `json:"wl_nm"`
	Value jfloat `json:"value"`
}

// outSpectraMeasured — измеренные входные данные для маркеров на спектрах.
type outSpectraMeasured struct {
	Alpha          []outSpectrumPoint `json:"alpha"`
	Beta           []outSpectrumPoint `json:"beta"`
	LidarRatio     []outSpectrumPoint `json:"lidar_ratio"`
	Depolarization []outSpectrumPoint `json:"depolarization"`
}

// outSpectra — α(λ), β(λ) и δ_l(λ): среднее и огибающие по ансамблю.
// Единицы: α — 1/m, β — 1/(m·sr), δ_l — %.
type outSpectra struct {
	WlNm           []jfloat           `json:"wl_nm"`
	Alpha          outSpectrumCurves  `json:"alpha"`
	Beta           outSpectrumCurves  `json:"beta"`
	LidarRatio     outSpectrumCurves  `json:"lidar_ratio"`
	Depolarization outSpectrumCurves  `json:"depolarization"`
	Measured       outSpectraMeasured `json:"measured"`
}

type outFile struct {
	Model          outModel      `json:"model"`
	MAvg           outMAvg       `json:"m_avg"`
	Moments        outMoments    `json:"moments"`
	UnitsInput     outUnits      `json:"units_input"`
	Channels       []outChannel  `json:"channels"`
	RUm            []jfloat      `json:"r_um"`
	NAvg           []jfloat      `json:"n_avg"`
	NLow           []jfloat      `json:"n_low"`
	NHigh          []jfloat      `json:"n_high"`
	VCum           []jfloat      `json:"v_cum_um3_per_cm3"`
	BestSolutions  []outSolution `json:"best_solutions"`
	NAvgRaw        []jfloat      `json:"n_avg_raw"`
	Modes          outModes      `json:"modes"`
	Depolarization outDepol      `json:"depolarization"`
	Spectra        *outSpectra   `json:"spectra,omitempty"`
}

func toJ(vals []float64) []jfloat {
	out := make([]jfloat, len(vals))
	for i, v := range vals {
		out[i] = jfloat(v)
	}
	return out
}

func toJPerCm3(vals []float64) []jfloat {
	out := make([]jfloat, len(vals))
	for i, v := range vals {
		out[i] = jfloat(v / cm3PerM3)
	}
	return out
}

func outModeFrom(m csMode) outMode {
	v := math.NaN()
	if isFinite(m.V) {
		v = m.V / cm3PerM3
	}
	return outMode{
		V:      jfloat(v),
		Rm:     jfloat(m.Rm),
		Sigma:  jfloat(m.Sigma),
		OK:     m.OK,
		Method: m.Method,
	}
}

// writeJSON — запись результатов (структура как в Python-версии).
func writeJSON(path string, sols *solutions, rGrid, nAvg, nAvgRaw, nLow,
	nHigh, vCum []float64, ch []channel, g1, gFit []float64, mAvgRe, mAvgIm float64,
	mom moments, md modes, deltaWlUm, deltaL []float64, sp *spectra) error {

	out := outFile{
		MAvg: outMAvg{Real: jfloat(mAvgRe), Imag: jfloat(mAvgIm)},
		Moments: outMoments{
			NCm3:             jfloat(mom.NCm3),
			VolMeanUm3:       jfloat(mom.VolMeanUm3),
			VolConcUm3PerCm3: jfloat(mom.VolConcUm3PerCm3),
			REffUm:           jfloat(mom.REffUm),
			RCovMinUm:        jfloat(mom.RCovMinUm),
			RCovMaxUm:        jfloat(mom.RCovMaxUm),
			CovFrac:          jfloat(mom.CovFrac),
		},
		UnitsInput: outUnits{Beta: "1/(m·sr)", Alpha: "1/m"},
		RUm:        toJ(rGrid),
		NAvg:       toJPerCm3(nAvg),
		NLow:       toJPerCm3(nLow),
		NHigh:      toJPerCm3(nHigh),
		VCum:       toJPerCm3(vCum),
		NAvgRaw:    toJPerCm3(nAvgRaw),
		Modes: outModes{
			Fine:    outModeFrom(md.Fine),
			Coarse:  outModeFrom(md.Coarse),
			Overall: outModeFrom(md.Overall),
		},
		Spectra: outSpectraFrom(sp),
	}
	out.Channels = make([]outChannel, len(ch))
	for j := range ch {
		k := "alpha"
		if ch[j].kind == chBeta {
			k = "beta"
		}
		out.Channels[j] = outChannel{
			Kind: k,
			WlNm: jfloat(ch[j].wlUm * 1e3),
			G1:   jfloat(g1[j]),
			GFit: jfloat(gFit[j]),
		}
	}
	out.BestSolutions = make([]outSolution, sols.n)
	for t := 0; t < sols.n; t++ {
		s := &sols.sol[t]
		out.BestSolutions[t] = outSolution{
			Rank:   t + 1,
			MReal:  jfloat(s.mRe),
			MImag:  jfloat(s.mIm),
			Degree: s.degree,
			Bins:   s.bins,
			R0:     jfloat(s.r0),
			R1:     jfloat(s.r1),
			Gamma:  jfloat(s.gamma),
			Err:    jfloat(s.err),
			C:      toJ(s.C),
		}
		if sols.method == methodMonotone {
			// для монотонного метода C — это приращения объёма по бинам
			out.BestSolutions[t].C = nil
			out.BestSolutions[t].VBins = toJPerCm3(s.C)
		}
	}

	ko := normalizeKernelOrder(sols.kernelOrder)
	model := outModel{
		KernelOrder:            ko,
		KernelDistribution:     kernelOrderLabel(ko),
		KernelVolumeConversion: kernelVolumeConversion(ko, sols.method),
	}
	if sols.method == methodMonotone {
		model.Name = string(sols.method)
		model.Bins = sols.bins
		model.RegOrder = sols.regOrder
	} else {
		model.Name = string(methodSpline)
	}
	out.Model = model

	if len(deltaWlUm) > 0 {
		wlNm := make([]float64, len(deltaWlUm))
		for i, wl := range deltaWlUm {
			wlNm[i] = wl * 1e3
		}
		out.Depolarization = outDepol{WlNm: toJ(wlNm), DeltaL: toJ(deltaL)}
	} else {
		out.Depolarization = outDepol{WlNm: []jfloat{}, DeltaL: []jfloat{}}
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to open file for writing: %w", err)
	}
	return nil
}

// outSpectraFrom — преобразование спектров в JSON-представление (nil, если
// спектры не считались).
func outSpectraFrom(sp *spectra) *outSpectra {
	if sp == nil {
		return nil
	}
	curves := func(s *spectrumSet) outSpectrumCurves {
		if s == nil {
			return outSpectrumCurves{Mean: []jfloat{}, Low: []jfloat{},
				High: []jfloat{}}
		}
		return outSpectrumCurves{Mean: toJ(s.mean), Low: toJ(s.low),
			High: toJ(s.high)}
	}
	points := func(pts []wlPoint) []outSpectrumPoint {
		out := make([]outSpectrumPoint, 0, len(pts))
		for _, p := range pts {
			out = append(out, outSpectrumPoint{WlNm: jfloat(p.wlNm),
				Value: jfloat(p.value)})
		}
		return out
	}
	meas := func(s *spectrumSet) []outSpectrumPoint {
		if s == nil {
			return []outSpectrumPoint{}
		}
		return points(s.meas)
	}
	return &outSpectra{
		WlNm:           toJ(sp.wlNm),
		Alpha:          curves(sp.alpha),
		Beta:           curves(sp.beta),
		LidarRatio:     curves(sp.lidar),
		Depolarization: curves(sp.delta),
		Measured: outSpectraMeasured{
			Alpha:          meas(sp.alpha),
			Beta:           meas(sp.beta),
			LidarRatio:     meas(sp.lidar),
			Depolarization: meas(sp.delta),
		},
	}
}
