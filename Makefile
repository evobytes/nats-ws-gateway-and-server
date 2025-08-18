SHELL := /bin/bash

# Find all subdirectories in cmd/ to determine the list of commands to build
CMDS := $(notdir $(wildcard cmd/*))

# Build matrix inputs (overridable from workflow env)
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0

# Where artifacts land
BINDIR := bin/$(GOOS)-$(GOARCH)

# Windows suffix
SUFFIX :=
ifeq ($(GOOS),windows)
SUFFIX := .exe
endif

# Versioning (overridable from workflow: export VERSION=1.2.3)
# If VERSION not provided, try git; fall back to "dev"
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Package path where you expose vars (adjust if not "main")
# e.g. in Go:  var version, commit, date string
PKG ?= main

LDFLAGS := -s -w -buildid= \
	-X '$(PKG).version=$(VERSION)' \
	-X '$(PKG).commit=$(COMMIT)' \
	-X '$(PKG).date=$(DATE)'

GOFLAGS ?=
# Example: GOFLAGS += -tags netgo

usage:
	@echo Usage $(CMDS)
	@echo
	@echo "make fmt"
	@echo "make build               # builds all or APP_NAME=<name>"
	@echo "make run cmd=<cmd-name>  # runs cmd/<name>"
	@echo "make print-vars"

fmt:
	go fmt ./...

print-vars:
	@echo GOOS=$(GOOS)
	@echo GOARCH=$(GOARCH)
	@echo CGO_ENABLED=$(CGO_ENABLED)
	@echo BINDIR=$(BINDIR)
	@echo VERSION=$(VERSION)
	@echo COMMIT=$(COMMIT)
	@echo DATE=$(DATE)
	@echo APP_NAME=$(APP_NAME)
	@echo CMDS=$(CMDS)

build: fmt
	@mkdir -p "$(BINDIR)"
ifeq ($(strip $(APP_NAME)),)
	@echo "Building all commands for $(GOOS)/$(GOARCH) (CGO_ENABLED=$(CGO_ENABLED))..."
	@set -e; for cmd in $(CMDS); do \
		[ -d "cmd/$$cmd" ] || continue; \
		echo "→ $$cmd"; \
		GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
		  go build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" \
		  -o "$(BINDIR)/$$cmd$(SUFFIX)" ./cmd/$$cmd; \
	done
else
	@[ -d "cmd/$(APP_NAME)" ] || { echo "Error: cmd/$(APP_NAME) does not exist"; exit 1; }
	@echo "Building $(APP_NAME) for $(GOOS)/$(GOARCH) (CGO_ENABLED=$(CGO_ENABLED))..."
	@GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
	  go build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" \
	  -o "$(BINDIR)/$(APP_NAME)$(SUFFIX)" ./cmd/$(APP_NAME)
endif
	@ls -hl "$(BINDIR)" || true

run: fmt
ifndef cmd
	@echo "Error: To run a command, specify:  make run cmd=<command-name>"; exit 1
endif
	@[ -d "cmd/$(cmd)" ] || { echo "Error: cmd/$(cmd) does not exist."; exit 1; }
	go run -C cmd/$(cmd) .
