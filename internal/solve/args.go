package solve

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
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

	Method   methodKind
	Degrees  []int
	NBasis   int
	VBins    int // число бинов монотонной сетки; 0 = auto
	RegOrder int // порядок штрафа за негладкость (0..2)

	R0Min, R0Max float64
	NR0          int
	R1Min, R1Max float64
	NR1          int
	MinRatio     float64
	Best         float64 // число решений или их доля, если значение в (0,1)

	GammaMin, GammaMax float64
	NGamma             int
	GammaSelect        gammaSelect
	GammaRel           float64

	Nonneg         bool
	NoZeroBoundary bool
	NoSmooth       bool
	SmoothLam      float64
	RSplit         float64
	Modes          modesKind
	Absolute       bool

	DxMax     float64
	NrOut     int
	QuadOrder int
	LogSpan   float64

	HasMReal, HasMImag bool
	MReal, MImag       float64

	Noise float64
	Seed  int

	OutDir       string
	Out          string
	JSON         string
	Report       string
	NoPlot       bool
	Plotting     plotBackend
	PlotFormat   plotFormat
	PlotDpi      float64
	PlotFontSize float64

	Batch          string
	BatchOutDir    string
	LidarRatioCons bool
	LidarRatioW    float64
	LidarRatioIter int

	DepolRatioCons bool
	// DepolRatioExplicit — флаг --depol-ratio-cons задан явно (а не
	// включён значением по умолчанию).
	DepolRatioExplicit bool
	DepolRatioW        float64
	DepolRatioMode     depolMode
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

// bestCount — число решений для усреднения при nTot найденных вариантах.
// --best из (0,1) трактуется как доля решений, иначе — как их количество.
// Доля, дающая меньше одного решения, приводится к 1.
func (a *Args) bestCount(nTot int) int {
	g := int(math.Round(a.Best))
	if a.Best < 1 {
		g = int(math.Round(float64(nTot) * a.Best))
	}
	if g < 1 {
		g = 1
	}
	if g > nTot {
		g = nTot
	}
	return g
}

// enumFlag — флаг с фиксированным набором значений: хранит типизированное
// значение, повтор флага допустим (последнее значение побеждает).
type enumFlag[T ~string] struct {
	allowed []T
	value   T
}

func (e *enumFlag[T]) String() string { return string(e.value) }

func (e *enumFlag[T]) Set(s string) error {
	for _, v := range e.allowed {
		if string(v) == s {
			e.value = v
			return nil
		}
	}
	return fmt.Errorf("must be one of %s", enumSetString(e.allowed))
}

// Value — текущее значение флага.
func (e *enumFlag[T]) Value() T { return e.value }

// enumSetString — представление набора значений "{a|b|c}" для сообщений.
func enumSetString[T ~string](vals []T) string {
	names := make([]string, len(vals))
	for i, v := range vals {
		names[i] = string(v)
	}
	return "{" + strings.Join(names, "|") + "}"
}

