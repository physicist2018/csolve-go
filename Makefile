GO ?= go

ifeq ($(OS),Windows_NT)
BIN := csolve.exe
else
BIN := csolve
endif
WIN_BIN := csolve.exe

.PHONY: all build build-windows test fmt vet clean

all: build

build:
	$(GO) build -trimpath -o $(BIN) ./cmd/csolve

# Кросс-сборка под Windows x86_64 (с macOS/Linux): make build-windows
build-windows:
	GOOS=windows GOARCH=amd64 $(GO) build -trimpath -o $(WIN_BIN) ./cmd/csolve

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

clean:
	rm -f $(BIN) $(WIN_BIN)
