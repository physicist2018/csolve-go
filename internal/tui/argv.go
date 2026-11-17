package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"csolve/internal/solve"
)

// buildArgv translates the form values into solver flags and validates them with
// solve.ParseArgs, so the TUI accepts exactly the same combinations as the CLI.
func buildArgv(s *state) ([]string, *solve.Args, error) {
	d := s.def
	var argv []string

	add := func(flag, value string) {
		argv = append(argv, flag, value)
	}
	addFlag := func(flag string) {
		argv = append(argv, flag)
	}

	switch s.mode {
	case modeDemo:
	case modeData:
		if strings.TrimSpace(s.dataPath) == "" {
			return nil, nil, errors.New("a JSON file with measurements is required")
		}
		add("--data", s.dataPath)
	case modeBatch:
		if strings.TrimSpace(s.batchPath) == "" {
			return nil, nil, errors.New("a batch TXT file is required")
		}
		add("--batch", s.batchPath)
	default:
		return nil, nil, fmt.Errorf("unknown input mode %q", s.mode)
	}

	if s.tablesKind == tablesExternal {
		if strings.TrimSpace(s.tablesDir) == "" {
			return nil, nil, errors.New("a directory with external tables is required")
		}
		add("--tables", s.tablesDir)
	} else {
		add("--type", s.builtinType)
	}
	if s.tableExtrapolate {
		addFlag("--table-extrapolate")
	}
	if changedFloat(s.tableRefWl, d.TableRefWl) {
		add("--table-ref-lambda-um", s.tableRefWl)
	}
	if s.tableCback == "4pi" {
		add("--table-cback", "4pi")
	}

	if s.degree != 1 {
		add("--degree", strconv.Itoa(s.degree))
	}
	if changedInt(s.nBasis, d.NBasis) {
		add("--n-basis", s.nBasis)
	}
	if changedFloat(s.r0Min, d.R0Min) {
		add("--r0-min", s.r0Min)
	}
	if changedFloat(s.r0Max, d.R0Max) {
		add("--r0-max", s.r0Max)
	}
	if changedInt(s.nr0, d.NR0) {
		add("--nr0", s.nr0)
	}
	if changedFloat(s.r1Min, d.R1Min) {
		add("--r1-min", s.r1Min)
	}
	if changedFloat(s.r1Max, d.R1Max) {
		add("--r1-max", s.r1Max)
	}
	if changedInt(s.nr1, d.NR1) {
		add("--nr1", s.nr1)
	}
	if changedFloat(s.minRatio, d.MinRatio) {
		add("--min-ratio", s.minRatio)
	}
	if changedFloat(s.best, d.Best) {
		add("--best", s.best)
	}

	if s.gammaSelect != "fixed" {
		add("--gamma-select", s.gammaSelect)
	}
	if changedFloat(s.gammaRel, d.GammaRel) {
		add("--gamma-rel", s.gammaRel)
	}
	if changedFloat(s.gammaMin, d.GammaMin) {
		add("--gamma-min", s.gammaMin)
	}
	if changedFloat(s.gammaMax, d.GammaMax) {
		add("--gamma-max", s.gammaMax)
	}
	if changedInt(s.nGamma, d.NGamma) {
		add("--ngamma", s.nGamma)
	}

	if s.nonneg {
		addFlag("--nonneg")
	}
	if s.noZeroBound {
		addFlag("--no-zero-boundary")
	}
	if s.noSmooth {
		addFlag("--no-smooth")
	}
	if changedFloat(s.smoothLam, d.SmoothLam) {
		add("--smooth-lam", s.smoothLam)
	}
	if changedFloat(s.rSplit, d.RSplit) {
		add("--r-split", s.rSplit)
	}
	if s.modes != "auto" {
		add("--modes", s.modes)
	}
	if s.absolute {
		addFlag("--absolute")
	}

	if s.depolCons {
		addFlag("--depol-ratio-cons")
		if changedFloat(s.depolW, d.DepolRatioW) {
			add("--depol-ratio-w", s.depolW)
		}
		if s.depolMode != "func" {
			add("--depol-ratio-mode", s.depolMode)
		}
	}

	// Lidar-ratio smoothness is only meaningful in batch mode.
	if s.mode == modeBatch && s.lidarCons {
		addFlag("--lidar-ratio-cons")
		if changedFloat(s.lidarW, d.LidarRatioW) {
			add("--lidar-ratio-w", s.lidarW)
		}
		if changedInt(s.lidarIter, d.LidarRatioIter) {
			add("--lidar-ratio-iter", s.lidarIter)
		}
	}

	if s.mode == modeDemo {
		if changedFloat(s.mReal, 1.45) {
			add("--m-real", s.mReal)
		}
		if changedFloat(s.mImag, -0.005) {
			add("--m-imag", s.mImag)
		}
		if changedFloat(s.noise, 0) {
			add("--noise", s.noise)
		}
		if changedInt(s.seed, d.Seed) {
			add("--seed", s.seed)
		}
	}

	if changedFloat(s.dxMax, d.DxMax) {
		add("--dx-max", s.dxMax)
	}
	if changedInt(s.nrOut, d.NrOut) {
		add("--nr-out", s.nrOut)
	}
	if changedInt(s.quadOrder, d.QuadOrder) {
		add("--quad-order", s.quadOrder)
	}
	if changedFloat(s.logSpan, d.LogSpan) {
		add("--log-span", s.logSpan)
	}

	if strings.TrimSpace(s.outDir) != "" {
		add("--out-dir", s.outDir)
	}
	if strings.TrimSpace(s.out) != "" {
		add("--out", s.out)
	}
	if strings.TrimSpace(s.jsonOut) != "" {
		add("--json", s.jsonOut)
	}
	if s.plotting != "internal" {
		add("--plotting", s.plotting)
	}
	// The internal renderer always writes vector PDF and rejects --plot-format
	// (other than pdf) and ignores --plot-dpi.
	if s.plotting == "gnuplot" {
		if s.plotFormat != "png" {
			add("--plot-format", s.plotFormat)
		}
		if changedFloat(s.plotDpi, d.PlotDpi) {
			add("--plot-dpi", s.plotDpi)
		}
	}
	if changedFloat(s.plotFontSize, 0) {
		add("--plot-font-size", s.plotFontSize)
	}
	if s.noPlot {
		addFlag("--no-plot")
	}

	a, err := solve.ParseArgs(argv)
	if err != nil {
		return nil, nil, err
	}
	return argv, a, nil
}

