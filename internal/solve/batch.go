package solve

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ---------------- разбор txt ----------------
type batchChannel struct {
	kind chKind
	wlUm float64
	name string
}

type batchInput struct {
	ch   []batchChannel
	nch  int
	alt  []float64
	data []float64 // nAlt x nch, row-major
	nAlt int
}

func parseBatchTxt(path string) *batchInput {
	f, err := os.Open(path)
	if err != nil {
		fatal("failed to open file: %s", path)
	}
	defer f.Close()

	b := &batchInput{}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	haveHeader, haveRow := false, false

	for scan.Scan() {
		s := strings.TrimSpace(scan.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		toks := strings.Fields(s)
		if len(toks) == 0 {
			continue
		}
		if !haveHeader {
			low := strings.ToLower(toks[0])
			if low != "altitude" && low != "alt" && low != "h" {
				fatal("first header field must be 'Altitude', got '%s'", toks[0])
			}
			b.nch = len(toks) - 1
			if b.nch <= 0 {
				fatal("no channels in header (expected b/a<nm>)")
			}
			b.ch = make([]batchChannel, b.nch)
			for i := 1; i < len(toks); i++ {
				nm := toks[i]
				c0 := nm[0]
				if c0 >= 'A' && c0 <= 'Z' {
					c0 = c0 - 'A' + 'a'
				}
				digits := len(nm) > 1
				for k := 1; k < len(nm); k++ {
					if nm[k] < '0' || nm[k] > '9' {
						digits = false
					}
				}
				if !digits || (c0 != 'b' && c0 != 'a') {
					fatal("unknown channel name '%s' (expected b<nm> or a<nm>)", nm)
				}
				ch := &b.ch[i-1]
				if c0 == 'b' {
					ch.kind = chBeta
				} else {
					ch.kind = chAlpha
				}
				wl, err := strconv.Atoi(nm[1:])
				if err != nil {
					fatal("unknown channel name '%s' (expected b<nm> or a<nm>)", nm)
				}
				ch.wlUm = float64(wl) * 1e-3
				ch.name = nm
			}
			haveHeader = true
			continue
		}

		if len(toks) != b.nch+1 {
			fatal("expected %d coefficients per row, got %d: '%s'",
				b.nch, len(toks)-1, s)
		}
		hv, err := strconv.ParseFloat(toks[0], 64)
		if err != nil {
			fatal("failed to parse line '%s'", s)
		}
		b.alt = append(b.alt, hv)
		for i := 0; i < b.nch; i++ {
			v, err := strconv.ParseFloat(toks[i+1], 64)
			if err != nil {
				fatal("failed to parse line '%s'", s)
			}
			b.data = append(b.data, v)
		}
		haveRow = true
	}
	if err := scan.Err(); err != nil {
		fatal("failed to read file %s: %v", path, err)
	}
	if !haveHeader {
		fatal("file %s is empty", path)
	}
	if !haveRow {
		fatal("no altitude rows in file %s", path)
	}
	b.nAlt = len(b.alt)
	return b
}

// ---------------- общие длины волн и лидарное отношение ----------------
type commonWl struct {
	wl float64
	ib int // индекс канала β
	ia int // индекс канала α
}

func commonWavelengths(b *batchInput) []commonWl {
	wls := []float64{}
	ibs := []int{}
	ias := []int{}
	for j := 0; j < b.nch; j++ {
		wl := b.ch[j].wlUm
		k := -1
		for i := range wls {
			if wls[i] == wl {
				k = i
				break
			}
		}
		if k < 0 {
			k = len(wls)
			wls = append(wls, wl)
			ibs = append(ibs, -1)
			ias = append(ias, -1)
		}
		// как словарь в Python: для одинаковых wl побеждает последний
		if b.ch[j].kind == chBeta {
			ibs[k] = j
		} else {
			ias[k] = j
		}
	}
	out := []commonWl{}
	for i := range wls {
		if ibs[i] >= 0 && ias[i] >= 0 {
			out = append(out, commonWl{wls[i], ibs[i], ias[i]})
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].wl < out[b].wl })
	return out
}

