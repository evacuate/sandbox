package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Manager for WebSocket connections
var (
	clients      = make(map[*websocket.Conn]bool)
	clientsMutex sync.Mutex
	upgrader     = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// CORS handling: adjust as needed
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

// Upgrade WebSocket connection at /ws endpoint
func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	// Add connection
	clientsMutex.Lock()
	clients[conn] = true
	clientsMutex.Unlock()

	log.Println("New client connected")

	// Read messages from client (waiting for disconnection)
	for {
		if _, _, err := conn.NextReader(); err != nil {
			break
		}
	}

	// Cleanup on disconnection
	clientsMutex.Lock()
	delete(clients, conn)
	clientsMutex.Unlock()
	log.Println("Client disconnected")
}

// Fetch data from specified API and return JSON as []map[string]interface{}
func fetchData() ([]map[string]interface{}, error) {
	resp, err := http.Get("https://api-v2-sandbox.p2pquake.net/v2/history?limit=100")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// Send each object from data to clients every 30 seconds
func broadcastData(data []map[string]interface{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	index := 0
	for {
		<-ticker.C

		// Convert current data to JSON
		msg, err := json.Marshal(data[index])
		if err != nil {
			log.Println("Error marshaling data:", err)
			continue
		}

		// Send to all clients
		clientsMutex.Lock()
		for conn := range clients {
			err := conn.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Println("Error writing message to client:", err)
				conn.Close()
				delete(clients, conn)
			}
		}
		clientsMutex.Unlock()

		// Move to next data
		index++
		if index >= len(data) {
			index = 0
		}
	}
}

func main() {
	// Fetch data from API
	data, err := fetchData()
	if err != nil {
		log.Fatal("Error fetching data:", err)
	}
	log.Printf("Fetched %d records", len(data))

	// Setup WebSocket endpoint
	http.HandleFunc("/ws", wsHandler)

	// Start goroutine for periodic broadcasting
	go broadcastData(data)

	log.Println("Server started on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
