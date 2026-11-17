package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"csolve/internal/solve"
)

// argvBuilder — накопитель аргументов командной строки для solve.ParseArgs.
type argvBuilder struct{ argv []string }

func (b *argvBuilder) add(flag, value string) { b.argv = append(b.argv, flag, value) }
func (b *argvBuilder) flag(flag string)       { b.argv = append(b.argv, flag) }

// buildArgv — перевод значений формы во флаги решателя и проверка через
// solve.ParseArgs: TUI принимает те же сочетания, что и CLI.
func buildArgv(s *state) ([]string, *solve.Args, error) {
	d := s.def
	b := &argvBuilder{}

	if err := b.input(s); err != nil {
		return nil, nil, err
	}
	if err := b.tables(s, d); err != nil {
		return nil, nil, err
	}
	b.grid(s, d)
	b.regularization(s, d)
	b.depolarization(s, d)
	b.lidarRatio(s, d)
	b.demo(s, d)
	b.discretization(s, d)
	b.output(s, d)

	a, err := solve.ParseArgs(b.argv)
	if err != nil {
		return nil, nil, err
	}
	return b.argv, a, nil
}

// input — режим входа: JSON с измерениями, пакетный TXT или демо.
func (b *argvBuilder) input(s *state) error {
	switch s.mode {
	case modeDemo:
	case modeData:
		if strings.TrimSpace(s.dataPath) == "" {
			return errors.New("a JSON file with measurements is required")
		}
		b.add("--data", s.dataPath)
	case modeBatch:
		if strings.TrimSpace(s.batchPath) == "" {
			return errors.New("a batch TXT file is required")
		}
		b.add("--batch", s.batchPath)
	default:
		return fmt.Errorf("unknown input mode %q", s.mode)
	}
	return nil
}

// tables — источник таблиц сечений и соглашения о столбцах.
func (b *argvBuilder) tables(s *state, d *solve.Args) error {
	if s.tablesKind == tablesExternal {
		if strings.TrimSpace(s.tablesDir) == "" {
			return errors.New("a directory with external tables is required")
		}
		b.add("--tables", s.tablesDir)
	} else {
		b.add("--type", s.builtinType)
	}
	if s.tableExtrapolate {
		b.flag("--table-extrapolate")
	}
	if changedFloat(s.tableRefWl, d.TableRefWl) {
		b.add("--table-ref-lambda-um", s.tableRefWl)
	}
	if s.tableCback == "4pi" {
		b.add("--table-cback", "4pi")
	}
	return nil
}

// grid — базис, сетка окон и число усредняемых решений.
func (b *argvBuilder) grid(s *state, d *solve.Args) {
	if s.degree != 1 {
		b.add("--degree", strconv.Itoa(s.degree))
	}
	if changedInt(s.nBasis, d.NBasis) {
		b.add("--n-basis", s.nBasis)
	}
	if changedFloat(s.r0Min, d.R0Min) {
		b.add("--r0-min", s.r0Min)
	}
	if changedFloat(s.r0Max, d.R0Max) {
		b.add("--r0-max", s.r0Max)
	}
	if changedInt(s.nr0, d.NR0) {
		b.add("--nr0", s.nr0)
	}
	if changedFloat(s.r1Min, d.R1Min) {
		b.add("--r1-min", s.r1Min)
	}
	if changedFloat(s.r1Max, d.R1Max) {
		b.add("--r1-max", s.r1Max)
	}
	if changedInt(s.nr1, d.NR1) {
		b.add("--nr1", s.nr1)
	}
	if changedFloat(s.minRatio, d.MinRatio) {
		b.add("--min-ratio", s.minRatio)
	}
	if changedFloat(s.best, d.Best) {
		b.add("--best", s.best)
	}
}

// regularization — выбор gamma, сглаживание и оценка мод.
func (b *argvBuilder) regularization(s *state, d *solve.Args) {
	if s.gammaSelect != "fixed" {
		b.add("--gamma-select", s.gammaSelect)
	}
	if changedFloat(s.gammaRel, d.GammaRel) {
		b.add("--gamma-rel", s.gammaRel)
	}
	if changedFloat(s.gammaMin, d.GammaMin) {
		b.add("--gamma-min", s.gammaMin)
	}
	if changedFloat(s.gammaMax, d.GammaMax) {
		b.add("--gamma-max", s.gammaMax)
	}
	if changedInt(s.nGamma, d.NGamma) {
		b.add("--ngamma", s.nGamma)
	}
	if s.nonneg {
		b.flag("--nonneg")
	}
	if s.noZeroBound {
		b.flag("--no-zero-boundary")
	}
	if s.noSmooth {
		b.flag("--no-smooth")
	}
	if changedFloat(s.smoothLam, d.SmoothLam) {
		b.add("--smooth-lam", s.smoothLam)
	}
	if changedFloat(s.rSplit, d.RSplit) {
		b.add("--r-split", s.rSplit)
	}
	if s.modes != "auto" {
		b.add("--modes", s.modes)
	}
	if s.absolute {
		b.flag("--absolute")
	}
}

