// nats-ws-gateway-and-server-logger
package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	// The file path for logging all NATS traffic.
	// NOTE: Writing to /var/log typically requires elevated (root) privileges.
	logFilePath = "/var/log/nats-ws-gateway-and-server/traffic.log"

	// The default NATS server URL to connect to. Can be overridden with NATS_URL env var.
	defaultNatsURL = "nats://127.0.0.1:5050"
)

// logEntry defines the structure for a single log record.
// This structure is designed to be marshaled into a JSON line for machine parsing.
type logEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Topic     string    `json:"topic"`
	Data      string    `json:"data"` // Data is now assumed to be a plain text string.
}

// logFileMutex protects the log file from concurrent writes.
var logFileMutex = &sync.Mutex{}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Determine NATS server URL from environment or use default
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = defaultNatsURL
	}

	// --- Log File Setup ---
	logDir := filepath.Dir(logFilePath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Error("❌ Failed to create log directory", "path", logDir, "err", err)
		os.Exit(1)
	}

	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("❌ Failed to open log file", "path", logFilePath, "err", err)
		os.Exit(1)
	}
	defer logFile.Close()
	slog.Info("📝 Logging traffic to file", "path", logFilePath)

	// --- NATS Connection ---
	slog.Info("🚀 Connecting to NATS server...", "url", natsURL)
	nc, err := nats.Connect(natsURL, nats.MaxReconnects(-1))
	if err != nil {
		slog.Error("❌ NATS connection failed", "err", err)
		os.Exit(1)
	}
	defer nc.Close()
	slog.Info("✅ NATS connection established")

	// --- NATS Subscription ---
	// Subscribe to ">" to capture all messages from all topics on the server.
	_, err = nc.Subscribe(">", func(m *nats.Msg) {
		entry := logEntry{
			Timestamp: time.Now().UTC(),
			Topic:     m.Subject,
			Data:      string(m.Data), // Directly convert message data to a string
		}

		jsonData, err := json.Marshal(entry)
		if err != nil {
			slog.Warn("❌ Failed to marshal log entry to JSON", "err", err, "topic", m.Subject)
			return
		}

		logFileMutex.Lock()
		defer logFileMutex.Unlock()

		if _, err := logFile.Write(append(jsonData, '\n')); err != nil {
			slog.Warn("❌ Failed to write to log file", "err", err)
		}
	})

	if err != nil {
		slog.Error("❌ Failed to subscribe to NATS", "topic", ">", "err", err)
		os.Exit(1)
	}
	slog.Info("🎧 Subscribed to all available topics (`>`). Logging traffic to", logFilePath)

	// --- Graceful Shutdown ---
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Warn("🛑 Shutting down...")
	if err := nc.Drain(); err != nil {
		slog.Warn("❌ NATS drain failed", "err", err)
	}
	slog.Info("✅ Exit complete")
}
