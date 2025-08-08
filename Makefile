SHELL := /bin/bash

# Find all subdirectories in cmd/ to determine the list of commands to build
CMDS := $(notdir $(wildcard cmd/*))

GOOS ?= linux
GOARCH ?= amd64

BINDIR := bin/$(GOOS)-$(GOARCH)

usage:
	@echo Usage $(CMDS)
	@echo
	@echo "make fmt"
	@echo "make build"
	@echo "make run cmd=<cmd-name>"

fmt:
	go fmt ./...

build: fmt
	-mkdir -p $(BINDIR)
	@if [ -z "$(APP_NAME)" ]; then \
		echo "Building all commands..."; \
		for cmd in $(CMDS); do \
			echo "Building $$cmd..."; \
			go build -C cmd/$$cmd -ldflags "-s -w" -o ../../$(BINDIR)/$$cmd .; \
		done \
	else \
		echo "Building $(APP_NAME)..."; \
		go build -C cmd/$(APP_NAME) -ldflags "-s -w" -o ../../$(BINDIR)/$(APP_NAME) .; \
	fi
	dir -hl $(BINDIR)

run: fmt
ifndef cmd
	@echo "Error: To run a command, you must specify 'cmd=<command-name>'"
	@exit 1
endif
	@if [ ! -d "cmd/$(cmd)" ]; then \
		echo "Error: Command 'cmd/$(cmd)' does not exist."; \
		exit 1; \
	fi
	go run -C cmd/$(cmd) .
