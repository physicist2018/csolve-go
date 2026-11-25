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

package version

import "testing"

func TestGet(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })

	Version = "v9.9.9-test"
	if got := Get().Version; got != "v9.9.9-test" {
		t.Fatalf("version = %q, want v9.9.9-test", got)
	}

	// без -X версия всё равно должна быть непустой
	Version = ""
	if got := Get().Version; got == "" {
		t.Fatal("fallback version is empty")
	}
	if got := String(); got == "" {
		t.Fatal("String() is empty")
	}
}
