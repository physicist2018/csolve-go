package gui

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Handler собирает маршруты GUI.
func (a *app) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", a.serveIndex)
	if sub, err := assetsSub(); err == nil {
		mux.Handle("GET /assets/", http.StripPrefix("/assets/",
			http.FileServer(http.FS(sub))))
	}
	mux.HandleFunc("GET /api/spec", a.specHandler)
	mux.HandleFunc("GET /api/help", a.helpHandler)
	mux.HandleFunc("POST /api/validate", a.validateHandler)
	mux.HandleFunc("POST /api/jobs", a.jobsHandler)
	mux.HandleFunc("GET /api/jobs/{id}/events", a.eventsHandler)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", a.cancelHandler)
	mux.HandleFunc("GET /api/jobs/{id}/result", a.resultHandler)
	mux.HandleFunc("GET /api/jobs/{id}/files", a.filesHandler)
	mux.HandleFunc("GET /api/jobs/{id}/files/{name}", a.fileHandler)
	mux.HandleFunc("POST /api/quit", a.quitHandler)
	mux.HandleFunc("GET /api/fs", a.fsHandler)
	mux.HandleFunc("POST /api/upload", a.uploadHandler)
	return securityHeaders(a.auth(mux))
}

func (a *app) serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	html, err := a.indexHTML()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "csolve_token", Value: a.token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(html)
}

// auth — защита API токеном и проверка Origin.
func (a *app) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-CSolve-Token") != a.token {
			if c, err := r.Cookie("csolve_token"); err != nil || c.Value != a.token {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			base := strings.TrimRight(a.url, "/")
			if origin != base {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// writeJSON — JSON-ответ.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeReq(w http.ResponseWriter, r *http.Request) (*Request, bool) {
	var req Request
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return nil, false
	}
	return &req, true
}

// specHandler — метаданные формы.
func (a *app) specHandler(w http.ResponseWriter, r *http.Request) {
	defs := defaultValues()
	// возвращаем Default каждого поля из defs
	flds := make([]Field, 0, len(fields))
	for _, f := range fields {
		if d, ok := defs[f.Key]; ok {
			f.Default = d
		}
		flds = append(flds, f)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":       1,
		"groups":        groups,
		"fields":        flds,
		"presets":       presetList,
		"table_types":   tableTypes(),
		"default_type":  defaultTableType(),
		"default_table": "agglomerate",
	})
}

