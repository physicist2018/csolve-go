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
	"runtime"
	"sync"
	"sync/atomic"
)

// jobsMax — верхняя граница числа воркеров (защита от опечаток в --jobs).
const jobsMax = 1024

// JobsDefault — значение --jobs по умолчанию: максимальное число процессов
// минус один (не меньше 1).
func JobsDefault() int {
	n := runtime.NumCPU() - 1
	if n < 1 {
		n = 1
	}
	if n > jobsMax {
		n = jobsMax
	}
	return n
}

// resolveJobs — фактическое число воркеров: 0 (и отрицательные) означают
// auto, то есть JobsDefault.
func resolveJobs(n int) int {
	if n <= 0 {
		return JobsDefault()
	}
	if n > jobsMax {
		n = jobsMax
	}
	return n
}

// Workers — число параллельных воркеров текущего запуска (0 в Args.Jobs =
// auto).
func (a *Args) Workers() int {
	if a == nil {
		return 1
	}
	return resolveJobs(a.Jobs)
}

// parallelFor вызывает fn для каждого i из [0, n). При workers <= 1 или
// n <= 1 цикл выполняется последовательно. fn не должен разделять изменяемое
// состояние между разными i: каждый индекс обязан писать только в собственные
// ячейки, тогда результат не зависит от планировщика.
func parallelFor(n, workers int, fn func(i int)) {
	if n <= 0 {
		return
	}
	if workers > n {
		workers = n
	}
	if workers <= 1 {
		for i := 0; i < n; i++ {
			fn(i)
		}
		return
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				fn(i)
			}
		}()
	}
	wg.Wait()
}
