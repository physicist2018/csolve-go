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

package gui

import (
	"net/http"
	"runtime"

	"csolve/internal/version"
)

// Адрес трекера. Держим его на сервере, чтобы не дублировать в JS и не
// расходиться с репозиторием.
const repoURL = "https://github.com/physicist2018/csolve-go"

// metaHandler — сведения о сборке и ссылки для кнопки «Сообщить об ошибке».
func (a *app) metaHandler(w http.ResponseWriter, r *http.Request) {
	v := version.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":  v.Version,
		"commit":   v.Commit,
		"modified": v.Modified,
		"go":       runtime.Version(),
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
		"repo":     repoURL,
		"issues":   repoURL + "/issues/new",
	})
}
