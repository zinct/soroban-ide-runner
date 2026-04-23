package port

import (
	"sync"
)

// Manager handles a pool of available ports for dev servers.
type Manager struct {
	mu           sync.Mutex
	availablePorts []int
	sessionPorts   map[string]int // sessionID -> port
}

// NewManager creates a new Port Manager with a range of ports.
func NewManager(startPort, count int) *Manager {
	ports := make([]int, count)
	for i := 0; i < count; i++ {
		ports[i] = startPort + i
	}

	return &Manager{
		availablePorts: ports,
		sessionPorts:   make(map[string]int),
	}
}

// GetPort returns the port assigned to a session, or assigns a new one.
// Returns 0 if no ports are available.
func (m *Manager) GetPort(sessionID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If already assigned, return it
	if port, assigned := m.sessionPorts[sessionID]; assigned {
		return port
	}

	// Assign new port
	if len(m.availablePorts) == 0 {
		return 0
	}

	port := m.availablePorts[0]
	m.availablePorts = m.availablePorts[1:]
	m.sessionPorts[sessionID] = port

	return port
}

// ReleasePort frees up a port when a session is finished.
func (m *Manager) ReleasePort(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if port, assigned := m.sessionPorts[sessionID]; assigned {
		m.availablePorts = append(m.availablePorts, port)
		delete(m.sessionPorts, sessionID)
	}
}
