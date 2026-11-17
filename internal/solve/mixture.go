package solve

import (
	"fmt"
	"math"
	"os"
)

// ---------------- смесь «сферы + несферические частицы» ----------------
//
// Матрица отклика смеси линейна по доле сфер f ∈ [0, 1]:
//
//	A(f) = (1-f)·A_non + f·A_sphere,
//
// поэтому матрицы обоих компонентов строятся один раз, а перебор идёт по
// (m, окно, f). Доля сфер отслеживается в каждом решении, в отчётах и в
// выходных файлах; ядра S11/S22 и проекции δ_l смешиваются так же линейно.
const (
	mixNone              = "none"
	mixSphereSpheroid    = "sphere+spheroid"
	mixSphereAgglomerate = "sphere+agglomerate"
)

// mixNames — значения --mix в порядке справки и сообщений.
var mixNames = []string{mixNone, mixSphereSpheroid, mixSphereAgglomerate}

// tableTypeNames — допустимые значения --type: одиночные наборы и смеси.
var tableTypeNames = append(append([]string{}, builtinTypeNames...),
	mixSphereSpheroid, mixSphereAgglomerate)

// mixtureTail — несферическая компонента смеси (значение --type).
func mixtureTail(mix string) string {
	switch mix {
	case mixSphereSpheroid:
		return "spheroid"
	case mixSphereAgglomerate:
		return "agglomerate"
	}
	return ""
}

// mixtureFromTypeName — имя смеси, если name задаёт смесь в --type.
func mixtureFromTypeName(name string) string {
	if mixtureTail(name) != "" {
		return name
	}
	return ""
}

// mixEnabled — включена ли смесь сфер с несферической компонентой.
func (a *Args) mixEnabled() bool { return mixtureTail(a.Mix) != "" }

// mixFractions — доли сфер f_k = f_min + k*(f_max-f_min)/f_n, k = 0..f_n.
// Вне смеси возвращается nil (перебор по f не выполняется).
func (a *Args) mixFractions() []float64 {
	if !a.mixEnabled() {
		return nil
	}
	n := a.FN
	if n < 1 {
		n = 1
	}
	return linspace(a.FMin, a.FMax, n+1)
}

// sphereTableSource — источник таблиц сферической компоненты смеси.
func (a *Args) sphereTableSource() *tableSource {
	if a.SphereTables != "" {
		return externalTableSource(a.SphereTables)
	}
	return builtinTableSource("sphere")
}

// tableSources — источники таблиц компонентов: [0] — несферическая
// (или единственная) компонента, [1] — сферы (nil вне смеси).
func (a *Args) tableSources() (*tableSource, *tableSource) {
	if !a.mixEnabled() {
		return a.tableSource(), nil
	}
	return a.tableSource(), a.sphereTableSource()
}

// mGrid — сетка m одного компонента вместе с источником её таблиц.
type mGrid struct {
	src    *tableSource
	re, im []float64
}

// alignMGrid — выравнивание сеток m двух компонент смеси.
//
// Базовой считается сетка первой компоненты (несферической): для каждого её
// значения ищется ближайшее m второй компоненты. При tol == 0 требуется
// точное совпадение, при tol > 0 допускается отклонение |ΔRe|, |ΔIm| <= tol
// (опция --mix-m-snap). Возвращаются базовые значения, соответствующие им
// значения второй компоненты (для чтения её таблиц) и число приближённых пар.
func alignMGrid(re1, im1, re2, im2 []float64, tol float64) (
	mre, mim, mre2, mim2 []float64, snapped int) {
	for i := range re1 {
		best, bestD := -1, math.Inf(1)
		for j := range re2 {
			d := math.Abs(re1[i]-re2[j]) + math.Abs(im1[i]-im2[j])
			if d < bestD {
				bestD, best = d, j
			}
		}
		if best < 0 {
			continue
		}
		dr := math.Abs(re1[i] - re2[best])
		di := math.Abs(im1[i] - im2[best])
		ok := dr == 0 && di == 0
		if tol > 0 {
			ok = dr <= tol && di <= tol
		}
		if !ok {
			continue
		}
		if dr != 0 || di != 0 {
			snapped++
		}
		mre = append(mre, re1[i])
		mim = append(mim, im1[i])
		mre2 = append(mre2, re2[best])
		mim2 = append(mim2, im2[best])
	}
	return mre, mim, mre2, mim2, snapped
}

