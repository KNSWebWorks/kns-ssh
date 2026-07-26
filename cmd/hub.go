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
	Agents       map[string]*websocket.Conn // AgentID -> WS
	Clients      map[string]*websocket.Conn // SessionID -> Web Client WS
	SessionAgent map[string]string          // SessionID -> AgentID
}

func NewHub() *Hub {
	return &Hub{
		Agents:       make(map[string]*websocket.Conn),
		Clients:      make(map[string]*websocket.Conn),
		SessionAgent: make(map[string]string),
	}
}

func (h *Hub) RegisterAgent(agentID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Agents[agentID] = conn
	log.Printf("Agent registered: %s", agentID)
}

func (h *Hub) UnregisterAgent(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, ok := h.Agents[agentID]; ok {
		conn.Close()
		delete(h.Agents, agentID)
	}
	log.Printf("Agent unregistered: %s", agentID)
}

func (h *Hub) RegisterClient(sessionID, agentID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Clients[sessionID] = conn
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
	agentConn, ok := h.Agents[agentID]
	h.mu.RUnlock()

	if ok {
		b, _ := json.Marshal(msg)
		agentConn.WriteMessage(websocket.TextMessage, b)
	}
}

func (h *Hub) RouteToClient(sessionID string, msg WsMessage) {
	h.mu.RLock()
	clientConn, ok := h.Clients[sessionID]
	h.mu.RUnlock()

	if ok {
		b, _ := json.Marshal(msg)
		clientConn.WriteMessage(websocket.TextMessage, b)
	}
}
