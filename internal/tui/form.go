// Copyright (C) 2026 Шмирко Константин Александрович <kshmirko@dvo.ru>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package tui

import (
	"errors"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"

	"csolve/internal/solve"
)

// page — страница формы с заголовком и пояснением в едином стиле.
func page(title, desc string, fields ...huh.Field) *huh.Group {
	g := huh.NewGroup(fields...)
	g.Title(title)
	if desc != "" {
		g.Description(desc)
	}
	return g
}

// runPresetForm — запрос пресета и того, нужна ли полная форма. Возвращает
// true, если решатель можно запускать сразу.
func runPresetForm(s *state) (bool, error) {
	presets := presetList()
	options := make([]huh.Option[string], len(presets))
	for i, p := range presets {
		options[i] = huh.NewOption(p.name, p.key)
	}

	key := presets[0].key
	review := false
	form := huh.NewForm(
		page("csolve — particle size retrieval",
			"Start from a ready-made parameter set or from the solver defaults.",
			huh.NewSelect[string]().
				Title("Preset").
				DescriptionFunc(func() string { return presetByKey(key).desc }, &key).
				Options(options...).
				Value(&key),
			huh.NewConfirm().
				Title("Review all parameters?").
				DescriptionFunc(func() string {
					if presetByKey(key).quick {
						return "“Run now” starts the retrieval immediately; “Review” opens the full form."
					}
					return "This preset always opens the full form: it needs an input file or a non-default setup."
				}, &key).
				Affirmative("Review").
				Negative("Run now").
				Value(&review),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return false, err
	}

	p := presetByKey(key)
	s.reset()
	if p.apply != nil {
		p.apply(s)
	}
	if p.quick && !review && s.readyToRun() {
		return true, nil
	}
	return false, nil
}

// runFullForm — редактирование всех параметров решателя. Возвращает
// errCancelled, если пользователь отказался от финального подтверждения.
func runFullForm(s *state) error {
	run := true
	form := huh.NewForm(
		inputGroup(s),
		jsonFileGroup(s),
		batchFileGroup(s),
		demoGroup(s),
		tablesGroup(s),
		mixtureGroup(s),
		gridGroup(s),
		regularizationGroup(s),
		constraintsGroup(s),
		numericsGroup(s),
		depolarizationGroup(s),
		lidarGroup(s),
		outputGroup(s),
		submitGroup(s, &run),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return err
	}
	if !run {
		return errCancelled
	}
	return nil
}

// inputGroup — режим входа и подсказка о том, что он потребует.
func inputGroup(s *state) *huh.Group {
	return page("Input data",
		"Where the measured coefficients come from; every other page is shared by all modes.",
		huh.NewSelect[string]().
			Title("Data source").
			DescriptionFunc(func() string {
				switch s.mode {
				case modeData:
					return "Measured β/α (and optionally δ_l) come from the JSON file on the next page."
				case modeBatch:
					return "A TXT profile with one row per altitude; the lidar-ratio options apply."
				default:
					return "Synthetic coefficients computed from the parameters on the “Demo data” page."
				}
			}, &s.mode).
			Options(
				huh.NewOption("Demo — synthetic coefficients", modeDemo),
				huh.NewOption("Single retrieval — JSON file", modeData),
				huh.NewOption("Batch profile — TXT file", modeBatch),
			).
			Value(&s.mode),
	)
}

func jsonFileGroup(s *state) *huh.Group {
	return page("Input data — JSON file",
		"Measured coefficients for a single retrieval.",
		huh.NewFilePicker().
			Title("Measurements JSON").
			Description("File with measured β/α coefficients (and optionally δ_l).").
			CurrentDirectory(".").
			FileAllowed(true).
			DirAllowed(false).
			AllowedTypes([]string{".json"}).
			Height(8).
			Value(&s.dataPath).
			Validate(func(v string) error {
				if strings.TrimSpace(v) == "" {
					return errors.New("choose a JSON file")
				}
				return nil
			}),
	).WithHideFunc(func() bool { return s.mode != modeData })
}

func batchFileGroup(s *state) *huh.Group {
	return page("Input data — batch profile",
		"Coefficients by altitude for a vertical profile. Optional altitude averaging "+
			"replaces rows within each W-metre window with their mean.",
		huh.NewFilePicker().
			Title("Batch profile TXT").
			Description("Header row: Altitude b<nm>... a<nm>... and optional dep<nm> columns.").
			CurrentDirectory(".").
			FileAllowed(true).
			DirAllowed(false).
			AllowedTypes([]string{".txt"}).
			Height(8).
			Value(&s.batchPath).
			Validate(func(v string) error {
				if strings.TrimSpace(v) == "" {
					return errors.New("choose a batch TXT file")
				}
				return nil
			}),
		huh.NewInput().
			Title("Altitude averaging window, m").
			Description("Rows within W-metre windows are averaged before retrieval; "+
				"0 = off (--batch-thin).").
			Placeholder("0").
			Value(&s.batchThin).
			Validate(vFloatNonNeg()),
	).WithHideFunc(func() bool { return s.mode != modeBatch })
}

// demoGroup — параметры генерации синтетических коэффициентов.
func demoGroup(s *state) *huh.Group {
	return page("Demo data",
		"Synthetic measurements used when no input file is given.",
		huh.NewInput().
			Title("True Re(m)").
			Description("Real part of the refractive index used to generate the data (--m-real).").
			Value(&s.mReal).
			Validate(vFloatPositive()),
		huh.NewInput().
			Title("True Im(m)").
			Description("Imaginary part, ≤ 0 (--m-imag).").
			Value(&s.mImag).
			Validate(vFloatNonPositive()),
		huh.NewInput().
			Title("Relative noise level").
			Description("Gaussian noise of the synthetic coefficients, ≥ 0 (--noise).").
			Value(&s.noise).
			Validate(vFloatNonNeg()),
		huh.NewInput().
			Title("Random seed").
			Description("RNG seed, makes a demo run reproducible (--seed).").
			Value(&s.seed).
			Validate(vInt()),
	).WithHideFunc(func() bool { return s.mode != modeDemo })
}

func tablesGroup(s *state) *huh.Group {
	builtin := make([]huh.Option[string], 0, len(solve.TableTypes()))
	for _, name := range solve.TableTypes() {
		builtin = append(builtin, huh.NewOption(name, name))
	}

	return page("Cross-section tables",
		"Source of the kernel tables and the column conventions used when they are read.",
		huh.NewSelect[string]().
			Title("Source").
			Options(
				huh.NewOption("Built-in tables", tablesBuiltin),
				huh.NewOption("External directory (--tables)", tablesExternal),
			).
			Value(&s.tablesKind),
		huh.NewSelect[string]().
			Title("Built-in table set").
			DescriptionFunc(func() string {
				if s.tablesKind == tablesExternal {
					return "Ignored while the source is “External directory”. Set with --type."
				}
				return "Cross-section data compiled into the binary (--type)."
			}, &s.tablesKind).
			Options(builtin...).
			Value(&s.builtinType),
		huh.NewInput().
			Title("External tables directory").
			DescriptionFunc(func() string {
				if s.tablesKind != tablesExternal {
					return "Ignored while the source is “Built-in tables”. Can be set with --tables."
				}
				return "Directory containing mre_<real>_mim_<imag>/data.txt subdirectories (--tables)."
			}, &s.tablesKind).
			Placeholder("./bases/large_aggl_100/shape02").
			Value(&s.tablesDir).
			Validate(func(v string) error {
				if s.tablesKind != tablesExternal {
					return nil
				}
				if strings.TrimSpace(v) == "" {
					return errors.New("enter the tables directory")
				}
				return nil
			}),
		huh.NewConfirm().
			Title("Allow cross-section extrapolation in x?").
			Description("--table-extrapolate.").
			Affirmative("Yes").
			Negative("No").
			Value(&s.tableExtrapolate),
		huh.NewInput().
			Title("Reference wavelength of the tables, µm").
			Description("Cross sections are rescaled as (λ/λ_ref)² (--table-ref-lambda-um).").
			Value(&s.tableRefWl).
			Validate(vFloatPositive()),
		huh.NewSelect[string]().
			Title("Cback column convention").
			Description("How the backscattering column is scaled (--table-cback).").
			Options(
				huh.NewOption("dsigma", "dsigma"),
				huh.NewOption("4pi", "4pi"),
			).
			Value(&s.tableCback),
	)
}

// mixtureGroup — параметры смеси sphere+X (доля сфер). Применяются только при
// --type sphere+spheroid / sphere+agglomerate; для остальных наборов
// игнорируются (значения по умолчанию не передаются).
func mixtureGroup(s *state) *huh.Group {
	desc := func(base string) func() string {
		return func() string {
			if !strings.Contains(s.builtinType, "+") {
				return "Applies only to --type sphere+spheroid / sphere+agglomerate (ignored now)."
			}
			return base
		}
	}
	return page("Sphere/nonsphere mixture",
		"Fraction of spherical particles; used with --type sphere+spheroid / sphere+agglomerate.",
		huh.NewInput().
			Title("Fixed sphere fraction sfrac (empty = scan)").
			DescriptionFunc(desc("--sphere-frac: fix sfrac (0..1) instead of scanning."), &s.builtinType).
			Placeholder("e.g. 0.5").
			Value(&s.sphereFrac).
			Validate(vFractionEmpty()),
		huh.NewInput().
			Title("Scan min sfrac").
			DescriptionFunc(desc("--sfrac-min: lower bound of the scan (0..1)."), &s.builtinType).
			Value(&s.sfracMin).
			Validate(vFraction()),
		huh.NewInput().
			Title("Scan max sfrac").
			DescriptionFunc(desc("--sfrac-max: upper bound of the scan (0..1)."), &s.builtinType).
			Value(&s.sfracMax).
			Validate(vFraction()),
		huh.NewInput().
			Title("Scan points").
			DescriptionFunc(desc("--n-sfrac: number of scanned sfrac values (>= 1)."), &s.builtinType).
			Value(&s.nSfrac).
			Validate(vIntMin(1)),
	)
}

// gridGroup — метод восстановления, базис/бины и окна перебора размеров.
func gridGroup(s *state) *huh.Group {
	return page("Method and size grid",
		"How the distribution is reconstructed and which r0/r1 windows the solver scans.",
		huh.NewSelect[string]().
			Title("Retrieval method").
			Description("Monotone functions reconstruct the cumulative volume V(r) from non-negative bin increments; spline is the previous B-spline algorithm (--method).").
			Options(
				huh.NewOption("monotone — V(r) from bin increments", "monotone"),
				huh.NewOption("spline — B-spline expansion", "spline"),
			).
			Value(&s.method),
		huh.NewSelect[int]().
			Title("Distribution solved for (kernel order)").
			Description("The inverse problem is solved for this distribution to improve numerical stability, then converted back to dV/dln r (--kernel-order).").
			Options(
				huh.NewOption("1 — dN/dln r", 1),
				huh.NewOption("2 — dS/dln r", 2),
				huh.NewOption("3 — dV/dln r (default)", 3),
				huh.NewOption("4 — d(r⁴N)/dln r", 4),
			).
			Value(&s.kernelOrder),
		huh.NewInput().
			Title("Size bins per window").
			DescriptionFunc(func() string {
				if s.method != "monotone" {
					return "Used only by the monotone method (--v-bins)."
				}
				return "0 = auto: max(2·channels, 8), at most 64 (--v-bins)."
			}, &s.method).
			Value(&s.vBins).
			Validate(vIntMin(0)),
		huh.NewSelect[int]().
			Title("Smoothness penalty order").
			DescriptionFunc(func() string {
				if s.method != "monotone" {
					return "Used only by the monotone method (--reg-order)."
				}
				return "Penalty on the bin volumes (--reg-order)."
			}, &s.method).
			Options(
				huh.NewOption("0 — minimal volume", 0),
				huh.NewOption("1 — few jumps in n", 1),
				huh.NewOption("2 — smooth n (curvature)", 2),
			).
			Value(&s.regOrder),
		huh.NewSelect[int]().
			Title("B-spline degree").
			DescriptionFunc(func() string {
				if s.method != "monotone" {
					return "Order of the piecewise polynomials (--degree)."
				}
				return "Used only by --method spline (--degree); ignored now."
			}, &s.method).
			Options(
				huh.NewOption("1 — piecewise constant", 1),
				huh.NewOption("2 — linear", 2),
				huh.NewOption("3 — quadratic", 3),
				huh.NewOption("4 — cubic", 4),
			).
			Value(&s.degree),
		huh.NewInput().
			Title("Basis functions per window").
			DescriptionFunc(func() string {
				if s.method != "monotone" {
					return "0 = as many as there are channels (--n-basis)."
				}
				return "Used only by --method spline (--n-basis); ignored now."
			}, &s.method).
			Value(&s.nBasis).
			Validate(vIntMin(0)),
		huh.NewInput().
			Title("r0 min, µm").
			Description("Lower bound of the r0 grid, > 0 (--r0-min).").
			Value(&s.r0Min).
			Validate(vFloatPositive()),
		huh.NewInput().
			Title("r0 max, µm").
			Description("Upper bound of the r0 grid (--r0-max).").
			Value(&s.r0Max).
			Validate(vFloatPositive()),
		huh.NewInput().
			Title("r0 grid points").
			Description("Size of the geometric r0 grid (--nr0).").
			Value(&s.nr0).
			Validate(vIntMin(1)),
		huh.NewInput().
			Title("r1 min, µm").
			Description("Lower bound of the r1 grid (--r1-min).").
			Value(&s.r1Min).
			Validate(vFloatPositive()),
		huh.NewInput().
			Title("r1 max, µm").
			Description("Upper bound; limited by the table coverage (--r1-max).").
			Value(&s.r1Max).
			Validate(vFloatPositive()),
		huh.NewInput().
			Title("r1 grid points").
			Description("Size of the geometric r1 grid (--nr1).").
			Value(&s.nr1).
			Validate(vIntMin(1)),
		huh.NewInput().
			Title("Minimum r1/r0 ratio").
			Description("Windows with r1 < ratio·r0 are dropped (--min-ratio).").
			Value(&s.minRatio).
			Validate(vFloatNonNeg()),
		huh.NewInput().
			Title("Solutions to average").
			Description("Integer count, or a fraction in (0,1) of all solutions (--best).").
			Value(&s.best).
			Validate(vFloatPositive()),
	)
}

func regularizationGroup(s *state) *huh.Group {
	return page("Regularization",
		"How the Tikhonov weight γ is selected; this drives how smooth the solution is.",
		huh.NewSelect[string]().
			Title("Gamma selection rule").
			Description("--gamma-select.").
			Options(
				huh.NewOption("fixed — γ = rel·‖A‖₂", "fixed"),
				huh.NewOption("cv — leave-one-out cross-validation", "cv"),
				huh.NewOption("err — minimum relative residual", "err"),
			).
			Value(&s.gammaSelect),
		huh.NewInput().
			Title("Gamma relative weight").
			Description("Used by the fixed rule, ≥ 0 (--gamma-rel).").
			Value(&s.gammaRel).
			Validate(vFloatNonNeg()),
		huh.NewInput().
			Title("Minimum lg γ").
			Description("Lower end of the γ grid, used by cv/err (--gamma-min).").
			Value(&s.gammaMin).
			Validate(vFloat()),
		huh.NewInput().
			Title("Maximum lg γ").
			Description("Upper end of the γ grid, used by cv/err (--gamma-max).").
			Value(&s.gammaMax).
			Validate(vFloat()),
		huh.NewInput().
			Title("Number of γ values").
			Description("Log grid size, ≥ 1 (--ngamma).").
			Value(&s.nGamma).
			Validate(vIntMin(1)),
	)
}

// constraintsGroup — ограничения на решение и усреднение по окнам.
func constraintsGroup(s *state) *huh.Group {
	return page("Constraints and averaging",
		"Physical constraints on the solution and how the window solutions are combined.",
		huh.NewConfirm().
			Title("Constrain C ≥ 0?").
			DescriptionFunc(func() string {
				if s.method == "monotone" {
					return "Implicit in the monotone method: non-negative increments ⇒ n(r) ≥ 0 (--nonneg)."
				}
				return "Non-negative least squares, Lawson–Hanson (--nonneg)."
			}, &s.method).
			Affirmative("Yes").
			Negative("No").
			Value(&s.nonneg),
		huh.NewConfirm().
			Title("Drop the boundary conditions n(r0) = n(r1) = 0?").
			DescriptionFunc(func() string {
				if s.method == "monotone" {
					return "Used only by --method spline (--no-zero-boundary); ignored now."
				}
				return "--no-zero-boundary."
			}, &s.method).
			Affirmative("Yes").
			Negative("No").
			Value(&s.noZeroBound),
		huh.NewConfirm().
			Title("Disable smoothing of the mean?").
			Description("--no-smooth.").
			Affirmative("Yes").
			Negative("No").
			Value(&s.noSmooth),
		huh.NewInput().
			Title("Smoothing parameter").
			Description("Spline smoothing weight, ≥ 0 (--smooth-lam).").
			Value(&s.smoothLam).
			Validate(vFloatNonNeg()),
		huh.NewInput().
			Title("Fine/coarse boundary, µm").
			Description("Splits the size range into fine and coarse parts (--r-split).").
			Value(&s.rSplit).
			Validate(vFloatPositive()),
		huh.NewSelect[string]().
			Title("Mode parameter estimation").
			Description("How the modal parameters are derived (--modes).").
			Options(
				huh.NewOption("auto", "auto"),
				huh.NewOption("fit", "fit"),
				huh.NewOption("stat", "stat"),
			).
			Value(&s.modes),
		huh.NewConfirm().
			Title("Least squares on absolute residuals?").
			Description("Relative residuals otherwise (--absolute).").
			Affirmative("Yes").
			Negative("No").
			Value(&s.absolute),
	)
}

// numericsGroup — дискретизация ядер и квадратура.
func numericsGroup(s *state) *huh.Group {
	return page("Numerics",
		"Discretization of the kernels and the quadrature; the defaults suit most runs.",
		huh.NewInput().
			Title("Max step in x for kernel tables").
			Description("> 0 (--dx-max).").
			Value(&s.dxMax).
			Validate(vFloatPositive()),
		huh.NewInput().
			Title("Output grid points").
			Description("≥ 2 (--nr-out).").
			Value(&s.nrOut).
			Validate(vIntMin(2)),
		huh.NewInput().
			Title("Gauss–Legendre order").
			Description("≥ 1 (--quad-order).").
			Value(&s.quadOrder).
			Validate(vIntMin(1)),
		huh.NewInput().
			Title("Max subsegment width in ln r").
			Description("> 0 (--log-span).").
			Value(&s.logSpan).
			Validate(vFloatPositive()),
		huh.NewInput().
			Title("Parallel workers").
			Description("0 = auto (CPU−1), 1 = sequential (--jobs).").
			Value(&s.jobs).
			Validate(vIntMin(0)),
	)
}

func depolarizationGroup(s *state) *huh.Group {
	return page("Depolarization",
		"Measured linear depolarization δ_l used as an extra constraint on the retrieval.",
		huh.NewConfirm().
			Title("Use measured depolarization δ_l?").
			Description("Requires a “depolarization” group in JSON or dep<nm> columns in TXT (--depol-ratio-cons).").
			Affirmative("Yes").
			Negative("No").
			Value(&s.depolCons),
		huh.NewInput().
			Title("Weight of the δ_l residual").
			DescriptionFunc(func() string {
				if !s.depolCons {
					return "Ignored while the δ_l constraint is off (--depol-ratio-w)."
				}
				return "≥ 0 (--depol-ratio-w)."
			}, &s.depolCons).
			Value(&s.depolW).
			Validate(vFloatNonNeg()),
		huh.NewSelect[string]().
			Title("How δ_l is applied").
			DescriptionFunc(func() string {
				if !s.depolCons {
					return "Ignored while the δ_l constraint is off (--depol-ratio-mode)."
				}
				return "--depol-ratio-mode."
			}, &s.depolCons).
			Options(
				huh.NewOption("func — residual row in the functional", "func"),
				huh.NewOption("rank — penalty when ranking solutions", "rank"),
			).
			Value(&s.depolMode),
	)
}

func lidarGroup(s *state) *huh.Group {
	return page("Lidar ratio",
		"Smoothness of S = α/β along altitude; batch mode only.",
		huh.NewConfirm().
			Title("Smooth the lidar ratio S = α/β along altitude?").
			Description("--lidar-ratio-cons.").
			Affirmative("Yes").
			Negative("No").
			Value(&s.lidarCons),
		huh.NewInput().
			Title("Weight of the S deviation penalty").
			Description("≥ 0 (--lidar-ratio-w).").
			Value(&s.lidarW).
			Validate(vFloatNonNeg()),
		huh.NewInput().
			Title("Iterations to refine reference S").
			Description("≥ 1 (--lidar-ratio-iter).").
			Value(&s.lidarIter).
			Validate(vIntMin(1)),
	).WithHideFunc(func() bool { return s.mode != modeBatch })
}

func outputGroup(s *state) *huh.Group {
	return page("Output",
		"Where the results go and how the plots are rendered.",
		huh.NewInput().
			Title("Output directory").
			Description("Relative --out/--json paths and batch results are placed here; empty = current directory (--out-dir).").
			Placeholder(".").
			Value(&s.outDir),
		huh.NewInput().
			Title("Plot file").
			Description("Empty = result.<format>; the internal renderer writes PDF only (--out).").
			Placeholder("result.pdf").
			Value(&s.out).
			Validate(s.validateOut),
		huh.NewInput().
			Title("JSON result file").
			Description("Empty = do not write JSON (--json).").
			Placeholder("result.json").
			Value(&s.jsonOut),
		huh.NewSelect[string]().
			Title("Plotting backend").
			Description("--plotting.").
			Options(
				huh.NewOption("internal — built-in PDF renderer", "internal"),
				huh.NewOption("gnuplot — external gnuplot", "gnuplot"),
			).
			Value(&s.plotting),
		huh.NewSelect[string]().
			Title("Plot format").
			DescriptionFunc(func() string {
				if s.plotting == "internal" {
					return "Ignored: the internal renderer always writes PDF (--plot-format)."
				}
				return "Format written by gnuplot (--plot-format)."
			}, &s.plotting).
			Options(
				huh.NewOption("png", "png"),
				huh.NewOption("pdf", "pdf"),
				huh.NewOption("svg", "svg"),
			).
			Value(&s.plotFormat),
		huh.NewInput().
			Title("Plot DPI").
			DescriptionFunc(func() string {
				if s.plotting == "internal" {
					return "Ignored by the internal renderer (--plot-dpi)."
				}
				return "png/svg resolution for gnuplot, > 0 (--plot-dpi)."
			}, &s.plotting).
			Value(&s.plotDpi).
			Validate(vFloatPositive()),
		huh.NewInput().
			Title("Plot font size, pt").
			Description("0 = default, otherwise 4–72 (--plot-font-size).").
			Value(&s.plotFontSize).
			Validate(vFontSize()),
		huh.NewInput().
			Title("Best solutions drawn on plots").
			Description("0 = all; the mean and envelopes always use the whole ensemble (--plot-best-limit).").
			Value(&s.plotBestLim).
			Validate(vIntMin(0)),
		huh.NewConfirm().
			Title("Skip plot generation?").
			Description("--no-plot.").
			Affirmative("Yes").
			Negative("No").
			Value(&s.noPlot),
	)
}

func submitGroup(s *state, run *bool) *huh.Group {
	return page("Summary",
		"Check the equivalent command and the files that will be written, then confirm.",
		huh.NewNote().
			Title("Review").
			DescriptionFunc(func() string {
				argv, a, err := buildArgv(s)
				if err != nil {
					return "Cannot run with these parameters:\n  " + err.Error()
				}
				var b strings.Builder
				b.WriteString("Equivalent command:\n  csolve " + shellJoin(argv) + "\n\n")
				b.WriteString(describeOutputs(s, a))
				return b.String()
			}, s.bindings()).
			Height(16),
		huh.NewConfirm().
			Title("Run the retrieval?").
			Affirmative("Run").
			Negative("Cancel").
			Value(run).
			Validate(func(ok bool) error {
				if !ok {
					return nil
				}
				_, _, err := buildArgv(s)
				return err
			}),
	)
}

// showError — показ ошибки проверки с выбором «назад» или «выход».
func showError(err error) (bool, error) {
	back := true
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Cannot run with these parameters").
				Description(err.Error()),
			huh.NewConfirm().
				Title("Edit parameters?").
				Affirmative("Edit").
				Negative("Quit").
				Value(&back),
		),
	).WithTheme(huh.ThemeCharm())
	if ferr := form.Run(); ferr != nil {
		return false, ferr
	}
	return back, nil
}

