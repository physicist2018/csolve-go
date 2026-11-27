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

package gui

import "csolve/internal/solve"

// Представление опций решателя для GUI: группы, поля (тип, диапазон, варианты,
// подсказки на двух языках), пресеты, значения по умолчанию. Поля — это
// флаги CLI; значения по умолчанию берутся из solve.DefaultArgs, поэтому не
// дублируются и не расходятся с CLI/TUI. Итоговая валидация всегда выполняется
// через solve.ParseArgs.

// Field — опция CLI, доступная в форме.
type Field struct {
	Key      string   `json:"key"`
	Group    string   `json:"group"`
	Type     string   `json:"type"` // bool|int|float|enum|optional
	Min      float64  `json:"min,omitempty"`
	Max      float64  `json:"max,omitempty"`
	Step     float64  `json:"step,omitempty"`
	Options  []string `json:"options,omitempty"`
	EN       string   `json:"label_en"`
	RU       string   `json:"label_ru"`
	HelpEN   string   `json:"help_en"`
	HelpRU   string   `json:"help_ru"`
	FalseKey string   `json:"false_key,omitempty"` // флаг для отрицания bool-поля
	Default  any      `json:"default"`
	// Visible — условия показа (AND по ключам, OR внутри массива значения).
	Visible map[string][]string `json:"visible,omitempty"`
}

// Group — раздел формы.
type Group struct {
	ID    string `json:"id"`
	EN    string `json:"label_en"`
	RU    string `json:"label_ru"`
	Order int    `json:"order"`
}

// Preset — готовый набор параметров на главном экране.
type Preset struct {
	Key    string         `json:"key"`
	EN     string         `json:"label_en"`
	RU     string         `json:"label_ru"`
	DescEN string         `json:"desc_en"`
	DescRU string         `json:"desc_ru"`
	Values map[string]any `json:"values"` // частичные значения (options + top-поля)
}

