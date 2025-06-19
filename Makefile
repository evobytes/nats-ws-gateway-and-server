SHELL := /bin/bash

cmd := $(shell basename `pwd`)
bin := bin/$(cmd)

usage:
	@echo Usage $(cmd)
	@echo
	@echo make fmt
	@echo make build

fmt:
	go fmt ./...

build: fmt
	mkdir -p bin
	go build -C cmd/$(cmd) -o ../../$(bin) .
	ls -lh $(bin)

run: fmt
	go run -C cmd/$(cmd) .