func (s *state) validateOut(v string) error {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	// Встроенный рендерер принимает только .pdf; проверяем заранее, чтобы
	// ошибка была рядом с полем. Окончательно проверяет solve.ParseArgs.
	if s.plotting == "internal" && !s.noPlot {
		if ext := strings.ToLower(filepath.Ext(v)); ext != "" && ext != ".pdf" {
			return errors.New("the internal renderer writes PDF only; use a .pdf name")
		}
	}
	return nil
}

// ---- валидаторы ----

func vFloat() func(string) error {
	return func(v string) error {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return errors.New("expected a number")
		}
		return nil
	}
}

func vInt() func(string) error {
	return func(v string) error {
		if _, err := strconv.Atoi(strings.TrimSpace(v)); err != nil {
			return errors.New("expected an integer")
		}
		return nil
	}
}

func vIntMin(minV int) func(string) error {
	return func(v string) error {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return errors.New("expected an integer")
		}
		if n < minV {
			return errors.New("must be ≥ " + strconv.Itoa(minV))
		}
		return nil
	}
}

func vFloatPositive() func(string) error {
	return func(v string) error {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return errors.New("expected a number")
		}
		if f <= 0 {
			return errors.New("must be > 0")
		}
		return nil
	}
}

func vFloatNonNeg() func(string) error {
	return func(v string) error {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return errors.New("expected a number")
		}
		if f < 0 {
			return errors.New("must be ≥ 0")
		}
		return nil
	}
}

// vFraction — число в диапазоне 0..1.
func vFraction() func(string) error {
	return func(v string) error {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return errors.New("expected a number")
		}
		if f < 0 || f > 1 {
			return errors.New("must be between 0 and 1")
		}
		return nil
	}
}

// vFractionEmpty — пустое значение или число 0..1.
func vFractionEmpty() func(string) error {
	return func(v string) error {
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return vFraction()(v)
	}
}

func vFloatNonPositive() func(string) error {
	return func(v string) error {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return errors.New("expected a number")
		}
		if f > 0 {
			return errors.New("must be ≤ 0")
		}
		return nil
	}
}

func vFontSize() func(string) error {
	return func(v string) error {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return errors.New("expected a number")
		}
		if f != 0 && (f < 4 || f > 72) {
			return errors.New("must be 0 or between 4 and 72")
		}
		return nil
	}
}
