package main

import (
	"fmt"
	"os"

	"github.com/nats-io/nats.go"
)

func runPub(args []string) {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: nats-cli pub <subject> <message>")
		os.Exit(1)
	}
	subject, message := args[0], args[1]

	url := resolveNatsURL()
	nc, err := nats.Connect(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to %s: %v\n", url, err)
		os.Exit(1)
	}
	defer nc.Close()

	if err := nc.Publish(subject, []byte(message)); err != nil {
		fmt.Fprintf(os.Stderr, "failed to publish to %q: %v\n", subject, err)
		os.Exit(1)
	}

	if err := nc.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to flush: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Published [%s] : '%s'\n", subject, message)
}
