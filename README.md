# csolve-go — табличный режим solve на Go

`csolve-go` — порт C-версии `csolve` (каталог `../csolve`) на **Go 1.24**.
Алгоритм и численные результаты совпадают с C-версией: ядра берутся из
внешних таблиц сечений `Cext(x)` / `Cback(x)`, далее выполняется тот же
конвейер обратной задачи (B-сплайны, квадратуры Гаусса–Лежандра,
регуляризованный МНК, перебор окон и `m`, ансамбль лучших решений,
сглаживание, моменты и логнормальные моды).

Графики, как и в C-версии, строятся через **gnuplot**.

## Отличия от C-версии

- **Go 1.24, один модуль, без cgo.** Линейная алгебра — `gonum`
  (`gonum.org/v1/gonum/mat`); сплайны, B-сплайны, квадратуры и NNLS
  портированы из C.
- **`--degree` принимает ровно одно значение** (`--degree 3`), по
  умолчанию `1`.
- **Линейная деполяризация по столбцам `S11`/`S22`.** Если таблицы нового
  формата (`# x Cext Cabs Cback Qext Qabs Qback S11 S22 n`), то `S11`/`S22`
  (значения при θ = 180°) читаются вместе с `Cext`/`Cback`, и после
  восстановления распределения считается
  `δ_l = (S11-S22)/(S11+S22)*100 %` — свёртка столбцов с тем же средним
  распределением, пересчитанная на 355 и 532 нм (`x = 2πr/λ`). В одиночном
  режиме значения печатаются в отчёте и пишутся в JSON
  (`depolarization`), в пакетном — в `delpol.txt` и на одну панель
  `delpol.<fmt>`. Старые базы без `S11`/`S22` (`base/average`, `miedir`)
  работают как раньше: δ_l не считается, печатается предупреждение.
- **JSON** читается и пишется через `encoding/json`; `NaN`/`Inf` в выводе
  кодируются как `null`, как в C. Добавлен блок `depolarization`
  (`wl_nm`, `delta_l_percent`), которого нет в C-версии.
- **Память** — обычные Go-слайсы (никаких `malloc/free`); индексация и
  порядок решений сохранены, чтобы `best_solutions[*].id`, сортировки и
  ансамбль совпадали с C.
- Формат текстовых пакетных файлов повторяет C (`%.6g`, `nan` в нижнем
  регистре), а значения в них совпадают численно.

## Сборка и тесты

```bash
cd csolve-go
make                # -> ./csolve
make build-windows  # -> ./csolve.exe (Windows x86_64)
make test           # юнит-тесты (внешние данные не нужны)
make vet
```

Или напрямую:

```bash
go build -o csolve ./cmd/csolve
go test ./...
```

### Сборка под Windows x86_64

Кросс-сборка с macOS/Linux:

```bash
make build-windows
# или
GOOS=windows GOARCH=amd64 go build -trimpath -o csolve.exe ./cmd/csolve
```

На Windows:

```bat
go build -trimpath -o csolve.exe .\cmd\csolve
```

`gonum` — чистый Go, cgo не требуется. Для графиков нужен `gnuplot.exe`
в `PATH`; пути в скриптах gnuplot переводятся на прямые слэши.

Требования:

- Go 1.24 (модуль объявляет `go 1.24`);
- `gonum` подтягивается автоматически через `go.sum`;
- `gnuplot` в `PATH` — только для графиков (при его отсутствии расчёт
  завершается с предупреждением).

## Запуск

Флаги и значения по умолчанию совпадают с C-версией (кроме `--degree`,
см. выше). Полная справка:

```bash
./csolve --help
```

Примеры:

```bash
# демонстрация на синтетических данных из таблиц
./csolve --tables ../base/average --table-extrapolate \
         --r1-min 0.5 --r1-max 1.9 --json out.json

# реальные измерения
./csolve --tables ../base/average --table-extrapolate \
         --data ../csolve/tests/data/input_example.json \
         --r1-min 0.5 --r1-max 1.9 --json out.json

# таблицы miedir записаны в конвенции Cback = 4π·dσ/dΩ
./csolve --tables ../miedir --table-cback 4pi --r1-min 1 --r1-max 5.6

# база со столбцами S11/S22: дополнительно считается δ_l(355) и δ_l(532)
./csolve --tables ../large_aggl_100/average --table-extrapolate \
         --r1-min 0.5 --r1-max 1.9 --json out.json

# одна степень B-сплайна
./csolve --tables ../base/average --table-extrapolate \
         --degree 3 --r1-min 0.5 --r1-max 1.9

# пакетно: все высоты из txt
./csolve --batch ../csolve/tests/data/measurements_example.txt \
         --tables ../base/average --table-extrapolate \
         --r1-min 0.5 --r1-max 1.9 \
         --lidar-ratio-cons --batch-out-dir out_batch
```

