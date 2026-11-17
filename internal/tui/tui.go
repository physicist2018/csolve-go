// Package tui — интерактивный режим csolve (charmbracelet/huh и bubbletea):
// пресеты, форма параметров, запуск решателя дочерним процессом и экран
// результата.
package tui

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
)

// errCancelled — пользователь отказался от финального подтверждения.
var errCancelled = errors.New("cancelled by user")

// Run — запуск интерактивного режима; возвращает код возврата процесса.
func Run() int {
	s, err := newState()
	if err != nil {
		return abortCode(err)
	}

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

// abortCode — код возврата по ошибке формы: обычная отмена не считается
// ошибкой, остальное печатается в stderr.
func abortCode(err error) int {
	if errors.Is(err, huh.ErrUserAborted) || errors.Is(err, errCancelled) {
		return 0
	}
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	return 1
}
