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

import "testing"

func hasPair(argv []string, flag, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

func TestBuildArgvMixture(t *testing.T) {
	s := newTestState(t)
	s.builtinType = "sphere+spheroid"
	s.sphereFrac = "0.45"
	s.sfracMin = "0.1"
	s.sfracMax = "0.9"
	s.nSfrac = "7"

	argv, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if a.Type != "sphere+spheroid" {
		t.Fatalf("type = %q", a.Type)
	}
	if !hasPair(argv, "--sphere-frac", "0.45") ||
		!hasPair(argv, "--sfrac-min", "0.1") ||
		!hasPair(argv, "--sfrac-max", "0.9") ||
		!hasPair(argv, "--n-sfrac", "7") {
		t.Fatalf("mixture flags missing: %v", argv)
	}
	if !a.SphereFracSet || a.SphereFrac != 0.45 {
		t.Fatalf("sphere-frac parsed: set=%v value=%g", a.SphereFracSet, a.SphereFrac)
	}
	if a.SFracMin != 0.1 || a.SFracMax != 0.9 || a.NSfrac != 7 {
		t.Fatalf("parsed sfrac: min=%g max=%g n=%d", a.SFracMin, a.SFracMax, a.NSfrac)
	}
}

// TestBuildArgvMixtureIgnoredNonMix — вне составного --type sfrac-флаги не
// передаются, даже если значения в форме изменены.
func TestBuildArgvMixtureIgnoredNonMix(t *testing.T) {
	s := newTestState(t)
	s.builtinType = "agglomerate"
	s.sphereFrac = "0.4"
	s.sfracMin = "0.2"
	s.sfracMax = "0.7"
	s.nSfrac = "3"

	argv, a, err := buildArgv(s)
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if contains(argv, "--sphere-frac") || contains(argv, "--sfrac-min") ||
		contains(argv, "--sfrac-max") || contains(argv, "--n-sfrac") {
		t.Fatalf("mixture flags must be skipped for non-mix: %v", argv)
	}
	if a.Type != "agglomerate" || a.SphereFracSet {
		t.Fatalf("type=%q sphereFracSet=%v", a.Type, a.SphereFracSet)
	}
	if a.NSfrac != 11 || a.SFracMin != 0 || a.SFracMax != 1 {
		t.Fatalf("defaults expected: n=%d min=%g max=%g", a.NSfrac, a.SFracMin, a.SFracMax)
	}
}
