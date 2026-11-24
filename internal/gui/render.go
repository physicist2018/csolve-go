package gui

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets
var assetsFS embed.FS

// indexHTML — главная страница с подставленным токеном сессии.
func (a *app) indexHTML() ([]byte, error) {
	raw, err := fs.ReadFile(assetsFS, "assets/index.html")
	if err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(raw, []byte("__CSOLVE_TOKEN__"), []byte(a.token)), nil
}

// assetsSub — встроенные статические ресурсы.
func assetsSub() (fs.FS, error) {
	return fs.Sub(assetsFS, "assets")
}

// securityHeaders добавляет заголовки безопасности.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self'; "+
				"img-src 'self' data:; connect-src 'self'; base-uri 'none'; "+
				"form-action 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
