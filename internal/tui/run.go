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
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"csolve/internal/solve"
)

// action — выбор пользователя после прогона решателя.
type action int

const (
	actionQuit action = iota
	actionRerun
	actionEdit
	actionPreset
)

type execFinishedMsg struct{ err error }

// runModel — запуск решателя дочерним процессом в терминале и показ сводки.
// Дочерний процесс (вместо solve.Main в том же процессе) сохраняет потоки
// stdout/stderr, позволяет прервать долгий пакетный прогон по Ctrl+C и не
// трогает состояние пакета решателя.
type runModel struct {
	cmd     *exec.Cmd
	argv    []string
	summary string
	done    bool
	err     error
	exit    int
	action  action
}

func (m *runModel) Init() tea.Cmd {
	return tea.ExecProcess(m.cmd, func(err error) tea.Msg {
		return execFinishedMsg{err: err}
	})
}

func (m *runModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case execFinishedMsg:
		m.done = true
		m.err = msg.err
		m.exit = exitCode(msg.err)
		return m, nil
	case tea.KeyMsg:
		if !m.done {
			return m, nil
		}
		switch msg.String() {
		case "r", "R":
			m.action = actionRerun
			return m, tea.Quit
		case "e", "E":
			m.action = actionEdit
			return m, tea.Quit
		case "p", "P":
			m.action = actionPreset
			return m, tea.Quit
		case "q", "Q", "esc", "ctrl+c":
			m.action = actionQuit
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *runModel) View() string {
	if !m.done {
		return ""
	}

	var b strings.Builder
	switch {
	case m.err == nil:
		b.WriteString("Finished: success.\n")
	case m.exit > 0:
		fmt.Fprintf(&b, "Finished: failure (exit code %d).\n", m.exit)
	default:
		fmt.Fprintf(&b, "Finished: could not start the solver: %v\n", m.err)
	}

	b.WriteString("\nCommand:\n  csolve " + shellJoin(m.argv) + "\n\n")
	b.WriteString(m.summary + "\n\n")
	b.WriteString("r  run again    e  edit parameters    p  choose preset    q  quit\n")
	return b.String()
}

// runSolve — запуск решателя и ожидание выбора пользователя.
func runSolve(s *state, argv []string, a *solve.Args) action {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot locate the csolve executable: %v\n", err)
		return actionQuit
	}

	m := &runModel{
		cmd:     exec.Command(exe, argv...),
		argv:    argv,
		summary: describeOutputs(s, a),
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	return m.action
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
		return -1
	}
	return -1
}
