package tui

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
)

// errCancelled is returned when the user declines the final confirmation.
var errCancelled = errors.New("cancelled by user")

// Run starts the interactive UI and returns the process exit code.
func Run() int {
	s := newState()

presetLoop:
	for {
		skipReview, err := runPresetForm(s)
		if err != nil {
			return abortCode(err)
		}

		needsForm := !skipReview
		for {
			if needsForm {
				if err := runFullForm(s); err != nil {
					if errors.Is(err, errCancelled) {
						continue presetLoop
					}
					return abortCode(err)
				}
				needsForm = false
			}

			argv, a, err := buildArgv(s)
			if err != nil {
				back, ferr := showError(err)
				if ferr != nil {
					return abortCode(ferr)
				}
				if !back {
					return 0
				}
				needsForm = true
				continue
			}

			switch runSolve(s, argv, a) {
			case actionRerun:
				continue
			case actionEdit:
				needsForm = true
				continue
			case actionPreset:
				continue presetLoop
			default:
				return 0
			}
		}
	}
}

// abortCode maps a form error to an exit code: normal cancellation is not an
// error, anything else is reported on stderr.
func abortCode(err error) int {
	if errors.Is(err, huh.ErrUserAborted) || errors.Is(err, errCancelled) {
		return 0
	}
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	return 1
}
