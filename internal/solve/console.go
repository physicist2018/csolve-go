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

package solve

import (
	"fmt"
	"io"
	"sync"
)

// console — потоки вывода программы: отчёты (out) и предупреждения (errOut).
// Весь вывод идёт через него, чтобы его можно было перенаправить (тесты) и
// чтобы в пакете не было завязки на глобальные os.Stdout/os.Stderr.
//
// Методы берут mu, поэтому консоль можно безопасно использовать из
// параллельных воркеров (например, при загрузке таблиц по значениям m).
type console struct {
	mu          sync.Mutex
	out, errOut io.Writer
	timing      bool
}

// newConsole — консоль с заданными потоками; timing включает хронометраж фаз.
func newConsole(out, errOut io.Writer, timing bool) *console {
	return &console{out: out, errOut: errOut, timing: timing}
}

// printf пишет строку отчёта в out.
func (c *console) printf(format string, a ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = fmt.Fprintf(c.out, format, a...)
}

// print пишет готовую строку в out.
func (c *console) print(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = io.WriteString(c.out, s)
}

// warnf пишет предупреждение (диагностику) в errOut.
func (c *console) warnf(format string, a ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = fmt.Fprintf(c.errOut, format, a...)
}

// phase печатает длительность фазы, если включён хронометраж.
func (c *console) phase(name string, t0 float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timing {
		_, _ = fmt.Fprintf(c.errOut, "[time] %-24s %.3f s\n", name, nowSec()-t0)
	}
}
