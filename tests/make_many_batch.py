#!/usr/bin/env python3
"""Сгенерировать пакетный txt из 7 высот для проверки ветки сглаживания
опорного профиля S (как в compare_reference.py: batch-many-alt).

Использование:
    python3 tests/make_many_batch.py out.txt
"""
import math
import sys


def main() -> int:
    if len(sys.argv) != 2:
        print(__doc__)
        return 2
    base = [9.879211342818602e-06, 1.06715705204743e-05,
            7.002700027432126e-06, 0.000302238873845872,
            0.000327611920803267]
    rows = ["Altitude  b355  b532  b1064 a355  a532"]
    for k in range(7):
        f = 1.0 + 0.05 * math.sin(1.7 * k) + 0.01 * k
        rows.append(f"{100 + 200 * k} " +
                    " ".join(f"{v * f:.17g}" for v in base))
    with open(sys.argv[1], "w", encoding="utf-8") as fh:
        fh.write("\n".join(rows) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