func lidarRatioValue(gFit []float64, common []commonWl, k int) float64 {
	b := gFit[common[k].ib]
	a := gFit[common[k].ia]
	if b > 0 && a > 0 {
		return a / b
	}
	return math.NaN()
}

// ---------------- медианы и опорный профиль S ----------------
func nanmedian(v []float64) float64 {
	tmp := make([]float64, 0, len(v))
	for _, x := range v {
		if isFinite(x) {
			tmp = append(tmp, x)
		}
	}
	if len(tmp) == 0 {
		return math.NaN()
	}
	sort.Float64s(tmp)
	m := len(tmp)
	if m%2 == 1 {
		return tmp[m/2]
	}
	return 0.5 * (tmp[m/2-1] + tmp[m/2])
}

// windowedMedianRef — скользящая медиана (окно 3, у краёв сужается).
func windowedMedianRef(S []float64, nAlt, nl int) []float64 {
	out := make([]float64, nAlt*nl)
	col := make([]float64, nAlt)
	for h := 0; h < nAlt; h++ {
		lo := 0
		if h > 0 {
			lo = h - 1
		}
		hi := h + 2
		if hi > nAlt {
			hi = nAlt
		}
		for li := 0; li < nl; li++ {
			m := 0
			for k := lo; k < hi; k++ {
				col[m] = S[k*nl+li]
				m++
			}
			out[h*nl+li] = nanmedian(col[:m])
		}
	}
	return out
}

func windowedMedianColumnInto(S []float64, nAlt, nl, li int, out []float64) {
	col := make([]float64, nAlt)
	for h := 0; h < nAlt; h++ {
		lo := 0
		if h > 0 {
			lo = h - 1
		}
		hi := h + 2
		if hi > nAlt {
			hi = nAlt
		}
		m := 0
		for k := lo; k < hi; k++ {
			col[m] = S[k*nl+li]
			m++
		}
		out[h*nl+li] = nanmedian(col[:m])
	}
}

// npMaxFloor — np.maximum(v, floor) с сохранением NaN.
func npMaxFloor(v, floorV float64) float64 {
	if math.IsNaN(v) {
		return math.NaN()
	}
	if v > floorV {
		return v
	}
	return floorV
}

func smoothRefProfile(alt, S []float64, nAlt, nl int) []float64 {
	out := make([]float64, nAlt*nl)
	for i := range out {
		out[i] = math.NaN()
	}
	if nAlt < 5 {
		return windowedMedianRef(S, nAlt, nl)
	}
	t := make([]float64, nAlt)
	for i := 0; i < nAlt; i++ {
		t[i] = alt[i] - alt[0]
	}
	span := t[nAlt-1] - t[0]
	if !(span > 0) {
		return windowedMedianRef(S, nAlt, nl)
	}
	for i := 0; i < nAlt; i++ {
		t[i] /= span
	}

	for li := 0; li < nl; li++ {
		ok := make([]int, 0, nAlt)
		for h := 0; h < nAlt; h++ {
			if isFinite(S[h*nl+li]) {
				ok = append(ok, h)
			}
		}
		cnt := len(ok)
		if cnt < 5 {
			windowedMedianColumnInto(S, nAlt, nl, li, out)
			continue
		}
		scale := 0.0
		for _, k := range ok {
			if v := math.Abs(S[k*nl+li]); v > scale {
				scale = v
			}
		}
		if !(scale > 0) || !isFinite(scale) {
			windowedMedianColumnInto(S, nAlt, nl, li, out)
			continue
		}
		xv := make([]float64, cnt)
		yv := make([]float64, cnt)
		for idx, k := range ok {
			xv[idx] = t[k]
			yv[idx] = S[k*nl+li] / scale
		}
		fit := smoothingSplineFit(xv, yv, nil, 1.0, xv)
		for idx, k := range ok {
			out[k*nl+li] = fit[idx] * scale
		}
	}
	return out
}

// ---------------- устойчивая сортировка решений по score ----------------
func reorderByScore(sols *solutions, score []float64) {
	n := sols.n
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return score[order[a]] < score[order[b]]
	})
	buf := make([]solution, n)
	for k := 0; k < n; k++ {
		buf[k] = sols.sol[order[k]]
	}
	copy(sols.sol, buf)
}

