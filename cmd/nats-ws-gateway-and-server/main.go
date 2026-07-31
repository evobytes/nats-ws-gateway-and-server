package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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

var topicValidator = regexp.MustCompile(`^[a-zA-Z0-9*>._-]+$`)

// topicStats tracks observed NATS traffic per subject, in memory only.
type topicStats struct {
	mu     sync.Mutex
	counts map[string]int
	last   map[string]time.Time
}

func newTopicStats() *topicStats {
	return &topicStats{
		counts: make(map[string]int),
		last:   make(map[string]time.Time),
	}
}

func (t *topicStats) record(subject string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts[subject]++
	t.last[subject] = time.Now().UTC()
}

// topicEntry is the JSON representation of a single observed subject.
type topicEntry struct {
	Name         string `json:"name"`
	LastMessage  string `json:"lastMessage"`
	MessageCount int    `json:"messageCount"`
}

func (t *topicStats) snapshot() []topicEntry {
	t.mu.Lock()
	defer t.mu.Unlock()

	entries := make([]topicEntry, 0, len(t.counts))
	for subject, count := range t.counts {
		entries = append(entries, topicEntry{
			Name:         subject,
			LastMessage:  t.last[subject].Format(time.RFC3339),
			MessageCount: count,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
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
	startTime := time.Now()

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
	natsPort := 5050 // Fallback to default if not set or conversion fails
	if natsPortStr != "" {
		if p, err := strconv.Atoi(natsPortStr); err == nil {
			natsPort = p
		} else {
			slog.Warn("Invalid NATS_PORT, using default", "value", natsPortStr, "default", natsPort)
		}
	}

	httpPortStr := os.Getenv("NATS_HTTP_PORT")
	if httpPortStr == "" {
		httpPortStr = "8080" // Fallback to default if not set
	}

	monitorPortStr := os.Getenv("NATS_MONITOR_PORT")
	monitorPort := 8222 // Fallback to default if not set or conversion fails
	if monitorPortStr != "" {
		if p, err := strconv.Atoi(monitorPortStr); err == nil {
			monitorPort = p
		} else {
			slog.Warn("Invalid NATS_MONITOR_PORT, using default", "value", monitorPortStr, "default", monitorPort)
		}
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
		HTTPPort:       monitorPort,
	}

	ns, err := natsserver.NewServer(opts)
	if err != nil {
		slog.Error("Failed to create NATS server", "err", err)
		os.Exit(1)
	}
	ns.SetLoggerV2(&customLogger{}, true, false, false)

	// Start the embedded NATS server in a goroutine
	go func() {
		slog.Info("Starting embedded NATS server", "host", opts.Host, "port", opts.Port)
		ns.Start()
	}()

	// Wait for it to be ready
	const maxReadyChecks = 50
	ready := false
	for i := 0; i < maxReadyChecks; i++ {
		if ns.ReadyForConnections(1 * time.Second) {
			ready = true
			break
		}
		slog.Warn("Waiting for NATS...", "i", i)
	}
	if !ready {
		slog.Error("NATS connection failed")
		os.Exit(1)
	}
	slog.Info("NATS server ready")

	nc, err := nats.Connect(ns.ClientURL(), nats.MaxReconnects(-1))
	if err != nil {
		slog.Error("NATS connection failed", "err", err)
		os.Exit(1)
	}
	defer nc.Close()

	// Observe all traffic to build the /status topic report
	tracker := newTopicStats()
	if _, err := nc.Subscribe(">", func(m *nats.Msg) {
		if strings.HasPrefix(m.Subject, "$SYS.") {
			return
		}
		tracker.record(m.Subject)
	}); err != nil {
		slog.Error("Failed to subscribe for traffic observation", "err", err)
		os.Exit(1)
	}

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

		switch {
		case topic == "status" && r.Method == "QUERY":
			// Handle status command
			handleStatus(w, r, startTime, tracker)
		case r.Method == http.MethodGet:
			// Handle WebSocket upgrade
			handleWebSocket(w, r, topic, nc)
		case r.Method == http.MethodPost:
			// Handle HTTP POST request
			handleHttpPost(w, r, topic, nc)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	server := &http.Server{Addr: httpAddr}

	// Start HTTP server
	go func() {
		slog.Info("Server on", "httpAddr", httpAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Warn("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Warn("HTTP shutdown failed", "err", err)
	}

	nc.Drain()
	ns.Shutdown()
	slog.Info("Exit complete")
}

// handleWebSocket manages the WebSocket connection logic
func handleWebSocket(w http.ResponseWriter, r *http.Request, topic string, nc *nats.Conn) {
	slog.Info("Client attempting WebSocket connection", "client", r.RemoteAddr, "topic", topic)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("WebSocket upgrade error", "err", err)
		return
	}
	slog.Info("WebSocket connected", "client", r.RemoteAddr, "topic", topic)

	// Subscribe to NATS for messages to send to the WebSocket client
	sub, err := nc.Subscribe(topic, func(m *nats.Msg) {
		// default to text message, but check if the data is gzip compressed (starts with 0x1f 0x8b)
		msgFormat := websocket.TextMessage
		if len(m.Data) >= 2 && m.Data[0] == 0x1f && m.Data[1] == 0x8b {
			msgFormat = websocket.BinaryMessage
		}
		err := conn.WriteMessage(msgFormat, m.Data)
		if err != nil {
			slog.Warn("Write to WS failed", "err", err, "client", r.RemoteAddr)
		}
	})
	if err != nil {
		slog.Warn("NATS subscribe failed", "err", err)
		conn.Close()
		return
	}
	slog.Info("Subscribed", "client", r.RemoteAddr, "topic", topic)

	defer sub.Unsubscribe()
	defer conn.Close()

	// Read WebSocket → publish to NATS
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			slog.Info("WS read closed", "client", r.RemoteAddr, "err", err)
			break
		}

		// --- AUGMENTATION START ---
		// Ignore "ping" messages from the client
		if string(msg) == "ping" {
			continue
		}
		// --- AUGMENTATION END ---

		slog.Info("WS -> NATS", "client", r.RemoteAddr, "topic", topic, "msg", string(msg))

		if err := nc.Publish(topic, msg); err != nil {
			slog.Warn("NATS publish failed", "err", err)
		}
	}
	slog.Info("WebSocket disconnected", "client", r.RemoteAddr)
}

// handleHttpPost manages the HTTP POST request logic
func handleHttpPost(w http.ResponseWriter, r *http.Request, topic string, nc *nats.Conn) {
	slog.Info("HTTP POST received", "client", r.RemoteAddr, "topic", topic)

	// Read the entire body of the request
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Warn("Failed to read request body", "err", err)
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// Check if the body is empty
	if len(body) == 0 {
		http.Error(w, "Empty body", http.StatusBadRequest)
		return
	}

	slog.Info("HTTP POST -> NATS", "topic", topic, "msg_size", len(body))

	// Publish the body content to NATS
	if err := nc.Publish(topic, body); err != nil {
		slog.Warn("NATS publish failed for HTTP POST", "err", err)
		http.Error(w, "Failed to publish message to NATS", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Message published to NATS topic: " + topic))
	slog.Info("HTTP POST handled successfully", "client", r.RemoteAddr, "topic", topic)
}

// statusResponse is the JSON body returned by handleStatus.
type statusResponse struct {
	Uptime string       `json:"uptime"`
	Topics []topicEntry `json:"topics"`
}

// handleStatus returns gateway uptime and the observed topics with real traffic.
func handleStatus(w http.ResponseWriter, r *http.Request, startTime time.Time, tracker *topicStats) {
	slog.Info("Status requested", "client", r.RemoteAddr)

	resp := statusResponse{
		Uptime: time.Since(startTime).Round(time.Millisecond).String(),
		Topics: tracker.snapshot(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Warn("Failed to encode status response", "err", err)
		return
	}
	slog.Info("Status handled successfully", "client", r.RemoteAddr, "topic_count", len(resp.Topics))
}
