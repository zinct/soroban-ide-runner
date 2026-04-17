package session

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"

	"soroban-studio-backend/internal/model"
)

// Session represents an active compilation session.
// It holds WebSocket connections and message buffers grouped by JobID.
type Session struct {
	mu         sync.Mutex
	conns      map[string]*websocket.Conn // jobID -> connection
	jobBuffers map[string][]model.OutputMessage
}

// Manager handles session lifecycle and WebSocket connections.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewManager creates a new session manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// GetOrCreate retrieves an existing session or creates a new one if it doesn't exist.
func (m *Manager) GetOrCreate(sessionID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[sessionID]; ok {
		return s
	}

	s := &Session{
		conns:      make(map[string]*websocket.Conn),
		jobBuffers: make(map[string][]model.OutputMessage),
	}
	m.sessions[sessionID] = s
	return s
}

// AddConnection registers a WebSocket connection for a session.
// If a jobID is provided, any buffered messages for that specific job
// are flushed immediately to the new connection.
// Returns false if the session does not exist.
func (m *Manager) AddConnection(sessionID string, jobID string, conn *websocket.Conn) bool {
	m.mu.RLock()
	s, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Flush history for the requested job
	if jobID != "" {
		if buffer, exists := s.jobBuffers[jobID]; exists {
			for _, msg := range buffer {
				data, _ := json.Marshal(msg)
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					return true // Connection is not added, but return true to keep WS handler alive
				}
			}
		}
	}

	s.conns[jobID] = conn
	return true
}

// RemoveConnection removes a specific WebSocket connection from a session.
func (m *Manager) RemoveConnection(sessionID string, conn *websocket.Conn) {
	m.mu.RLock()
	s, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, c := range s.conns {
		if c == conn {
			delete(s.conns, id)
			break
		}
	}
}

// Send broadcasts a message to all connected WebSocket clients for a session.
func (m *Manager) Send(sessionID string, msg model.OutputMessage) {
	m.mu.RLock()
	s, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Buffer the message if it has a JobID
	if msg.JobID != "" {
		if s.jobBuffers == nil {
			s.jobBuffers = make(map[string][]model.OutputMessage)
		}
		s.jobBuffers[msg.JobID] = append(s.jobBuffers[msg.JobID], msg)
	}

	// Send to all active connections
	for _, conn := range s.conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
		}
	}
}

// Remove cleans up a session entirely, closing all connections.
func (m *Manager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if s, ok := m.sessions[sessionID]; ok {
		s.mu.Lock()
		for _, conn := range s.conns {
			conn.Close()
		}
		s.mu.Unlock()
		delete(m.sessions, sessionID)
	}
}

// ClearBuffer removes all buffered messages from the session's history.
func (m *Manager) ClearBuffer(sessionID string) {
	m.mu.RLock()
	s, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobBuffers = make(map[string][]model.OutputMessage)
}
