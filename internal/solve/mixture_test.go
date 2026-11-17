package solve

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestAlignMGridExact(t *testing.T) {
	re1 := []float64{1.3, 1.4, 1.5}
	im1 := []float64{0, 0.01, 0.02}
	re2 := []float64{1.2, 1.3, 1.4, 1.5, 1.6}
	im2 := []float64{0.5, 0, 0.011, 0.02, 0.5}

	mre, mim, mre2, mim2, snapped := alignMGrid(re1, im1, re2, im2, 0)
	if snapped != 0 {
		t.Errorf("exact matching must not snap, got %d", snapped)
	}
	if len(mre) != 2 {
		t.Fatalf("expected 2 common m values, got %d (%v)", len(mre), mre)
	}
	checkClose(t, "mre[0]", mre[0], 1.3, 0)
	checkClose(t, "mim[0]", mim[0], 0, 0)
	checkClose(t, "mre[1]", mre[1], 1.5, 0)
	checkClose(t, "mim[1]", mim[1], 0.02, 0)
	checkClose(t, "mre2[0]", mre2[0], 1.3, 0)
	checkClose(t, "mim2[1]", mim2[1], 0.02, 0)

	// без допуска несовпавшее m (1.4/0.01) отбрасывается
	for i := range mre {
		if mre[i] == 1.4 {
			t.Errorf("m = 1.4+0.01j must be dropped without tolerance")
		}
	}

	// допуск подхватывает ближайшее m и считает приближённые пары
	mre, _, mre2, mim2, snapped = alignMGrid(re1, im1, re2, im2, 0.02)
	if len(mre) != 3 {
		t.Fatalf("tolerance must keep all 3 m values, got %d", len(mre))
	}
	if snapped != 1 {
		t.Errorf("expected 1 snapped pair, got %d", snapped)
	}
	checkClose(t, "snapped Re", mre2[1], 1.4, 0)
	checkClose(t, "snapped Im", mim2[1], 0.011, 0)

	// пустая вторая сетка
	if mre, _, _, _, _ := alignMGrid(re1, im1, nil, nil, 0.1); len(mre) != 0 {
		t.Errorf("empty second grid must give no common m")
	}
}

// mixtureTable — синтетическая таблица с Cext=Cback=x^exp.
func mixtureTable(t *testing.T, exp float64) *table {
	t.Helper()
	const n = 80
	var sb strings.Builder
	sb.WriteString("# x Cext Cabs Cback Qext Qabs Qback S11 S22 n\n")
	for i := 0; i < n; i++ {
		x := 0.05 * math.Pow(2000.0, float64(i)/float64(n-1))
		fmt.Fprintf(&sb, "%.17g %.10g 0 %.10g 0 0 0 1 1 1\n", x,
			math.Pow(x, exp), math.Pow(x, exp))
	}
	tb, _ := parseTable([]byte(sb.String()), "mix-data.txt", false)
	return tb
}

func mixtureChannels(tb *table) *channels {
	base := []channel{
		{chBeta, 0.355, tb, tableRefWlUm, 1},
		{chBeta, 0.532, tb, tableRefWlUm, 1},
		{chBeta, 1.064, tb, tableRefWlUm, 1},
		{chAlpha, 0.355, tb, tableRefWlUm, 1},
		{chAlpha, 0.532, tb, tableRefWlUm, 1},
		{chAlpha, 1.064, tb, tableRefWlUm, 1},
	}
	return &channels{ch: base, n: len(base), cap: math.Inf(1)}
}

// mixtureDesigns — два компонента с контрастными сечениями (x² и x⁶).
// Окно одно, неизвестных два (nBasis = 4 с нулевыми краями): этого хватает,
// чтобы доля сфер была идентифицируема по шести каналам.
func mixtureDesigns(t *testing.T, fracs []float64) (*design, *design) {
	return mixtureDesignsFill(t, fracs, 1, 1)
}

// mixtureDesignsFill — то же с заданными факторами заполнения компонент.
func mixtureDesignsFill(t *testing.T, fracs []float64, fill1, fill2 float64) (
	*design, *design) {
	t.Helper()
	const nch = 6
	const r0, r1 = 0.2, 2.0
	kts1 := []*ktable{ktableBuild(mixtureChannels(mixtureTable(t, 2)),
		r0, r1, 0.1, fill1)}
	kts2 := []*ktable{ktableBuild(mixtureChannels(mixtureTable(t, 6)),
		r0, r1, 0.1, fill2)}
	mre := []float64{1.45}
	mim := []float64{-0.01}
	deg := []int{1}
	d1 := designBuild(nil, nch, kts1, mre, mim, deg, []float64{r0},
		[]float64{r1}, 1, 4, true, 24, 0.1)
	d2 := designBuild(nil, nch, kts2, mre, mim, deg, []float64{r0},
		[]float64{r1}, 1, 4, true, 24, 0.1)
	d1.mixWith(d2, fracs)
	return d1, d2
}

