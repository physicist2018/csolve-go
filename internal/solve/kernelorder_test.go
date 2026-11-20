package solve

import (
	"math"
	"testing"
)

// TestSvdNormCond — норма и число обусловленности на простых матрицах.
func TestSvdNormCond(t *testing.T) {
	// diag(3, 1): ||A|| = 3, cond = 3
	norm, cond := svdNormCond([]float64{3, 0, 0, 1}, 2, 2)
	checkClose(t, "norm", norm, 3, 1e-12)
	checkClose(t, "cond", cond, 3, 1e-12)

	// единичная
	norm, cond = svdNormCond([]float64{1, 0, 0, 1}, 2, 2)
	checkClose(t, "norm", norm, 1, 1e-12)
	checkClose(t, "cond", cond, 1, 1e-12)

	// вырожденная (ранг 1 из 2): обусловленность бесконечна
	_, cond = svdNormCond([]float64{1, 0, 1, 0}, 2, 2)
	if !math.IsInf(cond, 1) {
		t.Errorf("rank-deficient cond = %g, want +Inf", cond)
	}

	// нулевая матрица
	_, cond = svdNormCond([]float64{0, 0, 0, 0}, 2, 2)
	if !math.IsInf(cond, 1) {
		t.Errorf("zero-matrix cond = %g, want +Inf", cond)
	}
}

// TestKernelWeightMatchesDvol — порядок 3 обязан в точности повторять прежний
// вес dvol(r) = 3/(4πr⁴), чтобы результаты по умолчанию не менялись.
func TestKernelWeightMatchesDvol(t *testing.T) {
	for _, r := range []float64{0.05, 0.1, 0.37, 1.0, 2.5} {
		got := kernelWeight(kernelOrderVolume, r)
		want := qDvol(r)
		if got != want {
			t.Errorf("kernelWeight(3, %g) = %g, want %g", r, got, want)
		}
	}
}

// TestKernelScaleWeightIdentity — вес и масштаб должны быть согласованы:
// w_k(r)·r·g_k(r) = 1, иначе пересчёт y_k → y_3 будет несогласован.
func TestKernelScaleWeightIdentity(t *testing.T) {
	for order := 1; order <= 4; order++ {
		for _, r := range []float64{0.05, 0.2, 1.0, 3.0} {
			got := kernelWeight(order, r) * r * kernelScale(order, r)
			if math.Abs(got-1) > 1e-12 {
				t.Errorf("order %d, r = %g: w*r*g = %g, want 1", order, r, got)
			}
		}
	}
}

// TestKernelToVolumeRatio — пересчёт y_k → dV/dln r должен давать g_3·y_1
// независимо от выбранного порядка.
func TestKernelToVolumeRatio(t *testing.T) {
	for _, r := range []float64{0.05, 0.2, 1.0, 3.0} {
		const y1 = 2.5 // произвольное dN/dln r
		want := kernelScale(kernelOrderVolume, r) * y1
		for order := 1; order <= 4; order++ {
			yk := kernelScale(order, r) * y1
			got := kernelToVolumeRatio(order, r) * yk
			if math.Abs(got-want) > 1e-12*math.Abs(want) {
				t.Errorf("order %d, r = %g: y3 = %g, want %g", order, r, got, want)
			}
		}
	}
}

// TestApplyKernelToVolume — на порядке 3 профиль не должен меняться, на
// остальных — умножаться на g_3/g_k.
func TestApplyKernelToVolume(t *testing.T) {
	r := []float64{0.1, 0.5, 1.0}
	base := []float64{1.0, 2.0, 3.0}

	v := append([]float64(nil), base...)
	applyKernelToVolume(kernelOrderVolume, r, v)
	for i := range v {
		if v[i] != base[i] {
			t.Errorf("order 3 changed value at %d: %g != %g", i, v[i], base[i])
		}
	}

	v = append([]float64(nil), base...)
	applyKernelToVolume(kernelOrderNumber, r, v)
	for i := range v {
		want := base[i] * kernelToVolumeRatio(kernelOrderNumber, r[i])
		if math.Abs(v[i]-want) > 1e-12*math.Abs(want) {
			t.Errorf("order 1 value[%d] = %g, want %g", i, v[i], want)
		}
	}
}

// TestBinsDesignKernelOrders — для разных порядков матрица A должна
// соответствовать весу w_k(r) = 1/(r·g_k(r)).
func TestBinsDesignKernelOrders(t *testing.T) {
	const r0, r1 = 0.1, 1.5
	const bins = 4
	const K = 2.5
	kt := testKtable(r0*0.9, r1*1.1, 400, func(_ int, _ float64) float64 { return K })
	for order := 1; order <= 4; order++ {
		d, err := designBuild(nil, 2, []*ktable{kt}, []float64{1.5},
			[]float64{0}, []float64{r0}, []float64{r1}, 0, designSpec{
				method:      methodMonotone,
				bins:        bins,
				quadOrder:   24,
				maxLogSpan:  0.1,
				regOrder:    2,
				kernelOrder: order,
			})
		if err != nil {
			t.Fatalf("order %d: designBuild: %v", order, err)
		}
		g := d.geom[0]
		for k := 0; k < bins; k++ {
			a, b := g.binEdges[k], g.binEdges[k+1]
			nodes, wts := quadratureGrid([]float64{a, b}, 64, 0.02)
			want := 0.0
			for q := range nodes {
				want += wts[q] * K * kernelWeight(order, nodes[q])
			}
			want /= math.Log(b / a)
			for j := 0; j < 2; j++ {
				got := d.A[(0*d.nW+0)*2*bins+j*bins+k]
				checkClose(t, "A from bins", got, want, 1e-8*want)
			}
		}
	}
}

// TestArgsKernelOrder — разбор --kernel-order и значение по умолчанию.
func TestArgsKernelOrder(t *testing.T) {
	a, err := ParseArgs([]string{"--tables", "dir"})
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}
	if a.KernelOrder != defaultKernelOrder {
		t.Errorf("default kernel-order = %d, want %d", a.KernelOrder,
			defaultKernelOrder)
	}
	for _, tc := range []struct {
		val  string
		want int
	}{
		{"1", 1},
		{"2", 2},
		{"3", 3},
		{"4", 4},
	} {
		a, err := ParseArgs([]string{"--tables", "dir", "--kernel-order", tc.val})
		if err != nil {
			t.Fatalf("--kernel-order %s: %v", tc.val, err)
		}
		if a.KernelOrder != tc.want {
			t.Errorf("--kernel-order %s = %d, want %d", tc.val,
				a.KernelOrder, tc.want)
		}
	}
	for _, bad := range []string{"0", "5", "-1"} {
		if _, err := ParseArgs([]string{"--tables", "dir", "--kernel-order",
			bad}); err == nil {
			t.Errorf("--kernel-order %s must be rejected", bad)
		}
	}
	if _, err := ParseArgs([]string{"--tables", "dir", "--kernel-order", "1",
		"--kernel-order", "2"}); err == nil {
		t.Errorf("repeated --kernel-order must be rejected")
	}
}