func topIdsEqual(sols *solutions, g int, prev []int, off int) bool {
	for k := 0; k < g; k++ {
		if sols.sol[k].id != prev[off+k] {
			return false
		}
	}
	return true
}

func storeTopIds(sols *solutions, g int, prev []int, off int) {
	for k := 0; k < g; k++ {
		prev[off+k] = sols.sol[k].id
	}
}

func applyLidarConstraint(per []*solutions, nAlt int, alt []float64, nl int,
	w float64, maxIter, best int, S [][]float64) {
	dbg := os.Getenv("CSOLVE_LIDAR_DEBUG") != ""
	gEff := make([]int, nAlt)
	kEff := make([]int, nAlt)
	for h := 0; h < nAlt; h++ {
		g := best
		if g > per[h].n {
			g = per[h].n
		}
		if g < 1 {
			g = 1
		}
		gEff[h] = g
		k := 3 * gEff[h]
		if k < 9 {
			k = 9
		}
		if k > per[h].n {
			k = per[h].n
		}
		kEff[h] = k
	}
	gmax := 1
	for h := 0; h < nAlt; h++ {
		if gEff[h] > gmax {
			gmax = gEff[h]
		}
	}

	prev := make([]int, nAlt*gmax)
	havePrev := false

	maxpool := nAlt
	for h := 0; h < nAlt; h++ {
		if kEff[h] > maxpool {
			maxpool = kEff[h]
		}
	}

	est := make([]float64, nAlt*nl)
	pool := make([]float64, max(maxpool, 1))
	logRef := make([]float64, max(nl, 1))
	iter := maxIter
	if iter < 1 {
		iter = 1
	}

	for it := 0; it < iter; it++ {
		// опорный профиль: медиана top-K решений каждой высоты, сглаженная
		for h := 0; h < nAlt; h++ {
			k := kEff[h]
			for li := 0; li < nl; li++ {
				for t := 0; t < k; t++ {
					pool[t] = S[h][per[h].sol[t].id*nl+li]
				}
				est[h*nl+li] = nanmedian(pool[:k])
			}
		}
		ref := smoothRefProfile(alt, est, nAlt, nl)
		for li := 0; li < nl; li++ {
			for h := 0; h < nAlt; h++ {
				pool[h] = ref[h*nl+li]
			}
			fill := nanmedian(pool[:nAlt])
			if isFinite(fill) {
				for h := 0; h < nAlt; h++ {
					if !isFinite(ref[h*nl+li]) {
						ref[h*nl+li] = fill
					}
				}
			}
		}
		if dbg {
			fmt.Printf("iter %d: ref = [", it)
			for h := 0; h < nAlt; h++ {
				if h > 0 {
					fmt.Printf(", ")
				}
				fmt.Printf("[")
				for li := 0; li < nl; li++ {
					if li > 0 {
						fmt.Printf(", ")
					}
					fmt.Printf("%.15g", ref[h*nl+li])
				}
				fmt.Printf("]")
			}
			fmt.Printf("]\n")
		}

		for h := 0; h < nAlt; h++ {
			n := per[h].n
			score := make([]float64, n)
			for li := 0; li < nl; li++ {
				rv := npMaxFloor(ref[h*nl+li], 1e-300)
				logRef[li] = math.Log(rv)
			}
			for t := 0; t < n; t++ {
				sv := S[h][per[h].sol[t].id*nl:]
				sum := 0.0
				cnt := 0
				for li := 0; li < nl; li++ {
					lv := math.Log(npMaxFloor(sv[li], 1e-300))
					d := math.Abs(lv - logRef[li])
					if isFinite(d) {
						sum += d
						cnt++
					}
				}
				dev := 0.0
				if cnt > 0 {
					dev = sum / float64(cnt)
				}
				if !isFinite(dev) {
					dev = 0
				}
				score[t] = per[h].sol[t].err + w*dev
			}
			reorderByScore(per[h], score)
			if dbg {
				for t := 0; t < n && t < 3; t++ {
					s := &per[h].sol[t]
					sv := S[h][s.id*nl:]
					sum := 0.0
					cnt := 0
					for li := 0; li < nl; li++ {
						lv := math.Log(npMaxFloor(sv[li], 1e-300))
						rv := math.Log(npMaxFloor(ref[h*nl+li], 1e-300))
						d := math.Abs(lv - rv)
						if isFinite(d) {
							sum += d
							cnt++
						}
					}
					dev := 0.0
					if cnt > 0 {
						dev = sum / float64(cnt)
					}
					s1 := 0.0
					if nl > 1 {
						s1 = sv[1]
					}
					fmt.Printf("  h=%g top%d: (score=%g, err=%g, dev=%g, id=%d, "+
						"m=%g%+gj, deg=%d, r0=%.5g, r1=%.5g) S=[%g, %g]\n",
						alt[h], t, s.err+w*dev, s.err, dev, s.id, s.mRe, s.mIm,
						s.degree, s.r0, s.r1, sv[0], s1)
				}
			}
		}

		changed := !havePrev
		off := 0
		for h := 0; h < nAlt; h++ {
			if havePrev && !topIdsEqual(per[h], gEff[h], prev, off) {
				changed = true
			}
			off += gmax
		}
		if !changed {
			break
		}
		off = 0
		for h := 0; h < nAlt; h++ {
			storeTopIds(per[h], gEff[h], prev, off)
			off += gmax
		}
		havePrev = true
	}
}

