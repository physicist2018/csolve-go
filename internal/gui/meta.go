package gui

import (
	"net/http"
	"runtime"

	"csolve/internal/version"
)

// Адрес трекера. Держим его на сервере, чтобы не дублировать в JS и не
// расходиться с репозиторием.
const repoURL = "https://github.com/physicist2018/csolve-go"

// metaHandler — сведения о сборке и ссылки для кнопки «Сообщить об ошибке».
func (a *app) metaHandler(w http.ResponseWriter, r *http.Request) {
	v := version.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":  v.Version,
		"commit":   v.Commit,
		"modified": v.Modified,
		"go":       runtime.Version(),
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
		"repo":     repoURL,
		"issues":   repoURL + "/issues/new",
	})
}
