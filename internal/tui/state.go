package tui

import (
	"strconv"

	"csolve/internal/solve"
)

// Режимы входа решателя.
const (
	modeDemo  = "demo"
	modeData  = "json"
	modeBatch = "batch"
)

// Источники таблиц сечений.
const (
	tablesBuiltin  = "builtin"
	tablesExternal = "external"
)

// state — значения формы. Числовые значения хранятся текстом, чтобы их
// проверяли и интерактивно (huh), и в конце — solve.ParseArgs.
type state struct {
	def *solve.Args

	mode      string
	dataPath  string
	batchPath string

	tablesKind       string
	builtinType      string
	tablesDir        string
	tableExtrapolate bool
	tableRefWl       string
	tableCback       string

	method      string
	degree      int
	nBasis      string
	vBins       string
	regOrder    int
	kernelOrder int
	r0Min       string
	r0Max       string
	nr0         string
	r1Min       string
	r1Max       string
	nr1         string
	minRatio    string
	best        string

	gammaSelect string
	gammaRel    string
	gammaMin    string
	gammaMax    string
	nGamma      string

	nonneg      bool
	noZeroBound bool
	noSmooth    bool
	smoothLam   string
	rSplit      string
	modes       string
	absolute    bool

	depolCons bool
	depolW    string
	depolMode string

	lidarCons bool
	lidarW    string
	lidarIter string

	mReal string
	mImag string
	noise string
	seed  string

	dxMax     string
	nrOut     string
	quadOrder string
	logSpan   string

	outDir       string
	out          string
	jsonOut      string
	plotting     string
	plotFormat   string
	plotDpi      string
	plotFontSize string
	noPlot       bool
}

func newState() (*state, error) {
	def, err := solve.DefaultArgs()
	if err != nil {
		return nil, err
	}
	s := &state{def: def}
	s.reset()
	return s, nil
}

// reset — возврат к значениям решателя по умолчанию.
func (s *state) reset() {
	d := s.def

	s.mode = modeDemo
	s.dataPath = ""
	s.batchPath = ""

	s.tablesKind = tablesBuiltin
	s.builtinType = d.Type
	s.tablesDir = ""
	s.tableExtrapolate = d.TableExtrapolate
	s.tableRefWl = formatFloat(d.TableRefWl)
	s.tableCback = "dsigma"
	if d.TableCback4pi {
		s.tableCback = "4pi"
	}

	s.method = string(d.Method)
	s.degree = 1
	if len(d.Degrees) > 0 {
		s.degree = d.Degrees[0]
	}
	s.nBasis = strconv.Itoa(d.NBasis)
	s.vBins = strconv.Itoa(d.VBins)
	s.regOrder = d.RegOrder
	s.kernelOrder = d.KernelOrder
	s.r0Min = formatFloat(d.R0Min)
	s.r0Max = formatFloat(d.R0Max)
	s.nr0 = strconv.Itoa(d.NR0)
	s.r1Min = formatFloat(d.R1Min)
	s.r1Max = formatFloat(d.R1Max)
	s.nr1 = strconv.Itoa(d.NR1)
	s.minRatio = formatFloat(d.MinRatio)
	s.best = formatFloat(d.Best)

	s.gammaSelect = string(d.GammaSelect)
	s.gammaRel = formatFloat(d.GammaRel)
	s.gammaMin = formatFloat(d.GammaMin)
	s.gammaMax = formatFloat(d.GammaMax)
	s.nGamma = strconv.Itoa(d.NGamma)

	s.nonneg = d.Nonneg
	s.noZeroBound = d.NoZeroBoundary
	s.noSmooth = d.NoSmooth
	s.smoothLam = formatFloat(d.SmoothLam)
	s.rSplit = formatFloat(d.RSplit)
	s.modes = string(d.Modes)
	s.absolute = d.Absolute

	s.depolCons = d.DepolRatioCons
	s.depolW = formatFloat(d.DepolRatioW)
	s.depolMode = string(d.DepolRatioMode)

	s.lidarCons = d.LidarRatioCons
	s.lidarW = formatFloat(d.LidarRatioW)
	s.lidarIter = strconv.Itoa(d.LidarRatioIter)

	s.mReal = "1.45"
	s.mImag = "-0.005"
	s.noise = "0"
	s.seed = strconv.Itoa(d.Seed)

	s.dxMax = formatFloat(d.DxMax)
	s.nrOut = strconv.Itoa(d.NrOut)
	s.quadOrder = strconv.Itoa(d.QuadOrder)
	s.logSpan = formatFloat(d.LogSpan)

	s.outDir = ""
	s.out = ""
	s.jsonOut = ""
	s.plotting = string(d.Plotting)
	s.plotFormat = string(d.PlotFormat)
	s.plotDpi = formatFloat(d.PlotDpi)
	s.plotFontSize = formatFloat(d.PlotFontSize)
	s.noPlot = d.NoPlot
}

