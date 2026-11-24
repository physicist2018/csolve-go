// Package gui — графический интерфейс csolve: локальный HTTP-сервер на
// 127.0.0.1, интерфейс в браузере (ресурсы встроены), запуск решателя
// дочерним процессом и показ результата. Без внешних зависимостей.
package gui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

// Run — запуск GUI; возвращает код возврата процесса. Собственные флаги:
// --port, --no-open, --help.
func Run(args []string) int {
	fs := flag.NewFlagSet("csolve --gui", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	port := fs.Int("port", 0, "TCP port on 127.0.0.1 (0 = random)")
	noOpen := fs.Bool("no-open", false, "do not open the browser")
	showHelp := fs.Bool("help", false, "show this help and exit")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "csolve --gui [--port N] [--no-open] [--help]\n\n"+
			"Starts a local web interface served by this binary.\n"+
			"  --port N     port on 127.0.0.1 (default: random)\n"+
			"  --no-open    print the URL instead of opening the browser\n"+
			"  --help       show this help and exit\n")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showHelp {
		fs.Usage()
		return 0
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "error: unexpected argument: %s\n", fs.Arg(0))
		return 2
	}

	app, err := newApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer app.Close()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot listen: %v\n", err)
		return 1
	}
	url := fmt.Sprintf("http://%s/", ln.Addr().String())

	srv := &http.Server{
		Handler:           app.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	app.url = url

	fmt.Fprintf(os.Stderr, "csolve GUI: %s\n", url)
	if !*noOpen {
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(os.Stderr, "could not open the browser (%v); open the URL above manually.\n", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
	case <-app.quit:
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "error: server: %v\n", err)
			return 1
		}
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	return 0
}

// newToken — случайный токен сессии.
func newToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// практически не случается; запасной вариант — время
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// openBrowser открывает URL в браузере по умолчанию и не ждёт закрытия.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// app — состояние сервера GUI.
type app struct {
	token    string
	tempRoot string
	url      string
	quit     chan struct{}
	manager  *manager
}

func newApp() (*app, error) {
	tmp, err := os.MkdirTemp("", "csolve-gui-*")
	if err != nil {
		return nil, fmt.Errorf("cannot create temp dir: %w", err)
	}
	return &app{
		token:    newToken(),
		tempRoot: tmp,
		quit:     make(chan struct{}),
		manager:  newManager(tmp),
	}, nil
}

// Close освобождает временные ресурсы.
func (a *app) Close() {
	_ = os.RemoveAll(a.tempRoot)
}

// baseDir — корень, из которого резолвятся относительные пути (текущий каталог
// на момент запуска, чтобы относительные пути в форме вели себя как в CLI).
func baseDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Clean(wd)
}
