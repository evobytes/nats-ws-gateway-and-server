package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 🚨 ALLOW ALL ORIGINS - safe for dev only
	},
}

var topicValidator = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type customLogger struct{}

func (cl *customLogger) Noticef(f string, a ...interface{}) {
	slog.Info(fmt.Sprintf(f, a...), "component", "nats", "level", "notice")
}

func (cl *customLogger) Warnf(f string, a ...interface{}) {
	slog.Warn(fmt.Sprintf(f, a...), "component", "nats")
}

func (cl *customLogger) Fatalf(f string, a ...interface{}) {
	slog.Error(fmt.Sprintf(f, a...), "component", "nats", "fatal", true)
	os.Exit(1)
}

func (cl *customLogger) Errorf(f string, a ...interface{}) {
	slog.Error(fmt.Sprintf(f, a...), "component", "nats")
}

func (cl *customLogger) Debugf(f string, a ...interface{}) {
	msg := fmt.Sprintf(f, a...)
	if strings.Contains(msg, "Client Ping Timer") || strings.Contains(msg, "Delaying PING") {
		return
	}
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

	// configuration - default and envvar overrides
	natsHost := os.Getenv("NATS_BIND")
	if natsHost == "" {
		natsHost = "127.0.0.1" // Fallback to default if not set
	}

	natsPortStr := os.Getenv("NATS_PORT")
	natsPort, err := strconv.Atoi(natsPortStr)
	if err != nil {
		natsPort = 5050 // Fallback to default if conversion fails
	}

	httpPortStr := os.Getenv("NATS_HTTP_PORT")
	if httpPortStr == "" {
		httpPortStr = "8080" // Fallback to default if not set
	}

	httpAddr := fmt.Sprintf("%s:%s", natsHost, httpPortStr)

	// NATS server setup
	opts := &natsserver.Options{
		Host:           natsHost,
		Port:           natsPort,
		NoLog:          false,
		NoSigs:         true,
		MaxControlLine: 256,
		JetStream:      false,
		Trace:          false,
		Debug:          false,
		HTTPPort:       8222,
	}

	ns, err := natsserver.NewServer(opts)
	if err != nil {
		slog.Error("❌ Failed to create NATS server", "err", err)
		os.Exit(1)
	}
	ns.SetLoggerV2(&customLogger{}, true, false, false)

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
		if !topicValidator.MatchString(topic) {
			http.Error(w, "Invalid topic", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			// Handle WebSocket upgrade
			handleWebSocket(w, r, topic, nc)
		case http.MethodPost:
			// Handle HTTP POST request
			handleHttpPost(w, r, topic, nc)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	server := &http.Server{Addr: httpAddr}

	// Start HTTP server
	go func() {
		slog.Info("🌐 Server on", "httpAddr", httpAddr)
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

// handleWebSocket manages the WebSocket connection logic
func handleWebSocket(w http.ResponseWriter, r *http.Request, topic string, nc *nats.Conn) {
	slog.Info("🔌 Client attempting WebSocket connection", "client", r.RemoteAddr, "topic", topic)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("❌ WebSocket upgrade error", "err", err)
		return
	}
	slog.Info("🔌 WebSocket connected", "client", r.RemoteAddr, "topic", topic)

	// Subscribe to NATS for messages to send to the WebSocket client
	sub, err := nc.Subscribe(topic, func(m *nats.Msg) {
		err := conn.WriteMessage(websocket.TextMessage, m.Data)
		if err != nil {
			slog.Warn("❌ Write to WS failed", "err", err, "client", r.RemoteAddr)
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

		// --- AUGMENTATION START ---
		// Ignore "ping" messages from the client
		if string(msg) == "ping" {
			continue
		}
		// --- AUGMENTATION END ---

		slog.Info("📤 WS → NATS", "client", r.RemoteAddr, "topic", topic, "msg", string(msg))

		if err := nc.Publish(topic, msg); err != nil {
			slog.Warn("❌ NATS publish failed", "err", err)
		}
	}
	slog.Info("🔌 WebSocket disconnected", "client", r.RemoteAddr)
}

// handleHttpPost manages the HTTP POST request logic
func handleHttpPost(w http.ResponseWriter, r *http.Request, topic string, nc *nats.Conn) {
	slog.Info("📥 HTTP POST received", "client", r.RemoteAddr, "topic", topic)

	// Read the entire body of the request
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Warn("❌ Failed to read request body", "err", err)
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// Check if the body is empty
	if len(body) == 0 {
		http.Error(w, "Empty body", http.StatusBadRequest)
		return
	}

	slog.Info("📤 HTTP POST → NATS", "topic", topic, "msg_size", len(body))

	// Publish the body content to NATS
	if err := nc.Publish(topic, body); err != nil {
		slog.Warn("❌ NATS publish failed for HTTP POST", "err", err)
		http.Error(w, "Failed to publish message to NATS", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Message published to NATS topic: " + topic))
	slog.Info("✅ HTTP POST handled successfully", "client", r.RemoteAddr, "topic", topic)
}
