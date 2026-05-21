package main

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// checkWebSocketOrigin validates the WebSocket origin header.
// Allows localhost for development, validates same-origin for production.
// Rejects requests without Origin header in production to prevent CSRF.
func checkWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	host := r.Host

	// Check if this is localhost first
	hostWithoutPort := host
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		hostWithoutPort = host[:colonIdx]
	}
	isLocalHost := hostWithoutPort == "localhost" || hostWithoutPort == "127.0.0.1" || hostWithoutPort == "::1"

	if origin == "" {
		// No origin header - only allow for localhost (development)
		// In production, reject to prevent CSRF from non-browser clients
		return isLocalHost
	}

	// For production, validate that origin matches the host
	// Origin format: "https://example.com" or "http://example.com:8080"
	originHost := origin
	if strings.HasPrefix(originHost, "https://") {
		originHost = originHost[8:]
	} else if strings.HasPrefix(originHost, "http://") {
		originHost = originHost[7:]
	}

	// Strip port from origin host for comparison
	if colonIdx := strings.LastIndex(originHost, ":"); colonIdx != -1 {
		originHost = originHost[:colonIdx]
	}

	return originHost == hostWithoutPort
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkWebSocketOrigin,
}

// Client represents a connected user on a specific board.
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	userID   string
	boardID  string
	send     chan []byte
	mu       sync.Mutex
	isClosed bool
}

// Hub manages all active WebSocket connections by board and user.
type Hub struct {
	// Map of Board ID -> Map of Client pointers
	boards map[string]map[*Client]bool
	// Map of User ID -> Map of Client pointers (for user-level events)
	users         map[string]map[*Client]bool
	broadcast     chan BroadcastMessage
	userBroadcast chan UserBroadcastMessage
	register      chan *Client
	unregister    chan *Client
	mu            sync.RWMutex
}

type BroadcastMessage struct {
	BoardID []byte // Raw json or just board ID string
	Payload []byte
}

type UserBroadcastMessage struct {
	UserID  string
	Payload []byte
}

var HubInstance *Hub

func InitHub() {
	HubInstance = &Hub{
		boards:        make(map[string]map[*Client]bool),
		users:         make(map[string]map[*Client]bool),
		broadcast:     make(chan BroadcastMessage),
		userBroadcast: make(chan UserBroadcastMessage),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			// Register to board channel if boardID is set
			if client.boardID != "" {
				if h.boards[client.boardID] == nil {
					h.boards[client.boardID] = make(map[*Client]bool)
				}
				h.boards[client.boardID][client] = true
			}
			// Always register to user channel
			if client.userID != "" {
				if h.users[client.userID] == nil {
					h.users[client.userID] = make(map[*Client]bool)
				}
				h.users[client.userID][client] = true
			}
			h.mu.Unlock()
			log.Printf("WS client registered: User %s on Board %s", client.userID, client.boardID)

		case client := <-h.unregister:
			h.mu.Lock()
			// Unregister from board channel
			if client.boardID != "" {
				if clients, ok := h.boards[client.boardID]; ok {
					if _, ok := clients[client]; ok {
						delete(clients, client)
						if len(clients) == 0 {
							delete(h.boards, client.boardID)
						}
					}
				}
			}
			// Unregister from user channel
			if client.userID != "" {
				if clients, ok := h.users[client.userID]; ok {
					if _, ok := clients[client]; ok {
						delete(clients, client)
						if len(clients) == 0 {
							delete(h.users, client.userID)
						}
					}
				}
			}
			// Close send channel only once
			client.mu.Lock()
			if !client.isClosed {
				client.isClosed = true
				close(client.send)
			}
			client.mu.Unlock()
			h.mu.Unlock()
			log.Printf("WS client unregistered: User %s from Board %s", client.userID, client.boardID)

		case msg := <-h.broadcast:
			boardIDStr := string(msg.BoardID)
			// Snapshot clients to avoid race condition during iteration
			h.mu.RLock()
			originalClients := h.boards[boardIDStr]
			clientsCopy := make([]*Client, 0, len(originalClients))
			for client := range originalClients {
				clientsCopy = append(clientsCopy, client)
			}
			h.mu.RUnlock()

			for _, client := range clientsCopy {
				select {
				case client.send <- msg.Payload:
				default:
					// If buffer is full, schedule unregister with timeout to prevent goroutine leak
					go func(c *Client) {
						select {
						case h.unregister <- c:
						case <-time.After(5 * time.Second):
							// Timeout - force close the connection to trigger cleanup via readPump
							c.conn.Close()
						}
					}(client)
				}
			}

		case msg := <-h.userBroadcast:
			// Snapshot clients to avoid race condition during iteration
			h.mu.RLock()
			originalClients := h.users[msg.UserID]
			clientsCopy := make([]*Client, 0, len(originalClients))
			for client := range originalClients {
				clientsCopy = append(clientsCopy, client)
			}
			h.mu.RUnlock()

			for _, client := range clientsCopy {
				select {
				case client.send <- msg.Payload:
				default:
					// If buffer is full, schedule unregister with timeout to prevent goroutine leak
					go func(c *Client) {
						select {
						case h.unregister <- c:
						case <-time.After(5 * time.Second):
							// Timeout - force close the connection to trigger cleanup via readPump
							c.conn.Close()
						}
					}(client)
				}
			}
		}
	}
}

// BroadcastToBoard is a public method helper to trigger broadcasts from API endpoints.
func (h *Hub) BroadcastToBoard(boardID string, payload []byte) {
	h.broadcast <- BroadcastMessage{
		BoardID: []byte(boardID),
		Payload: payload,
	}
}

// BroadcastToUser sends a message to all connections for a specific user.
func (h *Hub) BroadcastToUser(userID string, payload []byte) {
	h.userBroadcast <- UserBroadcastMessage{
		UserID:  userID,
		Payload: payload,
	}
}

// readPump pumps messages from the websocket connection to the hub.
// In our app, clients send actions via REST APIs, but we read to detect client disconnection.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WS read error: %v", err)
			}
			break
		}
		// We don't process incoming WS messages for DB actions since we use REST API endpoints.
		// This keeps state changes synchronous and transaction-safe via HTTP requests.
	}
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Client) writePump() {
	defer func() {
		c.conn.Close()
	}()

	for {
		message, ok := <-c.send
		if !ok {
			// The hub closed the channel.
			_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}

		c.mu.Lock()
		err := c.conn.WriteMessage(websocket.TextMessage, message)
		c.mu.Unlock()

		if err != nil {
			log.Printf("WS write error: %v", err)
			return
		}
	}
}

// ServeWS handles WebSocket requests from clients.
func ServeWS(w http.ResponseWriter, r *http.Request, userID, boardID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := &Client{
		hub:     HubInstance,
		conn:    conn,
		userID:  userID,
		boardID: boardID,
		send:    make(chan []byte, 256),
	}

	client.hub.register <- client

	// Start read and write pumps in goroutines
	go client.writePump()
	go client.readPump()
}

// ServeUserWS handles WebSocket connections for user-level events (no board required).
func ServeUserWS(w http.ResponseWriter, r *http.Request, userID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("User WebSocket upgrade failed: %v", err)
		return
	}

	client := &Client{
		hub:     HubInstance,
		conn:    conn,
		userID:  userID,
		boardID: "", // No board - user-level only
		send:    make(chan []byte, 256),
	}

	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}
