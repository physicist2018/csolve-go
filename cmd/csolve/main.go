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

package main

import (
	"fmt"
	"os"

	"csolve/internal/gui"
	"csolve/internal/solve"
	"csolve/internal/tui"
)

func main() {
	args := os.Args[1:]

	if len(args) >= 1 && (args[0] == "--gui" || args[0] == "-gui") {
		os.Exit(gui.Run(args[1:]))
	}

	if len(args) == 1 && (args[0] == "--tui" || args[0] == "-tui") {
		os.Exit(tui.Run())
	}
	for _, arg := range args {
		if arg == "--tui" || arg == "-tui" {
			fmt.Fprintln(os.Stderr, "error: --tui does not accept other options")
			os.Exit(1)
		}
		if arg == "--gui" || arg == "-gui" {
			fmt.Fprintln(os.Stderr, "error: --gui must be the first argument")
			os.Exit(1)
		}
	}

	os.Exit(solve.Main(args))
}
