package solve

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"csolve/bases"
)

// table — таблица сечений (формат mkbase / mkbase_spheres).
//
// Помимо Cext/Cback читаются S11/S22 (значения s11/s22 при θ = 180°), по
// которым считается линейная деполяризация обратного рассеяния; эти столбцы
// необязательны — в старых базах их нет.
type table struct {
	n                 int
	x, cext, cback    []float64
	s11, s22          []float64
	hasCext, hasCback bool
	hasS11, hasS22    bool
	spCext, spCback   *spline
	spS11, spS22      *spline
	xMin, xMax        float64
	extrapolate       bool
	sClampWarned      bool
	path              string
}

// tableColumnNames — колонки data.txt, которые читаются (выбор по имени).
var tableColumnNames = []string{"Cext", "Cback", "S11", "S22"}

// builtinTableDirs — встроенные наборы таблиц: значение --type -> каталог
// внутри пакета bases.
var builtinTableDirs = map[string]string{
	"sphere":      "base_sph_25/average",
	"spheroid":    "base_sphrd_25/average",
	"agglomerate": "large_aggl_100/average",
}

// builtinTypeNames — значения --type в порядке справки и сообщений.
var builtinTypeNames = []string{"sphere", "spheroid", "agglomerate"}

// defaultTableType — встроенный набор, используемый без --tables/--type.
const defaultTableType = "agglomerate"

type chKind int

const (
	chAlpha chKind = 0
	chBeta  chKind = 1
)

// channel — измерительный канал (β — обратное рассеяние, α — ослабление).
type channel struct {
	kind       chKind
	wlUm       float64
	tab        *table
	refWlUm    float64
	cbackScale float64
}

func (c channel) name() string {
	k := "alpha"
	if c.kind == chBeta {
		k = "beta"
	}
	return fmt.Sprintf("%s(%g nm)", k, c.wlUm*1e3)
}

type channels struct {
	ch  []channel
	n   int
	cap float64 // min x_max*wl/(2*pi) по каналам
}

// ---------------- источник таблиц ----------------

// tableSource — откуда берутся таблицы: встроенный набор в бинарнике или
// каталог на диске. Оба режима читаются через fs.FS, поэтому конвейер не
// зависит от способа хранения; встроенные таблицы дополнительно сжаты gzip.
type tableSource struct {
	fsys   fs.FS
	label  string // для сообщений: "builtin:sphere" или путь из --tables
	key    string // идентичность источника для кэша таблиц
	origin string // каталог встроенного набора; "" для внешнего

	mu       sync.Mutex
	warnedDx bool
}

// tableFileName — имя файла таблицы внутри каталога m.
const tableFileName = "data.txt"

// externalTableSource — таблицы из каталога на диске (--tables).
func externalTableSource(dir string) *tableSource {
	return &tableSource{fsys: os.DirFS(dir), label: dir, key: "dir:" + dir}
}

// builtinTableSource — встроенный набор таблиц по значению --type.
func builtinTableSource(kind string) *tableSource {
	dir, ok := builtinTableDirs[kind]
	if !ok {
		fatal("unknown built-in table set %q (expected one of: %s)",
			kind, strings.Join(builtinTypeNames, ", "))
	}
	sub, err := fs.Sub(bases.FS, dir)
	if err != nil {
		fatal("built-in table set %s is not embedded: %v", dir, err)
	}
	return &tableSource{
		fsys:   sub,
		label:  "builtin:" + kind,
		key:    "builtin:" + kind,
		origin: dir,
	}
}

// tableSource — выбор источника: --tables (внешние) или встроенный по --type.
func (a *Args) tableSource() *tableSource {
	if a.Tables != "" {
		return externalTableSource(a.Tables)
	}
	return builtinTableSource(a.Type)
}

func (s *tableSource) builtin() bool { return s.origin != "" }

