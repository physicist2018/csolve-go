package gui

import (
	"io"
	"net/http"

	"csolve"
)

// helpHandler — отдаёт README.md, встроенный в бинарник, для окна справки.
func (a *app) helpHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = io.WriteString(w, csolve.ReadmeMD)
}
