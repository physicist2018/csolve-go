package solve

import (
	"math"
	"strings"
	"testing"
)

// TestFillFactorKernels: ядра агломератов обратно пропорциональны φ
// (сечения — на частицу, а объём материала равен φ·(4/3)πr³).
func TestFillFactorKernels(t *testing.T) {
	const r0, r1 = 0.2, 2.0
	chs := mixtureChannels(mixtureTable(t, 2))
	base := ktableBuild(chs, r0, r1, 0.1, 1)
	for _, fill := range []float64{0.5, 0.04} {
		scaled := ktableBuild(chs, r0, r1, 0.1, fill)
		if scaled.n != base.n {
			t.Fatalf("grid size changed: %d vs %d", scaled.n, base.n)
		}
		for _, r := range []float64{0.2, 0.5, 1.0, 1.9} {
			for j := 0; j < chs.n; j++ {
				got := ktableKernel(scaled, j, r)
				want := ktableKernel(base, j, r) / fill
				checkClose(t, "K/φ", got, want, 1e-12*want)
			}
		}
	}
}

// TestFillFactorMoments: N обратно пропорциональна φ, C_v и r_eff
// инвариантны, ⟨V⟩ пропорциональна φ (n фиксировано).
func TestFillFactorMoments(t *testing.T) {
	nr := 32
	rGrid := geomspace(0.1, 2.0, nr)
	nAvg := make([]float64, nr)
	for i := range rGrid {
		nAvg[i] = math.Exp(-math.Pow(math.Log(rGrid[i]/0.4), 2.0))
	}
	base := sizeMoments(nAvg, rGrid, 1)
	const fill = 0.04
	scaled := sizeMoments(nAvg, rGrid, fill)

	checkClose(t, "N(φ)·φ", scaled.NCm3*fill, base.NCm3, 1e-12*base.NCm3)
	checkClose(t, "C_v(φ)", scaled.VolConcUm3PerCm3, base.VolConcUm3PerCm3, 0)
	checkClose(t, "r_eff(φ)", scaled.REffUm, base.REffUm, 0)
	checkClose(t, "⟨V⟩(φ)/φ", scaled.VolMeanUm3/fill, base.VolMeanUm3,
		1e-12*base.VolMeanUm3)
}

// TestFillFactorRetrieval: при заданном φ обратная задача возвращает n,
// масштабированное на φ (на хорошо обусловленной матрице, чтобы решение
// не зависело от выбора минимум-норм вектора).
func TestFillFactorRetrieval(t *testing.T) {
	const nch = 2
	const fill = 0.04
	Ctrue := []float64{1.0, 0.5}
	g1 := []float64{Ctrue[0], Ctrue[1]}

	mkDesign := func(scale float64) *design {
		return &design{
			nM: 1, nW: 1, nch: nch, nu: 2,
			A:      []float64{scale, 0, 0, scale},
			s0:     []float64{scale},
			degree: []int{1},
			r0:     []float64{0.2},
			r1:     []float64{2.0},
			mRe:    []float64{1.45},
			mIm:    []float64{-0.01},
		}
	}
	d1 := mkDesign(1.0)
	dF := mkDesign(1.0 / fill)

	sols1 := scanVariants(d1, g1, nch, gammaFixed, 0, -7, 5, 1, false, true, nil)
	solsF := scanVariants(dF, g1, nch, gammaFixed, 0, -7, 5, 1, false, true, nil)
	if sols1.n != 1 || solsF.n != 1 {
		t.Fatalf("expected one variant each, got %d / %d", sols1.n, solsF.n)
	}
	for i := range Ctrue {
		checkClose(t, "n(φ)/φ", solsF.sol[0].C[i]/fill, sols1.sol[0].C[i],
			1e-12*math.Abs(Ctrue[i]))
		checkClose(t, "C(φ=1)", sols1.sol[0].C[i], Ctrue[i], 1e-12)
	}
	if sols1.sol[0].err != solsF.sol[0].err {
		t.Errorf("Err must not depend on φ: %g vs %g", sols1.sol[0].err,
			solsF.sol[0].err)
	}
}

