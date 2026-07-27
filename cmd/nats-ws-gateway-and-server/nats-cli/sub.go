package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
)

func runSub(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: nats-cli sub <subject>")
		os.Exit(1)
	}
	subject := args[0]

	url := resolveNatsURL()
	nc, err := nats.Connect(url, nats.MaxReconnects(-1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to %s: %v\n", url, err)
		os.Exit(1)
	}
	defer nc.Close()

	count := 0
	sub, err := nc.Subscribe(subject, func(m *nats.Msg) {
		count++
		fmt.Printf("[%s] [#%d] Received on %q: %s\n", time.Now().Format("15:04:05"), count, m.Subject, string(m.Data))
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to subscribe to %q: %v\n", subject, err)
		os.Exit(1)
	}
	defer sub.Unsubscribe()

	fmt.Printf("Listening on %q [%s]\n", subject, url)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	nc.Drain()
}
