package middleware

import "net/http"

// CORS wraps an http.Handler with permissive CORS headers.
// This allows the frontend (running on a different port/domain) to
// communicate with the backend API.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow all origins
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Allow common methods
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE, PATCH")

		// Allow headers, including Authorization for SSO
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, X-Session-ID, Authorization, X-Requested-With, Origin")

		// Ensure Vary: Origin is set so browsers cache responses correctly per-origin
		w.Header().Set("Vary", "Origin")

		// Disable caching of preflight response to ensure changes take effect immediately
		w.Header().Set("Access-Control-Max-Age", "0")

		// Handle preflight OPTIONS requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
