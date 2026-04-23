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
}

// FileTreeNode represents a file or folder in the scanned workspace.
// Used for fileTreeUpdate WebSocket messages and GET /files responses.
type FileTreeNode struct {
	Id       string         `json:"id"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`             // "file" or "folder"
	Content  string         `json:"content,omitempty"` // File content (for files)
	Lazy     bool           `json:"lazy,omitempty"`    // true for large dirs
	Children []FileTreeNode `json:"children,omitempty"`
}

// ─── Wallet ──────────────────────────────────────────────────────────────────

// WalletStatusResponse is returned by GET /wallet/default/status.
type WalletStatusResponse struct {
	Exists  bool   `json:"exists"`
	Funded  bool   `json:"funded"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Balance string `json:"balance"` // XLM balance, e.g. "9999.9999900"
}

// ─── Contract Interface ───────────────────────────────────────────────────────

// ContractFn describes a single parsed Rust pub fn from a contract.
type ContractFn struct {
	Name     string       `json:"name"`
	Params   []FnParam    `json:"params"`
	Category string       `json:"category"` // "read" | "write" | "unknown"
}

// FnParam is a single parameter of a contract function.
type FnParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// InterfaceRequest is the body for POST /contract/interface.
type InterfaceRequest struct {
	Files map[string]string `json:"files"` // relative path → content
}

// InterfaceResponse is returned by POST /contract/interface.
type InterfaceResponse struct {
	Functions []ContractFn `json:"functions"`
}

// ─── Validation ───────────────────────────────────────────────────────────────

// ValidateRequest is the body for POST /validate/project.
type ValidateRequest struct {
	Files    map[string]string `json:"files"`    // relative path → content
	Category string            `json:"category"` // "ec-level" | "full-stack"
	RepoName string            `json:"repo_name"`
}

// CheckResult is a single validation check result.
type CheckResult struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Status   string `json:"status"`   // "pass" | "fail" | "warn"
	Message  string `json:"message"`
	FixHint  string `json:"fix_hint"`
	Evidence string `json:"evidence,omitempty"`
}

// ValidateResponse is returned by POST /validate/project.
type ValidateResponse struct {
	Category string        `json:"category"`
	Status   string        `json:"status"` // "valid" | "invalid"
	Checks   []CheckResult `json:"checks"`
	Remarks  string        `json:"remarks"`
}
