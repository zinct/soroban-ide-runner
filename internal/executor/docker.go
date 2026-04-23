package executor

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"soroban-studio-backend/internal/model"
	"soroban-studio-backend/internal/session"
)




// Executor handles running `docker exec` commands inside the shared Soroban
// runner container. It streams stdout/stderr in real-time via the session manager.
type Executor struct {
	sessionMgr    *session.Manager
	containerName string
	workspaceDir  string
}

// New creates a new Executor with configuration from environment variables.
func New(sessionMgr *session.Manager) *Executor {
	containerName := os.Getenv("RUNNER_CONTAINER")
	if containerName == "" {
		containerName = "soroban-runner"
	}

	workspaceDir := os.Getenv("WORKSPACE_DIR")
	if workspaceDir == "" {
		workspaceDir = "/app/workspaces"
	}

	return &Executor{
		sessionMgr:    sessionMgr,
		containerName: containerName,
		workspaceDir:  workspaceDir,
	}
}

// Execute runs the user's command inside the runner container for
// the given job. Output is streamed in real-time via WebSocket.
// Supports cancellation via the provided context.
func (e *Executor) Execute(ctx context.Context, job model.Job) {
	command := job.Command
	if command == "" {
		command = "stellar contract build"
	}

	// Parse the command into individual arguments (safe — no shell involved)
	cmdArgs := strings.Fields(command)

	workDir := fmt.Sprintf("/app/workspaces/%s", job.SessionID)

	// Build the docker exec argument list.
	// Each session runs in its own workspace — no shared symlinks, no race conditions.
	// Caching still works because:
	//   - CARGO_TARGET_DIR=/app/target (global, set in Dockerfile)
	//   - sccache caches compiled crates globally
	homeEnv := fmt.Sprintf("HOME=%s", workDir)
	targetEnv := "CARGO_TARGET_DIR=/app/target"
	
	// Explicitly pass cache-related env vars to ensure sccache is active
	dockerArgs := []string{
		"exec",
		"--workdir", workDir,
		"--env", homeEnv,
		"--env", targetEnv,
		"--env", "RUSTC_WRAPPER=sccache",
		"--env", "SCCACHE_DIR=/app/sccache",
		"--env", "CARGO_HOME=/app/cargo",
	}

	// Inject PORT if assigned (for dev servers)
	if job.Port > 0 {
		dockerArgs = append(dockerArgs, "--env", fmt.Sprintf("PORT=%d", job.Port))
		dockerArgs = append(dockerArgs, "--env", fmt.Sprintf("VITE_PORT=%d", job.Port))
		// Ensure Vite/Vite-based tools listen on all interfaces so the proxy can reach them
		dockerArgs = append(dockerArgs, "--env", "VITE_HOST=0.0.0.0")
		// Inform Vite about its base path when behind the preview proxy
		dockerArgs = append(dockerArgs, "--env", fmt.Sprintf("VITE_BASE_URL=/preview/%s/", job.SessionID))
	}

	dockerArgs = append(dockerArgs, e.containerName)
	dockerArgs = append(dockerArgs, cmdArgs...)

	// Use CommandContext to allow the process to be killed when the context is cancelled
	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)

	// Create pipes for real-time output streaming
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.sendError(job.SessionID, job.JobID, fmt.Sprintf("Failed to create stdout pipe: %v", err))
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		e.sendError(job.SessionID, job.JobID, fmt.Sprintf("Failed to create stderr pipe: %v", err))
		return
	}

	// Start the process (non-blocking)
	if err := cmd.Start(); err != nil {
		e.sendError(job.SessionID, job.JobID, fmt.Sprintf("Failed to start command: %v", err))
		return
	}

	// Stream stdout and stderr concurrently using goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	// Stream stdout
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			e.sessionMgr.Send(job.SessionID, model.OutputMessage{
				Type:    "stdout",
				Content: scanner.Text(),
				JobID:   job.JobID,
			})
		}
	}()

	// Stream stderr (Cargo/Rust output goes to stderr)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			e.sessionMgr.Send(job.SessionID, model.OutputMessage{
				Type:    "stderr",
				Content: scanner.Text(),
				JobID:   job.JobID,
			})
		}
	}()

	// Wait for all output to be read before calling cmd.Wait()
	wg.Wait()

	// Wait for the process to exit
	err = cmd.Wait()

	// ─── POST-COMMAND CLEANUP ────────────────────────────────────
	// Especially importante for long-running processes like 'npm run dev'
	// When the context is cancelled (Kill signal), we must ensure all 
	// children inside the container are also terminated.
	e.cleanup(job.SessionID)

	// Check the result
	if err != nil {
		// Only send error if it wasn't a manual cancellation
		if ctx.Err() == nil {
			e.sessionMgr.Send(job.SessionID, model.OutputMessage{
				Type:    "error",
				Content: fmt.Sprintf("Command failed: %v", err),
				JobID:   job.JobID,
			})
		}
	}

	// NOTE: Workspace is NOT cleaned up here.
	// It persists so the user can run more commands.

		// POST-COMMAND HOOKS:
		// [DEPRECATED] Backend-to-Frontend sync is disabled as per user architecture.
		// The frontend is now the sole source of truth.

	// Signal that execution is complete
	e.sessionMgr.Send(job.SessionID, model.OutputMessage{
		Type:    "done",
		Content: "",
		JobID:   job.JobID,
	})
}

