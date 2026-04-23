package model

// RunRequest represents the incoming request from a client.
// Files is a map of relative file paths to their content.
// Command is the CLI command to execute (e.g. "stellar contract build", "stellar --version").
type RunRequest struct {
	Files   map[string]string `json:"files"`
	Cwd     string            `json:"cwd"`
	Command string            `json:"command"`
}

// RunResponse is returned after a job has been enqueued.
type RunResponse struct {
	SessionID string `json:"session_id"`
	JobID     string `json:"job_id"`
}

// KillRequest represents a payload to stop a running job.
type KillRequest struct {
	SessionID string `json:"session_id"`
	JobID     string `json:"job_id"`
}

// OutputMessage represents a single output chunk sent via WebSocket.
// Type can be: "stdout", "stderr", "info", "error", "done", "fileTreeUpdate"
// JobID tags the message so the frontend can filter by job.
type OutputMessage struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	JobID   string `json:"job_id,omitempty"`
}

// Job represents a queued job ready for processing.
type Job struct {
	SessionID string
	JobID     string
	WorkDir   string
	Command   string // The validated command string to execute
	Port      int    // Assigned port for dev servers
}

// FileTreeNode represents a file or folder in the scanned workspace.
// Used for fileTreeUpdate WebSocket messages and GET /files responses.
type FileTreeNode struct {
	Id       string         `json:"id"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`               // "file" or "folder"
	Content  string         `json:"content,omitempty"`   // File content (for files)
	Lazy     bool           `json:"lazy,omitempty"`      // true for large dirs
	Children []FileTreeNode `json:"children,omitempty"`
}
