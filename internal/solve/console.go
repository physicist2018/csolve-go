package solve

import (
	"fmt"
	"io"
)

// console — потоки вывода программы: отчёты (out) и предупреждения (errOut).
// Весь вывод идёт через него, чтобы его можно было перенаправить (тесты) и
// чтобы в пакете не было завязки на глобальные os.Stdout/os.Stderr.
type console struct {
	out, errOut io.Writer
	timing      bool
}

// newConsole — консоль с заданными потоками; timing включает хронометраж фаз.
func newConsole(out, errOut io.Writer, timing bool) *console {
	return &console{out: out, errOut: errOut, timing: timing}
}

// printf пишет строку отчёта в out.
func (c *console) printf(format string, a ...any) {
	_, _ = fmt.Fprintf(c.out, format, a...)
}

// print пишет готовую строку в out.
func (c *console) print(s string) {
	_, _ = io.WriteString(c.out, s)
}

// warnf пишет предупреждение (диагностику) в errOut.
func (c *console) warnf(format string, a ...any) {
	_, _ = fmt.Fprintf(c.errOut, format, a...)
}

// phase печатает длительность фазы, если включён хронометраж.
func (c *console) phase(name string, t0 float64) {
	if c.timing {
		_, _ = fmt.Fprintf(c.errOut, "[time] %-24s %.3f s\n", name, nowSec()-t0)
	}
}
