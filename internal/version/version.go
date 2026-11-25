// Package version хранит сведения о версии сборки.
package version

import "runtime/debug"

// Version проставляется при сборке:
//
//	go build -ldflags "-X csolve/internal/version.Version=v1.2.3"
var Version = ""

// Info — сведения о сборке для интерфейса и отчётов об ошибках.
type Info struct {
	Version  string `json:"version"`
	Commit   string `json:"commit,omitempty"`
	Modified bool   `json:"modified,omitempty"`
}

// Get собирает версию из Version, а если она не задана — из метаданных
// сборки (ревизия VCS и версия модуля, доступные при `go install` и при
// сборке внутри репозитория).
func Get() Info {
	info := Info{Version: Version}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		if info.Version == "" {
			info.Version = "dev"
		}
		return info
	}
	var rev string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	info.Commit = rev
	if info.Version == "" {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			info.Version = v
		} else {
			info.Version = "dev"
		}
	}
	return info
}

// String — короткое представление версии.
func String() string { return Get().Version }
