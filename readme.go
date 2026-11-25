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

// Package csolve — корень модуля; здесь размещена встроенная в бинарник
// документация, которую показывает окно справки GUI. Отдельный пакет в корне
// нужен потому, что go:embed не умеет встраивать файлы выше каталога пакета.
package csolve

import _ "embed"

// ReadmeMD — содержимое README.md, встроенное в бинарник (окно справки GUI).
//
//go:embed README.md
var ReadmeMD string