// readyToRun — можно ли запускать без просмотра полной формы (быстрые
// пресеты).
func (s *state) readyToRun() bool {
	switch s.mode {
	case modeData:
		if s.dataPath == "" {
			return false
		}
	case modeBatch:
		if s.batchPath == "" {
			return false
		}
	}
	if s.tablesKind == tablesExternal && s.tablesDir == "" {
		return false
	}
	return true
}

// bindings — указатели на все значения формы, чтобы huh замечал изменения
// и пересчитывал динамические описания.
func (s *state) bindings() []any {
	return []any{
		&s.mode, &s.dataPath, &s.batchPath,
		&s.tablesKind, &s.builtinType, &s.tablesDir, &s.tableExtrapolate,
		&s.tableRefWl, &s.tableCback,
		&s.degree, &s.nBasis, &s.method, &s.vBins, &s.regOrder,
		&s.kernelOrder,
		&s.r0Min, &s.r0Max, &s.nr0, &s.r1Min, &s.r1Max, &s.nr1,
		&s.minRatio, &s.best,
		&s.gammaSelect, &s.gammaRel, &s.gammaMin, &s.gammaMax, &s.nGamma,
		&s.nonneg, &s.noZeroBound, &s.noSmooth, &s.smoothLam, &s.rSplit,
		&s.modes, &s.absolute,
		&s.depolCons, &s.depolW, &s.depolMode,
		&s.lidarCons, &s.lidarW, &s.lidarIter,
		&s.mReal, &s.mImag, &s.noise, &s.seed,
		&s.dxMax, &s.nrOut, &s.quadOrder, &s.logSpan,
		&s.outDir, &s.out, &s.jsonOut,
		&s.plotting, &s.plotFormat, &s.plotDpi, &s.plotFontSize, &s.noPlot,
	}
}

// preset — готовый набор параметров на первом экране.
type preset struct {
	key   string
	name  string
	desc  string
	quick bool // запуск без просмотра полной формы
	apply func(*state)
}

func presetList() []preset {
	return []preset{
		{
			key:   "demo-spheres",
			name:  "Demo · spheres, small particles",
			desc:  "Synthetic two-mode data, built-in sphere tables, r = 0.05–1.9 µm.",
			quick: true,
			apply: func(s *state) {
				s.mode = modeDemo
				s.tablesKind = tablesBuiltin
				s.builtinType = "sphere"
				s.tableExtrapolate = true
				s.r0Min, s.r0Max = "0.05", "0.3"
				s.r1Min, s.r1Max = "0.5", "1.9"
				s.outDir = "out"
			},
		},
		{
			key:   "demo-agglomerates",
			name:  "Demo · agglomerates with δ_l",
			desc:  "Built-in agglomerate tables, r = 0.1–0.6 µm; depolarization is reported.",
			quick: true,
			apply: func(s *state) {
				s.mode = modeDemo
				s.tablesKind = tablesBuiltin
				s.builtinType = "agglomerate"
				s.tableExtrapolate = true
				s.r1Min, s.r1Max = "0.1", "0.6"
				s.outDir = "out"
			},
		},
		{
			key:  "single-json",
			name: "Single retrieval from JSON",
			desc: "Measured β/α from a JSON file; review all parameters.",
			apply: func(s *state) {
				s.mode = modeData
				s.outDir = "out"
			},
		},
		{
			key:  "batch-txt",
			name: "Batch profile from TXT",
			desc: "Coefficients by altitude from a TXT file; review all parameters.",
			apply: func(s *state) {
				s.mode = modeBatch
				s.outDir = "out_batch"
			},
		},
		{
			key:  "custom",
			name: "Custom — start from defaults",
			desc: "Review every parameter; the synthetic demo input is the default.",
		},
	}
}

func presetByKey(key string) preset {
	for _, p := range presetList() {
		if p.key == key {
			return p
		}
	}
	return presetList()[len(presetList())-1]
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