func printHelp() {
	fmt.Fprint(os.Stdout, `csolve - retrieval of the particle size distribution from lidar
coefficients using external cross-section tables
(Go port of the tabular mode of solve).

Usage:
  csolve [OPTIONS]
  csolve --tui

Options:
  -h, --help                     show this help and exit
  --tui                          interactive terminal UI (huh); accepts no
                                 other options

  --data <JSON>                  measured coefficients; demo data if omitted
  --type {sphere|spheroid|agglomerate|agglomerate-cs}
                                 built-in cross-section tables [agglomerate]
  --tables <DIR>                 external tables: directory with
                                 mre_<real>_mim_<imag>/data.txt (mutually
                                 exclusive with --type)
  --table-extrapolate            allow cross-section extrapolation in x
  --table-ref-lambda-um <X>      reference wavelength of tables, um [2pi]
  --table-cback {dsigma|4pi}     Cback column convention [dsigma]

  --degree <1|2|3|4>             B-spline degree (single value) [1]
                                 (--method spline only)
  --n-basis <N>                  basis functions per window; 0 = number of
                                 channels (--method spline only) [7]
  --method {monotone|spline}     retrieval method: monotone V(r) reconstruction
                                 or B-spline expansion [monotone]
  --v-bins <N>                   size bins on [r0, r1] for --method monotone;
                                 0 = auto (max(2*channels, 8), at most 64) [0]
  --reg-order {0|1|2}            smoothness penalty on the bin volumes:
                                 0 = min volume, 1 = few jumps, 2 = smooth n
                                 (--method monotone only) [2]
  --r0-min <X>                   lower bound of r0 grid, > 0 [0.05]
  --r0-max <X>                   upper bound of r0 grid [0.3]
  --nr0 <N>                      number of r0 values, >= 1 [7]
  --r1-min <X>                   lower bound of r1 grid [0.45]
  --r1-max <X>                   upper bound of r1 grid [3]
  --nr1 <N>                      number of r1 values, >= 1 [7]
  --min-ratio <X>                drop windows with r1 < X*r0 [4]
  --best <G>                     solutions to average: an integer count,
                                 or a fraction in (0,1) of all solutions [0.05]

  --gamma-select {fixed|cv|err}  gamma selection rule [fixed]
  --gamma-rel <X>                gamma/||A||_2, >= 0 [0.1]
  --gamma-min <X>                min lg gamma [-7]
  --gamma-max <X>                max lg gamma [5]
  --ngamma <N>                   number of gamma values, >= 1 [41]

  --nonneg                       constrain C >= 0 (default: on)
  --no-nonneg                    disable the C >= 0 constraint
                                 (not allowed with --method monotone)
  --no-zero-boundary             no boundary conditions n(r0)=n(r1)=0
                                 (--method spline only)
  --no-smooth                    disable smoothing of the mean
  --smooth-lam <X>               smoothing parameter [1e-4]
  --r-split <X>                  fine/coarse boundary [0.5]
  --modes {auto|fit|stat}        mode parameters estimation [stat]
  --absolute                     least squares on absolute residuals

  --dx-max <X>                   max step in x for kernel tables, > 0 [0.1]
  --nr-out <N>                   points in output grid, >= 2 [25]
  --quad-order <N>               Gauss-Legendre order, >= 1 [24]
  --log-span <X>                 max subsegment width in ln r, > 0 [0.1]

  --m-real <X>                   true Re(m) for demo
  --m-imag <X>                   true Im(m) for demo
  --noise <X>                    demo noise level [0]
  --seed <N>                     RNG seed [1]

  --out-dir <DIR>                directory for output files; relative
                                 --out/--json paths are placed inside it;
                                 batch results also go here unless
                                 --batch-out-dir is given [current dir]
  --out <FILE>                   plot output file [result.<fmt>]
  --json <JSON>                  save results to JSON
  --report <FILE>                save console report and plot to a document
                                 (.docx or .odt; single mode only)
  --plotting {internal|gnuplot}
                                 plotting backend [internal]; internal
                                 renders PDF only, without gnuplot
  --plot-format {png|pdf|svg}    plot format (gnuplot backend) [pdf];
                                 internal backend requires pdf
  --plot-dpi <N>                 resolution for png/svg, dpi, > 0 [150]
  --plot-font-size <pt>          axis/series label size, pt (0 or 4..72) [12]
  --no-plot                      do not build plots

  --batch <TXT>                  batch processing from txt
                                 (Altitude b<nm>... a<nm>...)
  --batch-out-dir <DIR>          batch output directory [next to TXT];
                                 mutually exclusive with --out-dir
  --lidar-ratio-cons             constrain smoothness of S = alpha/beta
  --lidar-ratio-w <X>            weight of S deviation penalty, >= 0 [1]
  --lidar-ratio-iter <N>         iterations to refine reference S, >= 1 [5]

  --depol-ratio-cons             add a delta_l residual to the retrieval
                                 functional or to solution ranking
                                 (default: on; skipped if no measured
                                 depolarization is available)
  --no-depol-ratio-cons          disable the delta_l residual
  --depol-ratio-w <X>            weight of the delta_l residual, >= 0 [1]
  --depol-ratio-mode {func|rank}
                                 how delta_l is applied: functional residual
                                 or solution ranking [func]
`)
}

