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

	// Parse command respecting quoted strings (e.g. --arg "hello world")
	cmdArgs := splitArgs(command)

	workDir := fmt.Sprintf("/app/workspaces/%s", job.SessionID)

	// Keep HOME=/root so stellar CLI can find identity keys in /root/.config/stellar/
	// Use CARGO_HOME separately for per-session cargo isolation
	homeEnv := "HOME=/root"
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
		e.containerName,
	}
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

	// Wait for the process to exit and check the result
	if err := cmd.Wait(); err != nil {
		e.sessionMgr.Send(job.SessionID, model.OutputMessage{
			Type:    "error",
			Content: fmt.Sprintf("Command failed: %v", err),
			JobID:   job.JobID,
		})
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

// splitArgs splits a command string into arguments, respecting double-quoted strings.
func splitArgs(s string) []string {
	var args []string
	var cur []byte
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQuote = !inQuote
		} else if c == ' ' && !inQuote {
			if len(cur) > 0 {
				args = append(args, string(cur))
				cur = cur[:0]
			}
		} else {
			cur = append(cur, c)
		}
	}
	if len(cur) > 0 {
		args = append(args, string(cur))
	}
	return args
}