// specForValidate — тип ответа валидации.
type validateResp struct {
	Ok      bool     `json:"ok"`
	Argv    []string `json:"argv,omitempty"`
	Command string   `json:"command,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func (a *app) validateHandler(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeReq(w, r)
	if !ok {
		return
	}
	argv, _, err := buildArgv(req, filepath.Join(a.tempRoot, "_preview"))
	resp := validateResp{Ok: err == nil}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Argv = argv
		resp.Command = "csolve " + shellJoin(argv)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *app) jobsHandler(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeReq(w, r)
	if !ok {
		return
	}
	j, err := a.manager.start(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": j.ID, "status": j.Status})
}

func (a *app) cancelHandler(w http.ResponseWriter, r *http.Request) {
	j := a.manager.get(r.PathValue("id"))
	if j == nil {
		http.NotFound(w, r)
		return
	}
	j.cancel()
	writeJSON(w, http.StatusOK, map[string]any{"id": j.ID, "status": statusCancelled})
}

func (a *app) eventsHandler(w http.ResponseWriter, r *http.Request) {
	j := a.manager.get(r.PathValue("id"))
	if j == nil {
		http.NotFound(w, r)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	// отправляем историю и начальный статус
	j.mu.Lock()
	for _, ll := range j.Logs {
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(eventMsg{
			Seq: ll.Seq, Type: "log", Stream: ll.Stream, Text: ll.Text}))
	}
	st := j.Status
	j.mu.Unlock()
	if st != statusRunning {
		j.emitStatusSnapshot(w)
	}
	fl.Flush()

	sub, done := j.subscribe()
	defer done()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-sub:
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(e))
			fl.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": keepalive\n\n")
			fl.Flush()
		}
	}
}

func (j *job) emitStatusSnapshot(w http.ResponseWriter) {
	j.mu.Lock()
	e := eventMsg{Type: "status", Status: j.Status, ExitCode: j.ExitCode,
		Error: j.ErrText, DurationMs: j.Duration.Milliseconds()}
	j.mu.Unlock()
	fmt.Fprintf(w, "data: %s\n\n", mustJSON(e))
}

func (a *app) resultHandler(w http.ResponseWriter, r *http.Request) {
	j := a.manager.get(r.PathValue("id"))
	if j == nil {
		http.NotFound(w, r)
		return
	}
	if j.Req.Mode == "batch" {
		writeJSON(w, http.StatusOK, j.Batch)
		return
	}
	data, err := os.ReadFile(filepath.Join(j.Dir, "result.json"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": "no result json"})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(data)
}

func (a *app) filesHandler(w http.ResponseWriter, r *http.Request) {
	j := a.manager.get(r.PathValue("id"))
	if j == nil {
		http.NotFound(w, r)
		return
	}
	j.mu.Lock()
	list := make([]map[string]any, 0, len(j.Files))
	for _, f := range j.Files {
		list = append(list, map[string]any{
			"name": f.Name, "size": f.Size, "kind": f.Kind,
			"url": "/api/jobs/" + j.ID + "/files/" + url.PathEscape(f.Name),
		})
	}
	j.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"files": list})
}

func (a *app) fileHandler(w http.ResponseWriter, r *http.Request) {
	j := a.manager.get(r.PathValue("id"))
	if j == nil {
		http.NotFound(w, r)
		return
	}
	name, err := url.PathUnescape(r.PathValue("name"))
	if err != nil {
		http.Error(w, "bad name", http.StatusBadRequest)
		return
	}
	// защита от выхода из каталога задачи
	clean, err := filepath.Rel(j.Dir, filepath.Join(j.Dir, name))
	if err != nil || strings.HasPrefix(clean, "..") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	p := filepath.Join(j.Dir, filepath.Clean(name))
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		http.NotFound(w, r)
		return
	}
	ctype := mime.TypeByExtension(filepath.Ext(name))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	kind := fileKind(name)
	if kind == "image" {
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(name)+"\"")
	}
	http.ServeFile(w, r, p)
}

func (a *app) quitHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	select {
	case <-a.quit:
	default:
		close(a.quit)
	}
}

// fsHandler — обозреватель каталогов.
func (a *app) fsHandler(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		p = baseDir()
	}
	p = filepath.Clean(p)
	entries, err := os.ReadDir(p)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"path": p, "error": err.Error()})
		return
	}
	type ent struct {
		Name string `json:"name"`
		Dir  bool   `json:"dir"`
		Size int64  `json:"size"`
		Path string `json:"path"`
	}
	list := make([]ent, 0, len(entries))
	for _, e := range entries {
		info, ierr := e.Info()
		sz := int64(0)
		if ierr == nil {
			sz = info.Size()
		}
		list = append(list, ent{Name: e.Name(), Dir: e.IsDir(), Size: sz,
			Path: filepath.Join(p, e.Name())})
	}
	sort.Slice(list, func(i, k int) bool {
		if list[i].Dir != list[k].Dir {
			return list[i].Dir
		}
		return list[i].Name < list[k].Name
	})
	writeJSON(w, http.StatusOK, map[string]any{"path": p, "entries": list})
}

// uploadHandler — загрузка файлов (drag-and-drop / выбор). Сохраняет в temp.
func (a *app) uploadHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "upload too large"})
		return
	}
	upDir := filepath.Join(a.tempRoot, "uploads")
	if err := os.MkdirAll(upDir, 0700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "cannot create upload dir"})
		return
	}
	var saved []string
	for field, files := range r.MultipartForm.File {
		for _, fh := range files {
			rel := filepath.Clean(fh.Filename)
			if rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
				continue
			}
			if field == "files" {
				rel = filepath.Base(rel)
			}
			dst := filepath.Join(upDir, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
				continue
			}
			in, err := fh.Open()
			if err != nil {
				continue
			}
			out, err := os.Create(dst)
			if err != nil {
				in.Close()
				continue
			}
			_, cerr := io.Copy(out, in)
			in.Close()
			out.Close()
			if cerr == nil {
				saved = append(saved, dst)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"paths": saved})
}

// shellJoin — экранирование argv для командной строки.
func shellJoin(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\n\"'\\$`") {
			q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		} else {
			q[i] = a
		}
	}
	return strings.Join(q, " ")
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"encode"}`
	}
	return string(b)
}
