// Package solve — Go-порт табличного режима csolve.
//
// Численное ядро (сплайны, B-сплайны, квадратуры, регуляризованный МНК,
// ансамбль решений, моменты и моды) повторяет C-версию; линейная алгебра
// (SVD/МНК) выполняется через gonum.
package solve

import "fmt"

// fatalError воспроизводит поведение cs_fatal: сообщение печатается как
// "error: ..." и программа завершается с кодом 1. Внутри библиотеки
// используется panic, который перехватывается в Main.
type fatalError struct{ msg string }

func (e fatalError) Error() string { return e.msg }

func fatal(format string, a ...any) {
	panic(fatalError{msg: fmt.Sprintf(format, a...)})
}
