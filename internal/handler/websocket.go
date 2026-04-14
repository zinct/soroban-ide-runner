package handler

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"soroban-studio-backend/internal/session"
)

// upgrader configures the WebSocket upgrade behavior.
var upgrader = websocket.Upgrader{
	// Allow all origins for development. In production, restrict this
	// to your frontend domain.
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WSHandler handles WebSocket connections for streaming build output.
type WSHandler struct {
	sessionMgr *session.Manager
}

// NewWSHandler creates a new WebSocket handler.
func NewWSHandler(sessionMgr *session.Manager) *WSHandler {
	return &WSHandler{sessionMgr: sessionMgr}
}

// Handle upgrades the HTTP connection to a WebSocket and registers it
// with the session manager for real-time output streaming.
//
// Usage: GET /ws?session_id=abc12345
func (h *WSHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// Extract session_id and optional job_id from query parameters
	sessionID := r.URL.Query().Get("session_id")
	jobID := r.URL.Query().Get("job_id")

	if sessionID == "" {
		http.Error(w, `{"error":"session_id query parameter is required"}`, http.StatusBadRequest)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[websocket] upgrade failed: %v", err)
		return
	}

	log.Printf("[websocket] client connected: session=%s, job=%s", sessionID, jobID)

	// Register this connection with the session manager.
	// This also flushes any buffered messages for the specific jobID.
	if !h.sessionMgr.AddConnection(sessionID, jobID, conn) {
		log.Printf("[websocket] session not found: %s", sessionID)
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","content":"session not found"}`))
		conn.Close()
		return
	}

	// Keep the connection alive by reading incoming messages.
	// This handles WebSocket ping/pong and detects disconnects.
	// We don't expect any client messages, but we need to read
	// to keep the connection alive and detect closure.
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[websocket] client disconnected: session=%s, reason=%v", sessionID, err)
			break
		}
	}

	// Clean up the connection from the session manager
	h.sessionMgr.RemoveConnection(sessionID, conn)
}
