package solve

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// Args — разобранные параметры командной строки.
type Args struct {
	Data             string
	Tables           string
	Type             string
	TableExtrapolate bool
	TableRefWl       float64
	TableCback4pi    bool

	Degrees []int
	NBasis  int

	R0Min, R0Max float64
	NR0          int
	R1Min, R1Max float64
	NR1          int
	MinRatio     float64
	Best         int

	GammaMin, GammaMax float64
	NGamma             int
	GammaSelect        int
	GammaRel           float64

	Nonneg         bool
	NoZeroBoundary bool
	NoSmooth       bool
	SmoothLam      float64
	RSplit         float64
	Modes          int
	Absolute       bool

	DxMax     float64
	NrOut     int
	QuadOrder int
	LogSpan   float64

	HasMReal, HasMImag bool
	MReal, MImag       float64

	Noise float64
	Seed  int

	Out          string
	JSON         string
	NoPlot       bool
	PlotFormat   int
	PlotDpi      float64
	PlotFontSize float64

	Batch          string
	BatchOutDir    string
	LidarRatioCons bool
	LidarRatioW    float64
	LidarRatioIter int

	DepolRatioCons bool
	DepolRatioW    float64
	DepolRatioMode int
}

var errHelp = errors.New("help requested")

// singleIntFlag — целочисленный флаг, который принимает ровно одно значение
// (повтор флага — ошибка).
type singleIntFlag struct {
	set        bool
	value      int
	minV, maxV int
	name       string
}

func (f *singleIntFlag) String() string { return strconv.Itoa(f.value) }

func (f *singleIntFlag) Set(s string) error {
	if f.set {
		return fmt.Errorf("option --%s may be given only once", f.name)
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("invalid integer %q", s)
	}
	if v < f.minV || v > f.maxV {
		return fmt.Errorf("option --%s must be between %d and %d",
			f.name, f.minV, f.maxV)
	}
	f.set = true
	f.value = v
	return nil
}

type enumFlag struct {
	allowed []string
	value   string
}

func (e *enumFlag) String() string { return e.value }

func (e *enumFlag) Set(s string) error {
	for _, a := range e.allowed {
		if s == a {
			e.value = s
			return nil
		}
	}
	return fmt.Errorf("must be one of {%s}", strings.Join(e.allowed, "|"))
}

func printHelp() {
	fmt.Fprint(os.Stdout, `csolve - retrieval of the particle size distribution from lidar
coefficients using external cross-section tables
(Go port of the tabular mode of solve).

Usage:
  csolve [OPTIONS]

Options:
  -h, --help                     show this help and exit

  --data <JSON>                  measured coefficients; demo data if omitted
  --type {sphere|spheroid|agglomerate}
                                 built-in cross-section tables [agglomerate]
  --tables <DIR>                 external tables: directory with
                                 mre_<real>_mim_<imag>/data.txt (mutually
                                 exclusive with --type)
  --table-extrapolate            allow cross-section extrapolation in x
  --table-ref-lambda-um <X>      reference wavelength of tables, um [2pi]
  --table-cback {dsigma|4pi}     Cback column convention [dsigma]

  --degree <1|2|3|4>             B-spline degree (single value) [1]
  --n-basis <N>                  basis functions per window [number of channels]
  --r0-min <X>                   lower bound of r0 grid, > 0 [0.02]
  --r0-max <X>                   upper bound of r0 grid [0.5]
  --nr0 <N>                      number of r0 values, >= 1 [6]
  --r1-min <X>                   lower bound of r1 grid [2]
  --r1-max <X>                   upper bound of r1 grid [10]
  --nr1 <N>                      number of r1 values, >= 1 [4]
  --min-ratio <X>                drop windows with r1 < X*r0 [4]
  --best <G>                     number of best solutions G [5]

  --gamma-select {fixed|cv|err}  gamma selection rule [fixed]
  --gamma-rel <X>                gamma/||A||_2, >= 0 [0.1]
  --gamma-min <X>                min lg gamma [-7]
  --gamma-max <X>                max lg gamma [5]
  --ngamma <N>                   number of gamma values, >= 1 [41]

  --nonneg                       constrain C >= 0
  --no-zero-boundary             no boundary conditions n(r0)=n(r1)=0
  --no-smooth                    disable smoothing of the mean
  --smooth-lam <X>               smoothing parameter [1e-4]
  --r-split <X>                  fine/coarse boundary [0.5]
  --modes {auto|fit|stat}        mode parameters estimation [auto]
  --absolute                     least squares on absolute residuals

  --dx-max <X>                   max step in x for kernel tables, > 0 [0.1]
  --nr-out <N>                   points in output grid, >= 2 [256]
  --quad-order <N>               Gauss-Legendre order, >= 1 [24]
  --log-span <X>                 max subsegment width in ln r, > 0 [0.1]

  --m-real <X>                   true Re(m) for demo
  --m-imag <X>                   true Im(m) for demo
  --noise <X>                    demo noise level [0]
  --seed <N>                     RNG seed [1]

  --out <FILE>                   plot output file [result.<fmt>]
  --json <JSON>                  save results to JSON
  --plot-format {png|pdf|svg}    plot format [png]
  --plot-dpi <N>                 resolution for png/svg, dpi, > 0 [150]
  --plot-font-size <pt>          axis/series label size, pt (0 or 4..72) [0]
  --no-plot                      do not build plots

  --batch <TXT>                  batch processing from txt
                                 (Altitude b<nm>... a<nm>...)
  --batch-out-dir <DIR>          batch output directory [next to TXT]
  --lidar-ratio-cons             constrain smoothness of S = alpha/beta
  --lidar-ratio-w <X>            weight of S deviation penalty, >= 0 [0.5]
  --lidar-ratio-iter <N>         iterations to refine reference S, >= 1 [3]

  --depol-ratio-cons             add a delta_l residual to the retrieval
                                 functional or to solution ranking
  --depol-ratio-w <X>            weight of the delta_l residual, >= 0 [1]
  --depol-ratio-mode {func|rank}
                                 how delta_l is applied: functional residual
                                 or solution ranking [func]
`)
}