// describe — строка для отчёта о выбранных таблицах.
func (s *tableSource) describe() string {
	if s.builtin() {
		return fmt.Sprintf("builtin %s (%s, gzip)",
			strings.TrimPrefix(s.label, "builtin:"), s.origin)
	}
	return "external directory " + s.label
}

// display — путь файла таблицы для сообщений об ошибках.
func (s *tableSource) display(name string) string { return path.Join(s.label, name) }

// resolveName — имя существующего файла таблицы в каталоге mdir:
// сначала data.txt.gz (встроенные), затем data.txt (внешние).
func (s *tableSource) resolveName(mdir string) (string, error) {
	names := []string{
		path.Join(mdir, tableFileName+".gz"),
		path.Join(mdir, tableFileName),
	}
	for _, name := range names {
		if _, err := fs.Stat(s.fsys, name); err == nil {
			return name, nil
		}
	}
	return "", fs.ErrNotExist
}

// readTable — полное содержимое таблицы (с распаковкой gzip при наличии)
// и его отображаемое имя.
func (s *tableSource) readTable(mdir string) (data []byte, disp string, err error) {
	name, err := s.resolveName(mdir)
	if err != nil {
		return nil, "", err
	}
	f, err := s.fsys.Open(name)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	var r io.Reader = f
	if strings.HasSuffix(name, ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return nil, "", err
		}
		defer zr.Close()
		r = zr
	}
	data, err = io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	return data, s.display(name), nil
}

// warnLargeStep — предупреждение о слишком крупном шаге по x; печатается
// один раз на источник (у агрегатов иначе десятки одинаковых строк).
func (s *tableSource) warnLargeStep(disp string, dxMax float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.warnedDx {
		return
	}
	s.warnedDx = true
	fmt.Fprintf(os.Stderr, "[warning] tables %s: large step in x (max Δx = %g, "+
		"e.g. %s); Mie ripple may not be resolved\n", s.label, dxMax, disp)
}

// ---------------- разбор таблиц ----------------

func tokenIndex(toks []string, name string) int {
	for i, t := range toks {
		if t == name {
			return i
		}
	}
	return -1
}

// mTableDirname — имя каталога таблицы для заданного m.
func mTableDirname(re, im float64) string {
	// нормализуем отрицательный ноль
	if re == 0 {
		re = 0
	}
	if im == 0 {
		im = 0
	}
	return fmt.Sprintf("mre_%.2f_mim_%.5f", re, im)
}

// parseMDirname — разбор имени mre_<real>_mim_<imag>.
func parseMDirname(name string) (re, im float64, ok bool) {
	if !strings.HasPrefix(name, "mre_") {
		return 0, 0, false
	}
	rest := name[4:]
	i := strings.Index(rest, "_mim_")
	if i < 0 {
		return 0, 0, false
	}
	r, err := strconv.ParseFloat(strings.TrimSpace(rest[:i]), 64)
	if err != nil {
		return 0, 0, false
	}
	m, err := strconv.ParseFloat(strings.TrimSpace(rest[i+5:]), 64)
	if err != nil {
		return 0, 0, false
	}
	return r, m, true
}

// logLogSpline — кубический сплайн ln y от ln x (все значения должны быть > 0).
func logLogSpline(xs, ys []float64) *spline {
	lx := make([]float64, len(xs))
	ly := make([]float64, len(ys))
	for i := range xs {
		lx[i] = math.Log(xs[i])
		ly[i] = math.Log(ys[i])
	}
	return newSpline(lx, ly)
}

func loadColumnSplines(t *table) {
	if t.hasCext {
		t.spCext = logLogSpline(t.x, t.cext)
	}
	if t.hasCback {
		t.spCback = logLogSpline(t.x, t.cback)
	}
	if t.hasS11 {
		t.spS11 = logLogSpline(t.x, t.s11)
	}
	if t.hasS22 {
		t.spS22 = logLogSpline(t.x, t.s22)
	}
}

