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

// runPresetForm asks for a preset and whether the full parameter form should be
// shown. It returns true when the caller may run the solver right away.
func runPresetForm(s *state) (bool, error) {
	presets := presetList()
	options := make([]huh.Option[string], len(presets))
	for i, p := range presets {
		options[i] = huh.NewOption(p.name, p.key)
	}

	key := presets[0].key
	review := false
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Preset").
				Description("Ready-made parameter sets. Choose “Custom” to start from the defaults.").
				Options(options...).
				Value(&key),
			huh.NewConfirm().
				Title("Review all parameters?").
				Affirmative("Review").
				Negative("Run now").
				Value(&review),
		).Title("csolve — particle size retrieval"),
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

// runFullForm edits all solver parameters. It returns errCancelled when the
// user declines the final confirmation.
func runFullForm(s *state) error {
	run := true
	form := huh.NewForm(
		inputGroup(s),
		jsonFileGroup(s),
		batchFileGroup(s),
		tablesGroup(s),
		windowsGroup(s),
		regularizationGroup(s),
		processingGroup(s),
		depolarizationGroup(s),
		lidarGroup(s),
		demoGroup(s),
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

func inputGroup(s *state) *huh.Group {
	return huh.NewGroup(
		huh.NewSelect[string]().
			Title("Input data").
			Options(
				huh.NewOption("Demo — synthetic coefficients", modeDemo),
				huh.NewOption("Single retrieval — JSON file", modeData),
				huh.NewOption("Batch profile — TXT file", modeBatch),
			).
			Value(&s.mode),
		huh.NewInput().
			Title("Output directory").
			Description("Relative --out/--json paths and batch results are placed here. Empty = current directory.").
			Placeholder(".").
			Value(&s.outDir),
	).Title("Input")
}

func jsonFileGroup(s *state) *huh.Group {
	return huh.NewGroup(
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
	).Title("Input file").WithHideFunc(func() bool { return s.mode != modeData })
}

func batchFileGroup(s *state) *huh.Group {
	return huh.NewGroup(
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
	).Title("Input file").WithHideFunc(func() bool { return s.mode != modeBatch })
}

func tablesGroup(s *state) *huh.Group {
	builtin := make([]huh.Option[string], 0, len(solve.TableTypes()))
	for _, name := range solve.TableTypes() {
		builtin = append(builtin, huh.NewOption(name, name))
	}

	return huh.NewGroup(
		huh.NewSelect[string]().
			Title("Cross-section source").
			Options(
				huh.NewOption("Built-in tables", tablesBuiltin),
				huh.NewOption("External directory (--tables)", tablesExternal),
			).
			Value(&s.tablesKind),
		huh.NewSelect[string]().
			Title("Built-in table set").
			Description("Used when the source is “Built-in tables”.").
			Options(builtin...).
			Value(&s.builtinType),
		huh.NewInput().
			Title("External tables directory").
			Description("Directory containing mre_<real>_mim_<imag>/data.txt subdirectories.").
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
			Description("--table-extrapolate").
			Affirmative("Yes").
			Negative("No").
			Value(&s.tableExtrapolate),
		huh.NewInput().
			Title("Reference wavelength of the tables, µm").
			Description("Cross sections are rescaled as (λ/λ_ref)².").
			Value(&s.tableRefWl).
			Validate(vFloatPositive()),
		huh.NewSelect[string]().
			Title("Cback column convention").
			Options(
				huh.NewOption("dsigma", "dsigma"),
				huh.NewOption("4pi", "4pi"),
			).
			Value(&s.tableCback),
	).Title("Cross-section tables")
}

func windowsGroup(s *state) *huh.Group {
	return huh.NewGroup(
		huh.NewSelect[int]().
			Title("B-spline degree").
			Options(
				huh.NewOption("1", 1),
				huh.NewOption("2", 2),
				huh.NewOption("3", 3),
				huh.NewOption("4", 4),
			).
			Value(&s.degree),
		huh.NewInput().
			Title("Basis functions per window").
			Description("0 = as many as there are channels.").
			Value(&s.nBasis).
			Validate(vIntMin(0)),
		huh.NewInput().Title("r0 min, µm").Description("Lower bound of the r0 grid (> 0).").Value(&s.r0Min).Validate(vFloatPositive()),
		huh.NewInput().Title("r0 max, µm").Description("Upper bound of the r0 grid.").Value(&s.r0Max).Validate(vFloatPositive()),
		huh.NewInput().Title("r0 values").Description("Geometric grid size.").Value(&s.nr0).Validate(vIntMin(1)),
		huh.NewInput().Title("r1 min, µm").Description("Lower bound of the r1 grid.").Value(&s.r1Min).Validate(vFloatPositive()),
		huh.NewInput().Title("r1 max, µm").Description("Upper bound; limited by table coverage.").Value(&s.r1Max).Validate(vFloatPositive()),
		huh.NewInput().Title("r1 values").Description("Geometric grid size.").Value(&s.nr1).Validate(vIntMin(1)),
		huh.NewInput().Title("Min ratio r1/r0").Description("Windows with r1 < ratio·r0 are dropped.").Value(&s.minRatio).Validate(vFloatNonNeg()),
		huh.NewInput().
			Title("Best solutions to average").
			Description("Integer count, or a fraction in (0,1) of all solutions.").
			Value(&s.best).
			Validate(vFloatPositive()),
	).Title("Basis and windows")
}

func regularizationGroup(s *state) *huh.Group {
	return huh.NewGroup(
		huh.NewSelect[string]().
			Title("Gamma selection rule").
			Options(
				huh.NewOption("fixed — γ = rel·‖A‖₂", "fixed"),
				huh.NewOption("cv — leave-one-out cross-validation", "cv"),
				huh.NewOption("err — minimum relative residual", "err"),
			).
			Value(&s.gammaSelect),
		huh.NewInput().Title("Gamma relative weight").Description("Used by the fixed rule (≥ 0).").Value(&s.gammaRel).Validate(vFloatNonNeg()),
		huh.NewInput().Title("Minimum lg γ").Description("Used by cv/err.").Value(&s.gammaMin).Validate(vFloat()),
		huh.NewInput().Title("Maximum lg γ").Description("Used by cv/err.").Value(&s.gammaMax).Validate(vFloat()),
		huh.NewInput().Title("Number of gamma values").Description("Log grid size.").Value(&s.nGamma).Validate(vIntMin(1)),
	).Title("Regularization")
}

func processingGroup(s *state) *huh.Group {
	return huh.NewGroup(
		huh.NewConfirm().Title("Constrain C ≥ 0?").Description("NNLS, Lawson–Hanson.").Affirmative("Yes").Negative("No").Value(&s.nonneg),
		huh.NewConfirm().Title("Drop boundary conditions n(r0)=n(r1)=0?").Description("--no-zero-boundary").Affirmative("Yes").Negative("No").Value(&s.noZeroBound),
		huh.NewConfirm().Title("Disable smoothing of the mean?").Description("--no-smooth").Affirmative("Yes").Negative("No").Value(&s.noSmooth),
		huh.NewInput().Title("Smoothing parameter").Description("Spline smoothing weight (≥ 0).").Value(&s.smoothLam).Validate(vFloatNonNeg()),
		huh.NewInput().Title("Fine/coarse boundary, µm").Description("--r-split").Value(&s.rSplit).Validate(vFloatPositive()),
		huh.NewSelect[string]().
			Title("Mode parameter estimation").
			Options(
				huh.NewOption("auto", "auto"),
				huh.NewOption("fit", "fit"),
				huh.NewOption("stat", "stat"),
			).
			Value(&s.modes),
		huh.NewConfirm().Title("Least squares on absolute residuals?").Description("--absolute; default is relative residuals.").Affirmative("Yes").Negative("No").Value(&s.absolute),
		huh.NewInput().Title("Max step in x for kernel tables").Description("--dx-max (> 0).").Value(&s.dxMax).Validate(vFloatPositive()),
		huh.NewInput().Title("Output grid points").Description("--nr-out (≥ 2).").Value(&s.nrOut).Validate(vIntMin(2)),
		huh.NewInput().Title("Gauss–Legendre order").Description("--quad-order (≥ 1).").Value(&s.quadOrder).Validate(vIntMin(1)),
		huh.NewInput().Title("Max subsegment width in ln r").Description("--log-span (> 0).").Value(&s.logSpan).Validate(vFloatPositive()),
	).Title("Processing and numerics")
}

func depolarizationGroup(s *state) *huh.Group {
	return huh.NewGroup(
		huh.NewConfirm().
			Title("Use measured depolarization δ_l?").
			Description("Requires a “depolarization” group in JSON or dep<nm> columns in batch TXT.").
			Affirmative("Yes").
			Negative("No").
			Value(&s.depolCons),
		huh.NewInput().
			Title("Weight of the δ_l residual").
			Description("--depol-ratio-w (≥ 0).").
			Value(&s.depolW).
			Validate(vFloatNonNeg()),
		huh.NewSelect[string]().
			Title("How δ_l is applied").
			Options(
				huh.NewOption("func — residual row in the functional", "func"),
				huh.NewOption("rank — penalty when ranking solutions", "rank"),
			).
			Value(&s.depolMode),
	).Title("Depolarization")
}

func lidarGroup(s *state) *huh.Group {
	return huh.NewGroup(
		huh.NewConfirm().
			Title("Smooth the lidar ratio S = α/β along altitude?").
			Description("--lidar-ratio-cons; batch mode only.").
			Affirmative("Yes").
			Negative("No").
			Value(&s.lidarCons),
		huh.NewInput().Title("Weight of the S deviation penalty").Description("--lidar-ratio-w (≥ 0).").Value(&s.lidarW).Validate(vFloatNonNeg()),
		huh.NewInput().Title("Iterations to refine reference S").Description("--lidar-ratio-iter (≥ 1).").Value(&s.lidarIter).Validate(vIntMin(1)),
	).Title("Lidar ratio").WithHideFunc(func() bool { return s.mode != modeBatch })
}

func demoGroup(s *state) *huh.Group {
	return huh.NewGroup(
		huh.NewInput().Title("True Re(m) for the demo").Value(&s.mReal).Validate(vFloatPositive()),
		huh.NewInput().Title("True Im(m) for the demo").Value(&s.mImag).Validate(vFloatNonPositive()),
		huh.NewInput().Title("Relative noise level").Description("Gaussian noise of the synthetic coefficients (≥ 0).").Value(&s.noise).Validate(vFloatNonNeg()),
		huh.NewInput().Title("Random seed").Value(&s.seed).Validate(vInt()),
	).Title("Demo data").WithHideFunc(func() bool { return s.mode != modeDemo })
}

func outputGroup(s *state) *huh.Group {
	return huh.NewGroup(
		huh.NewInput().
			Title("Plot file").
			Description("Empty = result.<format>. The internal renderer writes PDF only.").
			Placeholder("result.pdf").
			Value(&s.out).
			Validate(s.validateOut),
		huh.NewInput().
			Title("JSON result file").
			Description("Empty = do not write JSON.").
			Placeholder("result.json").
			Value(&s.jsonOut),
		huh.NewSelect[string]().
			Title("Plotting backend").
			Options(
				huh.NewOption("internal — built-in PDF renderer", "internal"),
				huh.NewOption("gnuplot — external gnuplot", "gnuplot"),
			).
			Value(&s.plotting),
		huh.NewSelect[string]().
			Title("Plot format").
			Description("Used by the gnuplot backend; internal always writes PDF.").
			Options(
				huh.NewOption("png", "png"),
				huh.NewOption("pdf", "pdf"),
				huh.NewOption("svg", "svg"),
			).
			Value(&s.plotFormat),
		huh.NewInput().Title("Plot DPI").Description("png/svg resolution for gnuplot (> 0).").Value(&s.plotDpi).Validate(vFloatPositive()),
		huh.NewInput().Title("Plot font size, pt").Description("0 = default, otherwise 4–72.").Value(&s.plotFontSize).Validate(vFontSize()),
		huh.NewConfirm().Title("Skip plot generation?").Description("--no-plot").Affirmative("Yes").Negative("No").Value(&s.noPlot),
	).Title("Output")
}

func submitGroup(s *state, run *bool) *huh.Group {
	return huh.NewGroup(
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
	).Title("Summary")
}

// showError displays a validation error and lets the user go back or quit.
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
	// The internal renderer rejects non-PDF names; check early so the error is
	// shown next to the field. solve.ParseArgs performs the final check.
	if s.plotting == "internal" && !s.noPlot {
		if ext := strings.ToLower(filepath.Ext(v)); ext != "" && ext != ".pdf" {
			return errors.New("the internal renderer writes PDF only; use a .pdf name")
		}
	}
	return nil
}

// ---- validators ----

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