// ---------------- вывод файлов ----------------
func mkdirP(path string) {
	if err := os.MkdirAll(path, 0777); err != nil {
		fatal("failed to create directory %s: %v", path, err)
	}
}

func defaultOutDir(batchPath string) string {
	abs, err := filepath.Abs(batchPath)
	if err != nil {
		fatal("failed to get current directory")
	}
	return filepath.Dir(abs)
}

func openAppend(path string) *os.File {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fatal("failed to open file for writing: %s", path)
	}
	return f
}

func writeProfile(outDir string, h float64, b *batchInput, g1 []float64,
	p *profile) float64 {
	path := filepath.Join(outDir, fmt.Sprintf("dist_%s.txt", cG(h)))
	var sb strings.Builder
	fmt.Fprintf(&sb, "# altitude H = %g m\n", h)
	sb.WriteString("# input data (β [1/(m·sr)], α [1/m]): ")
	for j := 0; j < b.nch; j++ {
		if j > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%s=%s", b.ch[j].name, cG(g1[j]))
	}
	sb.WriteString("\n# r, µm  dV/dln r, µm³/cm³\n")
	for i := 0; i < p.nr; i++ {
		nv := p.nAvg[i] / cm3PerM3
		if isFinite(nv) {
			fmt.Fprintf(&sb, "%s  %s\n", cG(p.rGrid[i]), cG(nv))
		}
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		fatal("failed to open file for writing: %s", path)
	}

	mf := &p.modes.Fine
	mc := &p.modes.Coarse
	mo := &p.modes.Overall
	fm := openAppend(filepath.Join(outDir, "moments.txt"))
	fmt.Fprintf(fm, "%s  %s  %s  %s  %s  %s  %s  %s  %s  %s  %s  %s\n",
		cG(h), cG(mf.V/cm3PerM3), cG(mc.V/cm3PerM3),
		cG(p.moments.VolConcUm3PerCm3), cG(mf.Rm), cG(mc.Rm), cG(mo.Rm),
		cG(p.moments.REffUm), cG(p.moments.NCm3), cG(mf.Sigma),
		cG(mc.Sigma), cG(mo.Sigma))
	fm.Close()

	fr := openAppend(filepath.Join(outDir, "refractive.txt"))
	fmt.Fprintf(fr, "%s  %s  %s\n", cG(h), cG(p.mRe), cG(p.mIm))
	fr.Close()

	fe := openAppend(filepath.Join(outDir, "errors.txt"))
	fmt.Fprintf(fe, "%s", cG(h))
	errSum := 0.0
	for j := 0; j < b.nch; j++ {
		rel := math.Abs(p.gFit[j]-g1[j]) / g1[j]
		fmt.Fprintf(fe, "  %s", cG(rel))
		errSum += rel
	}
	errSum /= float64(b.nch)
	fmt.Fprintf(fe, "  %s\n", cG(errSum))
	fe.Close()
	return errSum
}