// tableLoad — загрузка таблицы для m-каталога mdir из источника src.
func tableLoad(src *tableSource, mdir string, extrapolate bool) *table {
	data, disp, err := src.readTable(mdir)
	if err != nil {
		fatal("cross-section table file not found: %s (expected %s[.gz] — "+
			"mkbase/mkbase_spheres format)", src.display(path.Join(mdir, tableFileName)),
			tableFileName)
	}
	t, dxMax := parseTable(data, disp, extrapolate)
	if dxMax > 1.0 {
		src.warnLargeStep(disp, dxMax)
	}
	return t
}

// parseTable — разбор содержимого data.txt. Возвращает таблицу и максимальный
// шаг по x (для диагностики разрешения Mie-ряби).
func parseTable(data []byte, disp string, extrapolate bool) (*table, float64) {
	ics := make(map[string]int, len(tableColumnNames))
	ixs := make(map[string]int, len(tableColumnNames))
	for _, n := range tableColumnNames {
		ics[n], ixs[n] = -1, -1
	}
	scan := bufio.NewScanner(bytes.NewReader(data))
	scan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scan.Scan() {
		s := strings.TrimLeft(scan.Text(), " \t\r\n\v\f")
		if !strings.HasPrefix(s, "#") {
			continue
		}
		toks := strings.Fields(s[1:])
		if len(toks) == 0 || toks[0] != "x" {
			continue
		}
		ix := tokenIndex(toks, "x")
		for _, n := range tableColumnNames {
			if ics[n] >= 0 {
				continue
			}
			if ic := tokenIndex(toks, n); ic >= 0 {
				ixs[n], ics[n] = ix, ic
			}
		}
	}
	if err := scan.Err(); err != nil {
		fatal("%s: failed to read table: %v", disp, err)
	}
	if ics["Cext"] < 0 && ics["Cback"] < 0 {
		fatal("%s: mkbase-format header with columns 'x' and 'Cext' not found "+
			"(expected, for example, '# x Cext Cabs Cback Qext Qabs Qback S11 S22 n')",
			disp)
	}

	ix, need := -1, 0
	for _, n := range tableColumnNames {
		if ics[n] < 0 {
			continue
		}
		if ix < 0 {
			ix = ixs[n]
		} else if ixs[n] != ix {
			fatal("%s: columns Cext/Cback/S11/S22 come from headers with "+
				"different x positions — ambiguous table format", disp)
		}
		if ics[n]+1 > need {
			need = ics[n] + 1
		}
	}
	if ix+1 > need {
		need = ix + 1
	}

	xs := []float64{}
	cols := make(map[string][]float64, len(tableColumnNames))
	scan = bufio.NewScanner(bytes.NewReader(data))
	scan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scan.Scan() {
		s := strings.TrimLeft(scan.Text(), " \t\r\n\v\f")
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		s = strings.TrimRight(s, " \t\r\n\v\f")
		toks := strings.Fields(s)
		if len(toks) < need {
			fatal("%s: no required column in row: '%s'", disp, s)
		}
		xv, err := strconv.ParseFloat(toks[ix], 64)
		if err != nil {
			fatal("%s: failed to parse column x in row: '%s'", disp, s)
		}
		xs = append(xs, xv)
		for _, n := range tableColumnNames {
			if ics[n] < 0 {
				continue
			}
			v, err := strconv.ParseFloat(toks[ics[n]], 64)
			if err != nil {
				fatal("%s: failed to parse %s in row: '%s'", disp, n, s)
			}
			cols[n] = append(cols[n], v)
		}
	}
	if err := scan.Err(); err != nil {
		fatal("%s: failed to read table: %v", disp, err)
	}

	n := len(xs)
	if n < 3 {
		fatal("%s: insufficient points (>= 3), found %d", disp, n)
	}
	for i := 1; i < n; i++ {
		if !(xs[i] > xs[i-1]) {
			fatal("%s: x must be strictly increasing", disp)
		}
	}
	for _, nm := range []string{"Cext", "Cback"} {
		if ics[nm] < 0 {
			continue
		}
		for _, v := range cols[nm] {
			if !(v > 0) || !isFinite(v) {
				fatal("%s: %s cross sections must be > 0 and finite", disp, nm)
			}
		}
	}
	// S11/S22 необязательны и могут содержать nan (mkbase пишет nan, если
	// столбца s22 нет в mueller): тогда δ_l по этой таблице не считается.
	validS := func(nm string) bool {
		if ics[nm] < 0 {
			return false
		}
		for _, v := range cols[nm] {
			if !(v > 0) || !isFinite(v) {
				return false
			}
		}
		return true
	}
	dxMax := 0.0
	for i := 1; i < n; i++ {
		if d := xs[i] - xs[i-1]; d > dxMax {
			dxMax = d
		}
	}

	t := &table{
		n:           n,
		x:           xs,
		xMin:        xs[0],
		xMax:        xs[n-1],
		extrapolate: extrapolate,
		path:        disp,
	}
	if ics["Cext"] >= 0 {
		t.hasCext = true
		t.cext = cols["Cext"]
	}
	if ics["Cback"] >= 0 {
		t.hasCback = true
		t.cback = cols["Cback"]
	}
	if validS("S11") {
		t.hasS11 = true
		t.s11 = cols["S11"]
	}
	if validS("S22") {
		t.hasS22 = true
		t.s22 = cols["S22"]
	}
	loadColumnSplines(t)
	return t, dxMax
}