// DefaultArgs — параметры по умолчанию (как при запуске без опций).
// Используется интерактивным режимом, чтобы не дублировать значения.
func DefaultArgs() (*Args, error) {
	return ParseArgs(nil)
}

// ParseArgs — разбор аргументов; --degree принимает ровно одно значение.
func ParseArgs(argv []string) (*Args, error) {
	a := &Args{
		TableRefWl:     tableRefWlUm,
		Type:           defaultTableType,
		Method:         methodMonotone,
		Degrees:        []int{1},
		NBasis:         7,
		RegOrder:       2,
		R0Min:          0.05,
		R0Max:          0.3,
		NR0:            7,
		R1Min:          0.45,
		R1Max:          3.0,
		NR1:            7,
		MinRatio:       4.0,
		Best:           0.05,
		GammaMin:       -7.0,
		GammaMax:       5.0,
		NGamma:         41,
		GammaSelect:    gammaFixed,
		GammaRel:       0.1,
		SmoothLam:      1e-4,
		RSplit:         0.5,
		Modes:          modesStat,
		DxMax:          0.1,
		NrOut:          25,
		QuadOrder:      24,
		LogSpan:        0.1,
		Seed:           1,
		Plotting:       plottingInternal,
		PlotFormat:     plotFmtPdf,
		PlotDpi:        150.0,
		PlotFontSize:   12.0,
		LidarRatioW:    1.0,
		LidarRatioIter: 5,
		DepolRatioW:    1.0,
		DepolRatioMode: depolModeFunc,
		Nonneg:         true,
		DepolRatioCons: true,
	}

	fs := flag.NewFlagSet("csolve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var help bool
	fs.BoolVar(&help, "h", false, "")
	fs.BoolVar(&help, "help", false, "")

	fs.StringVar(&a.Data, "data", "", "")
	fs.StringVar(&a.Tables, "tables", "", "")
	tableType := &enumFlag[string]{allowed: builtinTypeNames, value: defaultTableType}
	fs.Var(tableType, "type", "")
	fs.BoolVar(&a.TableExtrapolate, "table-extrapolate", false, "")
	fs.Float64Var(&a.TableRefWl, "table-ref-lambda-um", a.TableRefWl, "")
	tableCback := &enumFlag[string]{allowed: []string{"dsigma", "4pi"}, value: "dsigma"}
	fs.Var(tableCback, "table-cback", "")

	degree := &singleIntFlag{minV: 1, maxV: 4, name: "degree"}
	fs.Var(degree, "degree", "")
	fs.IntVar(&a.NBasis, "n-basis", a.NBasis, "")
	methodFlag := &enumFlag[methodKind]{allowed: methodKinds, value: a.Method}
	fs.Var(methodFlag, "method", "")
	vBins := &singleIntFlag{minV: 0, maxV: vBinsMax, name: "v-bins"}
	fs.Var(vBins, "v-bins", "")
	regOrder := &singleIntFlag{minV: 0, maxV: 2, name: "reg-order"}
	fs.Var(regOrder, "reg-order", "")
	fs.Float64Var(&a.R0Min, "r0-min", a.R0Min, "")
	fs.Float64Var(&a.R0Max, "r0-max", a.R0Max, "")
	fs.IntVar(&a.NR0, "nr0", a.NR0, "")
	fs.Float64Var(&a.R1Min, "r1-min", a.R1Min, "")
	fs.Float64Var(&a.R1Max, "r1-max", a.R1Max, "")
	fs.IntVar(&a.NR1, "nr1", a.NR1, "")
	fs.Float64Var(&a.MinRatio, "min-ratio", a.MinRatio, "")
	fs.Float64Var(&a.Best, "best", a.Best, "")

	gammaSel := &enumFlag[gammaSelect]{allowed: gammaSelects, value: a.GammaSelect}
	fs.Var(gammaSel, "gamma-select", "")
	fs.Float64Var(&a.GammaRel, "gamma-rel", a.GammaRel, "")
	fs.Float64Var(&a.GammaMin, "gamma-min", a.GammaMin, "")
	fs.Float64Var(&a.GammaMax, "gamma-max", a.GammaMax, "")
	fs.IntVar(&a.NGamma, "ngamma", a.NGamma, "")

	fs.BoolVar(&a.Nonneg, "nonneg", a.Nonneg, "")
	var noNonneg bool
	fs.BoolVar(&noNonneg, "no-nonneg", false, "")
	fs.BoolVar(&a.NoZeroBoundary, "no-zero-boundary", false, "")
	fs.BoolVar(&a.NoSmooth, "no-smooth", false, "")
	fs.Float64Var(&a.SmoothLam, "smooth-lam", a.SmoothLam, "")
	fs.Float64Var(&a.RSplit, "r-split", a.RSplit, "")
	modesFlag := &enumFlag[modesKind]{allowed: modesKinds, value: a.Modes}
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

	fs.StringVar(&a.OutDir, "out-dir", "", "")
	fs.StringVar(&a.Out, "out", "", "")
	fs.StringVar(&a.JSON, "json", "", "")
	fs.StringVar(&a.Report, "report", "", "")
	plotFormatFlag := &enumFlag[plotFormat]{allowed: plotFormats,
		value: a.PlotFormat}
	fs.Var(plotFormatFlag, "plot-format", "")
	plottingFlag := &enumFlag[plotBackend]{allowed: plotBackends,
		value: a.Plotting}
	fs.Var(plottingFlag, "plotting", "")
	fs.Float64Var(&a.PlotDpi, "plot-dpi", a.PlotDpi, "")
	fs.Float64Var(&a.PlotFontSize, "plot-font-size", a.PlotFontSize, "")
	fs.BoolVar(&a.NoPlot, "no-plot", false, "")

	fs.StringVar(&a.Batch, "batch", "", "")
	fs.StringVar(&a.BatchOutDir, "batch-out-dir", "", "")
	fs.BoolVar(&a.LidarRatioCons, "lidar-ratio-cons", false, "")
	fs.Float64Var(&a.LidarRatioW, "lidar-ratio-w", a.LidarRatioW, "")
	fs.IntVar(&a.LidarRatioIter, "lidar-ratio-iter", a.LidarRatioIter, "")
	fs.BoolVar(&a.DepolRatioCons, "depol-ratio-cons", a.DepolRatioCons, "")
	var noDepolCons bool
	fs.BoolVar(&noDepolCons, "no-depol-ratio-cons", false, "")
	fs.Float64Var(&a.DepolRatioW, "depol-ratio-w", a.DepolRatioW, "")
	depolModeFlag := &enumFlag[depolMode]{allowed: depolModes,
		value: a.DepolRatioMode}
	fs.Var(depolModeFlag, "depol-ratio-mode", "")

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
	if seen["out-dir"] && seen["batch-out-dir"] {
		return nil, fmt.Errorf("options --out-dir and --batch-out-dir are " +
			"mutually exclusive")
	}
	if seen["nonneg"] && seen["no-nonneg"] {
		return nil, fmt.Errorf("options --nonneg and --no-nonneg are " +
			"mutually exclusive")
	}
	if seen["depol-ratio-cons"] && seen["no-depol-ratio-cons"] {
		return nil, fmt.Errorf("options --depol-ratio-cons and " +
			"--no-depol-ratio-cons are mutually exclusive")
	}
	if noNonneg {
		a.Nonneg = false
	}
	if noDepolCons {
		a.DepolRatioCons = false
	}
	a.DepolRatioExplicit = seen["depol-ratio-cons"]

	if degree.set {
		a.Degrees = []int{degree.value}
	}
	if vBins.set {
		a.VBins = vBins.value
	}
	if regOrder.set {
		a.RegOrder = regOrder.value
	}
	a.Method = methodFlag.Value()
	if seen["m-real"] {
		a.HasMReal = true
		a.MReal = mReal
	}
	if seen["m-imag"] {
		a.HasMImag = true
		a.MImag = mImag
	}
	a.TableCback4pi = tableCback.Value() == "4pi"
	a.Type = tableType.Value()
	a.GammaSelect = gammaSel.Value()
	a.Modes = modesFlag.Value()
	a.DepolRatioMode = depolModeFlag.Value()
	a.PlotFormat = plotFormatFlag.Value()
	a.Plotting = plottingFlag.Value()
	if err := checkArgs(a, seen); err != nil {
		return nil, err
	}
	return a, nil
}

// checkArgs — согласование зависимых опций (встроенный backend ⇒ pdf,
// нецелый --best округляется) и проверка диапазонов числовых опций.
func checkArgs(a *Args, seen map[string]bool) error {
	if a.Report != "" {
		if a.Batch != "" {
			return fmt.Errorf("option --report is only supported in single mode")
		}
		switch strings.ToLower(filepath.Ext(a.Report)) {
		case ".docx", ".odt":
		default:
			return fmt.Errorf("option --report must have a .docx or .odt "+
				"extension, got %q", a.Report)
		}
	}
	if a.Plotting == plottingInternal {
		if seen["plot-format"] && a.PlotFormat != plotFmtPdf && !a.NoPlot {
			return fmt.Errorf("option --plot-format=%s is not "+
				"supported with --plotting=internal (only pdf)", a.PlotFormat)
		}
		a.PlotFormat = plotFmtPdf
		if seen["plot-dpi"] && !a.NoPlot {
			fmt.Fprintf(os.Stderr, "[warning] --plot-dpi has no effect with "+
				"--plotting=internal (vector PDF)\n")
		}
		if seen["out"] && !a.NoPlot {
			if ext := strings.ToLower(filepath.Ext(a.Out)); ext != "" &&
				ext != ".pdf" {
				return fmt.Errorf("option --out must have a .pdf "+
					"extension with --plotting=internal, got %q", a.Out)
			}
		}
	}

	if a.Method == methodMonotone && !a.Nonneg {
		return fmt.Errorf("option --no-nonneg is incompatible with " +
			"--method monotone: monotonicity of V(r) requires non-negative " +
			"increments")
	}
	if a.NR0 < 1 {
		return fmt.Errorf("option --nr0 must be >= 1")
	}
	if a.NR1 < 1 {
		return fmt.Errorf("option --nr1 must be >= 1")
	}
	if a.NrOut < 2 {
		return fmt.Errorf("option --nr-out must be >= 2")
	}
	if a.QuadOrder < 1 {
		return fmt.Errorf("option --quad-order must be >= 1")
	}
	if a.NGamma < 1 {
		return fmt.Errorf("option --ngamma must be >= 1")
	}
	if a.LidarRatioIter < 1 {
		return fmt.Errorf("option --lidar-ratio-iter must be >= 1")
	}
	if !(a.R0Min > 0) {
		return fmt.Errorf("option --r0-min must be > 0")
	}
	if !(a.DxMax > 0) {
		return fmt.Errorf("option --dx-max must be > 0")
	}
	if !(a.LogSpan > 0) {
		return fmt.Errorf("option --log-span must be > 0")
	}
	if !(a.PlotDpi > 0) {
		return fmt.Errorf("option --plot-dpi must be > 0")
	}
	if !(a.GammaRel >= 0) {
		return fmt.Errorf("option --gamma-rel must be >= 0")
	}
	if !(a.LidarRatioW >= 0) {
		return fmt.Errorf("option --lidar-ratio-w must be >= 0")
	}
	if !(a.DepolRatioW >= 0) {
		return fmt.Errorf("option --depol-ratio-w must be >= 0")
	}
	if math.IsNaN(a.Best) || math.IsInf(a.Best, 0) {
		return fmt.Errorf("option --best must be a finite number")
	}
	if !(a.Best > 0) {
		return fmt.Errorf("option --best must be > 0: a value in (0,1) " +
			"is a fraction of all solutions, otherwise it is a count")
	}
	if a.Best > 1 && a.Best != math.Trunc(a.Best) {
		rounded := math.Round(a.Best)
		fmt.Fprintf(os.Stderr, "[warning] --best=%g is not an integer; "+
			"rounding to %g\n", a.Best, rounded)
		a.Best = rounded
	}
	if a.PlotFontSize != 0 && !(a.PlotFontSize >= 4 && a.PlotFontSize <= 72) {
		return fmt.Errorf("option --plot-font-size must be 0 or between 4 and 72")
	}
	return nil
}