// TestDesignMix: A(f) = (1-f)A1 + fA2, а вне смеси возвращается исходный блок.
func TestDesignMix(t *testing.T) {
	fracs := []float64{0, 0.25, 0.5, 1}
	d1, d2 := mixtureDesigns(t, fracs)
	if d1.nf != len(fracs) || d1.A2 == nil {
		t.Fatalf("mixture not attached: nf=%d A2=%v", d1.nf, d1.A2 != nil)
	}
	size := d1.nch * d1.nu
	scratch := make([]float64, size)
	for kf, f := range fracs {
		A, s0 := d1.block(0, 0, kf, scratch)
		for i := 0; i < size; i++ {
			want := (1-f)*d1.A[i] + f*d2.A[i]
			checkClose(t, fmt.Sprintf("A(f=%g)[%d]", f, i), A[i], want, 1e-15)
		}
		checkClose(t, fmt.Sprintf("||A(f=%g)||", f), s0,
			svdNorm2(A, d1.nch, d1.nu), 1e-15)
	}

	// блок без смеси — тот же массив, без копии
	plain := &design{nM: 1, nW: 1, nch: 2, nu: 2,
		A: []float64{1, 2, 3, 4}, s0: []float64{5}}
	A, s0 := plain.block(0, 0, 0, nil)
	if &A[0] != &plain.A[0] {
		t.Errorf("without a mixture the stored block must be returned as is")
	}
	checkClose(t, "plain s0", s0, 5, 0)
}

// TestMixtureScan: перебор (m, окно, f) восстанавливает долю сфер, по которой
// были посчитаны измерения, и метит каждое решение своей f.
func TestMixtureScan(t *testing.T) {
	const nch = 6
	fracs := linspace(0, 1, 11)
	d1, _ := mixtureDesigns(t, fracs)
	if d1.nW != 1 || d1.nu != 2 {
		t.Fatalf("unexpected design shape: nW=%d nu=%d", d1.nW, d1.nu)
	}

	// измерения — точная смесь с известными коэффициентами
	Ctrue := []float64{1.0, 0.5}
	kf := 3
	fTrue := fracs[kf]
	A, _ := d1.block(0, 0, kf, make([]float64, nch*d1.nu))
	g1 := make([]float64, nch)
	for j := 0; j < nch; j++ {
		for i := 0; i < d1.nu; i++ {
			g1[j] += A[j*d1.nu+i] * Ctrue[i]
		}
	}

	sols := scanVariants(d1, g1, nch, gammaFixed, 0, -7, 5, 1, false, true, nil)
	if sols.n != len(fracs) {
		t.Fatalf("expected %d variants (one per f), got %d", len(fracs), sols.n)
	}
	best := &sols.sol[0]
	checkClose(t, "best sphere fraction", best.frac, fTrue, 1e-12)
	if !(best.err < 1e-8) {
		t.Errorf("exact mixture must be reproduced: Err = %g", best.err)
	}
	// gFit каждого решения соответствует смеси с его же f
	for s := range sols.sol[:] {
		sol := &sols.sol[s]
		ks := -1
		for k, f := range fracs {
			if math.Abs(f-sol.frac) <= 1e-12 {
				ks = k
			}
		}
		if ks < 0 {
			t.Fatalf("solution f = %g is not on the fraction grid", sol.frac)
		}
		As, _ := d1.block(0, 0, ks, make([]float64, nch*d1.nu))
		for j := 0; j < nch; j++ {
			p := 0.0
			for i := 0; i < d1.nu; i++ {
				p += As[j*d1.nu+i] * sol.C[i]
			}
			checkClose(t, fmt.Sprintf("gFit[%d] of f=%g", j, sol.frac), sol.gFit[j],
				p, 1e-9*math.Abs(p)+1e-30)
		}
	}
}

// depolMixTable — таблица с S11 = x^exp, S22 = 1 (положительные всюду).
func depolMixTable(t *testing.T, exp float64) *table {
	t.Helper()
	const n = 80
	var sb strings.Builder
	sb.WriteString("# x Cext Cabs Cback Qext Qabs Qback S11 S22 n\n")
	for i := 0; i < n; i++ {
		x := 0.05 * math.Pow(2000.0, float64(i)/float64(n-1))
		fmt.Fprintf(&sb, "%.17g %.6g 0 %.6g 0 0 0 %.10g 1 1\n", x, x, x,
			math.Pow(x, exp))
	}
	tb, _ := parseTable([]byte(sb.String()), "depol-mix.txt", false)
	return tb
}

