package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// log.Printf("🔍 WS Origin: %q", r.Header.Get("Origin"))
		return true // 🚨 ALLOW ALL ORIGINS - safe for dev only
	},
}

type customLogger struct{}

func (cl *customLogger) Noticef(f string, a ...interface{}) {
	slog.Info(fmt.Sprintf(f, a...), "component", "nats", "level", "notice")
}

func (cl *customLogger) Warnf(f string, a ...interface{}) {
	slog.Warn(fmt.Sprintf(f, a...), "component", "nats")
}

func (cl *customLogger) Fatalf(f string, a ...interface{}) {
	slog.Error(fmt.Sprintf(f, a...), "component", "nats", "fatal", true)
	os.Exit(1) // Required: slog.Error doesn’t exit
}

func (cl *customLogger) Errorf(f string, a ...interface{}) {
	slog.Error(fmt.Sprintf(f, a...), "component", "nats")
}

func (cl *customLogger) Debugf(f string, a ...interface{}) {
	slog.Debug(fmt.Sprintf(f, a...), "component", "nats")
}

func (cl *customLogger) Tracef(f string, a ...interface{}) {
	slog.Debug(fmt.Sprintf(f, a...), "component", "nats", "trace", true)
}

func main() {

	isProd := os.Getenv("PRODUCTION") == "1"
	level := slog.LevelDebug
	if isProd {
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))

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
		slog.Error("❌ Failed to create NATS server", "err", err)
		os.Exit(1)
	}
	ns.SetLoggerV2(&customLogger{}, true, false, false)
	// ns.SetLoggerV2(&customLogger{log.New(os.Stdout, "nats-server: ", log.LstdFlags)}, true, false, false)

	// Start the embedded NATS server in a goroutine
	go func() {
		slog.Info("🚀 Starting embedded NATS server", "host", opts.Host, "port", opts.Port)
		ns.Start()
	}()

	// Wait for it to be ready
	for i := 0; i < 50; i++ {
		if ns.ReadyForConnections(1 * time.Second) {
			break
		}
		slog.Warn("⏳ Waiting for NATS...", "i", i)
		if i == 49 {
			slog.Error("❌ NATS connection failed")
			os.Exit(1)
		}
	}
	slog.Info("✅ NATS server ready")

	nc, err := nats.Connect(ns.ClientURL(), nats.MaxReconnects(-1))
	if err != nil {
		slog.Error("❌ NATS connection failed", "err", err)
		os.Exit(1)
	}
	defer nc.Close()

	// WebSocket setup
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		topic := strings.Trim(r.URL.Path, "/")
		if topic == "" {
			topic = "default"
		}
		if !regexp.MustCompile(`^[a-z0-9._-]+$`).MatchString(topic) {
			http.Error(w, "Invalid topic", http.StatusBadRequest)
			return
		}
		slog.Info("🔌 Client connected", "client", r.RemoteAddr, "topic", topic)

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Warn("❌ WebSocket upgrade error", "err", err)
			return
		}
		slog.Info("🔌 WebSocket connected", "client", r.RemoteAddr, "topic", topic)

		sub, err := nc.Subscribe(topic, func(m *nats.Msg) {
			err := conn.WriteMessage(websocket.TextMessage, m.Data)
			if err != nil {
				slog.Warn("❌ Write to WS failed", "err", err)
			}
		})
		if err != nil {
			slog.Warn("❌ NATS subscribe failed", "err", err)
			conn.Close()
			return
		}
		slog.Info("✅ Subscribed", "client", r.RemoteAddr, "topic", topic)

		defer sub.Unsubscribe()
		defer conn.Close()

		// Read WebSocket → publish to NATS
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				slog.Info("⚠️ WS read closed", "client", r.RemoteAddr, "err", err)
				break
			}
			slog.Info("📤 WS → NATS", "client", r.RemoteAddr, "topic", topic, "msg", msg)

			if err := nc.Publish(topic, msg); err != nil {
				slog.Warn("❌ NATS publish failed", "err", err)
			}
		}
		slog.Info("🔌 WebSocket disconnected", "client", r.RemoteAddr)
	})

	server := &http.Server{Addr: ":8080"}

	// Start HTTP server
	go func() {
		slog.Info("🌐 WebSocket server on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("❌ HTTP server error", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Warn("🛑 Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Warn("❌ HTTP shutdown failed", "err", err)
	}

	nc.Drain()
	ns.Shutdown()
	slog.Info("✅ Exit complete")
}