// ParseArgs — разбор аргументов; --degree принимает ровно одно значение.
func ParseArgs(argv []string) (*Args, error) {
	a := &Args{
		TableRefWl:     tableRefWlUm,
		Type:           defaultTableType,
		Degrees:        []int{1},
		R0Min:          0.02,
		R0Max:          0.5,
		NR0:            6,
		R1Min:          2.0,
		R1Max:          10.0,
		NR1:            4,
		MinRatio:       4.0,
		Best:           5,
		GammaMin:       -7.0,
		GammaMax:       5.0,
		NGamma:         41,
		GammaSelect:    gammaFixed,
		GammaRel:       0.1,
		SmoothLam:      1e-4,
		RSplit:         0.5,
		Modes:          0,
		DxMax:          0.1,
		NrOut:          256,
		QuadOrder:      24,
		LogSpan:        0.1,
		Seed:           1,
		PlotFormat:     plotFmtPng,
		PlotDpi:        150.0,
		PlotFontSize:   0.0,
		LidarRatioW:    0.5,
		LidarRatioIter: 3,
		DepolRatioW:    1.0,
		DepolRatioMode: depolModeFunc,
	}

	fs := flag.NewFlagSet("csolve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var help bool
	fs.BoolVar(&help, "h", false, "")
	fs.BoolVar(&help, "help", false, "")

	fs.StringVar(&a.Data, "data", "", "")
	fs.StringVar(&a.Tables, "tables", "", "")
	tableType := &enumFlag{allowed: builtinTypeNames, value: defaultTableType}
	fs.Var(tableType, "type", "")
	fs.BoolVar(&a.TableExtrapolate, "table-extrapolate", false, "")
	fs.Float64Var(&a.TableRefWl, "table-ref-lambda-um", a.TableRefWl, "")
	tableCback := &enumFlag{allowed: []string{"dsigma", "4pi"}, value: "dsigma"}
	fs.Var(tableCback, "table-cback", "")

	degree := &singleIntFlag{minV: 1, maxV: 4, name: "degree"}
	fs.Var(degree, "degree", "")
	fs.IntVar(&a.NBasis, "n-basis", 0, "")
	fs.Float64Var(&a.R0Min, "r0-min", a.R0Min, "")
	fs.Float64Var(&a.R0Max, "r0-max", a.R0Max, "")
	fs.IntVar(&a.NR0, "nr0", a.NR0, "")
	fs.Float64Var(&a.R1Min, "r1-min", a.R1Min, "")
	fs.Float64Var(&a.R1Max, "r1-max", a.R1Max, "")
	fs.IntVar(&a.NR1, "nr1", a.NR1, "")
	fs.Float64Var(&a.MinRatio, "min-ratio", a.MinRatio, "")
	fs.IntVar(&a.Best, "best", a.Best, "")

	gammaSelect := &enumFlag{allowed: []string{"fixed", "cv", "err"}, value: "fixed"}
	fs.Var(gammaSelect, "gamma-select", "")
	fs.Float64Var(&a.GammaRel, "gamma-rel", a.GammaRel, "")
	fs.Float64Var(&a.GammaMin, "gamma-min", a.GammaMin, "")
	fs.Float64Var(&a.GammaMax, "gamma-max", a.GammaMax, "")
	fs.IntVar(&a.NGamma, "ngamma", a.NGamma, "")

	fs.BoolVar(&a.Nonneg, "nonneg", false, "")
	fs.BoolVar(&a.NoZeroBoundary, "no-zero-boundary", false, "")
	fs.BoolVar(&a.NoSmooth, "no-smooth", false, "")
	fs.Float64Var(&a.SmoothLam, "smooth-lam", a.SmoothLam, "")
	fs.Float64Var(&a.RSplit, "r-split", a.RSplit, "")
	modesFlag := &enumFlag{allowed: []string{"auto", "fit", "stat"}, value: "auto"}
	fs.Var(modesFlag, "modes", "")
	fs.BoolVar(&a.Absolute, "absolute", false, "")

	fs.Float64Var(&a.DxMax, "dx-max", a.DxMax, "")
	fs.IntVar(&a.NrOut, "nr-out", a.NrOut, "")
	fs.IntVar(&a.QuadOrder, "quad-order", a.QuadOrder, "")
	fs.Float64Var(&a.LogSpan, "log-span", a.LogSpan, "")

	mReal := math.NaN()
	mImag := math.NaN()
	fs.Float64Var(&mReal, "m-real", math.NaN(), "")
	fs.Float64Var(&mImag, "m-imag", math.NaN(), "")

	fs.Float64Var(&a.Noise, "noise", 0, "")
	fs.IntVar(&a.Seed, "seed", a.Seed, "")

	fs.StringVar(&a.Out, "out", "", "")
	fs.StringVar(&a.JSON, "json", "", "")
	plotFormat := &enumFlag{allowed: []string{"png", "pdf", "svg"}, value: "png"}
	fs.Var(plotFormat, "plot-format", "")
	fs.Float64Var(&a.PlotDpi, "plot-dpi", a.PlotDpi, "")
	fs.Float64Var(&a.PlotFontSize, "plot-font-size", a.PlotFontSize, "")
	fs.BoolVar(&a.NoPlot, "no-plot", false, "")

	fs.StringVar(&a.Batch, "batch", "", "")
	fs.StringVar(&a.BatchOutDir, "batch-out-dir", "", "")
	fs.BoolVar(&a.LidarRatioCons, "lidar-ratio-cons", false, "")
	fs.Float64Var(&a.LidarRatioW, "lidar-ratio-w", a.LidarRatioW, "")
	fs.IntVar(&a.LidarRatioIter, "lidar-ratio-iter", a.LidarRatioIter, "")
	fs.BoolVar(&a.DepolRatioCons, "depol-ratio-cons", false, "")
	fs.Float64Var(&a.DepolRatioW, "depol-ratio-w", a.DepolRatioW, "")
	depolMode := &enumFlag{allowed: []string{"func", "rank"}, value: "func"}
	fs.Var(depolMode, "depol-ratio-mode", "")

	if err := fs.Parse(argv); err != nil {
		return nil, err
	}
	if help {
		printHelp()
		return nil, errHelp
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}

	seen := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { seen[f.Name] = true })

	if seen["tables"] && seen["type"] {
		return nil, fmt.Errorf("options --tables and --type are mutually exclusive")
	}

	if degree.set {
		a.Degrees = []int{degree.value}
	}
	if seen["m-real"] {
		a.HasMReal = true
		a.MReal = mReal
	}
	if seen["m-imag"] {
		a.HasMImag = true
		a.MImag = mImag
	}
	a.TableCback4pi = tableCback.value == "4pi"
	a.Type = tableType.value
	a.GammaSelect = map[string]int{"fixed": gammaFixed, "cv": gammaCV,
		"err": gammaErr}[gammaSelect.value]
	a.Modes = map[string]int{"auto": 0, "fit": 1, "stat": 2}[modesFlag.value]
	a.DepolRatioMode = map[string]int{"func": depolModeFunc,
		"rank": depolModeRank}[depolMode.value]
	a.PlotFormat = map[string]int{"png": plotFmtPng, "pdf": plotFmtPdf,
		"svg": plotFmtSvg}[plotFormat.value]

	if a.NR0 < 1 {
		return nil, fmt.Errorf("option --nr0 must be >= 1")
	}
	if a.NR1 < 1 {
		return nil, fmt.Errorf("option --nr1 must be >= 1")
	}
	if a.NrOut < 2 {
		return nil, fmt.Errorf("option --nr-out must be >= 2")
	}
	if a.QuadOrder < 1 {
		return nil, fmt.Errorf("option --quad-order must be >= 1")
	}
	if a.NGamma < 1 {
		return nil, fmt.Errorf("option --ngamma must be >= 1")
	}
	if a.LidarRatioIter < 1 {
		return nil, fmt.Errorf("option --lidar-ratio-iter must be >= 1")
	}
	if !(a.R0Min > 0) {
		return nil, fmt.Errorf("option --r0-min must be > 0")
	}
	if !(a.DxMax > 0) {
		return nil, fmt.Errorf("option --dx-max must be > 0")
	}
	if !(a.LogSpan > 0) {
		return nil, fmt.Errorf("option --log-span must be > 0")
	}
	if !(a.PlotDpi > 0) {
		return nil, fmt.Errorf("option --plot-dpi must be > 0")
	}
	if !(a.GammaRel >= 0) {
		return nil, fmt.Errorf("option --gamma-rel must be >= 0")
	}
	if !(a.LidarRatioW >= 0) {
		return nil, fmt.Errorf("option --lidar-ratio-w must be >= 0")
	}
	if !(a.DepolRatioW >= 0) {
		return nil, fmt.Errorf("option --depol-ratio-w must be >= 0")
	}
	if a.PlotFontSize != 0 && !(a.PlotFontSize >= 4 && a.PlotFontSize <= 72) {
		return nil, fmt.Errorf("option --plot-font-size must be 0 or between 4 and 72")
	}
	return a, nil
}