func tableCrossSection(t *table, useCback bool, x float64) float64 {
	if !t.extrapolate && (x < t.xMin || x > t.xMax) {
		fatal("size parameter x outside table range [%g, %g] — extend the table "+
			"range or enable --table-extrapolate (%s)", t.xMin, t.xMax, t.path)
	}
	sp := t.spCext
	if useCback {
		sp = t.spCback
	}
	if sp == nil {
		fatal("%s: required cross-section column is missing", t.path)
	}
	return math.Exp(sp.eval(math.Log(x)))
}

// ---------------- сетка m ----------------
type mval struct{ re, im float64 }

func collectAvailable(src *tableSource) ([]mval, error) {
	entries, err := fs.ReadDir(src.fsys, ".")
	if err != nil {
		return nil, err
	}
	vals := []mval{}
	for _, e := range entries {
		re, im, ok := parseMDirname(e.Name())
		if !ok {
			continue
		}
		if _, err := src.resolveName(e.Name()); err != nil {
			continue
		}
		dup := false
		for _, v := range vals {
			if v.re == re && v.im == im {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		vals = append(vals, mval{re, im})
	}
	sort.Slice(vals, func(i, j int) bool {
		if vals[i].re != vals[j].re {
			return vals[i].re < vals[j].re
		}
		return vals[i].im < vals[j].im
	})
	return vals, nil
}

func layoutHint(src *tableSource) string {
	if src.builtin() {
		return ""
	}
	entries, err := fs.ReadDir(src.fsys, ".")
	if err != nil {
		return ""
	}
	names := []string{}
	for _, e := range entries {
		name := e.Name()
		digitsOK := true
		if strings.HasPrefix(name, "shape") {
			if len(name) == 5 {
				digitsOK = false
			} else {
				for i := 5; i < len(name); i++ {
					if name[i] < '0' || name[i] > '9' {
						digitsOK = false
					}
				}
			}
		}
		match := name == "average" || (strings.HasPrefix(name, "shape") && digitsOK)
		if !match {
			continue
		}
		if len(names) < 64 && e.IsDir() {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return fmt.Sprintf("; %s contains %s — specify --tables %s/average "+
		"or --tables <root>/<shapeNN>", src.label, strings.Join(names, ", "), src.label)
}

func mValuesStr(v []mval) string {
	var b strings.Builder
	for i := 0; i < len(v) && i < 10; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%g%+gj", v[i].re, v[i].im)
	}
	if len(v) > 10 {
		b.WriteString(", ...")
	}
	return b.String()
}

// tableMGrid — все доступные значения m из каталога таблиц.
func tableMGrid(src *tableSource) (re, im []float64) {
	avail, err := collectAvailable(src)
	if err != nil {
		fatal("table directory not found: %s%s", src.label, layoutHint(src))
	}
	if len(avail) == 0 {
		fatal("table directory %s contains no subdirectories "+
			"mre_<real>_mim_<imag>/%s[.gz]%s", src.label, tableFileName,
			layoutHint(src))
	}
	re = make([]float64, len(avail))
	im = make([]float64, len(avail))
	for i, v := range avail {
		re[i] = v.re
		im[i] = v.im
	}
	return re, im
}

// ---------------- кэш загруженных таблиц ----------------
type tableCacheKey struct {
	src         string
	re, im      float64
	extrapolate bool
}

var tableCache = struct {
	sync.Mutex
	m map[tableCacheKey]*table
}{m: make(map[tableCacheKey]*table)}

func missingTableError(src *tableSource, mdir string, mre, mim float64) {
	dir := mTableDirname(mre, mim)
	msg := fmt.Sprintf("table directory not found: %s (expected %s/%s[.gz] — "+
		"mkbase/mkbase_spheres format)", src.display(mdir), dir, tableFileName)
	if src.builtin() {
		avail, err := collectAvailable(src)
		if err == nil && len(avail) > 0 {
			fatal("%s; available m in %s: %s", msg, src.label, mValuesStr(avail))
		}
		fatal("%s; built-in set %s is empty", msg, src.label)
	}
	if _, err := fs.Stat(src.fsys, "."); err != nil {
		fatal("%s%s", msg, layoutHint(src))
	}
	avail, _ := collectAvailable(src)
	if len(avail) > 0 {
		fatal("%s; available m in %s: %s", msg, src.label, mValuesStr(avail))
	}
	fatal("%s%s", msg, layoutHint(src))
}

func tableCacheGet(src *tableSource, re, im float64, extrapolate bool) *table {
	key := tableCacheKey{src.key, re, im, extrapolate}
	tableCache.Lock()
	if t, ok := tableCache.m[key]; ok {
		tableCache.Unlock()
		return t
	}
	tableCache.Unlock()

	mdir := mTableDirname(re, im)
	if _, err := src.resolveName(mdir); err != nil {
		missingTableError(src, mdir, re, im)
	}
	t := tableLoad(src, mdir, extrapolate)

	tableCache.Lock()
	tableCache.m[key] = t
	tableCache.Unlock()
	return t
}

// loadTableChannels — привязка базовых каналов к таблице для заданного m.
func loadTableChannels(src *tableSource, mre, mim float64, base []channel,
	extrapolate bool, refWl, cbackScale float64) *channels {
	tab := tableCacheGet(src, mre, mim, extrapolate)
	out := &channels{
		n:   len(base),
		ch:  make([]channel, len(base)),
		cap: math.Inf(1),
	}
	for i, ch := range base {
		ch.tab = tab
		ch.refWlUm = refWl
		if ch.kind == chBeta {
			ch.cbackScale = cbackScale
		} else {
			ch.cbackScale = 1.0
		}
		if ch.kind == chAlpha && !tab.hasCext {
			fatal("%s: mkbase-format header with columns 'x' and 'Cext' not found "+
				"(expected, for example, '# x Cext Cabs Cback Qext Qabs Qback S11 S22 n')",
				tab.path)
		}
		if ch.kind == chBeta && !tab.hasCback {
			fatal("%s: mkbase-format header with columns 'x' and 'Cback' not found "+
				"(expected, for example, '# x Cext Cabs Cback Qext Qabs Qback S11 S22 n')",
				tab.path)
		}
		out.ch[i] = ch
		capv := tab.xMax * ch.wlUm / (2.0 * csPi)
		if capv < out.cap {
			out.cap = capv
		}
	}
	return out
}
