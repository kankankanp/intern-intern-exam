package service

import (
	"context"
	"database/sql"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"url-collector/internal/config"
	"url-collector/internal/domain"
	"url-collector/internal/httpclient"
	"url-collector/internal/repository"

	"golang.org/x/time/rate"
)

// Worker handles URL collection scheduling and execution
type Worker struct {
	urlRepo        *repository.URLRepository
	runRepo        *repository.RunRepository
	fetcher        *Fetcher
	config         *config.Config
	logger         *slog.Logger
	jobQueue       chan *domain.URL
	wg             sync.WaitGroup
	concurrencyMu  sync.Map // map[string]*sync.Mutex for concurrency groups
	domainLimiters sync.Map // map[string]*rate.Limiter for domain-specific rate limiting
}

// NewWorker creates a new worker
func NewWorker(db *sql.DB, cfg *config.Config, logger *slog.Logger) *Worker {
	clientConfig := httpclient.ClientConfig{
		Timeout:         cfg.RequestTimeout,
		MaxRedirects:    cfg.MaxRedirects,
		MaxResponseSize: cfg.MaxResponseSize,
		UserAgent:       cfg.UserAgent,
	}

	fetcherConfig := FetcherConfig{
		MaxRetryAttempts: cfg.MaxRetryAttempts,
		RetryBaseDelay:   cfg.RetryBaseDelay,
		RetryMaxDelay:    cfg.RetryMaxDelay,
		RateLimitPerSec:  cfg.RateLimitPerSec,
	}

	return &Worker{
		urlRepo:  repository.NewURLRepository(db),
		runRepo:  repository.NewRunRepository(db),
		fetcher:  NewFetcher(clientConfig, fetcherConfig),
		config:   cfg,
		logger:   logger,
		jobQueue: make(chan *domain.URL, cfg.BatchSize),
	}
}

// Run starts the worker
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("starting worker",
		"max_workers", w.config.MaxWorkers,
		"poll_interval", w.config.PollInterval,
		"rate_limit", w.config.RateLimitPerSec,
	)

	// Start worker goroutines
	for i := 0; i < w.config.MaxWorkers; i++ {
		w.wg.Add(1)
		go w.worker(ctx, i)
	}

	// Start scheduler
	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.pollAndDispatch(ctx)
		case <-ctx.Done():
			w.logger.Info("shutting down worker...")
			close(w.jobQueue)
			w.wg.Wait()
			w.logger.Info("worker shutdown complete")
			return nil
		}
	}
}

func (w *Worker) pollAndDispatch(ctx context.Context) {
	urls, err := w.urlRepo.FetchDueURLs(ctx, w.config.BatchSize)
	if err != nil {
		w.logger.Error("failed to fetch due URLs", "error", err)
		return
	}

	if len(urls) > 0 {
		w.logger.Info("dispatching URLs", "count", len(urls))
	}

	for _, url := range urls {
		select {
		case w.jobQueue <- url:
		case <-ctx.Done():
			return
		}
	}
}

func (w *Worker) worker(ctx context.Context, id int) {
	defer w.wg.Done()

	w.logger.Debug("worker started", "worker_id", id)

	for {
		select {
		case url, ok := <-w.jobQueue:
			if !ok {
				w.logger.Debug("worker stopping", "worker_id", id)
				return
			}
			w.processURL(ctx, id, url)
		case <-ctx.Done():
			return
		}
	}
}

func (w *Worker) processURL(ctx context.Context, workerID int, url *domain.URL) {
	startTime := time.Now()

	// Domain-specific rate limiting
	domain := extractDomain(url.URL)
	domainLimiter := w.getDomainLimiter(domain)
	if err := domainLimiter.Wait(ctx); err != nil {
		w.logger.Error("domain rate limit wait failed",
			"url_id", url.ID,
			"domain", domain,
			"error", err,
		)
		return
	}

	// Concurrency group handling (for complete serialization if needed)
	if url.ConcurrencyGroup != nil && *url.ConcurrencyGroup != "" {
		lock := w.getGroupLock(*url.ConcurrencyGroup)
		lock.Lock()
		defer lock.Unlock()
	}

	w.logger.Info("processing URL",
		"worker_id", workerID,
		"url_id", url.ID,
		"url", url.URL,
		"domain", domain,
	)

	// Fetch URL
	result := w.fetcher.Fetch(ctx, url)

	// Save run result
	if err := w.runRepo.Create(ctx, result.Run); err != nil {
		w.logger.Error("failed to save run",
			"url_id", url.ID,
			"error", err,
		)
	}

	// Update next_run_at
	nextRunAt := time.Now().Add(time.Duration(url.IntervalSeconds) * time.Second)
	if err := w.urlRepo.UpdateNextRunAt(ctx, url.ID, nextRunAt); err != nil {
		w.logger.Error("failed to update next_run_at",
			"url_id", url.ID,
			"error", err,
		)
	}

	// Log result
	duration := time.Since(startTime)
	logAttrs := []any{
		"worker_id", workerID,
		"url_id", url.ID,
		"url", url.URL,
		"domain", domain,
		"status", result.Run.Status,
		"latency_ms", result.Run.LatencyMs,
		"attempt", result.Run.Attempt,
		"duration_ms", duration.Milliseconds(),
	}

	if result.Run.HTTPStatus != nil {
		logAttrs = append(logAttrs, "http_status", *result.Run.HTTPStatus)
	}
	if result.Run.Title != nil {
		logAttrs = append(logAttrs, "title", *result.Run.Title)
	}
	if result.Run.ErrorCode != nil {
		logAttrs = append(logAttrs, "error_code", *result.Run.ErrorCode)
	}

	if result.Success {
		w.logger.Info("URL fetch completed", logAttrs...)
	} else {
		w.logger.Warn("URL fetch failed", logAttrs...)
	}
}

func (w *Worker) getGroupLock(group string) *sync.Mutex {
	lock, _ := w.concurrencyMu.LoadOrStore(group, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// extractDomain extracts domain from URL
func extractDomain(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL // fallback to raw URL
	}
	return parsedURL.Host
}

// getDomainLimiter gets or creates a rate limiter for the specified domain
func (w *Worker) getDomainLimiter(domain string) *rate.Limiter {
	limiter, _ := w.domainLimiters.LoadOrStore(
		domain,
		rate.NewLimiter(rate.Limit(w.config.DomainRateLimitPerSec), int(w.config.DomainRateLimitPerSec)),
	)
	return limiter.(*rate.Limiter)
}
