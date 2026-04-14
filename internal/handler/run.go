package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"soroban-studio-backend/internal/model"
	"soroban-studio-backend/internal/queue"
	"soroban-studio-backend/internal/session"

	"github.com/google/uuid"
)

// allowedPrefixes defines which command prefixes are permitted.
var allowedPrefixes = []string{
	"stellar", "soroban", "cargo",
}

// dangerousPatterns contains shell metacharacters that indicate injection attempts.
var dangerousPatterns = []string{"&&", "||", ";", "|", ">", "<", "$", "`", "(", ")", "{", "}"}

// RunHandler handles the POST /run endpoint.
// It receives project files + a command, writes them to a workspace, and enqueues a job.
type RunHandler struct {
	pool         *queue.WorkerPool
	sessionMgr   *session.Manager
	workspaceDir string
}

// NewRunHandler creates a new RunHandler with the given dependencies.
func NewRunHandler(pool *queue.WorkerPool, sessionMgr *session.Manager) *RunHandler {
	workspaceDir := os.Getenv("WORKSPACE_DIR")
	if workspaceDir == "" {
		workspaceDir = "/app/workspaces"
	}

	return &RunHandler{
		pool:         pool,
		sessionMgr:   sessionMgr,
		workspaceDir: workspaceDir,
	}
}

// validateCommand checks the command for safety.
// Returns an error message if invalid, or empty string if OK.
func validateCommand(command string) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "no command provided"
	}

	// Check for dangerous shell metacharacters
	for _, pattern := range dangerousPatterns {
		if strings.Contains(trimmed, pattern) {
			return "syntax error near unexpected token '" + pattern + "'"
		}
	}

	// Parse into words
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return "empty command"
	}

	// Validate prefix
	prefix := strings.ToLower(parts[0])
	allowed := false
	for _, ap := range allowedPrefixes {
		if prefix == ap {
			allowed = true
			break
		}
	}

	if !allowed {
		return parts[0] + ": command not found"
	}

	return ""
}

// sanitizeCommand applies automatic safety modifications to commands.
// - npm install/i → appends --ignore-scripts
func sanitizeCommand(command string) string {
	parts := strings.Fields(command)
	if len(parts) < 2 {
		return command
	}

	prefix := strings.ToLower(parts[0])
	sub := strings.ToLower(parts[1])

	// npm install safety: append --ignore-scripts if not already present
	if prefix == "npm" && (sub == "install" || sub == "i") {
		hasFlag := false
		for _, p := range parts {
			if p == "--ignore-scripts" {
				hasFlag = true
				break
			}
		}
		if !hasFlag {
			command = command + " --ignore-scripts"
			log.Printf("[handler] npm safety: appended --ignore-scripts")
		}
	}

	return command
}

