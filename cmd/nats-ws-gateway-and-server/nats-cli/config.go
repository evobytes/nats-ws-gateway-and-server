package main

import (
	"fmt"
	"os"
)

// resolveNatsURL follows the same env var conventions as the other cmds in
// this repo: NATS_URL wins outright, otherwise fall back to NATS_BIND +
// NATS_PORT (see cmd/nats-ws-gateway-and-server-logger/main.go).
func resolveNatsURL() string {
	if url := os.Getenv("NATS_URL"); url != "" {
		return url
	}

	natsHost := os.Getenv("NATS_BIND")
	if natsHost == "" {
		natsHost = "127.0.0.1"
	}

	natsPort := os.Getenv("NATS_PORT")
	if natsPort == "" {
		natsPort = "5050"
	}

	return fmt.Sprintf("nats://%s:%s", natsHost, natsPort)
}
