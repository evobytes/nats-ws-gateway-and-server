SHELL := /bin/bash

cmd := nats-ws-gateway-and-server

usage:
	@echo Usage $(cmd)
	@echo
	@echo make fmt
	@echo make build

fmt:
	go fmt ./...

build: fmt
	-mkdir -p bin
	go build -C cmd/$(cmd) -ldflags "-s -w" -o ../../bin/ .
	dir -l bin

run: fmt
	go run -C cmd/$(cmd) .

