package solve

import (
	"math"
)

// Порядки --kernel-order. Порядок k означает, что обратная задача решается
// относительно распределения y_k = dX_k/dln r, где X_k — момент r^e_k числа
// частиц:
//
//	1 — dN/dln r        (число)
//	2 — dS/dln r        (поверхность, S = ∫ 4πr² dN)
//	3 — dV/dln r        (объём, V = ∫ (4π/3)r³ dN)
//	4 — d(r⁴N)/dln r    (четвёртая степень)
//
// Связь с числовой концентрацией: n(r) = dN/dr = y_k/(r·g_k(r)), где
// g_k = y_k/(dN/dln r). Поэтому вклад узла квадратуры в матрицу A равен
// w_k(r) = 1/(r·g_k(r)); для порядка 3 это прежнее dvol = 3/(4πr⁴).
const (
	kernelOrderNumber  = 1
	kernelOrderSurface = 2
	kernelOrderVolume  = 3
	kernelOrderFourth  = 4
)

// defaultKernelOrder — порядок по умолчанию (dV/dln r, поведение до появления
// опции --kernel-order).
const defaultKernelOrder = kernelOrderVolume

// normalizeKernelOrder приводит порядок к диапазону 1..4; некорректное
// значение (в том числе нулевое designSpec) трактуется как объёмный порядок.
func normalizeKernelOrder(k int) int {
	if k < kernelOrderNumber || k > kernelOrderFourth {
		return defaultKernelOrder
	}
	return k
}

// kernelScale — g_k(r) = y_k/(dN/dln r): во сколько раз выбранное
// распределение больше числового в точке r.
func kernelScale(order int, r float64) float64 {
	switch normalizeKernelOrder(order) {
	case kernelOrderNumber:
		return 1
	case kernelOrderSurface:
		return 4.0 * math.Pi * r * r
	case kernelOrderFourth:
		return r * r * r * r
	default: // kernelOrderVolume
		return (4.0 * math.Pi / 3.0) * r * r * r
	}
}

// kernelWeight — множитель узла квадратуры w_k(r) = 1/(r·g_k(r)),
// переводящий коэффициенты y_k в числовую концентрацию. Для порядка 3 в
// точности совпадает с прежним dvol(r) = 3/(4πr⁴).
func kernelWeight(order int, r float64) float64 {
	if normalizeKernelOrder(order) == kernelOrderVolume {
		return qDvol(r)
	}
	return 1.0 / (r * kernelScale(order, r))
}

// kernelToVolumeRatio — множитель пересчёта выбранного распределения y_k в
// объёмное dV/dln r: y₃ = (g₃/g_k)·y_k. Для порядка 3 равен 1.
func kernelToVolumeRatio(order int, r float64) float64 {
	switch normalizeKernelOrder(order) {
	case kernelOrderNumber:
		return (4.0 * math.Pi / 3.0) * r * r * r
	case kernelOrderSurface:
		return r / 3.0
	case kernelOrderFourth:
		return (4.0 * math.Pi / 3.0) / r
	default: // kernelOrderVolume
		return 1
	}
}

// applyKernelToVolume пересчитывает профиль выбранного распределения y_k в
// dV/dln r на сетке rGrid (на месте).
func applyKernelToVolume(order int, rGrid, vals []float64) {
	if normalizeKernelOrder(order) == kernelOrderVolume {
		return
	}
	for i := range vals {
		vals[i] *= kernelToVolumeRatio(order, rGrid[i])
	}
}

// kernelOrderLabel — короткое обозначение распределения для справки и JSON.
func kernelOrderLabel(order int) string {
	switch normalizeKernelOrder(order) {
	case kernelOrderNumber:
		return "dN/dln r"
	case kernelOrderSurface:
		return "dS/dln r"
	case kernelOrderFourth:
		return "d(r^4 N)/dln r"
	default:
		return "dV/dln r"
	}
}

// kernelVolumeConversion — пояснение для JSON: что означают коэффициенты
// решения при выбранном порядке и как пересчитать их в dV/dln r.
func kernelVolumeConversion(order int, method methodKind) string {
	order = normalizeKernelOrder(order)
	if method == methodMonotone {
		switch order {
		case kernelOrderNumber:
			return "v_bins are increments of dN/dln r; volume increment " +
				"dV_k = v_bins[k] * (4*pi/3) * mean_bin(r^3), where " +
				"mean_bin(r^3) = (1/dln r_k) * integral over bin of r^3 dln r"
		case kernelOrderSurface:
			return "v_bins are increments of dS/dln r; volume increment " +
				"dV_k = v_bins[k] * mean_bin(r) / 3, where " +
				"mean_bin(r) = (1/dln r_k) * integral over bin of r dln r"
		case kernelOrderFourth:
			return "v_bins are increments of d(r^4 N)/dln r; volume increment " +
				"dV_k = v_bins[k] * (4*pi/3) * mean_bin(1/r), where " +
				"mean_bin(1/r) = (1/dln r_k) * integral over bin of r^-1 dln r"
		default:
			return "v_bins are increments of dV/dln r (no conversion needed)"
		}
	}
	switch order {
	case kernelOrderNumber:
		return "C are basis coefficients of dN/dln r; pointwise " +
			"dV/dln r = (4*pi/3)*r^3 * (dN/dln r distribution)"
	case kernelOrderSurface:
		return "C are basis coefficients of dS/dln r; pointwise " +
			"dV/dln r = (r/3) * (dS/dln r distribution)"
	case kernelOrderFourth:
		return "C are basis coefficients of d(r^4 N)/dln r; pointwise " +
			"dV/dln r = ((4*pi/3)/r) * (d(r^4 N)/dln r distribution)"
	default:
		return "C are basis coefficients of dV/dln r (no conversion needed)"
	}
}
