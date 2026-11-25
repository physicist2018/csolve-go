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

// Package solve — Go-порт табличного режима csolve.
//
// Основной алгоритм — метод монотонных функций (--method monotone): на каждом
// окне [r0, r1] восстанавливается монотонная функция V(r) = ∫ dV/dln r′ dln r′
// по неотрицательным приращениям объёма на бинах сетки ln r; невязка, штраф
// за негладкость (--reg-order) и условие v ≥ 0 образуют задачу NNLS. Прежний
// сплайновый алгоритм (--method spline) сохранён: B-сплайны на окне с
// перебором степени.
//
// Численное ядро (сплайны, B-сплайны, квадратуры, регуляризованный МНК,
// ансамбль решений, моменты и моды) повторяет C-версию; линейная алгебра
// (SVD/МНК) выполняется через gonum.
package solve