func depolMixSet(t *testing.T, exp float64, r []float64) *depolSet {
	t.Helper()
	tb := depolMixTable(t, exp)
	ds := &depolSet{wl: append([]float64(nil), depolWlsUm...),
		kt: make([][2]*ktable, 1)}
	for iw, wl := range ds.wl {
		ds.kt[0][iw] = depolKtableBuild(tb, wl, tableRefWlUm, r, 1)
	}
	ds.nOK = 1
	return ds
}

// TestDepolMix: δ_l смеси считается по линейно смешанным ядрам S11/S22 —
// и через проекции на базис, и через свёртку на узлах квадратуры.
func TestDepolMix(t *testing.T) {
	r := rGridFor(1.0, 2.0, 0.1, 0.355)
	ds1 := depolMixSet(t, 1, r)
	ds2 := depolMixSet(t, 3, r)

	rq := []float64{1.0, 1.5}
	wq := []float64{1, 1}
	nq := []float64{1, 1}
	d11a, d22a, da := depolFromProfile(ds1, []int{0}, rq, wq, nq)
	d11b, d22b, db := depolFromProfile(ds2, []int{0}, rq, wq, nq)

	_, _, d0 := depolFromProfileMix(ds1, ds2, []int{0}, []float64{0}, rq, wq, nq)
	_, _, d1 := depolFromProfileMix(ds1, ds2, []int{0}, []float64{1}, rq, wq, nq)
	h11, h22, dh := depolFromProfileMix(ds1, ds2, []int{0}, []float64{0.5}, rq, wq, nq)
	for iw := range d0 {
		checkClose(t, "delta_l(f=0)", d0[iw], da[iw], 1e-12)
		checkClose(t, "delta_l(f=1)", d1[iw], db[iw], 1e-12)
		checkClose(t, "D11(f=0.5)", h11[iw], 0.5*(d11a[iw]+d11b[iw]), 1e-12)
		checkClose(t, "D22(f=0.5)", h22[iw], 0.5*(d22a[iw]+d22b[iw]), 1e-12)
		den := h11[iw] + h22[iw]
		checkClose(t, "delta_l(f=0.5)", dh[iw], (h11[iw]-h22[iw])/den*100, 1e-12)
	}

	// проекции: d11d22 при f=0 и f=1 совпадают с отдельными наборами
	g := geomBuild(1.0, 2.0, 1, 3, false, 24, 0.1)
	d := &design{nM: 1, nW: 1, nu: g.nUnknown, geom: []*geom{g}}
	pMix := depolProjBuildMix(ds1, ds2, d, []int{0, 1})
	p1 := depolProjBuild(ds1, d, []int{0, 1})
	p2 := depolProjBuild(ds2, d, []int{0, 1})
	if pMix == nil || p1 == nil || p2 == nil {
		t.Fatalf("projections must be built")
	}
	C := []float64{1, 0.5, 0.25}
	for k := 0; k < 2; k++ {
		a11, a22, ok := p1.d11d22(0, 0, k, C, 0)
		if !ok {
			t.Fatalf("projection %d is not finite", k)
		}
		b11, b22, _ := p2.d11d22(0, 0, k, C, 0)
		m11, m22, _ := pMix.d11d22(0, 0, k, C, 0)
		checkClose(t, "proj D11(f=0)", m11, a11, 1e-12)
		checkClose(t, "proj D22(f=0)", m22, a22, 1e-12)
		m11, m22, _ = pMix.d11d22(0, 0, k, C, 1)
		checkClose(t, "proj D11(f=1)", m11, b11, 1e-12)
		checkClose(t, "proj D22(f=1)", m22, b22, 1e-12)
		m11, _, _ = pMix.d11d22(0, 0, k, C, 0.25)
		checkClose(t, "proj D11(f=0.25)", m11, 0.75*a11+0.25*b11, 1e-12)
	}
}

// TestSingleFractionMixture: вырожденный диапазон f (f_min == f_max) даёт
// одну долю, и блок всё равно смешивается по ней (nf == 1 не отключает смесь).
func TestSingleFractionMixture(t *testing.T) {
	a, err := ParseArgs([]string{"--mix", "sphere+spheroid", "--f-min", "0.5",
		"--f-max", "0.5"})
	if err != nil {
		t.Fatalf("--mix: %v", err)
	}
	f := a.mixFractions()
	if len(f) != 1 {
		t.Fatalf("degenerate range must give one fraction, got %v", f)
	}
	checkClose(t, "single f", f[0], 0.5, 0)

	d1, d2 := mixtureDesigns(t, f)
	if d1.nf != 1 {
		t.Fatalf("expected nf = 1, got %d", d1.nf)
	}
	size := d1.nch * d1.nu
	A, _ := d1.block(0, 0, 0, make([]float64, size))
	for i := 0; i < size; i++ {
		want := 0.5*d1.A[i] + 0.5*d2.A[i]
		checkClose(t, "A(f=0.5)", A[i], want, 1e-15*math.Abs(want)+1e-300)
	}
}

