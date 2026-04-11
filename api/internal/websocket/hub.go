package websocket

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // TODO: restrict in production
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 1024 * 1024 // 1MB
)

// Message represents a message sent over the WebSocket.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Hub manages active agent WebSocket connections.
type Hub struct {
	mu    sync.RWMutex
	conns map[string]*websocket.Conn // agent_id → connection

	// OnMessage is called when an agent sends a message.
	OnMessage func(agentID string, msg Message)
}

func NewHub() *Hub {
	return &Hub{
		conns: make(map[string]*websocket.Conn),
	}
}

// HandleConnection upgrades an HTTP request to WebSocket and manages the connection.
func (h *Hub) HandleConnection(w http.ResponseWriter, r *http.Request, agentID string) error {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("upgrading to websocket: %w", err)
	}

	h.register(agentID, conn)
	defer h.unregister(agentID)

	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Start ping ticker
	go h.pingLoop(agentID, conn)

	// Read loop
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("agent disconnected unexpectedly", "agent_id", agentID, "error", err)
			}
			return nil
		}

		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			slog.Warn("invalid message from agent", "agent_id", agentID, "error", err)
			continue
		}

		slog.Debug("message from agent", "agent_id", agentID, "type", msg.Type)

		if h.OnMessage != nil {
			h.OnMessage(agentID, msg)
		}
	}
}

// Send sends a message to a specific agent.
func (h *Hub) Send(agentID string, msg Message) error {
	h.mu.RLock()
	conn, ok := h.conns[agentID]
	h.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not connected", agentID)
	}

	conn.SetWriteDeadline(time.Now().Add(writeWait))
	return conn.WriteJSON(msg)
}

// IsConnected returns whether an agent is currently connected.
func (h *Hub) IsConnected(agentID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[agentID]
	return ok
}

// ConnectedAgents returns a list of currently connected agent IDs.
func (h *Hub) ConnectedAgents() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]string, 0, len(h.conns))
	for id := range h.conns {
		ids = append(ids, id)
	}
	return ids
}

func (h *Hub) register(agentID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Close existing connection if any
	if existing, ok := h.conns[agentID]; ok {
		existing.Close()
	}
	h.conns[agentID] = conn
	slog.Info("agent registered", "agent_id", agentID, "total_agents", len(h.conns))
}

func (h *Hub) unregister(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, ok := h.conns[agentID]; ok {
		conn.Close()
		delete(h.conns, agentID)
	}
	slog.Info("agent unregistered", "agent_id", agentID, "total_agents", len(h.conns))
}

func (h *Hub) pingLoop(agentID string, conn *websocket.Conn) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for range ticker.C {
		h.mu.RLock()
		current, ok := h.conns[agentID]
		h.mu.RUnlock()

		if !ok || current != conn {
			return
		}

		conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			return
		}
	}
}
