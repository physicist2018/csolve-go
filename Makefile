GO ?= go
GOLANGCI_LINT ?= golangci-lint

# Версия попадает в бинарник: GUI показывает её в отчёте об ошибке.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X csolve/internal/version.Version=$(VERSION)

ifeq ($(OS),Windows_NT)
BIN := csolve.exe
else
BIN := csolve
endif
WIN_BIN := csolve.exe

.PHONY: all build build-windows test fmt vet lint generate clean

all: build

build:
	$(GO) build -trimpath -ldflags='$(LDFLAGS)' -o $(BIN) ./cmd/csolve

# Кросс-сборка под Windows x86_64 (с macOS/Linux): make build-windows
build-windows:
	GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags='$(LDFLAGS)' -o $(WIN_BIN) ./cmd/csolve

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint:
	$(GOLANGCI_LINT) run

# Пересобрать сжатые таблицы для встроенных наборов (bases/*/average).
# Требуется после изменения bases/*/average/mre_*/data.txt.
generate:
	$(GO) generate ./bases

clean:
	rm -f $(BIN) $(WIN_BIN)
