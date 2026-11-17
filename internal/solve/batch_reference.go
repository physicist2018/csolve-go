package solve

import (
	"math"
	"os"
	"slices"
)

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
	slices.Sort(tmp)
	m := len(tmp)
	if m%2 == 1 {
		return tmp[m/2]
	}
	return 0.5 * (tmp[m/2-1] + tmp[m/2])
}

// windowedMedianRef — скользящая медиана (окно 3, у краёв сужается).
func windowedMedianRef(s []float64, nAlt, nl int) []float64 {
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
				col[m] = s[k*nl+li]
				m++
			}
			out[h*nl+li] = nanmedian(col[:m])
		}
	}
	return out
}

func windowedMedianColumnInto(s []float64, nAlt, nl, li int, out []float64) {
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
			col[m] = s[k*nl+li]
			m++
		}
		out[h*nl+li] = nanmedian(col[:m])
	}
}

// npFloor — нижняя граница перед логарифмированием (как 1e-300 в numpy).
const npFloor = 1e-300

// npMaxFloor — np.maximum(v, npFloor) с сохранением NaN.
func npMaxFloor(v float64) float64 {
	if math.IsNaN(v) {
		return math.NaN()
	}
	if v > npFloor {
		return v
	}
	return npFloor
}

func smoothRefProfile(alt, s []float64, nAlt, nl int) []float64 {
	out := make([]float64, nAlt*nl)
	for i := range out {
		out[i] = math.NaN()
	}
	if nAlt < 5 {
		return windowedMedianRef(s, nAlt, nl)
	}
	t := make([]float64, nAlt)
	for i := 0; i < nAlt; i++ {
		t[i] = alt[i] - alt[0]
	}
	span := t[nAlt-1] - t[0]
	if !(span > 0) {
		return windowedMedianRef(s, nAlt, nl)
	}
	for i := 0; i < nAlt; i++ {
		t[i] /= span
	}

	for li := 0; li < nl; li++ {
		ok := make([]int, 0, nAlt)
		for h := 0; h < nAlt; h++ {
			if isFinite(s[h*nl+li]) {
				ok = append(ok, h)
			}
		}
		cnt := len(ok)
		if cnt < 5 {
			windowedMedianColumnInto(s, nAlt, nl, li, out)
			continue
		}
		scale := 0.0
		for _, k := range ok {
			if v := math.Abs(s[k*nl+li]); v > scale {
				scale = v
			}
		}
		if !(scale > 0) || !isFinite(scale) {
			windowedMedianColumnInto(s, nAlt, nl, li, out)
			continue
		}
		xv := make([]float64, cnt)
		yv := make([]float64, cnt)
		for idx, k := range ok {
			xv[idx] = t[k]
			yv[idx] = s[k*nl+li] / scale
		}
		fit := smoothingSplineFit(xv, yv, nil, 1.0, xv)
		for idx, k := range ok {
			out[k*nl+li] = fit[idx] * scale
		}
	}
	return out
}

func reorderByScore(sols *solutions, score []float64) {
	n := sols.n
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return compareFloats(score[a], score[b])
	})
	buf := make([]solution, n)
	for k := 0; k < n; k++ {
		buf[k] = sols.sol[order[k]]
	}
	copy(sols.sol, buf)
}

func topIDsEqual(sols *solutions, g int, prev []int, off int) bool {
	for k := 0; k < g; k++ {
		if sols.sol[k].id != prev[off+k] {
			return false
		}
	}
	return true
}

func storeTopIDs(sols *solutions, g int, prev []int, off int) {
	for k := 0; k < g; k++ {
		prev[off+k] = sols.sol[k].id
	}
}

// lidarEffCounts — число усредняемых решений (g) и top-K для опорного
// профиля по каждой высоте.
func lidarEffCounts(a *Args, per []*solutions) (gEff, kEff []int, gmax int) {
	nAlt := len(per)
	gEff = make([]int, nAlt)
	kEff = make([]int, nAlt)
	gmax = 1
	for h := 0; h < nAlt; h++ {
		g := a.bestCount(per[h].n)
		gEff[h] = g
		k := 3 * g
		if k < 9 {
			k = 9
		}
		if k > per[h].n {
			k = per[h].n
		}
		kEff[h] = k
		if g > gmax {
			gmax = g
		}
	}
	return gEff, kEff, gmax
}

