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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Статусы задачи.
const (
	statusRunning   = "running"
	statusDone      = "done"
	statusFailed    = "failed"
	statusCancelled = "cancelled"
)

// logLine — строка лога решателя.
type logLine struct {
	Seq    int    `json:"seq"`
	Stream string `json:"stream"` // out | err
	Text   string `json:"text"`
}

// fileItem — файл результата во временном каталоге.
type fileItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	Kind string `json:"kind"` // image | json | text | doc | pdf
}

// job — одна задача запуска решателя.
type job struct {
	mu       sync.Mutex
	ID       string
	Req      *Request
	Argv     []string
	cmd      *exec.Cmd
	Dir      string
	Status   string
	ExitCode int
	ErrText  string
	Started  time.Time
	Duration time.Duration
	Logs     []logLine
	seq      int
	Subs     map[chan eventMsg]struct{}
	Files    []fileItem
	Batch    BatchSummary
}

// BatchSummary — столбцы сводных файлов пакетного режима.
type BatchSummary struct {
	Alt     []float64            `json:"alt"`
	Tables  map[string][]float64 `json:"tables"`
	Columns map[string][]string  `json:"columns"` // заголовки столбцов (без H) из файла
	Notes   map[string]string    `json:"notes"`   // длина волны/единицы из строк «# ...»
}

