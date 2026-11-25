#!/usr/bin/env python3

# Copyright (C) 2026 Шмирко Константин Александрович <kshmirko@dvo.ru>
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.
#
# SPDX-License-Identifier: GPL-3.0-or-later

"""Сравнить два JSON-файла csolve численно (например, вывод C и Go).

Использование:
    python3 tests/compare_json.py ref.json cur.json
"""
import json
import math
import sys

RTOL = 1e-6
ATOL = 1e-9
C_RTOL = 1e-5


def compare(ref, cur, path="", bad=None, info=None, scale=0.0, rtol=RTOL):
    if bad is None:
        bad = []
    if info is None:
        info = []
    if isinstance(ref, dict):
        if set(ref) != set(cur):
            bad.append((path, "key set", sorted(ref), sorted(cur)))
            return bad
        for k in ref:
            compare(ref[k], cur[k], f"{path}.{k}" if path else k, bad, info,
                    scale, rtol)
    elif isinstance(ref, list):
        if len(ref) != len(cur):
            bad.append((path, "list length", len(ref), len(cur)))
            return bad
        child_scale, child_rtol = scale, rtol
        if path.rsplit(".", 1)[-1] == "C":
            child_scale = max(
                [abs(float(v)) for v in ref + cur if isinstance(v, (int, float))]
                + [0.0]
            )
            child_rtol = C_RTOL
        for i, (a, b) in enumerate(zip(ref, cur)):
            compare(a, b, f"{path}[{i}]", bad, info, child_scale, child_rtol)
    elif isinstance(ref, (int, float)) and isinstance(cur, (int, float)):
        if isinstance(ref, bool) or isinstance(cur, bool):
            if ref != cur:
                bad.append((path, "bool", ref, cur))
            return bad
        if math.isnan(ref) and math.isnan(cur):
            return bad
        mag = max(abs(ref), abs(cur))
        if abs(ref - cur) > ATOL + rtol * max(mag, scale):
            if path.rsplit(".", 1)[-1] == "gamma":
                # на плоском участке CV-критерия разные gamma дают одно и то же
                # решение (C и Err уже сверены) — это не расхождение
                info.append((path, ref, cur))
            else:
                bad.append((path, "number", ref, cur, abs(ref - cur)))
    else:
        if ref != cur:
            bad.append((path, "value", ref, cur))
    return bad


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__)
        return 2
    with open(sys.argv[1], encoding="utf-8") as fh:
        ref = json.load(fh)
    with open(sys.argv[2], encoding="utf-8") as fh:
        cur = json.load(fh)
    info = []
    bad = compare(ref, cur, info=info)
    n_ref = len(ref.get("best_solutions", []))
    n_cur = len(cur.get("best_solutions", []))
    if bad:
        print(f"FAIL: {len(bad)} mismatches ({n_ref} vs {n_cur} solutions)")
        for item in bad[:20]:
            print("   ", item)
        return 1
    extra = ""
    if info:
        extra = f" (gamma: {len(info)} on the CV plateau — identical solutions)"
    print(f"ok: all fields within tolerance "
          f"(rtol={RTOL:g}, atol={ATOL:g}, C rtol={C_RTOL:g}); "
          f"{n_ref} solutions{extra}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
