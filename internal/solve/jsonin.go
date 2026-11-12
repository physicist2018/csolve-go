package solve

import (
	"encoding/json"
	"os"
)

type jsonMFloat struct {
	Real *float64 `json:"real"`
	Imag *float64 `json:"imag"`
}

type jsonGroup struct {
	WavelengthsNm []float64 `json:"wavelengths_nm"`
	Coefficients  []float64 `json:"coefficients"`
}

type jsonInput struct {
	M              *jsonMFloat `json:"m"`
	Backscatter    *jsonGroup  `json:"backscatter"`
	Extinction     *jsonGroup  `json:"extinction"`
	Depolarization *jsonGroup  `json:"depolarization"`
}

// loadJSONInput — чтение измерений из JSON (структура как в Python-версии).
// Дополнительно возвращаются измеренные значения δ_l (проценты) и их длины
// волн (µm) для --depol-ratio-cons.
func loadJSONInput(path string, trueRe, trueIm *float64) ([]channel, []float64,
	[]float64, []float64) {
	text, err := os.ReadFile(path)
	if err != nil {
		fatal("failed to open file: %s", path)
	}
	var root jsonInput
	if err := json.Unmarshal(text, &root); err != nil {
		fatal("failed to parse input JSON %s: %v", path, err)
	}
	if root.M != nil {
		if root.M.Real != nil {
			*trueRe = *root.M.Real
		}
		if root.M.Imag != nil {
			*trueIm = *root.M.Imag
		}
	}

	chans := []channel{}
	g1 := []float64{}
	add := func(grp *jsonGroup, kind chKind) {
		if grp == nil {
			return
		}
		cnt := len(grp.WavelengthsNm)
		if len(grp.Coefficients) < cnt {
			cnt = len(grp.Coefficients)
		}
		for i := 0; i < cnt; i++ {
			if len(chans) >= 32 {
				fatal("too many channels in input data")
			}
			chans = append(chans, channel{
				kind:       kind,
				wlUm:       grp.WavelengthsNm[i] * 1e-3,
				tab:        nil,
				refWlUm:    tableRefWlUm,
				cbackScale: 1.0,
			})
			g1 = append(g1, grp.Coefficients[i])
		}
	}
	add(root.Backscatter, chBeta)
	add(root.Extinction, chAlpha)
	if len(chans) == 0 {
		fatal("no channels in data")
	}

	depWl := []float64{}
	depVal := []float64{}
	if grp := root.Depolarization; grp != nil {
		cnt := len(grp.WavelengthsNm)
		if len(grp.Coefficients) < cnt {
			cnt = len(grp.Coefficients)
		}
		for i := 0; i < cnt; i++ {
			depWl = append(depWl, grp.WavelengthsNm[i]*1e-3)
			depVal = append(depVal, grp.Coefficients[i])
		}
	}
	return chans, g1, depWl, depVal
}
