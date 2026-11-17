package tui

import (
	"strconv"

	"csolve/internal/solve"
)

// Input modes of the solver.
const (
	modeDemo  = "demo"
	modeData  = "json"
	modeBatch = "batch"
)

// Cross-section table sources.
const (
	tablesBuiltin  = "builtin"
	tablesExternal = "external"
)

// state holds the form values. Numeric values are kept as text so that they are
// validated both interactively (huh) and, finally, by solve.ParseArgs.
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

	degree   int
	nBasis   string
	r0Min    string
	r0Max    string
	nr0      string
	r1Min    string
	r1Max    string
	nr1      string
	minRatio string
	best     string

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

func newState() *state {
	s := &state{def: solve.DefaultArgs()}
	s.reset()
	return s
}

// reset restores the solver defaults.
func (s *state) reset() {
	d := s.def

	s.mode = modeDemo
	s.dataPath = ""
	s.batchPath = ""

	s.tablesKind = tablesBuiltin
	s.builtinType = d.Type
	s.tablesDir = ""
	s.tableExtrapolate = false
	s.tableRefWl = formatFloat(d.TableRefWl)
	s.tableCback = "dsigma"

	s.degree = 1
	if len(d.Degrees) > 0 {
		s.degree = d.Degrees[0]
	}
	s.nBasis = strconv.Itoa(d.NBasis)
	s.r0Min = formatFloat(d.R0Min)
	s.r0Max = formatFloat(d.R0Max)
	s.nr0 = strconv.Itoa(d.NR0)
	s.r1Min = formatFloat(d.R1Min)
	s.r1Max = formatFloat(d.R1Max)
	s.nr1 = strconv.Itoa(d.NR1)
	s.minRatio = formatFloat(d.MinRatio)
	s.best = formatFloat(d.Best)

	s.gammaSelect = "fixed"
	s.gammaRel = formatFloat(d.GammaRel)
	s.gammaMin = formatFloat(d.GammaMin)
	s.gammaMax = formatFloat(d.GammaMax)
	s.nGamma = strconv.Itoa(d.NGamma)

	s.nonneg = false
	s.noZeroBound = false
	s.noSmooth = false
	s.smoothLam = formatFloat(d.SmoothLam)
	s.rSplit = formatFloat(d.RSplit)
	s.modes = "auto"
	s.absolute = false

	s.depolCons = false
	s.depolW = formatFloat(d.DepolRatioW)
	s.depolMode = "func"

	s.lidarCons = false
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
	s.plotting = "internal"
	s.plotFormat = "png"
	s.plotDpi = formatFloat(d.PlotDpi)
	s.plotFontSize = formatFloat(d.PlotFontSize)
	s.noPlot = false
}

// readyToRun reports whether the current values can be launched without
// reviewing the full form (used by quick presets).
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

// bindings returns pointers to every form value so that huh can detect changes
// and re-evaluate dynamic descriptions.
func (s *state) bindings() []any {
	return []any{
		&s.mode, &s.dataPath, &s.batchPath,
		&s.tablesKind, &s.builtinType, &s.tablesDir, &s.tableExtrapolate,
		&s.tableRefWl, &s.tableCback,
		&s.degree, &s.nBasis,
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

// preset is a ready-made parameter set shown on the first screen.
type preset struct {
	key   string
	name  string
	desc  string
	quick bool // run without reviewing the full form
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