type metrics struct {
	err, V, Vc, Vf []float64
	rmf, rmc, reff []float64
	Nc             []float64
	sgf, sgc, sgo  []float64
	mRe, mIm       []float64
	S              []float64   // nl x nAlt
	dpol           [][]float64 // ndepol x nAlt, δ_l, %
	dpolWl         []float64   // длины волн δ_l, µm
	nAlt, nl       int
}

func emitProfile(a *Args, outDir string, h float64, b *batchInput,
	solveCh []channel, g1 []float64, sols *solutions, kts []*ktable,
	mre, mim []float64, geom []*geom, r1Max float64, common []commonWl,
	nl int, writeLidar bool, met *metrics, hi int, ds *depolSet) {
	prof := profileFinalize(a, solveCh, kts, mre, mim, sols, geom, a.R0Min,
		r1Max, ds)
	err := writeProfile(outDir, h, b, g1, prof)

	dStr := ""
	if ds != nil && len(ds.wl) > 0 {
		fd := openAppend(filepath.Join(outDir, "delpol.txt"))
		fmt.Fprintf(fd, "%s", cG(h))
		for i := range ds.wl {
			v := math.NaN()
			if i < len(prof.deltaL) {
				v = prof.deltaL[i]
			}
			fmt.Fprintf(fd, "  %s", cG(v))
		}
		fd.WriteString("\n")
		fd.Close()
	}

	if len(prof.deltaL) > 0 {
		var acc strings.Builder
		acc.WriteString("  δ_l = ")
		for i, v := range prof.deltaL {
			if i > 0 {
				acc.WriteString("/")
			}
			if isFinite(v) {
				fmt.Fprintf(&acc, "%.4g", v)
			} else {
				acc.WriteString("nan")
			}
		}
		acc.WriteString(" %")
		dStr = acc.String()
		if met != nil {
			for i := range prof.deltaL {
				if i < len(met.dpol) {
					met.dpol[i][hi] = prof.deltaL[i]
				}
			}
		}
	}

	if met != nil {
		met.err[hi] = err
		met.V[hi] = prof.moments.VolConcUm3PerCm3
		met.Vc[hi] = prof.modes.Coarse.V / cm3PerM3
		met.Vf[hi] = prof.modes.Fine.V / cm3PerM3
		met.rmf[hi] = prof.modes.Fine.Rm
		met.rmc[hi] = prof.modes.Coarse.Rm
		met.reff[hi] = prof.moments.REffUm
		met.Nc[hi] = prof.moments.NCm3
		met.sgf[hi] = prof.modes.Fine.Sigma
		met.sgc[hi] = prof.modes.Coarse.Sigma
		met.sgo[hi] = prof.modes.Overall.Sigma
		met.mRe[hi] = prof.mRe
		met.mIm[hi] = prof.mIm
		for k := 0; k < nl && met.nl > 0; k++ {
			met.S[k*met.nAlt+hi] = lidarRatioValue(prof.gFit, common, k)
		}
	}

	if !a.NoPlot {
		po := plotOpts{a.PlotFormat, a.PlotDpi, a.PlotFontSize}
		pp := filepath.Join(outDir,
			fmt.Sprintf("plot_%s.%s", cG(h), plotFormatExt(a.PlotFormat)))
		plotProfile(&po, pp, prof.rGrid, prof.nAvg, solveCh, g1, prof.gFit, h)
	}

	sStr := ""
	if writeLidar {
		fl := openAppend(filepath.Join(outDir, "lidar_ratios.txt"))
		fmt.Fprintf(fl, "%s", cG(h))
		for k := 0; k < nl; k++ {
			v := lidarRatioValue(prof.gFit, common, k)
			fmt.Fprintf(fl, "  %s", cG(v))
		}
		fl.WriteString("\n")
		fl.Close()

		var acc strings.Builder
		acc.WriteString("  S = ")
		for k := 0; k < nl; k++ {
			if k > 0 {
				acc.WriteString(", ")
			}
			v := lidarRatioValue(prof.gFit, common, k)
			if isFinite(v) {
				fmt.Fprintf(&acc, "%g", v)
			} else {
				acc.WriteString("nan")
			}
		}
		sStr = acc.String()
	}

	fmt.Printf("  altitude %s m: ⟨m⟩ = %.3f%+.3fj, N = %.3g cm⁻³%s%s\n",
		cG(h), prof.mRe, prof.mIm, prof.moments.NCm3, sStr, dStr)
}