// spec — срез всех полей опций в порядке отображения.
var fields = []Field{
	// Tables
	{Key: "table-extrapolate", Group: "tables", Type: "bool",
		EN: "Allow table extrapolation", RU: "Разрешить экстраполяцию таблиц",
		HelpEN: "--table-extrapolate: allow cross-section extrapolation in x.",
		HelpRU: "--table-extrapolate: разрешить экстраполяцию сечений по x."},
	{Key: "table-ref-lambda-um", Group: "tables", Type: "float", Min: 0, Step: 0.001,
		EN: "Reference wavelength, µm", RU: "Опорная длина волны, мкм",
		HelpEN: "--table-ref-lambda-um: reference wavelength of the tables, µm.",
		HelpRU: "--table-ref-lambda-um: опорная длина волны таблиц, мкм."},
	{Key: "table-cback", Group: "tables", Type: "enum", Options: []string{"dsigma", "4pi"},
		EN: "Cback convention", RU: "Соглашение столбца Cback",
		HelpEN: "--table-cback: Cback column convention (dsigma or 4π·dσ/dΩ).",
		HelpRU: "--table-cback: конвенция столбца Cback (dsigma или 4π·dσ/dΩ)."},

	// Sphere/nonsphere mixture (--type sphere+spheroid | sphere+agglomerate)
	{Key: "sphere-frac", Group: "mixture", Type: "optional",
		Visible: map[string][]string{"tables_kind": {"builtin"}, "builtin_type": {"sphere+spheroid", "sphere+agglomerate"}},
		EN:      "Fixed sphere fraction sfrac (empty = scan)",
		RU:      "Фиксированная доля сфер sfrac (пусто — скан)",
		HelpEN:  "--sphere-frac: fix sfrac (0..1) instead of scanning; mixture types only.",
		HelpRU:  "--sphere-frac: зафиксировать sfrac (0..1) вместо сканирования; только sphere+X."},
	{Key: "sfrac-min", Group: "mixture", Type: "float", Min: 0, Max: 1, Step: 0.05,
		Visible: map[string][]string{"tables_kind": {"builtin"}, "builtin_type": {"sphere+spheroid", "sphere+agglomerate"}},
		EN:      "sfrac scan min", RU: "Доля сфер: минимум скана",
		HelpEN: "--sfrac-min: lower bound of the scanned sphere fraction (0..1).",
		HelpRU: "--sfrac-min: нижняя граница сканирования доли сфер (0..1)."},
	{Key: "sfrac-max", Group: "mixture", Type: "float", Min: 0, Max: 1, Step: 0.05,
		Visible: map[string][]string{"tables_kind": {"builtin"}, "builtin_type": {"sphere+spheroid", "sphere+agglomerate"}},
		EN:      "sfrac scan max", RU: "Доля сфер: максимум скана",
		HelpEN: "--sfrac-max: upper bound of the scanned sphere fraction (0..1).",
		HelpRU: "--sfrac-max: верхняя граница сканирования доли сфер (0..1)."},
	{Key: "n-sfrac", Group: "mixture", Type: "int", Min: 1, Step: 1,
		Visible: map[string][]string{"tables_kind": {"builtin"}, "builtin_type": {"sphere+spheroid", "sphere+agglomerate"}},
		EN:      "sfrac scan points", RU: "Число точек скана доли сфер",
		HelpEN: "--n-sfrac: number of scanned sphere-fraction values (>= 1).",
		HelpRU: "--n-sfrac: число значений доли сфер при сканировании (>= 1)."},

	// Method and grid
	{Key: "method", Group: "method", Type: "enum", Options: []string{"monotone", "spline"},
		EN: "Retrieval method", RU: "Метод восстановления",
		HelpEN: "--method: monotone V(r) reconstruction or B-spline expansion.",
		HelpRU: "--method: монотонная V(r) по бинам или B-сплайны."},
	{Key: "v-bins", Group: "method", Type: "int", Min: 0, Max: 64, Step: 1,
		Visible: map[string][]string{"method": {"monotone"}},
		EN:      "Bins per window", RU: "Бинов на окно",
		HelpEN: "--v-bins: number of bins on [r0,r1] for monotone; 0 = auto.",
		HelpRU: "--v-bins: число бинов на окно для monotone; 0 = авто."},
	{Key: "reg-order", Group: "method", Type: "enum", Options: []string{"0", "1", "2"},
		Visible: map[string][]string{"method": {"monotone"}},
		EN:      "Smoothness penalty order", RU: "Порядок штрафа за негладкость",
		HelpEN: "--reg-order: smoothness penalty on bin volumes (0..2).",
		HelpRU: "--reg-order: порядок штрафа за негладкость приращений (0..2)."},
	{Key: "degree", Group: "method", Type: "int", Min: 1, Max: 4, Step: 1,
		Visible: map[string][]string{"method": {"spline"}},
		EN:      "B-spline degree", RU: "Степень B-сплайна",
		HelpEN: "--degree: B-spline degree (1..4), spline only.",
		HelpRU: "--degree: степень B-сплайна (1..4), только spline."},
	{Key: "n-basis", Group: "method", Type: "int", Min: 0, Step: 1,
		Visible: map[string][]string{"method": {"spline"}},
		EN:      "Basis functions per window", RU: "Базисных функций на окно",
		HelpEN: "--n-basis: basis functions per window; 0 = number of channels.",
		HelpRU: "--n-basis: базисных функций на окно; 0 = по числу каналов."},
	{Key: "kernel-order", Group: "method", Type: "enum", Options: []string{"1", "2", "3", "4"},
		EN: "Kernel order", RU: "Порядок ядра",
		HelpEN: "--kernel-order: distribution to solve for (numerical stability), 1..4.",
		HelpRU: "--kernel-order: распределение, относительно которого решается задача (устойчивость), 1..4."},

	// Size grid
	{Key: "r0-min", Group: "grid", Type: "float", Min: 0, Step: 0.001,
		EN: "r0 grid min, µm", RU: "Сетка r0, минимум, мкм", HelpEN: "--r0-min.",
		HelpRU: "--r0-min: нижняя граница сетки r0, мкм (> 0)."},
	{Key: "r0-max", Group: "grid", Type: "float", Min: 0, Step: 0.001,
		EN: "r0 grid max, µm", RU: "Сетка r0, максимум, мкм", HelpEN: "--r0-max.",
		HelpRU: "--r0-max: верхняя граница сетки r0, мкм."},
	{Key: "nr0", Group: "grid", Type: "int", Min: 1, Step: 1,
		EN: "Number of r0 values", RU: "Число значений r0",
		HelpEN: "--nr0: number of r0 grid points (geometric).",
		HelpRU: "--nr0: число значений r0 (геометрическая сетка)."},
	{Key: "r1-min", Group: "grid", Type: "float", Min: 0, Step: 0.001,
		EN: "r1 grid min, µm", RU: "Сетка r1, минимум, мкм", HelpEN: "--r1-min.",
		HelpRU: "--r1-min: нижняя граница сетки r1, мкм."},
	{Key: "r1-max", Group: "grid", Type: "float", Min: 0, Step: 0.001,
		EN: "r1 grid max, µm", RU: "Сетка r1, максимум, мкм", HelpEN: "--r1-max.",
		HelpRU: "--r1-max: верхняя граница сетки r1, мкм."},
	{Key: "nr1", Group: "grid", Type: "int", Min: 1, Step: 1,
		EN: "Number of r1 values", RU: "Число значений r1",
		HelpEN: "--nr1: number of r1 grid points.", HelpRU: "--nr1: число значений r1."},
	{Key: "min-ratio", Group: "grid", Type: "float", Min: 0, Step: 0.1,
		EN: "Min r1/r0 ratio", RU: "Минимальное r1/r0",
		HelpEN: "--min-ratio: drop windows with r1 < X·r0.",
		HelpRU: "--min-ratio: отбрасывать окна с r1 < X·r0."},
	{Key: "best", Group: "grid", Type: "float", Min: 0, Step: 0.001,
		EN: "Best solutions to average", RU: "Лучших решений для усреднения",
		HelpEN: "--best: integer count or fraction in (0,1) of solutions.",
		HelpRU: "--best: целое число или доля в (0,1) решений для ансамбля."},
	{Key: "plot-best-limit", Group: "grid", Type: "int", Min: 0, Step: 1,
		EN: "Best solutions drawn on plots (0 = all)",
		RU: "Лучших решений на графиках (0 — все)",
		HelpEN: "--plot-best-limit: how many best-solution curves to draw; " +
			"0 = all. Mean and envelopes always use the whole ensemble.",
		HelpRU: "--plot-best-limit: сколько кривых лучших решений рисовать; " +
			"0 — все. Среднее и огибающие всегда по всему ансамблю."},

	// Regularization
	{Key: "gamma-select", Group: "reg", Type: "enum", Options: []string{"fixed", "cv", "err"},
		EN: "γ selection rule", RU: "Правило выбора γ",
		HelpEN: "--gamma-select: fixed | cv | err.", HelpRU: "--gamma-select: fixed | cv | err."},
	{Key: "gamma-rel", Group: "reg", Type: "float", Min: 0, Step: 0.01,
		EN: "Fixed γ = rel·‖A‖₂", RU: "Фиксированный γ = rel·‖A‖₂",
		HelpEN: "--gamma-rel: γ relative to ‖A‖₂ (fixed).",
		HelpRU: "--gamma-rel: γ относительно ‖A‖₂ (fixed)."},
	{Key: "gamma-min", Group: "reg", Type: "float", Step: 0.1,
		EN: "Min lg γ", RU: "Нижний lg γ", HelpEN: "--gamma-min.", HelpRU: "--gamma-min."},
	{Key: "gamma-max", Group: "reg", Type: "float", Step: 0.1,
		EN: "Max lg γ", RU: "Верхний lg γ", HelpEN: "--gamma-max.", HelpRU: "--gamma-max."},
	{Key: "ngamma", Group: "reg", Type: "int", Min: 1, Step: 1,
		EN: "Number of γ values", RU: "Число значений γ",
		HelpEN: "--ngamma: number of γ values on the log grid.",
		HelpRU: "--ngamma: число значений γ на лог-сетке."},

	// Constraints
	{Key: "nonneg", Group: "constraints", Type: "bool", FalseKey: "no-nonneg",
		Visible: map[string][]string{"method": {"spline"}},
		EN:      "Constrain C ≥ 0", RU: "Ограничение C ≥ 0",
		HelpEN: "--nonneg / --no-nonneg: nonnegativity (NNLS).",
		HelpRU: "--nonneg / --no-nonneg: неотрицательность (NNLS)."},
	{Key: "no-zero-boundary", Group: "constraints", Type: "bool",
		Visible: map[string][]string{"method": {"spline"}},
		EN:      "No zero boundary conditions", RU: "Без краевых условий",
		HelpEN: "--no-zero-boundary: drop n(r0)=n(r1)=0 (spline only).",
		HelpRU: "--no-zero-boundary: без краевых условий n(r0)=n(r1)=0 (spline)."},
	{Key: "no-smooth", Group: "constraints", Type: "bool",
		EN: "Disable mean smoothing", RU: "Отключить сглаживание среднего",
		HelpEN: "--no-smooth: disable smoothing of the mean solution.",
		HelpRU: "--no-smooth: отключить сглаживание среднего решения."},
	{Key: "smooth-lam", Group: "constraints", Type: "float", Min: 0, Step: 1e-5,
		EN: "Smoothing parameter", RU: "Параметр сглаживания",
		HelpEN: "--smooth-lam: smoothing spline parameter.",
		HelpRU: "--smooth-lam: параметр сглаживающего сплайна."},
	{Key: "r-split", Group: "constraints", Type: "float", Step: 0.05,
		EN: "Fine/coarse boundary, µm", RU: "Граница fine/coarse, мкм",
		HelpEN: "--r-split: fine/coarse boundary in µm.", HelpRU: "--r-split: граница fine/coarse, мкм."},
	{Key: "modes", Group: "constraints", Type: "enum", Options: []string{"auto", "fit", "stat"},
		EN: "Mode parameters method", RU: "Метод параметров мод",
		HelpEN: "--modes: auto | fit | stat.", HelpRU: "--modes: auto | fit | stat."},
	{Key: "absolute", Group: "constraints", Type: "bool",
		EN: "Absolute residuals", RU: "Абсолютные невязки",
		HelpEN: "--absolute: least squares on absolute residuals.",
		HelpRU: "--absolute: МНК по абсолютным невязкам."},
	{Key: "depol-ratio-cons", Group: "constraints", Type: "bool", FalseKey: "no-depol-ratio-cons",
		EN: "Depolarization δ_l constraint", RU: "Ограничение по δ_l",
		HelpEN: "--depol-ratio-cons / --no-depol-ratio-cons.",
		HelpRU: "--depol-ratio-cons / --no-depol-ratio-cons: учёт измеренного δ_l."},
	{Key: "depol-ratio-w", Group: "constraints", Type: "float", Min: 0, Step: 0.1,
		Visible: map[string][]string{"depol-ratio-cons": {"true"}},
		EN:      "δ_l constraint weight", RU: "Вес ограничения δ_l",
		HelpEN: "--depol-ratio-w: weight ≥ 0.", HelpRU: "--depol-ratio-w: вес ≥ 0."},
	{Key: "depol-ratio-mode", Group: "constraints", Type: "enum", Options: []string{"func", "rank"},
		Visible: map[string][]string{"depol-ratio-cons": {"true"}},
		EN:      "δ_l application mode", RU: "Способ учёта δ_l",
		HelpEN: "--depol-ratio-mode: func or rank.",
		HelpRU: "--depol-ratio-mode: func (в функционале) или rank (при ранжировании)."},

	// Numerics
	{Key: "dx-max", Group: "numerics", Type: "float", Min: 0, Step: 0.01,
		EN: "Max x step", RU: "Макс. шаг по x", HelpEN: "--dx-max.", HelpRU: "--dx-max."},
	{Key: "nr-out", Group: "numerics", Type: "int", Min: 2, Step: 1,
		EN: "Output grid points", RU: "Точек в выходной сетке",
		HelpEN: "--nr-out: points in the output r grid.", HelpRU: "--nr-out: точек в выходной сетке r."},
	{Key: "quad-order", Group: "numerics", Type: "int", Min: 1, Step: 1,
		EN: "Quadrature order", RU: "Порядок квадратуры",
		HelpEN: "--quad-order: Gauss–Legendre order.", HelpRU: "--quad-order: порядок квадратуры Гаусса–Лежандра."},
	{Key: "log-span", Group: "numerics", Type: "float", Min: 0, Step: 0.01,
		EN: "Max ln r subsegment", RU: "Макс. ширина подотрезка в ln r",
		HelpEN: "--log-span.", HelpRU: "--log-span."},

	// Demo
	{Key: "m-real", Group: "demo", Type: "optional",
		EN: "True Re(m) (demo)", RU: "Истинный Re(m) (демо)",
		HelpEN: "--m-real: true Re(m) for synthetic data.", HelpRU: "--m-real: истинный Re(m) для демо."},
	{Key: "m-imag", Group: "demo", Type: "optional",
		EN: "True Im(m) (demo)", RU: "Истинный Im(m) (демо)",
		HelpEN: "--m-imag: true Im(m) for synthetic data.", HelpRU: "--m-imag: истинный Im(m) для демо."},
	{Key: "noise", Group: "demo", Type: "float", Min: 0, Step: 0.001,
		EN: "Demo noise", RU: "Шум демо-данных",
		HelpEN: "--noise: relative Gaussian noise.", HelpRU: "--noise: относительный гауссов шум."},
	{Key: "seed", Group: "demo", Type: "int", Step: 1,
		EN: "RNG seed", RU: "Зерно ГПСЧ", HelpEN: "--seed.", HelpRU: "--seed: зерно ГПСЧ."},

	// Batch
	{Key: "batch-thin", Group: "batch", Type: "float", Min: 0, Step: 0.1,
		Visible: map[string][]string{"mode": {"batch"}},
		EN:      "Altitude averaging window, m", RU: "Окно усреднения по высоте, м",
		HelpEN: "--batch-thin: average input rows in W-metre windows (0 = off).",
		HelpRU: "--batch-thin: усреднение входных строк в окнах W м (0 = выкл)."},
	{Key: "lidar-ratio-cons", Group: "batch", Type: "bool",
		Visible: map[string][]string{"mode": {"batch"}},
		EN:      "Constrain S = α/β smoothness", RU: "Ограничение гладкости S = α/β",
		HelpEN: "--lidar-ratio-cons: constrain S along altitude.",
		HelpRU: "--lidar-ratio-cons: ограничить гладкость S по высоте."},
	{Key: "lidar-ratio-w", Group: "batch", Type: "float", Min: 0, Step: 0.1,
		Visible: map[string][]string{"mode": {"batch"}, "lidar-ratio-cons": {"true"}},
		EN:      "S deviation penalty weight", RU: "Вес штрафа за отклонение S",
		HelpEN: "--lidar-ratio-w: weight ≥ 0.", HelpRU: "--lidar-ratio-w: вес ≥ 0."},
	{Key: "lidar-ratio-iter", Group: "batch", Type: "int", Min: 1, Step: 1,
		Visible: map[string][]string{"mode": {"batch"}, "lidar-ratio-cons": {"true"}},
		EN:      "Reference S iterations", RU: "Итераций уточнения опорного S",
		HelpEN: "--lidar-ratio-iter: iterations ≥ 1.", HelpRU: "--lidar-ratio-iter: итераций ≥ 1."},

	// Spectra
	{Key: "spectra", Group: "spectra", Type: "bool", FalseKey: "no-spectra",
		EN: "Compute spectra", RU: "Считать спектры",
		HelpEN: "--spectra / --no-spectra: α(λ), β(λ), S(λ), δ_l(λ).",
		HelpRU: "--spectra / --no-spectra: α(λ), β(λ), S(λ), δ_l(λ)."},
	{Key: "spectrum-points", Group: "spectra", Type: "int", Min: 2, Max: 512, Step: 1,
		Visible: map[string][]string{"spectra": {"true"}},
		EN:      "Spectrum grid points", RU: "Точек в сетке спектров",
		HelpEN: "--spectrum-points: 2..512 wavelengths.", HelpRU: "--spectrum-points: 2..512 длин волн."},
	{Key: "spectrum-wl-min", Group: "spectra", Type: "float", Min: 0, Step: 1,
		Visible: map[string][]string{"spectra": {"true"}},
		EN:      "Spectrum λ min, nm", RU: "Спектр, нижняя граница, нм",
		HelpEN: "--spectrum-wl-min: nm, 0 = auto.", HelpRU: "--spectrum-wl-min: нм, 0 = авто."},
	{Key: "spectrum-wl-max", Group: "spectra", Type: "float", Min: 0, Step: 1,
		Visible: map[string][]string{"spectra": {"true"}},
		EN:      "Spectrum λ max, nm", RU: "Спектр, верхняя граница, нм",
		HelpEN: "--spectrum-wl-max: nm, 0 = auto.", HelpRU: "--spectrum-wl-max: нм, 0 = авто."},
}