// TestFillFactorMixture: веса компонент в смеси — (1-f)/φ_non и f/φ_sphere,
// а эффективный фактор заполнения даётся гармоническим смешением.
func TestFillFactorMixture(t *testing.T) {
	fracs := []float64{0, 0.5, 1}
	const fill1 = 0.05
	d1, _ := mixtureDesignsFill(t, fracs, fill1, 1)
	ref1, ref2 := mixtureDesignsFill(t, fracs, 1, 1)
	if d1.nf != len(fracs) {
		t.Fatalf("mixture not attached")
	}
	size := d1.nch * d1.nu
	scratch := make([]float64, size)
	for kf, f := range fracs {
		A, _ := d1.block(0, 0, kf, scratch)
		for i := 0; i < size; i++ {
			want := (1-f)/fill1*ref1.A[i] + f*ref2.A[i]
			checkClose(t, "A_mix", A[i], want, 1e-14*math.Abs(want)+1e-300)
		}
		if kf == 0 {
			// f = 0: чистая несферическая компонента с φ
			for i := 0; i < size; i++ {
				checkClose(t, "A(f=0)=A1/φ", A[i], ref1.A[i]/fill1,
					1e-14*math.Abs(A[i])+1e-300)
			}
		}
	}

	checkClose(t, "φ_eff(f=0)", effectiveFill(fill1, 1, 0), fill1, 0)
	checkClose(t, "φ_eff(f=1)", effectiveFill(fill1, 1, 1), 1, 0)
	checkClose(t, "φ_eff(f=0.5)", effectiveFill(fill1, 1, 0.5),
		1.0/(0.5/1+0.5/fill1), 1e-15)
}

func TestArgsFillFactor(t *testing.T) {
	{
		a, err := ParseArgs([]string{"--tables", "dir"})
		if err != nil {
			t.Fatalf("defaults: %v", err)
		}
		checkClose(t, "default --fill-factor", a.FillFactor, 1.0, 0)
	}
	{
		a, err := ParseArgs([]string{"--type", "agglomerate", "--fill-factor", "0.04"})
		if err != nil {
			t.Fatalf("agglomerate with phi: %v", err)
		}
		checkClose(t, "--fill-factor", a.FillFactor, 0.04, 0)
	}
	{
		a, err := ParseArgs([]string{"--mix", "sphere+agglomerate",
			"--fill-factor", "0.04"})
		if err != nil {
			t.Fatalf("mixture with agglomerate and phi: %v", err)
		}
		checkClose(t, "--fill-factor", a.FillFactor, 0.04, 0)
	}
	{
		// r сфер и сфероидов объёмно-эквивалентный — φ ≠ 1 запрещён
		for _, args := range [][]string{
			{"--type", "sphere", "--fill-factor", "0.5"},
			{"--type", "spheroid", "--fill-factor", "0.5"},
			{"--mix", "sphere+spheroid", "--fill-factor", "0.5"},
		} {
			if _, err := ParseArgs(args); err == nil {
				t.Errorf("%v must be rejected", args)
			} else if !strings.Contains(err.Error(), "agglomerate") {
				t.Errorf("%v: unexpected error: %v", args, err)
			}
		}
	}
	{
		// φ = 1 допустим для любого типа
		for _, args := range [][]string{
			{"--type", "sphere", "--fill-factor", "1"},
			{"--type", "spheroid", "--fill-factor", "1"},
		} {
			if _, err := ParseArgs(args); err != nil {
				t.Errorf("%v must be accepted: %v", args, err)
			}
		}
	}
	{
		for _, phi := range []string{"0", "-0.1", "1.1", "nan"} {
			if _, err := ParseArgs([]string{"--type", "agglomerate",
				"--fill-factor", phi}); err == nil {
				t.Errorf("--fill-factor %s must be rejected", phi)
			}
		}
	}
}