// getOrCreateSessionID retrieves session ID from header, cookie, or creates a new one.
// Priority: X-Session-ID header > cookie > new UUID.
func (h *RunHandler) getOrCreateSessionID(w http.ResponseWriter, r *http.Request) string {
	// 1. Check X-Session-ID header (sent by frontend via localStorage)
	//    This is the primary method for cross-origin deployments (e.g., Vercel + Railway)
	if headerID := r.Header.Get("X-Session-ID"); headerID != "" {
		log.Printf("[handler] using session from X-Session-ID header: %s", headerID)
		return headerID
	}

	// 2. Fallback: try cookie (works for same-origin deployments)
	cookie, err := r.Cookie("workspace_session")
	if err == nil && cookie.Value != "" {
		sessionPath := filepath.Join(h.workspaceDir, cookie.Value)
		if _, err := os.Stat(sessionPath); err == nil {
			log.Printf("[handler] using existing session from cookie: %s", cookie.Value)
			return cookie.Value
		}
		log.Printf("[handler] session directory not found, creating new session")
	}

	// 3. Last resort: generate a new UUID
	sessionID := uuid.New().String()

	http.SetCookie(w, &http.Cookie{
		Name:     "workspace_session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60, // 30 days
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	log.Printf("[handler] created new persistent session: %s", sessionID)
	return sessionID
}

// Handle processes POST /run requests.
//
// Request body:
//
//	{
//	  "command": "stellar contract build",
//	  "files": {
//	    "Cargo.toml": "...",
//	    "src/lib.rs": "..."
//	  }
//	}
//
// Response:
//
//	{ "session_id": "abc12345" }
func (h *RunHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Limit the total request body size to 10MB
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)

	// Parse request body
	var req model.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[handler] invalid request body: %v", err)
		http.Error(w, `{"error":"invalid request body or file too large"}`, http.StatusBadRequest)
		return
	}

	// Validate files: size and extensions
	for filename, content := range req.Files {
		// Individual file limit: 1MB
		if len(content) > 1024*1024 {
			log.Printf("[handler] file too large rejected: %s (%d bytes)", filename, len(content))
			http.Error(w, `{"error":"file too large: `+filename+`"}`, http.StatusBadRequest)
			return
		}

		// Extension check (secondary defense)
		ext := strings.ToLower(filepath.Ext(filename))
		isBlocked := false
		blockedExts := []string{".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico", ".exe", ".bin"}
		for _, b := range blockedExts {
			if ext == b {
				isBlocked = true
				break
			}
		}
		if isBlocked {
			log.Printf("[handler] blocked file type rejected: %s", filename)
			http.Error(w, `{"error":"file type not allowed: `+filename+`"}`, http.StatusBadRequest)
			return
		}
	}

	// Validate the command
	command := strings.TrimSpace(req.Command)
	if command == "" {
		// Backward compatibility: default to "stellar contract build" if no command provided
		command = "stellar contract build"
	}

	if errMsg := validateCommand(command); errMsg != "" {
		log.Printf("[handler] command rejected: %s (input: %q)", errMsg, command)
		http.Error(w, `{"error":"`+errMsg+`"}`, http.StatusBadRequest)
		return
	}

	// Sanitize: auto-append safety flags
	command = sanitizeCommand(command)

	log.Printf("[handler] received command: %q", command)

	// Files are optional if a command is provided (e.g. for 'stellar contract init')
	// But we still create the workspace directory below.

	// Get or create persistent session ID
	// Try to get from cookie first, otherwise create a persistent one
	sessionID := h.getOrCreateSessionID(w, r)

	// Create workspace directory structure and write files
	workDir := filepath.Join(h.workspaceDir, sessionID)

	// Clear existing workspace if it exists, but preserve .config (identities)
	if entries, err := os.ReadDir(workDir); err == nil {
		log.Printf("[handler] selectively clearing workspace: %s", workDir)
		for _, entry := range entries {
			// Preserve .config (identities) and target (build cache)
			if entry.Name() == ".config" || entry.Name() == "target" {
				continue // Preserve identities and build artifacts
			}
			os.RemoveAll(filepath.Join(workDir, entry.Name()))
		}
	}

	// Create workspace directory if it doesn't exist
	if err := os.MkdirAll(workDir, 0755); err != nil {
		log.Printf("[handler] failed to create workspace directory %s: %v", workDir, err)
		http.Error(w, `{"error":"failed to create workspace"}`, http.StatusInternalServerError)
		return
	}

	// Write files (overwrite existing ones)
	for filename, content := range req.Files {
		filePath := filepath.Join(workDir, filename)
		dir := filepath.Dir(filePath)

		// Create parent directories (e.g., src/)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("[handler] failed to create directory %s: %v", dir, err)
			http.Error(w, `{"error":"failed to create workspace"}`, http.StatusInternalServerError)
			return
		}

		// Write file content
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			log.Printf("[handler] failed to write file %s: %v", filePath, err)
			http.Error(w, `{"error":"failed to write files"}`, http.StatusInternalServerError)
			return
		}
	}

	log.Printf("[handler] workspace created: session=%s, files=%d, command=%q", sessionID, len(req.Files), command)

	// Create a session for WebSocket tracking (must be done before enqueueing
	// so the session exists when the worker starts sending output)
	h.sessionMgr.Create(sessionID)

	// Enqueue the job with the user's command
	h.pool.Enqueue(model.Job{
		SessionID: sessionID,
		WorkDir:   workDir,
		Command:   command,
	})

	// Return the session ID to the client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(model.RunResponse{
		SessionID: sessionID,
	})
}
