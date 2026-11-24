package solve

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// batchSetup — всё, что готовится один раз на пакет: вход, таблицы, ядра и
// матрицы A.
type batchSetup struct {
	b        *batchInput
	src      *tableSource
	mre, mim []float64
	kts      []*ktable
	ds       *depolSet
	design   *design
	solveCh  []channel
	r1MaxEff float64
}

func (s *batchSetup) nm() int { return len(s.mre) }

// prepareBatch — разбор пакетного входа, покрытие таблиц, ядра и матрицы A.
func prepareBatch(a *Args, c *console) (*batchSetup, error) {
	b, err := parseBatchTxt(a.Batch)
	if err != nil {
		return nil, err
	}
	c.printf("batch processing: %s\n", a.Batch)
	if a.BatchThin > 0 {
		b, err = thinBatchAltitudes(b, a.BatchThin, c)
		if err != nil {
			return nil, err
		}
	}
	if a.DepolRatioCons && b.ndep == 0 && a.DepolRatioExplicit {
		return nil, errors.New("option --depol-ratio-cons requires a dep<nm> column in the " +
			"batch file (e.g. dep532), got none")
	}
	src, err := a.tableSource(c)
	if err != nil {
		return nil, err
	}
	mre, mim, err := tableMGrid(src)
	if err != nil {
		return nil, err
	}
	st := &batchSetup{b: b, src: src, mre: mre, mim: mim}
	nm := st.nm()

	c.printf("  altitudes: %d; channels: %d\n", b.nAlt, b.nch)
	for j := 0; j < b.nch; j++ {
		kind := "alpha"
		if b.ch[j].kind == chBeta {
			kind = "beta"
		}
		c.printf("  channel: %s (%s, %g nm)\n", b.ch[j].name, kind,
			b.ch[j].wlUm*1e3)
	}
	c.printf("  units: β [1/(m·sr)], α [1/m]\n")
	for k, wl := range b.depWl {
		c.printf("  measured δ_l column %d: %.6g nm, %%\n", k+1, wl*1e3)
	}
	if a.LidarRatioCons {
		c.printf("lidar ratio S=α/β smoothness constraint along altitude: "+
			"enabled (weight %g, iterations <= %d)\n", a.LidarRatioW,
			a.LidarRatioIter)
	} else {
		c.printf("lidar ratio S=α/β smoothness constraint along altitude: " +
			"disabled\n")
	}

	// каналы решения
	st.solveCh = make([]channel, b.nch)
	for j := 0; j < b.nch; j++ {
		st.solveCh[j] = channel{
			kind:       b.ch[j].kind,
			wlUm:       b.ch[j].wlUm,
			refWlUm:    tableRefWlUm,
			cbackScale: 1.0,
		}
	}

	cbackScale := 1.0
	if a.TableCback4pi {
		cbackScale = 1.0 / (4.0 * math.Pi)
	}

	// покрытие таблиц -> r1_max_eff
	capmin := math.Inf(1)
	for i := 0; i < nm; i++ {
		chs, err := loadTableChannels(src, mre[i], mim[i], st.solveCh,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		if err != nil {
			return nil, err
		}
		if chs.cap < capmin {
			capmin = chs.cap
		}
	}
	st.r1MaxEff = math.Min(a.R1Max, capmin)
	if st.r1MaxEff < a.R1Min {
		return nil, fmt.Errorf("table coverage in x limits r1_max = %g µm < "+
			"r1_min = %g — decrease --r1-min, choose another --type, "+
			"or extend the tables (%s)", st.r1MaxEff, a.R1Min, src.describe())
	}

	c.printf("table-based kernel mode (external cross sections), %s\n",
		src.describe())
	c.printf("  table λ_ref = %g µm (cross sections are rescaled as "+
		"(λ/λ_ref)²)\n", a.TableRefWl)
	if a.TableCback4pi {
		c.printf("  Cback convention = 4pi\n")
	} else {
		c.printf("  Cback convention = dsigma\n")
	}
	{
		c0, err := loadTableChannels(src, mre[0], mim[0], st.solveCh,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		if err != nil {
			return nil, err
		}
		for j := 0; j < b.nch; j++ {
			c.printf("  %-14s x_max = %g, r_max = %.3g µm\n",
				c0.ch[j].name(), c0.ch[j].tab.xMax,
				c0.ch[j].tab.xMax*c0.ch[j].wlUm/(2.0*math.Pi))
		}
	}
	c.printf("  effective r1_max = %g µm (requested: %g) — windows and "+
		"r grid are limited\n", st.r1MaxEff, a.R1Max)

	// таблицы ядер — один раз на пакет
	c.printf("building kernel tables (%d m values, once per batch)...\n", nm)
	st.kts = make([]*ktable, nm)
	for i := 0; i < nm; i++ {
		chs, err := loadTableChannels(src, mre[i], mim[i], st.solveCh,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		if err != nil {
			return nil, err
		}
		kt, err := ktableBuild(chs, a.R0Min, st.r1MaxEff, a.DxMax)
		if err != nil {
			return nil, err
		}
		st.kts[i] = kt
	}
	c.printf("  table: %d r points per m\n", st.kts[0].n)

	// ядра S11/S22 для линейной деполяризации — один раз на пакет;
	// δ_l считается на всех длинах волн каналов и столбцов dep<nm>
	depWls := make([]float64, 0, len(b.ch)+len(b.depWl))
	for _, ch := range b.ch {
		depWls = append(depWls, ch.wlUm)
	}
	depWls = append(depWls, b.depWl...)
	st.ds, err = depolSetBuild(src, mre, mim, depWls, a.TableRefWl, a.R0Min,
		st.r1MaxEff, a.DxMax, a.TableExtrapolate)
	if err != nil {
		return nil, err
	}
	depolSetReport(st.ds, src, nm, c)

	// матрицы A — тоже один раз
	r0Grid := geomspace(a.R0Min, a.R0Max, a.NR0)
	r1Grid := geomspace(a.R1Min, st.r1MaxEff, a.NR1)
	st.design, err = designBuild(st.solveCh, b.nch, st.kts, mre, mim, r0Grid,
		r1Grid, a.MinRatio, designSpec{
			method:       a.Method,
			degrees:      a.Degrees,
			nBasis:       a.NBasis,
			bins:         a.VBins,
			zeroBoundary: !a.NoZeroBoundary,
			quadOrder:    a.QuadOrder,
			maxLogSpan:   a.LogSpan,
			regOrder:     a.RegOrder,
			kernelOrder:  a.KernelOrder,
		})
	if err != nil {
		return nil, err
	}
	c.printf("  matrices A precomputed: %d windows × %d m\n", st.design.nW, nm)
	reportCondition(st.design, c)
	if a.Method == methodMonotone {
		c.printf("  method: monotone functions, %d bins per window, "+
			"regularization order %d\n", st.design.bins, st.design.regOrder)
		if a.NoZeroBoundary {
			c.warnf("[warning] --no-zero-boundary has no effect with " +
				"--method monotone; ignored\n")
		}
	}
	c.printf("  kernel order: %d (%s); result converted to dV/dln r\n",
		st.design.kernelOrder, kernelOrderLabel(st.design.kernelOrder))
	reportEnsembleSize(a, st.design.nW*st.design.nM, c)
	return st, nil
}

// batchOutDir — каталог результатов: --batch-out-dir, затем --out-dir,
// затем каталог входного файла.
func batchOutDir(a *Args) (string, error) {
	if a.BatchOutDir != "" {
		return a.BatchOutDir, nil
	}
	if a.OutDir != "" {
		return a.OutDir, nil
	}
	return defaultOutDir(a.Batch)
}

// writeBatchHeaders — шапки общих файлов результата (обнуляют прошлый прогон).
func writeBatchHeaders(outDir string, b *batchInput, ds *depolSet,
	common []commonWl, writeLidar bool) error {
	writeHeader := func(name, content string) error {
		path := filepath.Join(outDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to open file for writing: %w", err)
		}
		return nil
	}

	if err := writeHeader("moments.txt",
		"# H  Vf  Vc  V  rmf  rmc  rm  reff  N  sigma_f  sigma_c  sigma\n"+
			"# V — µm³/cm³; rm/reff — µm; N — cm⁻³; sigma — dimensionless.\n"); err != nil {
		return err
	}
	if err := writeHeader("refractive.txt", "# H  Re(m)  Im(m)\n"); err != nil {
		return err
	}
	{
		var sb strings.Builder
		sb.WriteString("# H  ")
		for j := 0; j < b.nch; j++ {
			if j > 0 {
				sb.WriteString("  ")
			}
			sb.WriteString(b.ch[j].name)
		}
		sb.WriteString("  Err\n# relative errors |g_fit-g1|/g1 " +
			"(Err = mean over channels)\n")
		if err := writeHeader("errors.txt", sb.String()); err != nil {
			return err
		}
	}

	// δ_l — файл формируется всегда; при отсутствии столбцов S11/S22 — nan
	{
		var sb strings.Builder
		sb.WriteString("# H")
		for _, wl := range ds.wl {
			fmt.Fprintf(&sb, "  delta_l_%g", wl*1e3)
		}
		for _, wl := range b.depWl {
			fmt.Fprintf(&sb, "  delta_l_meas_%g", wl*1e3)
		}
		sb.WriteString("\n# linear depolarization at 180 deg, % " +
			"(delta_l = (S11-S22)/(S11+S22)*100)\n")
		if !depolSetAvailable(ds) {
			sb.WriteString("# nan: tables have no valid S11/S22 columns\n")
		}
		if b.ndep > 0 {
			sb.WriteString("# delta_l_meas_* — measured depolarization from " +
				"the dep<nm> columns\n")
		}
		if err := writeHeader("delpol.txt", sb.String()); err != nil {
			return err
		}
	}

	if writeLidar {
		var sb strings.Builder
		sb.WriteString("# H  ")
		for k := 0; k < len(common); k++ {
			if k > 0 {
				sb.WriteString("  ")
			}
			fmt.Fprintf(&sb, "S_%g", common[k].wl*1e3)
		}
		sb.WriteString("\n# retrieved lidar ratio S = α/β at common wavelengths\n")
		if err := writeHeader("lidar_ratios.txt", sb.String()); err != nil {
			return err
		}
	}
	return nil
}

// newBatchMetrics — накопители по высотам для файлов и сводных графиков.
func newBatchMetrics(b *batchInput, ds *depolSet, nl int) *metrics {
	met := &metrics{nAlt: b.nAlt, nl: nl}
	met.err = make([]float64, b.nAlt)
	met.V = make([]float64, b.nAlt)
	met.Vc = make([]float64, b.nAlt)
	met.Vf = make([]float64, b.nAlt)
	met.rmf = make([]float64, b.nAlt)
	met.rmc = make([]float64, b.nAlt)
	met.rmo = make([]float64, b.nAlt)
	met.reff = make([]float64, b.nAlt)
	met.Nc = make([]float64, b.nAlt)
	met.sgf = make([]float64, b.nAlt)
	met.sgc = make([]float64, b.nAlt)
	met.sgo = make([]float64, b.nAlt)
	met.mRe = make([]float64, b.nAlt)
	met.mIm = make([]float64, b.nAlt)
	met.S = make([]float64, b.nAlt*max(nl, 1))
	if depolSetAvailable(ds) {
		met.dpolWl = append([]float64(nil), ds.wl...)
		met.dpol = make([][]float64, len(ds.wl))
		for i := range met.dpol {
			met.dpol[i] = make([]float64, b.nAlt)
		}
	}
	if b.ndep > 0 {
		met.dpolMeasWl = append([]float64(nil), b.depWl...)
		met.dpolMeas = make([][]float64, b.ndep)
		for k := range met.dpolMeas {
			met.dpolMeas[k] = make([]float64, b.nAlt)
			for h := 0; h < b.nAlt; h++ {
				met.dpolMeas[k][h] = b.dep[h*b.ndep+k]
			}
		}
	}
	return met
}

// batchDepol — ограничение по измеренной δ_l в пакетном режиме: строка в
// функционале (mode=func) или штраф при ранжировании (mode=rank).
type batchDepol struct {
	b        *batchInput
	mre, mim []float64
	proj     *depolProj
	cols     []int
	w        float64
	mode     depolMode
}

// setupBatchDepol — сопоставление измеренных δ_l с длинами волн S11/S22 и
// проекция на базис (proj == nil — ограничение не применяется).
func setupBatchDepol(a *Args, c *console, st *batchSetup) *batchDepol {
	bd := &batchDepol{b: st.b, mre: st.mre, mim: st.mim, w: a.DepolRatioW,
		mode: a.DepolRatioMode}
	if !a.DepolRatioCons {
		return bd
	}
	if !depolSetAvailable(st.ds) {
		c.warnf("[warning] --depol-ratio-cons: tables have no " +
			"valid S11/S22 columns; constraint is not applied\n")
		return bd
	}
	var iw []int
	iw, bd.cols = depolColumnMatch(st.ds, st.b.depWl, a.Batch, c)
	if len(iw) == 0 {
		c.warnf("[warning] --depol-ratio-cons: no measured " +
			"depolarization matching S11/S22 wavelengths; constraint is " +
			"not applied\n")
		return bd
	}
	if !(a.DepolRatioW > 0) {
		c.printf("depolarization ratio constraint δ_l: disabled " +
			"(--depol-ratio-w 0)\n")
		return bd
	}
	bd.proj = depolProjBuild(st.ds, st.design, iw)
	if bd.proj == nil {
		return bd
	}
	var sb strings.Builder
	for k := range iw {
		if k > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "%.6g nm", st.ds.wl[iw[k]]*1e3)
	}
	c.printf("depolarization ratio constraint δ_l: enabled "+
		"(weight %g, mode %s; measured at %s)\n", a.DepolRatioW,
		a.DepolRatioMode, sb.String())
	return bd
}

// targets — измеренные δ_l для высоты h по сопоставленным столбцам.
func (d *batchDepol) targets(h int) []float64 {
	out := make([]float64, len(d.cols))
	for k, col := range d.cols {
		out[k] = d.b.dep[h*d.b.ndep+col]
	}
	return out
}

// cons — ограничение для функционала высоты h (nil — не применяется).
func (d *batchDepol) cons(h int) *depolCons {
	if d.proj == nil || d.mode != depolModeFunc || !(d.w > 0) {
		return nil
	}
	return &depolCons{proj: d.proj, w: d.w, targets: d.targets(h)}
}

// rank — штраф для ранжирования решений высоты h (nil — не применяется).
func (d *batchDepol) rank(sols *solutions, h int) []float64 {
	if d.proj == nil || d.mode != depolModeRank || !(d.w > 0) {
		return nil
	}
	return rankByDepol(sols, d.proj, d.mre, d.mim, d.targets(h), d.w)
}

// solveBatchAltitudes — решения по всем высотам и запись профилей. При
// --lidar-ratio-cons решения всех высот удерживаются в памяти, чтобы
// наложить ограничение на гладкость S = α/β.
func solveBatchAltitudes(a *Args, c *console, st *batchSetup, outDir string,
	common []commonWl, nl int, writeLidar bool, met *metrics, bd *batchDepol,
	rimg *batchReportImages) error {
	b := st.b
	retain := a.LidarRatioCons && b.nAlt >= 2 && nl > 0
	if retain {
		per := make([]*solutions, b.nAlt)
		depPen := make([][]float64, b.nAlt)
		for h := 0; h < b.nAlt; h++ {
			g1 := b.data[h*b.nch:]
			c.printf("  scanning altitude %g m...\n", b.alt[h])
			sols, err := scanVariants(st.design, g1, b.nch, a.GammaSelect,
				a.GammaRel, a.GammaMin, a.GammaMax, a.NGamma, a.Nonneg,
				!a.Absolute, bd.cons(h), c)
			if err != nil {
				return err
			}
			if sols.n == 0 {
				return errors.New("no valid [r0, r1] window")
			}
			per[h] = sols
			depPen[h] = bd.rank(sols, h)
		}

		c.printf("  applying constraint: common wavelengths ")
		for k := 0; k < nl; k++ {
			if k > 0 {
				c.printf(", ")
			}
			c.printf("%g nm", common[k].wl*1e3)
		}
		c.printf("\n")

		S := make([][]float64, b.nAlt)
		for h := 0; h < b.nAlt; h++ {
			n := per[h].n
			S[h] = make([]float64, n*max(nl, 1))
			for t := 0; t < n; t++ {
				id := per[h].sol[t].id
				for k := 0; k < nl; k++ {
					S[h][id*nl+k] = lidarRatioValue(per[h].sol[t].gFit,
						common, k)
				}
			}
		}
		applyLidarConstraint(per, b.nAlt, b.alt, nl, a.LidarRatioW,
			a.LidarRatioIter, a, S, depPen, c)

		for h := 0; h < b.nAlt; h++ {
			g1 := b.data[h*b.nch:]
			if err := emitProfile(a, outDir, b.alt[h], b, st.solveCh, g1, per[h],
				st.kts, st.mre, st.mim, st.design.geom, st.r1MaxEff, common, nl,
				writeLidar, met, h, st.ds, st.src, rimg, c); err != nil {
				return err
			}
		}
		return nil
	}

	if a.LidarRatioCons {
		if b.nAlt < 2 {
			c.printf("  [warning] --lidar-ratio-cons: at least 2 " +
				"altitudes required; constraint is not applied\n")
		} else {
			c.printf("  [warning] --lidar-ratio-cons: no common " +
				"β∩α wavelengths for the lidar ratio; constraint is " +
				"not applied\n")
		}
	}
	for h := 0; h < b.nAlt; h++ {
		g1 := b.data[h*b.nch:]
		c.printf("  scanning altitude %g m...\n", b.alt[h])
		sols, err := scanVariants(st.design, g1, b.nch, a.GammaSelect,
			a.GammaRel, a.GammaMin, a.GammaMax, a.NGamma, a.Nonneg,
			!a.Absolute, bd.cons(h), c)
		if err != nil {
			return err
		}
		if sols.n == 0 {
			return errors.New("no valid [r0, r1] window")
		}
		bd.rank(sols, h)
		if err := emitProfile(a, outDir, b.alt[h], b, st.solveCh, g1, sols,
			st.kts, st.mre, st.mim, st.design.geom, st.r1MaxEff, common, nl,
			writeLidar, met, h, st.ds, st.src, rimg, c); err != nil {
			return err
		}
	}
	return nil
}

// batchPlotSpec — описание сводного профиля пакета: одинаково используется
// для файлов графиков и для PNG-картинок отчёта.
type batchPlotSpec struct {
	name    string
	title   string
	xlabel  string
	caption string
	vals    [][]float64
	names   []string
	dashed  []bool
	logAuto bool
}

// batchProfilePlotSpecs — серии сводных профилей (кроме refractive).
func batchProfilePlotSpecs(b *batchInput, met *metrics, common []commonWl,
	nl int) []batchPlotSpec {
	errPct := make([]float64, b.nAlt)
	for i := 0; i < b.nAlt; i++ {
		errPct[i] = met.err[i] * 100.0
	}
	specs := []batchPlotSpec{
		{name: "error", title: "Mean relative retrieval error, %",
			xlabel: "Err, %", caption: "Средняя относительная ошибка восстановления",
			vals: [][]float64{errPct}, names: []string{"Err"}, logAuto: true},
		{name: "volumes", title: "Volume concentration of modes",
			xlabel: "V, µm³/cm³", caption: "Объёмная концентрация мод",
			vals:  [][]float64{met.V, met.Vc, met.Vf},
			names: []string{"V", "Vc", "Vf"}, logAuto: true},
		{name: "radii", title: "Mode radii", xlabel: "r, µm",
			caption: "Радиусы мод",
			vals:    [][]float64{met.rmf, met.rmc, met.reff},
			names:   []string{"rmf", "rmc", "reff"}, logAuto: true},
		{name: "concentration", title: "Number concentration",
			xlabel: "N, cm⁻³", caption: "Счётная концентрация",
			vals: [][]float64{met.Nc}, names: []string{"N"}, logAuto: true},
		{name: "sigma", title: "Lognormal mode widths", xlabel: "sigma",
			caption: "Ширины логнормальных мод",
			vals:    [][]float64{met.sgf, met.sgc, met.sgo},
			names:   []string{"sigma_f", "sigma_c", "sigma"}, logAuto: true},
	}
	if nl > 0 {
		nSer := nl
		if nSer > 32 {
			nSer = 32
		}
		svals := make([][]float64, nSer)
		snames := make([]string, nSer)
		for k := 0; k < nSer; k++ {
			svals[k] = met.S[k*b.nAlt:]
			snames[k] = fmt.Sprintf("S_{%g}", common[k].wl*1e3)
		}
		specs = append(specs, batchPlotSpec{
			name: "lidar_ratios", title: "Lidar ratio S = α/β",
			xlabel: "S = α/β", caption: "Лидарное отношение S = α/β",
			vals: svals, names: snames, logAuto: true})
	}
	if len(met.dpol) > 0 || len(met.dpolMeas) > 0 {
		series := make([][]float64, 0, len(met.dpol)+len(met.dpolMeas))
		series = append(series, met.dpol...)
		series = append(series, met.dpolMeas...)
		if anyFiniteSeries(series) {
			dnames := make([]string, 0, len(series))
			dashed := make([]bool, 0, len(series))
			for k := range met.dpol {
				dnames = append(dnames, fmt.Sprintf("δ_l(%g nm)",
					met.dpolWl[k]*1e3))
				dashed = append(dashed, false)
			}
			for k := range met.dpolMeas {
				dnames = append(dnames, fmt.Sprintf("δ_l^{meas}(%g nm)",
					met.dpolMeasWl[k]*1e3))
				dashed = append(dashed, true)
			}
			specs = append(specs, batchPlotSpec{
				name: "delpol", title: "Linear depolarization ratio",
				xlabel: "δ_l, %", caption: "Линейная деполяризация δ_l",
				vals: series, names: dnames, dashed: dashed, logAuto: false})
		}
	}
	return specs
}

// batchPlots — сводные графики по пакету.
func batchPlots(a *Args, c *console, outDir string, b *batchInput,
	met *metrics, common []commonWl, nl int) {
	po := plotOpts{format: a.PlotFormat, dpi: a.PlotDpi,
		fontSize: a.PlotFontSize, backend: a.Plotting, con: c}
	ext := plotFormatExt(a.PlotFormat)

	pp := filepath.Join(outDir, "refractive."+ext)
	plotRefractiveProfile(&po, pp, b.alt, met.mRe, met.mIm)

	for _, sp := range batchProfilePlotSpecs(b, met, common, nl) {
		pp = filepath.Join(outDir, sp.name+"."+ext)
		plotProfilesStyled(&po, pp, sp.title, sp.xlabel, len(sp.vals), b.alt,
			sp.vals, sp.names, sp.dashed, sp.logAuto)
	}
}

func runBatch(a *Args, c *console, rep *reportCapture) error {
	st, err := prepareBatch(a, c)
	if err != nil {
		return err
	}
	bd := setupBatchDepol(a, c, st)

	outDir, err := batchOutDir(a)
	if err != nil {
		return err
	}
	if err := mkdirP(outDir); err != nil {
		return err
	}

	common := commonWavelengths(st.b)
	nl := len(common)
	writeLidar := a.LidarRatioCons && nl > 0
	if err := writeBatchHeaders(outDir, st.b, st.ds, common, writeLidar); err != nil {
		return err
	}
	met := newBatchMetrics(st.b, st.ds, nl)
	var rimg *batchReportImages
	if rep != nil && !a.NoPlot {
		rimg = &batchReportImages{}
	}

	if err := solveBatchAltitudes(a, c, st, outDir, common, nl, writeLidar,
		met, bd, rimg); err != nil {
		return err
	}
	if !a.NoPlot {
		batchPlots(a, c, outDir, st.b, met, common, nl)
	}

	c.printf("batch processing finished; results: %s\n", outDir)
	if rep != nil {
		if err := writeBatchReportDoc(a, c, rep, outDir, st, met, common, nl,
			rimg); err != nil {
			return err
		}
	}
	return nil
}