func TestArgsMix(t *testing.T) {
	{
		a, err := ParseArgs([]string{"--tables", "dir"})
		if err != nil {
			t.Fatalf("defaults: %v", err)
		}
		if a.Mix != mixNone {
			t.Errorf("default --mix must be none, got %q", a.Mix)
		}
		checkClose(t, "--f-min default", a.FMin, 0, 0)
		checkClose(t, "--f-max default", a.FMax, 1, 0)
		if a.FN != 4 {
			t.Errorf("default --f-n must be 4, got %d", a.FN)
		}
		if a.HasFTrue {
			t.Errorf("--f-true must be unset by default")
		}
		if a.mixEnabled() {
			t.Errorf("mixture must be disabled by default")
		}
	}
	{
		a, err := ParseArgs([]string{"--mix", "sphere+agglomerate", "--f-min", "0.2",
			"--f-max", "0.8", "--f-n", "6"})
		if err != nil {
			t.Fatalf("--mix: %v", err)
		}
		if a.Mix != mixSphereAgglomerate || a.Type != "agglomerate" {
			t.Errorf("mix=%q type=%q", a.Mix, a.Type)
		}
		checkClose(t, "--f-min", a.FMin, 0.2, 0)
		checkClose(t, "--f-max", a.FMax, 0.8, 0)
		if a.FN != 6 {
			t.Errorf("--f-n = %d", a.FN)
		}
		f := a.mixFractions()
		if len(f) != 7 {
			t.Fatalf("expected 7 f values, got %d", len(f))
		}
		checkClose(t, "f[0]", f[0], 0.2, 0)
		checkClose(t, "f[6]", f[6], 0.8, 0)
	}
	{
		// --type sphere+... — синоним --mix
		a, err := ParseArgs([]string{"--type", "sphere+spheroid"})
		if err != nil {
			t.Fatalf("--type mixture: %v", err)
		}
		if a.Mix != mixSphereSpheroid || a.Type != "spheroid" {
			t.Errorf("mix=%q type=%q", a.Mix, a.Type)
		}
		if _, err := ParseArgs([]string{"--type", "sphere+spheroid",
			"--tables", "dir"}); err == nil {
			t.Errorf("mixture --type with --tables must be rejected")
		} else if !strings.Contains(err.Error(), "--mix") {
			t.Errorf("error for --type mixture with --tables must suggest --mix: %v",
				err)
		}
	}
	{
		// несогласованная несферическая компонента
		if _, err := ParseArgs([]string{"--mix", "sphere+spheroid",
			"--type", "agglomerate"}); err == nil {
			t.Errorf("--mix sphere+spheroid with --type agglomerate must be rejected")
		}
		if _, err := ParseArgs([]string{"--mix", "sphere+spheroid",
			"--type", "sphere"}); err == nil {
			t.Errorf("--type sphere cannot be the non-spherical component")
		}
	}
	{
		// флаги смеси без смеси — ошибка
		for _, args := range [][]string{
			{"--tables-sphere", "dir"},
			{"--f-n", "3"},
			{"--f-min", "0.1"},
			{"--f-true", "0.5"},
			{"--mix-m-snap", "0.01"},
		} {
			if _, err := ParseArgs(args); err == nil {
				t.Errorf("%v must be rejected without --mix", args)
			}
		}
	}
	{
		// диапазон f и --f-true
		for _, args := range [][]string{
			{"--mix", "sphere+spheroid", "--f-min", "-0.1"},
			{"--mix", "sphere+spheroid", "--f-max", "1.1"},
			{"--mix", "sphere+spheroid", "--f-min", "0.7", "--f-max", "0.3"},
			{"--mix", "sphere+spheroid", "--f-n", "0"},
			{"--mix", "sphere+spheroid", "--mix-m-snap", "-1"},
			{"--type", "sphere+spheroid", "--f-true", "1.5"},
		} {
			if _, err := ParseArgs(args); err == nil {
				t.Errorf("%v must be rejected", args)
			}
		}
		a, err := ParseArgs([]string{"--type", "sphere+spheroid",
			"--f-true", "0.25"})
		if err != nil {
			t.Fatalf("--f-true: %v", err)
		}
		if !a.HasFTrue {
			t.Errorf("--f-true must be recorded")
		}
		checkClose(t, "--f-true", a.FTrue, 0.25, 0)
	}
	{
		// внешние таблицы обоих компонентов
		a, err := ParseArgs([]string{"--mix", "sphere+agglomerate",
			"--tables", "aggl", "--tables-sphere", "sph"})
		if err != nil {
			t.Fatalf("external mixture tables: %v", err)
		}
		if a.Tables != "aggl" || a.SphereTables != "sph" {
			t.Errorf("tables=%q sphere=%q", a.Tables, a.SphereTables)
		}
	}
}
