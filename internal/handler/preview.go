package handler

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"soroban-studio-backend/internal/port"
)

// PreviewHandler implements a reverse proxy for frontend previews.
// It maps /preview/:sessionID/* to a local port running a dev server.
type PreviewHandler struct {
	portMgr    *port.Manager
	runnerHost string
}

// NewPreviewHandler creates a new PreviewHandler.
func NewPreviewHandler(portMgr *port.Manager, runnerHost string) *PreviewHandler {
	return &PreviewHandler{
		portMgr:    portMgr,
		runnerHost: runnerHost,
	}
}

// Handle processes the preview request by proxying it to the correct port.
// Uses Go 1.22+ path parameters: /preview/{sessionID}/{path...}
func (h *PreviewHandler) Handle(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, `{"error":"sessionID is required"}`, http.StatusBadRequest)
		return
	}

	p := h.portMgr.GetPort(sessionID)
	if p == 0 {
		// Session exists but no port assigned yet.
		// This can happen if the dev server isn't running.
		http.Error(w, `{"error":"dev server not running for this session"}`, http.StatusNotFound)
		return
	}

	targetURL := fmt.Sprintf("http://%s:%d", h.runnerHost, p)
	target, err := url.Parse(targetURL)
	if err != nil {
		log.Printf("[preview] invalid target URL: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Create the reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize the director to strip the preview prefix and set headers
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		
		// Keep the full original path (including /preview/sessionID/)
		// because Vite is now configured with the same base path.
		req.URL.Path = r.URL.Path
		if req.URL.RawPath != "" {
			req.URL.RawPath = r.URL.RawPath
		}

		// Important: Set the Host header so Vite/React dev server doesn't reject the request
		req.Host = target.Host

		// Ensure WebSocket headers are preserved (ReverseProxy handles the hijacking)
		if req.Header.Get("Upgrade") == "websocket" {
			req.Header.Set("Connection", "Upgrade")
		}
	}

	// Serve the request
	proxy.ServeHTTP(w, r)
}
