package cmd

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Message structures
type WsMessage struct {
	Type      string `json:"type"`       // "terminal_data", "resize", "ping"
	SessionID string `json:"session_id"` // ID of the terminal session
	Data      string `json:"data"`       // Base64 or plain string
	Cols      int    `json:"cols,omitempty"`
	Rows      int    `json:"rows,omitempty"`
}

type Hub struct {
	mu           sync.RWMutex
	Agents       map[string]*AgentEntry // AgentID (token) -> agent connection + metadata
	Clients      map[string]*safeConn   // SessionID -> Web Client WS
	SessionAgent map[string]string      // SessionID -> AgentID
}

// AgentEntry is a connected agent with its owner metadata.
type AgentEntry struct {
	conn   *safeConn
	Name   string
	UserID string
}

// safeConn serializes writes: gorilla/websocket panics on concurrent writes.
type safeConn struct {
	conn *websocket.Conn
	wmu  sync.Mutex
}

func (s *safeConn) WriteJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, b)
}

func (s *safeConn) Close() {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	s.conn.Close()
}

func NewHub() *Hub {
	return &Hub{
		Agents:       make(map[string]*AgentEntry),
		Clients:      make(map[string]*safeConn),
		SessionAgent: make(map[string]string),
	}
}

func (h *Hub) RegisterAgent(agentID, name, userID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Agents[agentID] = &AgentEntry{conn: &safeConn{conn: conn}, Name: name, UserID: userID}
	log.Printf("Agent registered: %s (name=%q user=%s)", agentID, name, userID)
}

func (h *Hub) UnregisterAgent(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if entry, ok := h.Agents[agentID]; ok {
		entry.conn.Close()
		delete(h.Agents, agentID)
	}
	log.Printf("Agent unregistered: %s", agentID)
}

// IsOnline reports whether an agent with the given token is connected.
func (h *Hub) IsOnline(agentID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.Agents[agentID]
	return ok
}

func (h *Hub) RegisterClient(sessionID, agentID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Clients[sessionID] = &safeConn{conn: conn}
	h.SessionAgent[sessionID] = agentID
	log.Printf("Client registered for session: %s on agent: %s", sessionID, agentID)
}

func (h *Hub) UnregisterClient(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, ok := h.Clients[sessionID]; ok {
		conn.Close()
		delete(h.Clients, sessionID)
		delete(h.SessionAgent, sessionID)
	}
	log.Printf("Client unregistered: %s", sessionID)
}

func (h *Hub) RouteToAgent(sessionID string, msg WsMessage) {
	h.mu.RLock()
	agentID, ok := h.SessionAgent[sessionID]
	if !ok {
		h.mu.RUnlock()
		return
	}
	entry, ok := h.Agents[agentID]
	h.mu.RUnlock()

	if ok {
		entry.conn.WriteJSON(msg)
	}
}

func (h *Hub) RouteToClient(sessionID string, msg WsMessage) {
	h.mu.RLock()
	clientConn, ok := h.Clients[sessionID]
	h.mu.RUnlock()

	if ok {
		clientConn.WriteJSON(msg)
	}
}
