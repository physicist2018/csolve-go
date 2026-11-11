package solve

import (
	"encoding/json"
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
	R0     jfloat   `json:"r0"`
	R1     jfloat   `json:"r1"`
	Gamma  jfloat   `json:"gamma"`
	Err    jfloat   `json:"err"`
	C      []jfloat `json:"C"`
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

type outFile struct {
	MAvg           outMAvg       `json:"m_avg"`
	Moments        outMoments    `json:"moments"`
	UnitsInput     outUnits      `json:"units_input"`
	Channels       []outChannel  `json:"channels"`
	RUm            []jfloat      `json:"r_um"`
	NAvg           []jfloat      `json:"n_avg"`
	BestSolutions  []outSolution `json:"best_solutions"`
	NAvgRaw        []jfloat      `json:"n_avg_raw"`
	Modes          outModes      `json:"modes"`
	Depolarization outDepol      `json:"depolarization"`
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
func writeJSON(path string, sols *solutions, rGrid, nAvg, nAvgRaw []float64,
	ch []channel, g1, gFit []float64, mAvgRe, mAvgIm float64,
	mom moments, md modes, deltaWlUm, deltaL []float64) {

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
		NAvgRaw:    toJPerCm3(nAvgRaw),
		Modes: outModes{
			Fine:    outModeFrom(md.Fine),
			Coarse:  outModeFrom(md.Coarse),
			Overall: outModeFrom(md.Overall),
		},
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
			R0:     jfloat(s.r0),
			R1:     jfloat(s.r1),
			Gamma:  jfloat(s.gamma),
			Err:    jfloat(s.err),
			C:      toJ(s.C),
		}
	}

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
		fatal("failed to encode JSON: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		fatal("failed to open file for writing: %s", path)
	}
}
