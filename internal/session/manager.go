package session

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"

	"soroban-studio-backend/internal/model"
)

// Session represents an active compilation session.
// It holds WebSocket connections and message buffers grouped by JobID.
type Session struct {
	mu         sync.Mutex
	conns      []*websocket.Conn
	// Map: JobID -> []Messages
	jobBuffers map[string][]model.OutputMessage
}

// Manager handles session lifecycle and WebSocket connections.
// Uses sync.Map for thread-safe concurrent access across goroutines.
type Manager struct {
	sessions sync.Map // map[string]*Session
}

// NewManager creates a new session manager.
func NewManager() *Manager {
	return &Manager{}
}

// GetOrCreate retrieves an existing session or creates a new one if it doesn't exist.
func (m *Manager) GetOrCreate(sessionID string) *Session {
	actual, _ := m.sessions.LoadOrStore(sessionID, &Session{
		conns:      make([]*websocket.Conn, 0),
		jobBuffers: make(map[string][]model.OutputMessage),
	})
	s := actual.(*Session)
	log.Printf("[session] retrieved/created: %s", sessionID)
	return s
}

// AddConnection registers a WebSocket connection for a session.
// If a jobID is provided, any buffered messages for that specific job
// are flushed immediately to the new connection.
// Returns false if the session does not exist.
func (m *Manager) AddConnection(sessionID string, jobID string, conn *websocket.Conn) bool {
	val, ok := m.sessions.Load(sessionID)
	if !ok {
		return false
	}

	s := val.(*Session)
	
	// We lock for the entire duration of history flushing and appending.
	// This is CRITICAL because gorilla/websocket does not support concurrent writes
	// to the same connection. By holding the session lock, we ensure that:
	// 1. Send() cannot broadcast to any connection until this flush is complete.
	// 2. This new connection is only added to the list AFTER its history is flushed.
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.jobBuffers == nil {
		s.jobBuffers = make(map[string][]model.OutputMessage)
	}

	// Flush history for the requested job
	if jobID != "" {
		if buffer, exists := s.jobBuffers[jobID]; exists {
			for _, msg := range buffer {
				data, _ := json.Marshal(msg)
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					log.Printf("[session] failed to flush job %s buffer: %v", jobID, err)
					return true // Connection is not added, but return true to keep WS handler alive
				}
			}
		}
	}

	s.conns = append(s.conns, conn)
	log.Printf("[session] connection added: session=%s, job=%s, total_conns=%d", sessionID, jobID, len(s.conns))
	return true
}

// RemoveConnection removes a specific WebSocket connection from a session.
func (m *Manager) RemoveConnection(sessionID string, conn *websocket.Conn) {
	val, ok := m.sessions.Load(sessionID)
	if !ok {
		return
	}

	s := val.(*Session)
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, c := range s.conns {
		if c == conn {
			s.conns = append(s.conns[:i], s.conns[i+1:]...)
			log.Printf("[session] connection removed: session=%s, remaining=%d", sessionID, len(s.conns))
			break
		}
	}
}

// Send broadcasts a message to all connected WebSocket clients for a session.
func (m *Manager) Send(sessionID string, msg model.OutputMessage) {
	val, ok := m.sessions.Load(sessionID)
	if !ok {
		return
	}

	s := val.(*Session)

	// Marshall data once
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[session] failed to marshal message: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.jobBuffers == nil {
		s.jobBuffers = make(map[string][]model.OutputMessage)
	}

	// Buffer the message
	if msg.JobID != "" {
		s.jobBuffers[msg.JobID] = append(s.jobBuffers[msg.JobID], msg)
	}

	// Broadcast to all active connections
	activeConns := make([]*websocket.Conn, 0, len(s.conns))
	for _, conn := range s.conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[session] removing dead connection: %v", err)
			conn.Close()
		} else {
			activeConns = append(activeConns, conn)
		}
	}
	s.conns = activeConns
}

// Remove cleans up a session entirely, closing all connections.
func (m *Manager) Remove(sessionID string) {
	val, ok := m.sessions.LoadAndDelete(sessionID)
	if !ok {
		return
	}

	s := val.(*Session)
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, conn := range s.conns {
		conn.Close()
	}
	log.Printf("[session] removed: %s", sessionID)
}

// ClearBuffer removes all buffered messages from the session's history.
func (m *Manager) ClearBuffer(sessionID string) {
	val, ok := m.sessions.Load(sessionID)
	if !ok {
		return
	}

	s := val.(*Session)
	s.mu.Lock()
	defer s.mu.Unlock()

	s.jobBuffers = make(map[string][]model.OutputMessage)
	log.Printf("[session] all job buffers cleared: %s", sessionID)
}
