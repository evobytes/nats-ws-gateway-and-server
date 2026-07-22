// nats-cli is a minimal replacement for the standard `nats` CLI's pub/sub
// commands, bundled alongside nats-ws-gateway-and-server and configured via
// the same NATS_URL / NATS_BIND / NATS_PORT env vars.
package main

import (
	"fmt"
	"os"
)

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  nats-cli pub <subject> <message>
  nats-cli sub <subject>`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "pub":
		runPub(os.Args[2:])
	case "sub":
		runSub(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}
