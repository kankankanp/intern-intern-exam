package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"url-collector/internal/domain"
	"url-collector/internal/httpclient"
	"url-collector/internal/repository"
)

// Handler handles HTTP requests
type Handler struct {
	urlRepo   *repository.URLRepository
	runRepo   *repository.RunRepository
	ssrfGuard *httpclient.SSRFGuard
	logger    *slog.Logger
}

// NewHandler creates a new handler
func NewHandler(db *sql.DB, logger *slog.Logger) *Handler {
	return &Handler{
		urlRepo:   repository.NewURLRepository(db),
		runRepo:   repository.NewRunRepository(db),
		ssrfGuard: httpclient.NewSSRFGuard(),
		logger:    logger,
	}
}

// Response structures
type response struct {
	Data  interface{} `json:"data,omitempty"`
	Error *errorResp  `json:"error,omitempty"`
}

type errorResp struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type paginationResp struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
}

// Request structures
type createURLRequest struct {
	URL              string   `json:"url"`
	Tags             []string `json:"tags"`
	IntervalSeconds  int      `json:"intervalSeconds"`
	Enabled          *bool    `json:"enabled"`
	ConcurrencyGroup *string  `json:"maxConcurrencyGroup"`
}

type updateURLRequest struct {
	Tags             *[]string `json:"tags"`
	IntervalSeconds  *int      `json:"intervalSeconds"`
	Enabled          *bool     `json:"enabled"`
	ConcurrencyGroup *string   `json:"maxConcurrencyGroup"`
}

// Helper functions
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response{Data: data})
}

func (h *Handler) writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response{Error: &errorResp{Code: code, Message: message}})
}

func (h *Handler) getIDFromPath(path string) (int64, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return 0, sql.ErrNoRows
	}
	return strconv.ParseInt(parts[1], 10, 64)
}

// Handlers

// HandleHealth handles GET /health
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "healthy",
		"database":  "connected",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleCreateURL handles POST /urls
func (h *Handler) HandleCreateURL(w http.ResponseWriter, r *http.Request) {
	var req createURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	// Validate URL
	if req.URL == "" {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "url is required")
		return
	}

	// SSRF validation
	if err := h.ssrfGuard.ValidateURL(req.URL); err != nil {
		h.writeError(w, http.StatusBadRequest, "SSRF_BLOCKED", err.Error())
		return
	}

	// Validate interval
	if req.IntervalSeconds < 60 {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "intervalSeconds must be at least 60")
		return
	}

	// Normalize URL
	normalizedURL, err := domain.NormalizeURL(req.URL)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_URL", err.Error())
		return
	}

	// Create URL entity
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	url := &domain.URL{
		URL:              req.URL,
		NormalizedURL:    normalizedURL,
		Enabled:          enabled,
		IntervalSeconds:  req.IntervalSeconds,
		Tags:             tags,
		ConcurrencyGroup: req.ConcurrencyGroup,
	}

	// Save to database
	if err := h.urlRepo.Create(r.Context(), url); err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			h.writeError(w, http.StatusConflict, "URL_DUPLICATE", "URL already exists")
			return
		}
		h.logger.Error("failed to create URL", "error", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create URL")
		return
	}

	h.writeJSON(w, http.StatusCreated, url)
}

// HandleListURLs handles GET /urls
func (h *Handler) HandleListURLs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	params := repository.URLListParams{
		Query:    query.Get("q"),
		Tag:      query.Get("tag"),
		Page:     1,
		PageSize: 20,
	}

	if enabled := query.Get("enabled"); enabled != "" {
		b := enabled == "true"
		params.Enabled = &b
	}

	if page := query.Get("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			params.Page = p
		}
	}

	if pageSize := query.Get("pageSize"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil {
			params.PageSize = ps
		}
	}

	urls, total, err := h.urlRepo.List(r.Context(), params)
	if err != nil {
		h.logger.Error("failed to list URLs", "error", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list URLs")
		return
	}

	totalPages := (total + params.PageSize - 1) / params.PageSize

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": urls,
		"pagination": paginationResp{
			Page:       params.Page,
			PageSize:   params.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// HandleGetURL handles GET /urls/{id}
func (h *Handler) HandleGetURL(w http.ResponseWriter, r *http.Request) {
	id, err := h.getIDFromPath(r.URL.Path)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid URL ID")
		return
	}

	url, err := h.urlRepo.GetByID(r.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			h.writeError(w, http.StatusNotFound, "URL_NOT_FOUND", "URL not found")
			return
		}
		h.logger.Error("failed to get URL", "error", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get URL")
		return
	}

	h.writeJSON(w, http.StatusOK, url)
}

// HandleUpdateURL handles PATCH /urls/{id}
func (h *Handler) HandleUpdateURL(w http.ResponseWriter, r *http.Request) {
	id, err := h.getIDFromPath(r.URL.Path)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid URL ID")
		return
	}

	var req updateURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	url, err := h.urlRepo.GetByID(r.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			h.writeError(w, http.StatusNotFound, "URL_NOT_FOUND", "URL not found")
			return
		}
		h.logger.Error("failed to get URL", "error", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get URL")
		return
	}

	// Apply updates
	if req.Enabled != nil {
		url.Enabled = *req.Enabled
	}
	if req.IntervalSeconds != nil {
		if *req.IntervalSeconds < 60 {
			h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "intervalSeconds must be at least 60")
			return
		}
		url.IntervalSeconds = *req.IntervalSeconds
	}
	if req.Tags != nil {
		url.Tags = *req.Tags
	}
	if req.ConcurrencyGroup != nil {
		url.ConcurrencyGroup = req.ConcurrencyGroup
	}

	if err := h.urlRepo.Update(r.Context(), url); err != nil {
		h.logger.Error("failed to update URL", "error", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update URL")
		return
	}

	h.writeJSON(w, http.StatusOK, url)
}

