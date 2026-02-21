package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// Server is the user-facing HTTP server that proxies requests to llama-server.
type Server struct {
	port       int
	backendURL string
	modelName  string
	proxy      *httputil.ReverseProxy
}

// NewServer creates an API server that proxies inference to the backend.
func NewServer(port int, backendURL, modelName string) *Server {
	target, _ := url.Parse(backendURL)
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Ensure streaming works: disable response buffering
	proxy.FlushInterval = 50 * time.Millisecond

	return &Server{
		port:       port,
		backendURL: backendURL,
		modelName:  modelName,
		proxy:      proxy,
	}
}

// ListenAndServe starts the HTTP server (blocking).
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/chat/completions", s.cors(s.proxyPost))
	mux.HandleFunc("/v1/completions", s.cors(s.proxyPost))
	mux.HandleFunc("/v1/embeddings", s.cors(s.proxyPost))
	mux.HandleFunc("/v1/models", s.cors(s.handleModels))
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", s.handleRoot)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}
	return srv.ListenAndServe()
}

// ------- handlers -------

func (s *Server) proxyPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}
	s.proxy.ServeHTTP(w, r)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	list := ModelList{
		Object: "list",
		Data: []ModelInfo{
			{
				ID:      s.modelName,
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "local",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get(s.backendURL + "/health")
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, "backend unavailable", "server_error")
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	info := map[string]interface{}{
		"name":    "LLM Gateway",
		"version": "1.0.0",
		"model":   s.modelName,
		"endpoints": map[string]string{
			"chat_completions": "/v1/chat/completions",
			"completions":      "/v1/completions",
			"models":           "/v1/models",
			"health":           "/health",
		},
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(info)
}

// ------- helpers -------

func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg, errType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{Message: msg, Type: errType},
	})
}