## Структура

```
csolve-go/
  go.mod, go.sum
  cmd/csolve/main.go        # тонкая обёртка: solve.Main(os.Args[1:])
  internal/solve/           # весь конвейер одним пакетом
    util.go                 # константы, сетки, trapz, interp
    spline.go               # not-a-knot кубический сплайн
    linalg.go               # SVD/МНК (gonum), ленточный Холецкий, NNLS
    tables.go               # data.txt, сетка m, кэш таблиц, каналы
    kernels.go              # таблица ядер K(r)
    depol.go                # ядра S11/S22 и δ_l = (S11-S22)/(S11+S22)
    basis.go                # Гаусс–Лежандр, B-сплайны, геометрия окна
    design.go               # матрицы A для всех (m, окно)
    inverse.go              # γ, CV/Err, перебор (m, r0, r1, degree)
    ensemble.go             # ансамбль и сглаживание среднего
    smoothing.go            # сглаживающий сплайн (как scipy)
    stats.go                # моменты и логнормальные моды
    profile.go              # итоговый профиль одной высоты
    jsonin.go/jsonout.go    # JSON вход/выход
    report.go               # консольный отчёт
    plot.go                 # gnuplot
    args.go                 # CLI (flag + проверки)
    run.go                  # одиночный режим, Main
    batch.go                # --batch и --lidar-ratio-cons
  tests/                    # утилиты сверки C↔Go
```

Соответствие C-модулям:

| C | Go |
|---|---|
| `src/util.c` | `util.go` |
| `src/spline.c` | `spline.go` |
| `src/linalg.c` | `linalg.go` (SVD/МНК — gonum; Холецкий и NNLS — порт) |
| `src/tables.c` | `tables.go` |
| `src/kernels.c` | `kernels.go` |
| — (нет в C) | `depol.go` — линейная деполяризация по `S11`/`S22` |
| `src/basis.c` | `basis.go` |
| `src/design.c` | `design.go` |
| `src/inverse.c` | `inverse.go` |
| `src/ensemble.c` | `ensemble.go` |
| `src/smoothing.c` | `smoothing.go` |
| `src/stats.c` | `stats.go` |
| `src/profile.c` | `profile.go` |
| `src/json_in.c` | `jsonin.go` |
| `src/json_out.c` | `jsonout.go` |
| `src/report.c` | `report.go` |
| `src/plot.c` | `plot.go` |
| `src/cli.c` | `args.go` |
| `src/main.c` | `run.go` + `cmd/csolve/main.go` |
| `src/batch.c` | `batch.go` |

## Проверка эквивалентности

1. **Юнит-тесты** (`make test`) используют те же эталоны, что и
   `csolve/tests/unit_tests.c` (значения получены из Python/scipy):
   not-a-knot сплайн, Гаусс–Лежандр, квадратуры, B-сплайны, ленточный
   Холецкий, NNLS, разбор таблиц, сглаживание (`λ = 1e-4` и `λ = 1`).
2. **Сверка с C-бинарником.** Скрипты в `tests/` сравнивают вывод двух
   реализаций:

   ```bash
   # одиночный режим: JSON
   python3 tests/compare_json.py c.json go.json

   # пакетный режим: каталоги результатов
   python3 tests/compare_dirs.py ref_dir cur_dir

   # генератор 7 высот для проверки ветки сглаживания опорного профиля
   python3 tests/make_many_batch.py /tmp/many.txt
   ```

   Проверены сценарии: `base/average`, `base_aggl/average`, `miedir`
   (`--table-cback 4pi`), вход из JSON, `--gamma-select cv|err`,
   `--nonneg`, `--absolute`, `--no-zero-boundary`, `--no-smooth`,
   `--modes stat`, степени `--degree 1|2|3|4`, а также пакетные прогоны
   (2 и 7 высот, `--lidar-ratio-cons`). При сверке C-версии передавался
   тот же `--degree`. Все численные поля совпадают с точностью
   `rtol = 1e-6`, коэффициенты `C` — `rtol = 1e-5`. Единственное
   отличие — выбранное `gamma` на плоском участке CV-критерия: решения
   при этом идентичны (этот случай и в C-версии считается допустимым).
   Блок `depolarization` в JSON пишется только Go-версией (C его не знает) —
   при сверке JSON его нужно исключать.

## Ограничения

Mie-режим (`miepython`) не переносился — как и в C-версии, ядра берутся
только из таблиц.
