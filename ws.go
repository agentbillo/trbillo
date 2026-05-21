package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// In production, tighten this. For local development, allow any origin.
		return true
	},
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
			h.mu.RLock()
			clients := h.boards[boardIDStr]
			h.mu.RUnlock()

			for client := range clients {
				select {
				case client.send <- msg.Payload:
				default:
					// If buffer is full, unregister client
					h.mu.Lock()
					delete(clients, client)
					client.mu.Lock()
					if !client.isClosed {
						client.isClosed = true
						close(client.send)
					}
					client.mu.Unlock()
					h.mu.Unlock()
				}
			}

		case msg := <-h.userBroadcast:
			h.mu.RLock()
			clients := h.users[msg.UserID]
			h.mu.RUnlock()

			for client := range clients {
				select {
				case client.send <- msg.Payload:
				default:
					// If buffer is full, unregister client
					h.mu.Lock()
					delete(clients, client)
					client.mu.Lock()
					if !client.isClosed {
						client.isClosed = true
						close(client.send)
					}
					client.mu.Unlock()
					h.mu.Unlock()
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
