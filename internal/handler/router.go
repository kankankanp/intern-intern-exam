package handler

import (
	"net/http"
	"strings"
)

// Router handles HTTP routing
type Router struct {
	handler *Handler
}

// NewRouter creates a new router
func NewRouter(handler *Handler) *Router {
	return &Router{handler: handler}
}

// ServeHTTP implements http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	method := req.Method

	// Health check
	if path == "/health" && method == http.MethodGet {
		r.handler.HandleHealth(w, req)
		return
	}

	// URL endpoints
	if strings.HasPrefix(path, "/urls") {
		r.handleURLs(w, req)
		return
	}

	http.NotFound(w, req)
}

func (r *Router) handleURLs(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	method := req.Method

	// POST /urls - Create URL
	if path == "/urls" && method == http.MethodPost {
		r.handler.HandleCreateURL(w, req)
		return
	}

	// GET /urls - List URLs
	if path == "/urls" && method == http.MethodGet {
		r.handler.HandleListURLs(w, req)
		return
	}

	// Parse path parts
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// /urls/{id}
	if len(parts) == 2 {
		switch method {
		case http.MethodGet:
			r.handler.HandleGetURL(w, req)
		case http.MethodPatch:
			r.handler.HandleUpdateURL(w, req)
		case http.MethodDelete:
			r.handler.HandleDeleteURL(w, req)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// /urls/{id}/latest
	if len(parts) == 3 && parts[2] == "latest" && method == http.MethodGet {
		r.handler.HandleGetLatestRun(w, req)
		return
	}

	// /urls/{id}/runs
	if len(parts) == 3 && parts[2] == "runs" && method == http.MethodGet {
		r.handler.HandleListRuns(w, req)
		return
	}

	// /urls/{id}/enqueue
	if len(parts) == 3 && parts[2] == "enqueue" && method == http.MethodPost {
		r.handler.HandleEnqueue(w, req)
		return
	}

	http.NotFound(w, req)
}
