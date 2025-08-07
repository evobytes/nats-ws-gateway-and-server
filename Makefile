SHELL := /bin/bash

cmd := nats-ws-gateway-and-server

BINDIR := bin/$(GOOS)-$(GOARCH)

usage:
	@echo Usage $(cmd)
	@echo
	@echo make fmt
	@echo make build

fmt:
	go fmt ./...

build: fmt
	-mkdir -p $(BINDIR)
	go build -C cmd/$(cmd) -ldflags "-s -w" -o ../../$(BINDIR)/ .
	dir -l $(BINDIR)

run: fmt
	go run -C cmd/$(cmd) .