// scanDirectoryWithRetry attempts to scan a directory multiple times if it appears empty.
// This helps mitigate Docker volume synchronization delays between containers.
func (e *Executor) scanDirectoryWithRetry(dirPath string, retries int) ([]model.FileTreeNode, error) {
	for i := 0; i <= retries; i++ {
		tree, err := e.scanDirectory(dirPath)
		// If we found files, or if there was a real error (not just empty), return
		if err == nil && len(tree) > 0 {
			return tree, nil
		}
		if err != nil {
			return nil, err
		}

		if i < retries {
			log.Printf("[executor] scan result empty for %s, retrying (%d/%d)...", dirPath, i+1, retries)
			time.Sleep(300 * time.Millisecond)
		}
	}
	return e.scanDirectory(dirPath)
}

// scanDirectory recursively walks a directory and returns a tree of FileTreeNodes.
func (e *Executor) scanDirectory(dirPath string) ([]model.FileTreeNode, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var nodes []model.FileTreeNode
	for _, entry := range entries {
		name := entry.Name()
		// Skip hidden files, target, and git directories
		if strings.HasPrefix(name, ".") || name == "target" || name == ".git" {
			continue
		}

		fullPath := filepath.Join(dirPath, name)
		node := model.FileTreeNode{
			Name: name,
		}

		if entry.IsDir() {
			node.Type = "folder"
			children, err := e.scanDirectory(fullPath)
			if err != nil {
				return nil, err
			}
			node.Children = children
		} else {
			node.Type = "file"
			// Read file content and store it in the node
			content, err := os.ReadFile(fullPath)
			if err == nil {
				node.Content = string(content)
			}
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// sendError sends an error message followed by a done signal.
func (e *Executor) sendError(sessionID, jobID, msg string) {
	log.Printf("[executor] error for session %s: %s", sessionID, msg)
	e.sessionMgr.Send(sessionID, model.OutputMessage{
		Type:    "error",
		Content: msg,
		JobID:   jobID,
	})
	e.sessionMgr.Send(sessionID, model.OutputMessage{
		Type:    "done",
		Content: "",
		JobID:   jobID,
	})
}

// cleanup ensures that no orphaned processes remain in the session's workspace inside the container.
// It uses pkill -f to target any process whose command line contains the session's absolute path.
func (e *Executor) cleanup(sessionID string) {
	// Root workspace path inside the container: /app/workspaces/:sessionID
	workspacePath := filepath.Join(e.workspaceDir, sessionID)

	// Command: pkill -f /app/workspaces/:sessionID
	// -f matches the full command line
	// This will kill node, vite, cargo, etc. started for this session.
	cleanupCmd := exec.Command("docker", "exec", e.containerName, "pkill", "-f", workspacePath)
	if err := cleanupCmd.Run(); err != nil {
		// pkill returns 1 if no processes were matched, which is fine
		log.Printf("[executor] cleanup for %s (no procs killed or err): %v", sessionID, err)
	} else {
		log.Printf("[executor] cleanup success for %s", sessionID)
	}
}
