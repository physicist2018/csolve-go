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

// measurementInput — измерения из входного JSON: каналы, коэффициенты и
// необязательный показатель преломления m (nil — если он не задан).
type measurementInput struct {
	channels []channel
	g1       []float64
	depWl    []float64 // длины волн измеренной δ_l, µm
	depVal   []float64 // измеренная δ_l, %
	mRe, mIm *float64
}

// loadJSONInput — чтение измерений из JSON (структура как в Python-версии).
func loadJSONInput(path string) measurementInput {
	text, err := os.ReadFile(path)
	if err != nil {
		fatal("failed to open file: %s", path)
	}
	var root jsonInput
	if err := json.Unmarshal(text, &root); err != nil {
		fatal("failed to parse input JSON %s: %v", path, err)
	}

	var in measurementInput
	if root.M != nil {
		in.mRe = root.M.Real
		in.mIm = root.M.Imag
	}

	add := func(grp *jsonGroup, kind chKind) {
		if grp == nil {
			return
		}
		cnt := len(grp.WavelengthsNm)
		if len(grp.Coefficients) < cnt {
			cnt = len(grp.Coefficients)
		}
		for i := 0; i < cnt; i++ {
			if len(in.channels) >= 32 {
				fatal("too many channels in input data")
			}
			in.channels = append(in.channels, channel{
				kind:       kind,
				wlUm:       grp.WavelengthsNm[i] * 1e-3,
				tab:        nil,
				refWlUm:    tableRefWlUm,
				cbackScale: 1.0,
			})
			in.g1 = append(in.g1, grp.Coefficients[i])
		}
	}
	add(root.Backscatter, chBeta)
	add(root.Extinction, chAlpha)
	if len(in.channels) == 0 {
		fatal("no channels in data")
	}

	if grp := root.Depolarization; grp != nil {
		cnt := len(grp.WavelengthsNm)
		if len(grp.Coefficients) < cnt {
			cnt = len(grp.Coefficients)
		}
		for i := 0; i < cnt; i++ {
			in.depWl = append(in.depWl, grp.WavelengthsNm[i]*1e-3)
			in.depVal = append(in.depVal, grp.Coefficients[i])
		}
	}
	return in
}