// depolarization — ограничение по измеренному δ_l.
func (b *argvBuilder) depolarization(s *state, d *solve.Args) {
	if !s.depolCons {
		return
	}
	b.flag("--depol-ratio-cons")
	if changedFloat(s.depolW, d.DepolRatioW) {
		b.add("--depol-ratio-w", s.depolW)
	}
	if s.depolMode != "func" {
		b.add("--depol-ratio-mode", s.depolMode)
	}
}

// lidarRatio — ограничение гладкости S = α/β (только пакетный режим).
func (b *argvBuilder) lidarRatio(s *state, d *solve.Args) {
	if s.mode != modeBatch || !s.lidarCons {
		return
	}
	b.flag("--lidar-ratio-cons")
	if changedFloat(s.lidarW, d.LidarRatioW) {
		b.add("--lidar-ratio-w", s.lidarW)
	}
	if changedInt(s.lidarIter, d.LidarRatioIter) {
		b.add("--lidar-ratio-iter", s.lidarIter)
	}
}

// demo — истинные m, шум и зерно генератора (только демо-режим).
func (b *argvBuilder) demo(s *state, d *solve.Args) {
	if s.mode != modeDemo {
		return
	}
	if changedFloat(s.mReal, 1.45) {
		b.add("--m-real", s.mReal)
	}
	if changedFloat(s.mImag, -0.005) {
		b.add("--m-imag", s.mImag)
	}
	if changedFloat(s.noise, 0) {
		b.add("--noise", s.noise)
	}
	if changedInt(s.seed, d.Seed) {
		b.add("--seed", s.seed)
	}
}

// discretization — шаг по x, выходная сетка и квадратура.
func (b *argvBuilder) discretization(s *state, d *solve.Args) {
	if changedFloat(s.dxMax, d.DxMax) {
		b.add("--dx-max", s.dxMax)
	}
	if changedInt(s.nrOut, d.NrOut) {
		b.add("--nr-out", s.nrOut)
	}
	if changedInt(s.quadOrder, d.QuadOrder) {
		b.add("--quad-order", s.quadOrder)
	}
	if changedFloat(s.logSpan, d.LogSpan) {
		b.add("--log-span", s.logSpan)
	}
}

// output — файлы результатов и графики.
func (b *argvBuilder) output(s *state, d *solve.Args) {
	if strings.TrimSpace(s.outDir) != "" {
		b.add("--out-dir", s.outDir)
	}
	if strings.TrimSpace(s.out) != "" {
		b.add("--out", s.out)
	}
	if strings.TrimSpace(s.jsonOut) != "" {
		b.add("--json", s.jsonOut)
	}
	if s.plotting != "internal" {
		b.add("--plotting", s.plotting)
	}
	// Встроенный рендерер всегда пишет векторный PDF, отвергает --plot-format
	// (кроме pdf) и игнорирует --plot-dpi.
	if s.plotting == "gnuplot" {
		if s.plotFormat != "png" {
			b.add("--plot-format", s.plotFormat)
		}
		if changedFloat(s.plotDpi, d.PlotDpi) {
			b.add("--plot-dpi", s.plotDpi)
		}
	}
	if changedFloat(s.plotFontSize, 0) {
		b.add("--plot-font-size", s.plotFontSize)
	}
	if s.noPlot {
		b.flag("--no-plot")
	}
}

// changedFloat — отличается ли текст от значения по умолчанию. Неверный
// текст считается изменённым, чтобы ошибку сообщил solve.ParseArgs.
func changedFloat(text string, def float64) bool {
	v, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return true
	}
	return v != def
}

// changedInt — отличается ли текст от значения по умолчанию.
func changedInt(text string, def int) bool {
	v, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return true
	}
	return v != def
}

// describeOutputs — описание файлов, которые запишет решатель.
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

// shellJoin — экранирование аргументов, чтобы команду можно было вставить
// в оболочку.
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
