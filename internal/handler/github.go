package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

// GitHubHandler proxies GitHub OAuth Device Flow requests to bypass CORS.
type GitHubHandler struct{}

func NewGitHubHandler() *GitHubHandler {
	return &GitHubHandler{}
}

// HandleDeviceCode proxies POST /github/device-code → GitHub Device Flow initiation.
func (h *GitHubHandler) HandleDeviceCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	req, err := http.NewRequest("POST", "https://github.com/login/device/code", strings.NewReader(string(body)))
	if err != nil {
		http.Error(w, `{"error":"failed to create request"}`, http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[GitHub Proxy] Device code request failed: %v", err)
		http.Error(w, `{"error":"failed to contact GitHub"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// HandleAccessToken proxies POST /github/access-token → GitHub token polling.
func (h *GitHubHandler) HandleAccessToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", strings.NewReader(string(body)))
	if err != nil {
		http.Error(w, `{"error":"failed to create request"}`, http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[GitHub Proxy] Access token request failed: %v", err)
		http.Error(w, `{"error":"failed to contact GitHub"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// HandleUserRepos proxies GET /github/repos → list user repos (authenticated).
func (h *GitHubHandler) HandleUserRepos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
		return
	}

	page := r.URL.Query().Get("page")
	if page == "" {
		page = "1"
	}

	apiURL := "https://api.github.com/user/repos?sort=updated&per_page=30&page=" + page

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		http.Error(w, `{"error":"failed to create request"}`, http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[GitHub Proxy] Repos request failed: %v", err)
		http.Error(w, `{"error":"failed to contact GitHub"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// HandleGitHubProxy is a generic proxy for GitHub API requests.
// It forwards any request to api.github.com with the provided auth token.
func (h *GitHubHandler) HandleProxy(w http.ResponseWriter, r *http.Request) {
	// Extract the GitHub API path from the request
	path := strings.TrimPrefix(r.URL.Path, "/github/api")
	if path == "" {
		path = "/"
	}

	apiURL := "https://api.github.com" + path
	if r.URL.RawQuery != "" {
		apiURL += "?" + r.URL.RawQuery
	}

	// Read the original request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(r.Method, apiURL, bodyReader)
	if err != nil {
		log.Printf("[GitHub Proxy] Request creation failed: %v", err)
		http.Error(w, `{"error":"failed to create github request"}`, http.StatusInternalServerError)
		return
	}

	// Forward client headers
	for k, v := range r.Header {
		if k == "Authorization" || k == "Content-Type" || k == "Accept" {
			req.Header.Set(k, v[0])
		}
	}

	// Ensure required headers are set
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/vnd.github.v3+json")
	}
	req.Header.Set("User-Agent", "Soroban-Studio-Backend")

	// Execute request to GitHub
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[GitHub Proxy] API request to %s failed: %v", apiURL, err)
		http.Error(w, `{"error":"failed to contact GitHub API"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// Log detailed error from GitHub
	if resp.StatusCode >= 400 {
		log.Printf("[GitHub Proxy] GitHub returned error %d for %s: %s", resp.StatusCode, apiURL, string(respBody))
	}

	// Forward response headers back to client
	for k, v := range resp.Header {
		if k != "Content-Length" && k != "Connection" && k != "Server" {
			w.Header().Set(k, v[0])
		}
	}

	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}