var groups = []Group{
	{ID: "tables", EN: "Cross-section tables", RU: "Таблицы сечений", Order: 1},
	{ID: "mixture", EN: "Sphere/nonsphere mixture", RU: "Смесь сфер и несфер", Order: 2},
	{ID: "method", EN: "Method and basis", RU: "Метод и базис", Order: 3},
	{ID: "grid", EN: "Size grid", RU: "Сетка размеров", Order: 4},
	{ID: "reg", EN: "Regularization", RU: "Регуляризация", Order: 5},
	{ID: "constraints", EN: "Constraints and depolarization", RU: "Ограничения и деполяризация", Order: 6},
	{ID: "numerics", EN: "Numerics", RU: "Численные параметры", Order: 7},
	{ID: "demo", EN: "Demo data", RU: "Демо-данные", Order: 8},
	{ID: "batch", EN: "Batch", RU: "Пакетный режим", Order: 9},
	{ID: "spectra", EN: "Spectra", RU: "Спектры", Order: 10},
}

// presetList — быстрые пресеты первого экрана.
var presetList = []Preset{
	{Key: "demo-spheres", EN: "Demo · spheres, small particles", RU: "Демо · сферы, мелкие частицы",
		DescEN: "Synthetic two-mode data, built-in sphere tables, r = 0.05–1.9 µm.",
		DescRU: "Синтетические двухмодальные данные, встроенные таблицы сфер, r = 0.05–1.9 мкм.",
		Values: map[string]any{"mode": "demo", "tables_kind": "builtin", "builtin_type": "sphere",
			"table-extrapolate": true, "r0-min": 0.05, "r0-max": 0.3, "r1-min": 0.5, "r1-max": 1.9}},
	{Key: "demo-agglomerates", EN: "Demo · agglomerates with δ_l", RU: "Демо · агрегаты с δ_l",
		DescEN: "Built-in agglomerate tables, r = 0.1–0.6 µm; depolarization is reported.",
		DescRU: "Встроенные таблицы агрегатов, r = 0.1–0.6 мкм; считается деполяризация.",
		Values: map[string]any{"mode": "demo", "tables_kind": "builtin", "builtin_type": "agglomerate",
			"table-extrapolate": true, "r1-min": 0.1, "r1-max": 0.6}},
	{Key: "single-json", EN: "Single retrieval from JSON", RU: "Одиночное восстановление из JSON",
		DescEN: "Measured β/α from a JSON file; review all parameters.",
		DescRU: "Измеренные β/α из JSON-файла; параметры можно менять.",
		Values: map[string]any{"mode": "json"}},
	{Key: "batch-txt", EN: "Batch profile from TXT", RU: "Пакетный профиль из TXT",
		DescEN: "Coefficients by altitude from a TXT file; review all parameters.",
		DescRU: "Коэффициенты по высотам из TXT-файла; параметры можно менять.",
		Values: map[string]any{"mode": "batch"}},
	{Key: "custom", EN: "Custom — start from defaults", RU: "Свой вариант — из значений по умолчанию",
		DescEN: "Review every parameter; the synthetic demo input is the default.",
		DescRU: "Можно менять все параметры; по умолчанию — синтетические демо-данные."},
}

