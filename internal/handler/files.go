package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// FileHandler handles direct file content requests.
type FileHandler struct{}

// NewFileHandler creates a new file handler.
func NewFileHandler() *FileHandler {
	return &FileHandler{}
}

// HandleGetFile returns the content of a specific file.
// Usage: GET /files?session_id=xxx&path=contracts/hello-world/src/lib.rs
func (h *FileHandler) HandleGetFile(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	path := r.URL.Query().Get("path")

	if sessionID == "" || path == "" {
		http.Error(w, `{"error":"session_id and path are required"}`, http.StatusBadRequest)
		return
	}

	// Security: Prevent directory traversal (allow only paths inside /app/workspaces)
	if strings.Contains(path, "..") {
		http.Error(w, `{"error":"illegal path"}`, http.StatusForbidden)
		return
	}

	fullPath := filepath.Join("/app/workspaces", sessionID, path)

	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		} else {
			http.Error(w, `{"error":"failed to read file"}`, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

// HandleSaveFile (Optional: if we need to implement saving back later)
func (h *FileHandler) HandleSaveFile(w http.ResponseWriter, r *http.Request) {
	// Not implemented for now — everything still goes through the atomic /run
	http.Error(w, `{"error":"not implemented"}`, http.StatusNotImplemented)
}

// WriteJSON is a helper to write JSON responses.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