// kernels — ядра обоих компонентов смеси на общей сетке m (kts2/ds2 == nil
// вне смеси).
type kernels struct {
	mix      bool
	mre, mim []float64
	kts      []*ktable
	kts2     []*ktable
	ds       *depolSet
	ds2      *depolSet
}

// coverageCap — минимум x_max*wl/(2π) по всем m всех компонент смеси:
// предел r1_max по покрытию таблиц.
func coverageCap(a *Args, grids []mGrid, base []channel, cbackScale float64) float64 {
	capmin := math.Inf(1)
	for _, g := range grids {
		for i := range g.re {
			chs := loadTableChannels(g.src, g.re[i], g.im[i], base,
				a.TableExtrapolate, a.TableRefWl, cbackScale)
			if chs.cap < capmin {
				capmin = chs.cap
			}
		}
	}
	return capmin
}

// buildComponentKernels — таблицы ядер одного компонента смеси.
func buildComponentKernels(a *Args, g mGrid, base []channel, r1Max,
	cbackScale float64) []*ktable {
	kts := make([]*ktable, len(g.re))
	for i := range g.re {
		chs := loadTableChannels(g.src, g.re[i], g.im[i], base,
			a.TableExtrapolate, a.TableRefWl, cbackScale)
		kts[i] = ktableBuild(chs, a.R0Min, r1Max, a.DxMax)
	}
	return kts
}

// buildComponentDepolSet — ядра S11/S22 одного компонента смеси.
func buildComponentDepolSet(a *Args, g mGrid, r1Max float64) *depolSet {
	return depolSetBuild(g.src, g.re, g.im, a.TableRefWl, a.R0Min, r1Max,
		a.DxMax, a.TableExtrapolate)
}

// forwardMeasurementsMix — прямой расчёт g1 по демо-распределению для смеси:
// (1-f)·g(K1) + f·g(K2); при kt2 == nil — обычный расчёт.
func forwardMeasurementsMix(kt, kt2 *ktable, frac float64,
	nf func(float64) float64, rMin, rMax float64, order int,
	span float64) []float64 {
	g1 := forwardMeasurements(kt, nf, rMin, rMax, order, span)
	if kt2 == nil {
		return g1
	}
	g2 := forwardMeasurements(kt2, nf, rMin, rMax, order, span)
	for j := range g1 {
		g1[j] = (1.0-frac)*g1[j] + frac*g2[j]
	}
	return g1
}

// reportMixSetup — строка о включённой смеси и сетке долей сфер.
func reportMixSetup(a *Args) {
	if !a.mixEnabled() {
		return
	}
	fmt.Printf("mixture: %s + spheres; sphere fraction f scanned in [%g, %g] "+
		"with %d intervals (%d values)\n", mixtureTail(a.Mix), a.FMin, a.FMax,
		a.FN, a.FN+1)
}

// reportMixGrid — сведения и предупреждения о выравнивании сеток m.
func reportMixGrid(a *Args, g1, g2 mGrid, nCommon, snapped int) {
	if nCommon == 0 {
		fatal("mixture tables have no common m values (non-spherical %s: %d, "+
			"sphere %s: %d) — use --mix-m-snap <tol> to pair the nearest m "+
			"values", g1.src.label, len(g1.re), g2.src.label, len(g2.re))
	}
	fmt.Printf("  non-spherical tables: %s\n", g1.src.describe())
	fmt.Printf("  spherical tables:     %s\n", g2.src.describe())
	fmt.Printf("  common m grid: %d of %d non-spherical values", nCommon,
		len(g1.re))
	if snapped > 0 {
		fmt.Printf(" (%d paired with the nearest sphere m, tol = %g)", snapped,
			a.MixMSnap)
	}
	fmt.Printf("\n")
	if nCommon < len(g1.re) {
		fmt.Fprintf(os.Stderr, "[warning] mixture: only %d of %d non-spherical m "+
			"values exist in the sphere tables; use --mix-m-snap <tol> to pair "+
			"the nearest m values\n", nCommon, len(g1.re))
	}
}