// fieldByKey — поиск поля по ключу.
func fieldByKey(key string) *Field {
	for i := range fields {
		if fields[i].Key == key {
			return &fields[i]
		}
	}
	return nil
}

// defaultValues — значения по умолчанию опций из решателя (истинный источник).
func defaultValues() map[string]any {
	a, _ := solve.DefaultArgs()
	cback := "dsigma"
	if a.TableCback4pi {
		cback = "4pi"
	}
	deg := 1
	if len(a.Degrees) > 0 {
		deg = a.Degrees[0]
	}
	return map[string]any{
		"table-extrapolate":   a.TableExtrapolate,
		"table-ref-lambda-um": a.TableRefWl,
		"table-cback":         cback,
		"sphere-frac":         "",
		"sfrac-min":           a.SFracMin,
		"sfrac-max":           a.SFracMax,
		"n-sfrac":             a.NSfrac,
		"method":              string(a.Method),
		"v-bins":              a.VBins,
		"reg-order":           string(rune('0' + a.RegOrder)),
		"degree":              deg,
		"n-basis":             a.NBasis,
		"kernel-order":        string(rune('0' + a.KernelOrder)),
		"r0-min":              a.R0Min, "r0-max": a.R0Max, "nr0": a.NR0,
		"r1-min": a.R1Min, "r1-max": a.R1Max, "nr1": a.NR1,
		"min-ratio": a.MinRatio, "best": a.Best,
		"plot-best-limit": a.PlotBestLimit,
		"gamma-select":    string(a.GammaSelect), "gamma-rel": a.GammaRel,
		"gamma-min": a.GammaMin, "gamma-max": a.GammaMax, "ngamma": a.NGamma,
		"nonneg":           a.Nonneg,
		"no-zero-boundary": a.NoZeroBoundary,
		"no-smooth":        a.NoSmooth,
		"smooth-lam":       a.SmoothLam, "r-split": a.RSplit,
		"modes": string(a.Modes), "absolute": a.Absolute,
		"depol-ratio-cons": a.DepolRatioCons, "depol-ratio-w": a.DepolRatioW,
		"depol-ratio-mode": string(a.DepolRatioMode),
		"dx-max":           a.DxMax, "nr-out": a.NrOut, "quad-order": a.QuadOrder,
		"log-span": a.LogSpan,
		"m-real":   "", "m-imag": "", "noise": a.Noise, "seed": a.Seed,
		"batch-thin":       a.BatchThin,
		"lidar-ratio-cons": a.LidarRatioCons,
		"lidar-ratio-w":    a.LidarRatioW,
		"lidar-ratio-iter": a.LidarRatioIter,
		"spectra":          a.Spectra,
		"spectrum-points":  a.SpectrumPoints,
		"spectrum-wl-min":  a.SpectrumWlMin,
		"spectrum-wl-max":  a.SpectrumWlMax,
	}
}

// tableName — названия наборов таблиц (--type).
func tableTypes() []string { return solve.TableTypes() }

func defaultTableType() string {
	a, _ := solve.DefaultArgs()
	return a.Type
}