// changedFloat reports whether the text differs from the default value. Invalid
// text counts as changed so that solve.ParseArgs can report the problem.
func changedFloat(text string, def float64) bool {
	v, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return true
	}
	return v != def
}

// changedInt reports whether the text differs from the default value.
func changedInt(text string, def int) bool {
	v, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return true
	}
	return v != def
}

// describeOutputs renders the paths the solver writes.
func describeOutputs(s *state, a *solve.Args) string {
	if a.Batch != "" {
		dir := a.BatchOutDir
		if dir == "" {
			dir = a.OutDir
		}
		if dir == "" {
			dir = filepath.Dir(a.Batch)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Batch results go to: %s\n", dir)
		b.WriteString("  dist_<H>.txt, moments.txt, refractive.txt, errors.txt, delpol.txt")
		if a.LidarRatioCons {
			b.WriteString(", lidar_ratios.txt")
		}
		b.WriteString("\n  plots: plot_<H>.<ext> and summary profiles")
		return b.String()
	}

	ext := "pdf"
	if s.plotting != "internal" {
		ext = s.plotFormat
	}
	var paths []string
	if !a.NoPlot {
		name := a.Out
		if name == "" {
			name = "result." + ext
		}
		if a.OutDir != "" && !filepath.IsAbs(name) {
			name = filepath.Join(a.OutDir, name)
		}
		paths = append(paths, name)
	}
	if a.JSON != "" {
		name := a.JSON
		if a.OutDir != "" && !filepath.IsAbs(name) {
			name = filepath.Join(a.OutDir, name)
		}
		paths = append(paths, name)
	}
	if len(paths) == 0 {
		return "No output files: plotting is disabled and --json is not set."
	}
	return "Output files:\n  " + strings.Join(paths, "\n  ")
}

// shellJoin quotes arguments so the command can be pasted into a shell.
func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if arg == "" || strings.ContainsAny(arg, " \t\n\"'\\$`") {
			quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
		} else {
			quoted[i] = arg
		}
	}
	return strings.Join(quoted, " ")
}
