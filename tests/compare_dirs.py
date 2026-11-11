#!/usr/bin/env python3
"""Численно сравнить два каталога результатов пакетного режима csolve.

Комментарии (# ...) игнорируются, графические файлы тоже. Токены
сравниваются как числа с относительным допуском (форматирование C и Go
отличается, значения — нет).

Использование:
    python3 tests/compare_dirs.py ref_dir cur_dir
"""
import math
import pathlib
import sys

RTOL = 1e-5
ATOL = 1e-9
PLOT_EXTS = {".png", ".pdf", ".svg"}


def token_equal(a: str, b: str) -> bool:
    if a == b:
        return True
    try:
        fa, fb = float(a), float(b)
        if math.isnan(fa) and math.isnan(fb):
            return True
        return abs(fa - fb) <= ATOL + RTOL * max(abs(fa), abs(fb))
    except ValueError:
        pass
    if "=" in a and "=" in b:
        na, va = a.split("=", 1)
        nb, vb = b.split("=", 1)
        if na == nb:
            try:
                fa, fb = float(va), float(vb)
                return abs(fa - fb) <= ATOL + RTOL * max(abs(fa), abs(fb))
            except ValueError:
                return False
    return False


def compare(ref_dir: str, cur_dir: str) -> int:
    ref = pathlib.Path(ref_dir)
    cur = pathlib.Path(cur_dir)
    ref_files = {p.name for p in ref.iterdir() if p.suffix not in PLOT_EXTS}
    cur_files = {p.name for p in cur.iterdir() if p.suffix not in PLOT_EXTS}
    problems = []
    if ref_files != cur_files:
        problems.append(("file set", sorted(ref_files), sorted(cur_files)))
    for fname in sorted(ref_files & cur_files):
        a_lines = [ln for ln in (ref / fname).read_text(encoding="utf-8").splitlines()
                   if not ln.lstrip().startswith("#")]
        b_lines = [ln for ln in (cur / fname).read_text(encoding="utf-8").splitlines()
                   if not ln.lstrip().startswith("#")]
        if len(a_lines) != len(b_lines):
            problems.append((f"{fname}: line count", len(a_lines), len(b_lines)))
            continue
        for ln, (la, lb) in enumerate(zip(a_lines, b_lines), 1):
            ta, tb = la.split(), lb.split()
            if len(ta) != len(tb):
                problems.append((f"{fname}:{ln}: token count", ta, tb))
                continue
            for ta_i, tb_i in zip(ta, tb):
                if not token_equal(ta_i, tb_i):
                    problems.append((f"{fname}:{ln}", ta_i, tb_i))
                    break
    if problems:
        print(f"FAIL: {len(problems)} mismatches")
        for p in problems[:20]:
            print("   ", p)
        return 1
    print(f"ok: files match {', '.join(sorted(ref_files))}")
    return 0


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__)
        return 2
    return compare(sys.argv[1], sys.argv[2])


if __name__ == "__main__":
    raise SystemExit(main())