// eventMsg — событие задачи для SSE.
type eventMsg struct {
	Seq        int    `json:"seq"`
	Type       string `json:"type"` // log | status
	Stream     string `json:"stream,omitempty"`
	Text       string `json:"text,omitempty"`
	Status     string `json:"status,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	HasFiles   bool   `json:"has_files,omitempty"`
}

// manager — реестр задач.
type manager struct {
	mu         sync.Mutex
	tempRoot   string
	jobs       map[string]*job
	next       int
	solverPath string   // по умолчанию os.Executable
	solverEnv  []string // доп. переменные окружения для решателя (тесты)
}

func newManager(tempRoot string) *manager {
	return &manager{tempRoot: tempRoot, jobs: map[string]*job{}}
}

func (m *manager) get(id string) *job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs[id]
}

// start — построить argv, создать каталог и запустить решателя.
func (m *manager) start(req *Request) (*job, error) {
	m.mu.Lock()
	id := fmt.Sprintf("job-%d", m.next)
	m.next++
	m.mu.Unlock()

	dir := filepath.Join(m.tempRoot, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("cannot create job dir: %w", err)
	}
	argv, _, err := buildArgv(req, dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	exe := m.solverPath
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("cannot locate the executable: %w", err)
		}
	}

	cmd := exec.Command(exe, argv...)
	if len(m.solverEnv) > 0 {
		cmd.Env = append(os.Environ(), m.solverEnv...)
	}
	hideWindow(cmd)

	j := &job{
		ID:      id,
		Req:     req,
		Argv:    argv,
		Dir:     dir,
		Status:  statusRunning,
		Started: time.Now(),
		Subs:    map[chan eventMsg]struct{}{},
	}
	cmd.Stdout = &lineWriter{j: j, stream: "out"}
	cmd.Stderr = &lineWriter{j: j, stream: "err"}
	j.cmd = cmd
	m.mu.Lock()
	m.jobs[id] = j
	m.mu.Unlock()

	go func() {
		if err := j.cmd.Start(); err != nil {
			j.failStart(err.Error())
			return
		}
		runErr := j.cmd.Wait()
		j.flushLog()
		j.afterWait(runErr)
	}()
	return j, nil
}

func (j *job) failStart(errText string) {
	j.mu.Lock()
	j.Status = statusFailed
	j.ErrText = errText
	j.Duration = time.Since(j.Started)
	subs := snapshotSubs(j)
	j.mu.Unlock()
	publish(subs, eventMsg{Type: "status", Status: statusFailed,
		Error: errText, DurationMs: j.Duration.Milliseconds()})
}

func (j *job) afterWait(runErr error) {
	j.mu.Lock()
	wasCancelled := j.Status == statusCancelled
	code := -1
	if j.cmd.ProcessState != nil {
		code = j.cmd.ProcessState.ExitCode()
	}
	if wasCancelled {
		runErr = nil
	}
	st := statusDone
	if wasCancelled {
		st = statusCancelled
	} else if runErr != nil {
		st = statusFailed
	}
	j.ExitCode = code
	j.Status = st
	j.ErrText = exitErrString(runErr)
	j.Duration = time.Since(j.Started)
	scan := st == statusDone
	if scan {
		j.Files = scanFiles(j.Dir)
		if j.Req.Mode == "batch" {
			j.Batch = parseBatchSummary(j.Dir)
		}
	}
	subs := snapshotSubs(j)
	j.mu.Unlock()
	publish(subs, eventMsg{
		Type:       "status",
		Status:     st,
		ExitCode:   j.ExitCode,
		Error:      j.ErrText,
		DurationMs: j.Duration.Milliseconds(),
		HasFiles:   scan,
	})
}

// lineWriter — пишет вывод решателя в лог задачи, разбивая на строки.
type lineWriter struct {
	j      *job
	stream string
	buf    []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		line = strings.TrimSuffix(line, "\r")
		w.j.appendLog(w.stream, line)
	}
	return len(p), nil
}

// flushLog — выводит остаток без завершающего перевода строки.
func (j *job) flushLog() {
	if w := j.stdoutLineWriter(); w != nil {
		if len(w.buf) > 0 {
			j.appendLog(w.stream, string(w.buf))
			w.buf = nil
		}
	}
}

func (j *job) stdoutLineWriter() *lineWriter {
	if w, ok := j.cmd.Stdout.(*lineWriter); ok {
		return w
	}
	return nil
}

func (j *job) appendLog(stream, text string) {
	j.mu.Lock()
	j.seq++
	ll := logLine{Seq: j.seq, Stream: stream, Text: text}
	j.Logs = append(j.Logs, ll)
	subs := snapshotSubs(j)
	j.mu.Unlock()
	publish(subs, eventMsg{Seq: ll.Seq, Type: "log", Stream: stream, Text: text})
}

func (j *job) cancel() {
	j.mu.Lock()
	if j.Status == statusRunning {
		j.Status = statusCancelled
		if j.cmd != nil && j.cmd.Process != nil {
			_ = j.cmd.Process.Kill()
		}
	}
	j.mu.Unlock()
}

func (j *job) subscribe() (chan eventMsg, func()) {
	c := make(chan eventMsg, 256)
	j.mu.Lock()
	j.Subs[c] = struct{}{}
	j.mu.Unlock()
	done := func() {
		j.mu.Lock()
		delete(j.Subs, c)
		j.mu.Unlock()
	}
	return c, done
}

// snapshotSubs возвращает копию подписчиков; вызывающий должен держать j.mu.
func snapshotSubs(j *job) []chan eventMsg {
	out := make([]chan eventMsg, 0, len(j.Subs))
	for c := range j.Subs {
		out = append(out, c)
	}
	return out
}

func publish(subs []chan eventMsg, e eventMsg) {
	for _, c := range subs {
		select {
		case c <- e:
		default:
		}
	}
}

func exitErrString(err error) string {
	if err == nil {
		return ""
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.Error()
	}
	return err.Error()
}

func scanFiles(dir string) []fileItem {
	var items []fileItem
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}
		items = append(items, fileItem{
			Name: info.Name(),
			Path: p,
			Size: info.Size(),
			Kind: fileKind(info.Name()),
		})
		return nil
	})
	sort.Slice(items, func(i, k int) bool { return items[i].Name < items[k].Name })
	return items
}

func fileKind(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".svg":
		return "image"
	case ".json":
		return "json"
	case ".pdf":
		return "pdf"
	case ".docx", ".odt":
		return "doc"
	default:
		return "text"
	}
}

// parseBatchSummary — читает сводные TXT-файлы пакетного режима, включая
// их шапки («# H col1 col2 ...») и строки-примечания с единицами.
func parseBatchSummary(dir string) BatchSummary {
	bs := BatchSummary{
		Tables:  map[string][]float64{},
		Columns: map[string][]string{},
		Notes:   map[string]string{},
	}
	order := []string{"moments", "refractive", "errors", "delpol", "lidar_ratios"}

	files := make(map[string]summaryFile, len(order))
	for _, name := range order {
		files[name] = readSummaryFile(filepath.Join(dir, name+".txt"))
	}

	// Alt — по первому файлу с данными (H — первый столбец во всех).
	for _, name := range order {
		rows := files[name].rows
		if len(rows) == 0 {
			continue
		}
		for _, r := range rows {
			bs.Alt = append(bs.Alt, r[0])
		}
		break
	}
	if len(bs.Alt) == 0 {
		return bs
	}

	for _, name := range order {
		f := files[name]
		if len(f.rows) == 0 {
			continue
		}
		ncols := 0
		for _, r := range f.rows {
			if len(r)-1 > ncols {
				ncols = len(r) - 1
			}
		}
		if ncols == 0 {
			continue // только столбец H, данных нет
		}
		if len(f.cols) > 0 {
			bs.Columns[name] = f.cols
		}
		if f.note != "" {
			bs.Notes[name] = f.note
		}
		series := make([]float64, len(f.rows)*ncols)
		for i, r := range f.rows {
			base := i * ncols
			for c := 1; c < len(r); c++ {
				series[base+c-1] = r[c]
			}
		}
		bs.Tables[name] = series
	}
	return bs
}

// summaryFile — разобранный сводный файл.
type summaryFile struct {
	rows [][]float64
	cols []string
	note string
}

// readSummaryFile читает файл: строки «# H ...» — заголовки столбцов,
// остальные «# ...» — примечания; числа — данные.
func readSummaryFile(path string) summaryFile {
	data, err := os.ReadFile(path)
	if err != nil {
		return summaryFile{}
	}
	var out summaryFile
	var notes []string
	headerSeen := false
	for _, ln := range strings.Split(string(data), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if strings.HasPrefix(ln, "#") {
			body := strings.TrimSpace(strings.TrimPrefix(ln, "#"))
			if body == "" {
				continue
			}
			fields := strings.Fields(body)
			if !headerSeen && len(fields) > 0 && fields[0] == "H" {
				out.cols = append([]string(nil), fields[1:]...)
				headerSeen = true
				continue
			}
			notes = append(notes, body)
			continue
		}
		var row []float64
		for _, tok := range strings.Fields(ln) {
			if f, err := strconv.ParseFloat(tok, 64); err == nil {
				row = append(row, f)
			}
		}
		if len(row) > 0 {
			out.rows = append(out.rows, row)
		}
	}
	out.note = strings.Join(notes, " ")
	return out
}
