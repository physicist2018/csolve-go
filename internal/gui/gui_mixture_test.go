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

import "testing"

func hasPair(argv []string, flag, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

func hasFlag(argv []string, flag string) bool {
	for _, a := range argv {
		if a == flag {
			return true
		}
	}
	return false
}

func TestBuildArgvMixture(t *testing.T) {
	req := &Request{
		Mode:        "demo",
		TablesKind:  "builtin",
		BuiltinType: "sphere+agglomerate",
		Options: map[string]any{
			"n-sfrac":     5,
			"sfrac-min":   0.2,
			"sfrac-max":   0.8,
			"sphere-frac": "",
		},
		Output: Output{PlotFormat: "none"},
	}
	argv, a, err := buildArgv(req, t.TempDir())
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if a.Type != "sphere+agglomerate" {
		t.Fatalf("type = %q", a.Type)
	}
	if !hasPair(argv, "--n-sfrac", "5") || !hasPair(argv, "--sfrac-min", "0.2") ||
		!hasPair(argv, "--sfrac-max", "0.8") {
		t.Fatalf("mixture flags missing in argv: %v", argv)
	}
	if a.NSfrac != 5 || a.SFracMin != 0.2 || a.SFracMax != 0.8 {
		t.Fatalf("parsed sfrac args: n=%d min=%g max=%g", a.NSfrac,
			a.SFracMin, a.SFracMax)
	}
	if a.SphereFracSet {
		t.Errorf("sphere-frac must stay unset when empty")
	}
}

func TestBuildArgvMixtureSphereFrac(t *testing.T) {
	req := &Request{
		Mode:        "demo",
		TablesKind:  "builtin",
		BuiltinType: "sphere+spheroid",
		Options:     map[string]any{"sphere-frac": "0.35"},
		Output:      Output{PlotFormat: "none"},
	}
	argv, a, err := buildArgv(req, t.TempDir())
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if !hasPair(argv, "--sphere-frac", "0.35") {
		t.Fatalf("--sphere-frac missing: %v", argv)
	}
	if !a.SphereFracSet || a.SphereFrac != 0.35 {
		t.Fatalf("sphere-frac parsed: set=%v value=%g", a.SphereFracSet,
			a.SphereFrac)
	}
}

// TestBuildArgvMixtureIgnoredForNonMix — sfrac-опции не передаются вне
// составного --type (иначе несовместимый запуск).
func TestBuildArgvMixtureIgnoredForNonMix(t *testing.T) {
	req := &Request{
		Mode:        "demo",
		TablesKind:  "builtin",
		BuiltinType: "agglomerate",
		Options: map[string]any{
			"n-sfrac":   5,
			"sfrac-min": 0.2,
		},
		Output: Output{PlotFormat: "none"},
	}
	argv, a, err := buildArgv(req, t.TempDir())
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if hasFlag(argv, "--n-sfrac") || hasFlag(argv, "--sfrac-min") {
		t.Fatalf("mixture flags must be skipped for non-mix: %v", argv)
	}
	if a.NSfrac != 11 || a.SFracMin != 0 {
		t.Fatalf("defaults expected: n=%d min=%g", a.NSfrac, a.SFracMin)
	}
}