// lidarReference — опорный профиль S: медиана top-K решений каждой высоты,
// сглаженная по высоте; пропуски заполняются медианой столбца.
func lidarReference(alt []float64, ratios [][]float64, per []*solutions,
	kEff []int, nAlt, nl int) []float64 {
	maxK := 1
	for _, k := range kEff {
		if k > maxK {
			maxK = k
		}
	}
	pool := make([]float64, max(maxK, nAlt))
	est := make([]float64, nAlt*nl)
	for h := 0; h < nAlt; h++ {
		k := kEff[h]
		for li := 0; li < nl; li++ {
			for t := 0; t < k; t++ {
				pool[t] = ratios[h][per[h].sol[t].id*nl+li]
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
	return ref
}

// lidarScore — score решений высоты: err + w·|ln S − ln S_ref| плюс штраф δ_l
// (penalty индексируется по id решения, nil — штрафа нет).
func lidarScore(per *solutions, ratios, ref, penalty []float64, nl int,
	w float64) []float64 {
	logRef := make([]float64, max(nl, 1))
	for li := 0; li < nl; li++ {
		logRef[li] = math.Log(npMaxFloor(ref[li]))
	}
	score := make([]float64, per.n)
	for t := 0; t < per.n; t++ {
		sv := ratios[per.sol[t].id*nl:]
		sum := 0.0
		cnt := 0
		for li := 0; li < nl; li++ {
			d := math.Abs(math.Log(npMaxFloor(sv[li])) - logRef[li])
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
		score[t] = per.sol[t].err + w*dev
		if id := per.sol[t].id; id >= 0 && id < len(penalty) {
			score[t] += penalty[id]
		}
	}
	return score
}

func applyLidarConstraint(per []*solutions, nAlt int, alt []float64, nl int,
	w float64, maxIter int, a *Args, ratios [][]float64, pen [][]float64,
	c *console) {
	dbg := os.Getenv("CSOLVE_LIDAR_DEBUG") != ""
	gEff, kEff, gmax := lidarEffCounts(a, per)

	prev := make([]int, nAlt*gmax)
	havePrev := false
	iter := maxIter
	if iter < 1 {
		iter = 1
	}

	for it := 0; it < iter; it++ {
		ref := lidarReference(alt, ratios, per, kEff, nAlt, nl)
		if dbg {
			c.printf("iter %d: ref = [", it)
			for h := 0; h < nAlt; h++ {
				if h > 0 {
					c.printf(", ")
				}
				c.printf("[")
				for li := 0; li < nl; li++ {
					if li > 0 {
						c.printf(", ")
					}
					c.printf("%.15g", ref[h*nl+li])
				}
				c.printf("]")
			}
			c.printf("]\n")
		}

		for h := 0; h < nAlt; h++ {
			var penalty []float64
			if pen != nil && h < len(pen) {
				penalty = pen[h]
			}
			score := lidarScore(per[h], ratios[h], ref[h*nl:(h+1)*nl], penalty, nl, w)
			reorderByScore(per[h], score)
			if dbg {
				lidarDebugTop(c, per[h], ratios[h], ref[h*nl:(h+1)*nl], alt[h], w, nl)
			}
		}

		changed := !havePrev
		off := 0
		for h := 0; h < nAlt; h++ {
			if havePrev && !topIDsEqual(per[h], gEff[h], prev, off) {
				changed = true
			}
			off += gmax
		}
		if !changed {
			break
		}
		off = 0
		for h := 0; h < nAlt; h++ {
			storeTopIDs(per[h], gEff[h], prev, off)
			off += gmax
		}
		havePrev = true
	}
}

// lidarDebugTop — отладочная печать лучших решений высоты (CSOLVE_LIDAR_DEBUG).
func lidarDebugTop(c *console, per *solutions, ratios, ref []float64,
	alt float64, w float64, nl int) {
	for t := 0; t < per.n && t < 3; t++ {
		s := &per.sol[t]
		sv := ratios[s.id*nl:]
		sum := 0.0
		cnt := 0
		for li := 0; li < nl; li++ {
			d := math.Abs(math.Log(npMaxFloor(sv[li])) - math.Log(npMaxFloor(ref[li])))
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
		c.printf("  h=%g top%d: (score=%g, err=%g, dev=%g, id=%d, "+
			"m=%g%+gj, deg=%d, r0=%.5g, r1=%.5g) S=[%g, %g]\n",
			alt, t, s.err+w*dev, s.err, dev, s.id, s.mRe, s.mIm,
			s.degree, s.r0, s.r1, sv[0], s1)
	}
}
