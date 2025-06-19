package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

func main() {
	url := "wss://myserver.domain/hive-ws/"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatalf("Dial error: %v", err)
	}
	defer conn.Close()

	go func() {
		for {
			var msg Message
			err := conn.ReadJSON(&msg)
			if err != nil {
				log.Printf("Read error: %v", err)
				return
			}
			log.Printf("Received: %s -> %s", msg.Type, msg.Data)
		}
	}()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now().UTC().Format(time.RFC3339)
			msg := Message{Type: "clock", Data: now}
			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("Write error: %v", err)
				return
			}
		}
	}
}
