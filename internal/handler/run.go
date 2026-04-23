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
	"stellar", "soroban", "cargo", "git",
}

// dangerousPatterns contains shell metacharacters that indicate injection attempts.
var dangerousPatterns = []string{"&&", "||", ";", "|", ">", "<", "$", "(", ")", "{", "}"}

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

	// Check for dangerous shell metacharacters outside of quoted strings
	inQuote := false
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] == '"' {
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		for _, pattern := range dangerousPatterns {
			if strings.HasPrefix(trimmed[i:], pattern) {
				return "syntax error near unexpected token '" + pattern + "'"
			}
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
		return headerID
	}

	// 2. Fallback: try cookie (works for same-origin deployments)
	cookie, err := r.Cookie("workspace_session")
	if err == nil && cookie.Value != "" {
		sessionPath := filepath.Join(h.workspaceDir, cookie.Value)
		if _, err := os.Stat(sessionPath); err == nil {
			return cookie.Value
		}
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

	// Files are optional if a command is provided (e.g. for 'stellar contract init')
	// But we still create the workspace directory below.

	// Get or create persistent session ID
	// Try to get from cookie first, otherwise create a persistent one
	sessionID := h.getOrCreateSessionID(w, r)

	// Prepare session workspace directory (create if doesn't exist)
	sessionBaseDir := filepath.Join(h.workspaceDir, sessionID)
	if err := os.MkdirAll(sessionBaseDir, 0755); err != nil {
		log.Printf("[handler] failed to create session directory %s: %v", sessionBaseDir, err)
		http.Error(w, `{"error":"failed to create workspace"}`, http.StatusInternalServerError)
		return
	}

	// Calculate absolute WorkDir for the job based on Terminal CWD
	// Terminal CWD looks like "~/project[/subfolder]"
	relPath := strings.TrimPrefix(req.Cwd, "~/project")
	relPath = strings.TrimPrefix(relPath, "/")
	jobWorkDir := filepath.Join(sessionBaseDir, relPath)

	// Ensure the command's execution directory exists
	if err := os.MkdirAll(jobWorkDir, 0755); err != nil {
		log.Printf("[handler] failed to create job workdir %s: %v", jobWorkDir, err)
		http.Error(w, `{"error":"failed to prepare task directory"}`, http.StatusInternalServerError)
		return
	}

	// Smart Workdir Detection: If we are in the root and there's no Cargo.toml, 
	// but there's a unique subdirectory with a Cargo.toml (like after 'init hello-world'),
	// automatically use that subdirectory as the execution context.
	if _, err := os.Stat(filepath.Join(jobWorkDir, "Cargo.toml")); os.IsNotExist(err) {
		entries, err := os.ReadDir(jobWorkDir)
		if err == nil {
			var subDirs []string
			for _, entry := range entries {
				if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
					subDirs = append(subDirs, entry.Name())
				}
			}
			// If exactly one project folder exists, dive into it
			if len(subDirs) == 1 {
				potentialDir := filepath.Join(jobWorkDir, subDirs[0])
				if _, err := os.Stat(filepath.Join(potentialDir, "Cargo.toml")); err == nil {
					jobWorkDir = potentialDir
				}
			}
		}
	}

	// We NO LONGER clear the workspace. With persistent volumes, we only
	// add or update the files explicitly provided by the frontend.
	// This allows lazy loading to work: the backend keeps whatever is on disk.

	// Write files (overwrite existing ones)
	for filename, content := range req.Files {
		filePath := filepath.Join(sessionBaseDir, filename)
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

	// Ensure session exists for WebSocket tracking.
	// Note: We do NOT clear the buffer here. With jobId-based filtering,
	// each WebSocket listener only processes messages from its own job.
	// Clearing the buffer risks wiping undelivered messages (like fileTreeUpdate).
	h.sessionMgr.GetOrCreate(sessionID)

	// Generate a unique JobID for this execution.
	// The frontend uses this to filter WebSocket messages and only display
	// output belonging to the command it triggered (prevents init/build mixing).
	jobID := uuid.New().String()

	// Enqueue the job with the user's command
	h.pool.Enqueue(model.Job{
		SessionID: sessionID,
		JobID:     jobID,
		WorkDir:   jobWorkDir,
		Command:   command,
	})

	// Return the session ID and job ID to the client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(model.RunResponse{
		SessionID: sessionID,
		JobID:     jobID,
	})
}

// Kill processes POST /kill requests to stop a running job.
func (h *RunHandler) Kill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1024) // Small payload expected

	var req model.KillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.JobID == "" {
		http.Error(w, `{"error":"job_id is required"}`, http.StatusBadRequest)
		return
	}

	// Send kill signal to the worker pool
	h.pool.Kill(req.JobID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "job_id": req.JobID})
}
