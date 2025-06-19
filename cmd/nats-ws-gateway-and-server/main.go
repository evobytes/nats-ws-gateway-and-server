package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type customLogger struct {
	l *log.Logger
}

func (cl *customLogger) Noticef(f string, a ...interface{}) { cl.l.Printf("[NOTICE] "+f, a...) }
func (cl *customLogger) Warnf(f string, a ...interface{})   { cl.l.Printf("[WARN] "+f, a...) }
func (cl *customLogger) Fatalf(f string, a ...interface{})  { cl.l.Printf("[FATAL] "+f, a...) }
func (cl *customLogger) Errorf(f string, a ...interface{})  { cl.l.Printf("[ERROR] "+f, a...) }
func (cl *customLogger) Debugf(f string, a ...interface{})  { cl.l.Printf("[DEBUG] "+f, a...) }
func (cl *customLogger) Tracef(f string, a ...interface{})  { cl.l.Printf("[TRACE] "+f, a...) }

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// NATS server setup
	opts := &natsserver.Options{
		Host:           "127.0.0.1",
		Port:           5050,
		NoLog:          false,
		NoSigs:         true,
		MaxControlLine: 256,
		JetStream:      false,
		Trace:          false,
		Debug:          false,
	}

	ns, err := natsserver.NewServer(opts)
	if err != nil {
		log.Fatalf("❌ Failed to create NATS server: %v", err)
	}

	ns.SetLoggerV2(&customLogger{l: log.New(os.Stdout, "nats-server: ", log.LstdFlags)}, true, false, false)

	// Start the embedded NATS server in a goroutine
	go func() {
		log.Printf("🚀 Starting embedded NATS server on %s:%d", opts.Host, opts.Port)
		ns.Start()
	}()

	// Wait for it to be ready
	for i := 0; i < 50; i++ {
		if ns.ReadyForConnections(1 * time.Second) {
			break
		}
		log.Printf("Waiting for NATS to become connectable... (%d)", i)
		if i == 49 {
			log.Fatal("❌ NATS server didn't become ready in time")
		}
	}

	log.Printf("✅ Connected to NATS server")

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		log.Fatalf("❌ Unable to connect to embedded NATS server: %v", err)
	}
	defer nc.Drain()

	// WebSocket setup
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 1024, 1024)
		if err != nil {
			log.Printf("❌ WebSocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Printf("❌ WebSocket read error: %v", err)
				return
			}

			// Publish to NATS
			if err := nc.Publish("chat", msg); err != nil {
				log.Printf("❌ NATS publish failed: %v", err)
				return
			}
		}
	})

	// Graceful shutdown
	srv := &http.Server{Addr: ":8080"}
	go func() {
		log.Printf("🌐 WebSocket server listening on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ HTTP server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Printf("🛑 Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("❌ HTTP shutdown failed: %v", err)
	}

	nc.Drain()
	ns.Shutdown()
	log.Printf("✅ Exit complete")
}
