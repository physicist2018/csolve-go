# Copyright (C) 2026 Шмирко Константин Александрович <kshmirko@dvo.ru>
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.
#
# SPDX-License-Identifier: GPL-3.0-or-later

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
