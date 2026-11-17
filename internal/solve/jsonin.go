package solve

import (
	"encoding/json"
	"errors"
	"fmt"
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
func loadJSONInput(path string) (measurementInput, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return measurementInput{}, fmt.Errorf("failed to open file: %w", err)
	}
	var root jsonInput
	if err := json.Unmarshal(text, &root); err != nil {
		return measurementInput{}, fmt.Errorf("failed to parse input JSON %s: %w", path, err)
	}

	var in measurementInput
	if root.M != nil {
		in.mRe = root.M.Real
		in.mIm = root.M.Imag
	}

	add := func(grp *jsonGroup, kind chKind) error {
		if grp == nil {
			return nil
		}
		cnt := len(grp.WavelengthsNm)
		if len(grp.Coefficients) < cnt {
			cnt = len(grp.Coefficients)
		}
		for i := 0; i < cnt; i++ {
			if len(in.channels) >= 32 {
				return errors.New("too many channels in input data")
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
		return nil
	}
	if err := add(root.Backscatter, chBeta); err != nil {
		return measurementInput{}, err
	}
	if err := add(root.Extinction, chAlpha); err != nil {
		return measurementInput{}, err
	}
	if len(in.channels) == 0 {
		return measurementInput{}, errors.New("no channels in data")
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
	return in, nil
}