// ---------------- основной сценарий ----------------
func runBatch(a *Args) {
	b := parseBatchTxt(a.Batch)
	src := a.tableSource()
	mre, mim := tableMGrid(src)
	nm := len(mre)

	fmt.Printf("batch processing: %s\n", a.Batch)
	fmt.Printf("  altitudes: %d; channels: %d\n", b.nAlt, b.nch)
	for j := 0; j < b.nch; j++ {
		kind := "alpha"
		if b.ch[j].kind == chBeta {
			kind = "beta"
		}
		fmt.Printf("  channel: %s (%s, %g nm)\n", b.ch[j].name, kind,
			b.ch[j].wlUm*1e3)
	}
	fmt.Printf("  units: β [1/(m·sr)], α [1/m]\n")
	if a.LidarRatioCons {
		fmt.Printf("lidar ratio S=α/β smoothness constraint along altitude: "+
			"enabled (weight %g, iterations <= %d)\n", a.LidarRatioW,
			a.LidarRatioIter)
	} else {
		fmt.Printf("lidar ratio S=α/β smoothness constraint along altitude: " +
			"disabled\n")
	}

	// каналы решения
	solveCh := make([]channel, b.nch)
	for j := 0; j < b.nch; j++ {
		solveCh[j] = channel{b.ch[j].kind, b.ch[j].wlUm, nil, tableRefWlUm, 1.0}
	}

	cbackScale := 1.0
	if a.TableCback4pi {
		cbackScale = 1.0 / (4.0 * csPi)
	}

	// покрытие таблиц -> r1_max_eff
	capmin := math.Inf(1)
	for i := 0; i < nm; i++ {
		chs := loadTableChannels(src, mre[i], mim[i], solveCh,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		if chs.cap < capmin {
			capmin = chs.cap
		}
	}
	r1MaxEff := math.Min(a.R1Max, capmin)
	if r1MaxEff < a.R1Min {
		fatal("table coverage in x limits r1_max = %g µm < "+
			"r1_min = %g — decrease --r1-min, choose another --type, "+
			"or extend the tables (%s)", r1MaxEff, a.R1Min, src.describe())
	}

	fmt.Printf("table-based kernel mode (external cross sections), %s\n",
		src.describe())
	fmt.Printf("  table λ_ref = %g µm (cross sections are rescaled as "+
		"(λ/λ_ref)²)\n", a.TableRefWl)
	if a.TableCback4pi {
		fmt.Printf("  Cback convention = 4pi\n")
	} else {
		fmt.Printf("  Cback convention = dsigma\n")
	}
	{
		c0 := loadTableChannels(src, mre[0], mim[0], solveCh,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		for j := 0; j < b.nch; j++ {
			fmt.Printf("  %-14s x_max = %g, r_max = %.3g µm\n",
				c0.ch[j].name(), c0.ch[j].tab.xMax,
				c0.ch[j].tab.xMax*c0.ch[j].wlUm/(2.0*csPi))
		}
	}
	fmt.Printf("  effective r1_max = %g µm (requested: %g) — windows and "+
		"r grid are limited\n", r1MaxEff, a.R1Max)

	// таблицы ядер — один раз на пакет
	fmt.Printf("building kernel tables (%d m values, once per batch)...\n", nm)
	kts := make([]*ktable, nm)
	for i := 0; i < nm; i++ {
		chs := loadTableChannels(src, mre[i], mim[i], solveCh,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		kts[i] = ktableBuild(chs, a.R0Min, r1MaxEff, a.DxMax)
	}
	fmt.Printf("  table: %d r points per m\n", kts[0].n)

	// ядра S11/S22 для линейной деполяризации — тоже один раз на пакет
	ds := depolSetBuild(src, mre, mim, a.TableRefWl, a.R0Min, r1MaxEff,
		a.DxMax, a.TableExtrapolate)
	depolSetReport(ds, src, nm)

	// матрицы A — тоже один раз
	r0Grid := geomspace(a.R0Min, a.R0Max, a.NR0)
	r1Grid := geomspace(a.R1Min, r1MaxEff, a.NR1)
	design := designBuild(solveCh, b.nch, kts, mre, mim, a.Degrees, r0Grid,
		r1Grid, a.MinRatio, a.NBasis, !a.NoZeroBoundary, a.QuadOrder,
		a.LogSpan)
	fmt.Printf("  matrices A precomputed: %d windows × %d m\n", design.nW, nm)

	outDir := a.BatchOutDir
	if outDir == "" {
		outDir = defaultOutDir(a.Batch)
	}
	mkdirP(outDir)

	// шапки общих файлов
	writeHeader := func(name, content string) {
		path := filepath.Join(outDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			fatal("failed to open file for writing: %s", path)
		}
	}
	writeHeader("moments.txt",
		"# H  Vf  Vc  V  rmf  rmc  rm  reff  N  sigma_f  sigma_c  sigma\n"+
			"# V — µm³/cm³; rm/reff — µm; N — cm⁻³; sigma — dimensionless.\n")
	writeHeader("refractive.txt", "# H  Re(m)  Im(m)\n")
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
		writeHeader("errors.txt", sb.String())
	}

	common := commonWavelengths(b)
	nl := len(common)
	writeLidar := a.LidarRatioCons && nl > 0
	retain := a.LidarRatioCons && b.nAlt >= 2 && nl > 0

	// δ_l — файл формируется всегда; при отсутствии столбцов S11/S22 — nan
	{
		var sb strings.Builder
		sb.WriteString("# H")
		for _, wl := range ds.wl {
			fmt.Fprintf(&sb, "  delta_l_%g", wl*1e3)
		}
		sb.WriteString("\n# linear depolarization at 180 deg, % " +
			"(delta_l = (S11-S22)/(S11+S22)*100)\n")
		if !depolSetAvailable(ds) {
			sb.WriteString("# nan: tables have no valid S11/S22 columns\n")
		}
		writeHeader("delpol.txt", sb.String())
	}

	met := metrics{nAlt: b.nAlt, nl: nl}
	met.err = make([]float64, b.nAlt)
	met.V = make([]float64, b.nAlt)
	met.Vc = make([]float64, b.nAlt)
	met.Vf = make([]float64, b.nAlt)
	met.rmf = make([]float64, b.nAlt)
	met.rmc = make([]float64, b.nAlt)
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

	if writeLidar {
		var sb strings.Builder
		sb.WriteString("# H  ")
		for k := 0; k < nl; k++ {
			if k > 0 {
				sb.WriteString("  ")
			}
			fmt.Fprintf(&sb, "S_%g", common[k].wl*1e3)
		}
		sb.WriteString("\n# retrieved lidar ratio S = α/β at common wavelengths\n")
		writeHeader("lidar_ratios.txt", sb.String())
	}

	if retain {
		per := make([]*solutions, b.nAlt)
		for h := 0; h < b.nAlt; h++ {
			g1 := b.data[h*b.nch:]
			fmt.Printf("scan over (m, r0, r1, degree)...\n")
			per[h] = scanVariants(design, g1, b.nch, a.GammaSelect,
				a.GammaRel, a.GammaMin, a.GammaMax, a.NGamma, a.Nonneg,
				!a.Absolute)
			if per[h].n == 0 {
				fatal("no valid [r0, r1] window")
			}
			fmt.Printf("  altitude %g m: %d variants\n", b.alt[h], per[h].n)
		}

		fmt.Printf("  applying constraint: common wavelengths ")
		for k := 0; k < nl; k++ {
			if k > 0 {
				fmt.Printf(", ")
			}
			fmt.Printf("%g nm", common[k].wl*1e3)
		}
		fmt.Printf("\n")

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
			a.LidarRatioIter, a.Best, S)

		for h := 0; h < b.nAlt; h++ {
			g1 := b.data[h*b.nch:]
			emitProfile(a, outDir, b.alt[h], b, solveCh, g1, per[h], kts,
				mre, mim, design.geom, r1MaxEff, common, nl, writeLidar,
				&met, h, ds)
		}
	} else {
		if a.LidarRatioCons {
			if b.nAlt < 2 {
				fmt.Printf("  [warning] --lidar-ratio-cons: at least 2 " +
					"altitudes required; constraint is not applied\n")
			} else {
				fmt.Printf("  [warning] --lidar-ratio-cons: no common " +
					"β∩α wavelengths for the lidar ratio; constraint is " +
					"not applied\n")
			}
		}
		for h := 0; h < b.nAlt; h++ {
			g1 := b.data[h*b.nch:]
			fmt.Printf("scan over (m, r0, r1, degree)...\n")
			sols := scanVariants(design, g1, b.nch, a.GammaSelect,
				a.GammaRel, a.GammaMin, a.GammaMax, a.NGamma, a.Nonneg,
				!a.Absolute)
			if sols.n == 0 {
				fatal("no valid [r0, r1] window")
			}
			fmt.Printf("  altitude %g m: %d variants\n", b.alt[h], sols.n)
			emitProfile(a, outDir, b.alt[h], b, solveCh, g1, sols, kts,
				mre, mim, design.geom, r1MaxEff, common, nl, writeLidar,
				&met, h, ds)
		}
	}

	if !a.NoPlot {
		po := plotOpts{a.PlotFormat, a.PlotDpi, a.PlotFontSize}
		ext := plotFormatExt(a.PlotFormat)

		pp := filepath.Join(outDir, "refractive."+ext)
		plotRefractiveProfile(&po, pp, b.alt, met.mRe, met.mIm)

		pp = filepath.Join(outDir, "error."+ext)
		{
			errPct := make([]float64, b.nAlt)
			for i := 0; i < b.nAlt; i++ {
				errPct[i] = met.err[i] * 100.0
			}
			plotProfiles(&po, pp, "Mean relative retrieval error, %",
				"Err, %", 1, b.alt, [][]float64{errPct}, []string{"Err"}, true)
		}

		pp = filepath.Join(outDir, "volumes."+ext)
		plotProfiles(&po, pp, "Volume concentration of modes",
			"V, µm³/cm³", 3, b.alt,
			[][]float64{met.V, met.Vc, met.Vf},
			[]string{"V", "Vc", "Vf"}, true)

		pp = filepath.Join(outDir, "radii."+ext)
		plotProfiles(&po, pp, "Mode radii", "r, µm", 3, b.alt,
			[][]float64{met.rmf, met.rmc, met.reff},
			[]string{"rmf", "rmc", "reff"}, true)

		pp = filepath.Join(outDir, "concentration."+ext)
		plotProfiles(&po, pp, "Number concentration", "N, cm⁻³", 1, b.alt,
			[][]float64{met.Nc}, []string{"N"}, true)

		pp = filepath.Join(outDir, "sigma."+ext)
		plotProfiles(&po, pp, "Lognormal mode widths", "sigma", 3, b.alt,
			[][]float64{met.sgf, met.sgc, met.sgo},
			[]string{"sigma_f", "sigma_c", "sigma"}, true)

		if nl > 0 {
			nSer := nl
			if nSer > 32 {
				nSer = 32
			}
			pp = filepath.Join(outDir, "lidar_ratios."+ext)
			svals := make([][]float64, nSer)
			snames := make([]string, nSer)
			for k := 0; k < nSer; k++ {
				svals[k] = met.S[k*b.nAlt:]
				snames[k] = fmt.Sprintf("S_{%g}", common[k].wl*1e3)
			}
			plotProfiles(&po, pp, "Lidar ratio S = α/β", "S = α/β", nSer,
				b.alt, svals, snames, true)
		}

		if len(met.dpol) > 0 && anyFiniteSeries(met.dpol) {
			pp = filepath.Join(outDir, "delpol."+ext)
			dnames := make([]string, len(met.dpol))
			for k := range met.dpol {
				dnames[k] = fmt.Sprintf("δ_l(%g nm)", met.dpolWl[k]*1e3)
			}
			plotProfiles(&po, pp, "Linear depolarization ratio",
				"δ_l, %", len(met.dpol), b.alt, met.dpol, dnames, false)
		}
	}

	fmt.Printf("batch processing finished; results: %s\n", outDir)
}