// HandleDeleteURL handles DELETE /urls/{id}
func (h *Handler) HandleDeleteURL(w http.ResponseWriter, r *http.Request) {
	id, err := h.getIDFromPath(r.URL.Path)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid URL ID")
		return
	}

	if err := h.urlRepo.Delete(r.Context(), id); err != nil {
		h.logger.Error("failed to delete URL", "error", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete URL")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetLatestRun handles GET /urls/{id}/latest
func (h *Handler) HandleGetLatestRun(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path like /urls/123/latest
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid path")
		return
	}

	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid URL ID")
		return
	}

	// Verify URL exists
	if _, err := h.urlRepo.GetByID(r.Context(), id); err != nil {
		if err == sql.ErrNoRows {
			h.writeError(w, http.StatusNotFound, "URL_NOT_FOUND", "URL not found")
			return
		}
		h.logger.Error("failed to get URL", "error", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get URL")
		return
	}

	run, err := h.runRepo.GetLatest(r.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			h.writeError(w, http.StatusNotFound, "URL_NOT_FOUND", "no runs found")
			return
		}
		h.logger.Error("failed to get latest run", "error", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get latest run")
		return
	}

	h.writeJSON(w, http.StatusOK, run)
}

// HandleListRuns handles GET /urls/{id}/runs
func (h *Handler) HandleListRuns(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid path")
		return
	}

	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid URL ID")
		return
	}

	// Verify URL exists
	if _, err := h.urlRepo.GetByID(r.Context(), id); err != nil {
		if err == sql.ErrNoRows {
			h.writeError(w, http.StatusNotFound, "URL_NOT_FOUND", "URL not found")
			return
		}
		h.logger.Error("failed to get URL", "error", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get URL")
		return
	}

	query := r.URL.Query()
	params := repository.RunListParams{
		URLID:    id,
		Page:     1,
		PageSize: 20,
	}

	if from := query.Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			params.From = &t
		}
	}

	if to := query.Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			params.To = &t
		}
	}

	if status := query.Get("status"); status != "" {
		s := domain.RunStatus(status)
		params.Status = &s
	}

	if page := query.Get("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			params.Page = p
		}
	}

	if pageSize := query.Get("pageSize"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil {
			params.PageSize = ps
		}
	}

	runs, total, err := h.runRepo.List(r.Context(), params)
	if err != nil {
		h.logger.Error("failed to list runs", "error", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list runs")
		return
	}

	totalPages := (total + params.PageSize - 1) / params.PageSize

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": runs,
		"pagination": paginationResp{
			Page:       params.Page,
			PageSize:   params.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// HandleEnqueue handles POST /urls/{id}/enqueue
func (h *Handler) HandleEnqueue(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid path")
		return
	}

	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid URL ID")
		return
	}

	if err := h.urlRepo.Enqueue(r.Context(), id); err != nil {
		if err == sql.ErrNoRows {
			h.writeError(w, http.StatusNotFound, "URL_NOT_FOUND", "URL not found or disabled")
			return
		}
		h.logger.Error("failed to enqueue URL", "error", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to enqueue URL")
		return
	}

	h.writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"message":    "URL enqueued for immediate processing",
		"urlId":      id,
		"enqueuedAt": time.Now().UTC().Format(time.RFC3339),
	})
}
