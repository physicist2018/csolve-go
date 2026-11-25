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
