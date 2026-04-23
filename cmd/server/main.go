package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"soroban-studio-backend/internal/executor"
	"soroban-studio-backend/internal/handler"
	"soroban-studio-backend/internal/middleware"
	"soroban-studio-backend/internal/queue"
	"soroban-studio-backend/internal/session"
)

// readFile reads a file and returns its content as a string (empty string on error).
func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// ─── Configuration ───────────────────────────────────────────────
	port := getEnv("PORT", "8080")
	maxWorkers := getEnvInt("MAX_WORKERS", 3)

	// ─── Initialize Components ───────────────────────────────────────
	// Session manager: tracks WebSocket connections per session
	sessionMgr := session.NewManager()

	// Executor: runs docker exec commands inside the Stellar runner
	exec := executor.New(sessionMgr)

	// Worker pool: processes build jobs with limited concurrency
	pool := queue.NewWorkerPool(maxWorkers, exec)
	pool.Start()

	// ─── Setup HTTP Handlers ─────────────────────────────────────────
	runHandler := handler.NewRunHandler(pool, sessionMgr)
	wsHandler := handler.NewWSHandler(sessionMgr)
	githubHandler := handler.NewGitHubHandler()
	templateHandler := handler.NewTemplateHandler("./templates")
	walletHandler := handler.NewWalletHandler()
	interfaceHandler := handler.NewInterfaceHandler()
	hwReadme := readFile("./templates/hello-world/README.md")
	wsReadme := readFile("./templates/stellar-workshop/README.md")
	validateHandler := handler.NewValidateHandler(hwReadme, wsReadme)

	mux := http.NewServeMux()

	// POST /run - Submit files for compilation
	mux.HandleFunc("/run", runHandler.Handle)

	// POST /kill - Interrupt/Kill a running job
	mux.HandleFunc("/kill", runHandler.Kill)

	// GET /ws?session_id=xxx - Stream build output via WebSocket
	mux.HandleFunc("/ws", wsHandler.Handle)

	// GET /templates?name=xxx - Get project template structure
	mux.HandleFunc("/templates", templateHandler.HandleGetTemplate)

	// GitHub proxy endpoints (bypass CORS for Device Flow)
	mux.HandleFunc("/github/device-code", githubHandler.HandleDeviceCode)
	mux.HandleFunc("/github/access-token", githubHandler.HandleAccessToken)
	mux.HandleFunc("/github/repos", githubHandler.HandleUserRepos)
	mux.HandleFunc("/github/api/", githubHandler.HandleProxy)

	// Wallet endpoints
	mux.HandleFunc("/wallet/default/init", walletHandler.HandleInit)
	mux.HandleFunc("/wallet/default/status", walletHandler.HandleStatus)
	mux.HandleFunc("/wallet/freighter/register", walletHandler.HandleRegisterFreighter)

	// Contract interface parser
	mux.HandleFunc("/contract/interface", interfaceHandler.Handle)

	// Project validation
	mux.HandleFunc("/validate/project", validateHandler.Handle)

	// GET /health - Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"soroban-studio-backend"}`))
	})

	// ─── Auto-init default wallet on startup ────────────────────────
	go func() {
		// Wait for runner container to be ready
		time.Sleep(3 * time.Second)
		if _, err := walletHandler.InitDefault(); err != nil {
			log.Printf("[startup] wallet init failed: %v", err)
		} else {
			log.Printf("[startup] default wallet ready")
		}
	}()

	// ─── Start Server ────────────────────────────────────────────────
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      middleware.CORS(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // No timeout for WebSocket connections
		IdleTimeout:  60 * time.Second,
	}

	// Run server in a goroutine so we can handle shutdown signals
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Failed to start server: %v", err)
		}
	}()

	// ─── Graceful Shutdown ───────────────────────────────────────────
	// Wait for interrupt signal (Ctrl+C or Docker stop)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Give in-flight requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop accepting new jobs and wait for workers to finish
	pool.Stop()

	// Shutdown HTTP server
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("❌ Server forced to shutdown: %v", err)
	}

}

// getEnv returns the value of an environment variable, or a default if not set.
func getEnv(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// getEnvInt returns the value of an environment variable as an int, or a default.
func getEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			return n
		}
	}
	return defaultValue
}
