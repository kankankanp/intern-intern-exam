package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strings"
	"time"

	"url-collector/internal/domain"
	"url-collector/internal/httpclient"

	"golang.org/x/time/rate"
)

var (
	ErrTimeout    = errors.New("request timeout")
	ErrConnection = errors.New("connection error")
	ErrDNS        = errors.New("dns resolution failed")
)

// FetcherConfig holds fetcher configuration
type FetcherConfig struct {
	MaxRetryAttempts int
	RetryBaseDelay   time.Duration
	RetryMaxDelay    time.Duration
	RateLimitPerSec  float64
}

// Fetcher handles URL fetching with retry and rate limiting
type Fetcher struct {
	client      *httpclient.SafeClient
	rateLimiter *rate.Limiter
	config      FetcherConfig
}

// NewFetcher creates a new fetcher
func NewFetcher(clientConfig httpclient.ClientConfig, fetcherConfig FetcherConfig) *Fetcher {
	return &Fetcher{
		client:      httpclient.NewSafeClient(clientConfig),
		rateLimiter: rate.NewLimiter(rate.Limit(fetcherConfig.RateLimitPerSec), int(fetcherConfig.RateLimitPerSec)),
		config:      fetcherConfig,
	}
}

// FetchResult holds the result of a fetch with retry
type FetchResult struct {
	Run     *domain.Run
	Success bool
}

// Fetch fetches a URL with retry logic
func (f *Fetcher) Fetch(ctx context.Context, urlEntity *domain.URL) *FetchResult {
	var lastErr error
	var lastResult *httpclient.FetchResult

	for attempt := 1; attempt <= f.config.MaxRetryAttempts; attempt++ {
		// Rate limiting
		if err := f.rateLimiter.Wait(ctx); err != nil {
			return f.createFailedRun(urlEntity, "rate_limit", err.Error(), attempt)
		}

		// Fetch
		result, err := f.client.Fetch(ctx, urlEntity.URL)

		// Check for success (2xx, 3xx, 4xx are not retried)
		if err == nil && result.StatusCode < 500 {
			// Success or client error - return immediately
			return f.createSuccessRun(urlEntity, result, attempt)
		}

		// Record error/result for retry
		if err != nil {
			// Network/protocol error
			lastErr = err
			lastResult = result
			// Check if retryable
			if !f.isRetryable(err) {
				break
			}
		} else {
			// 5xx server error - always retryable
			lastErr = fmt.Errorf("HTTP %d error", result.StatusCode)
			lastResult = result
			// 5xx is retryable, continue to retry logic
		}

		// Wait before retry (except for last attempt)
		if attempt < f.config.MaxRetryAttempts {
			delay := f.calculateDelay(attempt)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return f.createFailedRun(urlEntity, "cancelled", ctx.Err().Error(), attempt)
			}
		}
	}

	return f.createFailedRunFromError(urlEntity, lastErr, lastResult, f.config.MaxRetryAttempts)
}

func (f *Fetcher) createSuccessRun(urlEntity *domain.URL, result *httpclient.FetchResult, attempt int) *FetchResult {
	run := &domain.Run{
		URLID:     urlEntity.ID,
		Status:    domain.RunStatusSucceeded,
		LatencyMs: result.LatencyMs,
		Attempt:   attempt,
		RunAt:     time.Now(),
	}

	// Set HTTP status
	run.HTTPStatus = &result.StatusCode

	// Set final URL
	run.FinalURL = &result.FinalURL

	// Set content type
	if result.ContentType != "" {
		run.ContentType = &result.ContentType
	}

	// Parse HTML if applicable
	if IsHTML(result.ContentType) && len(result.Body) > 0 {
		if meta, err := ParseHTML(result.Body); err == nil {
			if meta.Title != "" {
				run.Title = &meta.Title
			}
			if meta.Description != "" {
				run.Description = &meta.Description
			}
			if meta.OGTitle != "" {
				run.OGTitle = &meta.OGTitle
			}
			if meta.OGDescription != "" {
				run.OGDescription = &meta.OGDescription
			}
			if meta.OGImage != "" {
				run.OGImage = &meta.OGImage
			}
			if meta.OGURL != "" {
				run.OGURL = &meta.OGURL
			}
			if meta.OGSiteName != "" {
				run.OGSiteName = &meta.OGSiteName
			}
		}
	}

	// Check for HTTP errors
	if result.StatusCode >= 400 {
		run.Status = domain.RunStatusFailed
		errorCode := "4xx"
		if result.StatusCode >= 500 {
			errorCode = "5xx"
		}
		run.ErrorCode = &errorCode
		errorMsg := "HTTP error"
		run.ErrorMessage = &errorMsg
	}

	return &FetchResult{
		Run:     run,
		Success: run.Status == domain.RunStatusSucceeded,
	}
}

func (f *Fetcher) createFailedRun(urlEntity *domain.URL, errorCode, errorMessage string, attempt int) *FetchResult {
	run := &domain.Run{
		URLID:        urlEntity.ID,
		Status:       domain.RunStatusFailed,
		LatencyMs:    0,
		ErrorCode:    &errorCode,
		ErrorMessage: &errorMessage,
		Attempt:      attempt,
		RunAt:        time.Now(),
	}

	return &FetchResult{
		Run:     run,
		Success: false,
	}
}

func (f *Fetcher) createFailedRunFromError(urlEntity *domain.URL, err error, result *httpclient.FetchResult, attempt int) *FetchResult {
	errorCode := f.classifyError(err)
	errorMessage := err.Error()

	run := &domain.Run{
		URLID:        urlEntity.ID,
		Status:       domain.RunStatusFailed,
		LatencyMs:    0,
		ErrorCode:    &errorCode,
		ErrorMessage: &errorMessage,
		Attempt:      attempt,
		RunAt:        time.Now(),
	}

	if result != nil {
		run.LatencyMs = result.LatencyMs
		if result.StatusCode > 0 {
			run.HTTPStatus = &result.StatusCode
		}
		if result.FinalURL != "" {
			run.FinalURL = &result.FinalURL
		}
	}

	return &FetchResult{
		Run:     run,
		Success: false,
	}
}

func (f *Fetcher) isRetryable(err error) bool {
	// SSRF errors are not retryable
	if errors.Is(err, httpclient.ErrPrivateIP) ||
		errors.Is(err, httpclient.ErrLoopback) ||
		errors.Is(err, httpclient.ErrLinkLocal) ||
		errors.Is(err, httpclient.ErrInvalidScheme) ||
		errors.Is(err, httpclient.ErrRedirectBlocked) {
		return false
	}

	// Too many redirects is not retryable
	if errors.Is(err, httpclient.ErrTooManyRedirects) {
		return false
	}

	// Context cancelled is not retryable
	if errors.Is(err, context.Canceled) {
		return false
	}

	// URL parse errors are not retryable
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		// Timeout is retryable
		if urlErr.Timeout() {
			return true
		}
	}

	// Network errors are generally retryable
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	// DNS errors are retryable
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	return true
}

func (f *Fetcher) classifyError(err error) string {
	if errors.Is(err, httpclient.ErrPrivateIP) ||
		errors.Is(err, httpclient.ErrLoopback) ||
		errors.Is(err, httpclient.ErrLinkLocal) ||
		errors.Is(err, httpclient.ErrInvalidScheme) ||
		errors.Is(err, httpclient.ErrRedirectBlocked) {
		return "ssrf_blocked"
	}

	if errors.Is(err, httpclient.ErrTooManyRedirects) {
		return "too_many_redirects"
	}

	if errors.Is(err, httpclient.ErrResponseTooLarge) {
		return "response_too_large"
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return "timeout"
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns"
	}

	if strings.Contains(err.Error(), "connection") {
		return "connection"
	}

	return "unknown"
}

func (f *Fetcher) calculateDelay(attempt int) time.Duration {
	// Exponential backoff: base * 2^(attempt-1)
	delay := f.config.RetryBaseDelay * time.Duration(1<<uint(attempt-1))

	// Cap at max delay
	if delay > f.config.RetryMaxDelay {
		delay = f.config.RetryMaxDelay
	}

	// Add jitter (±10%)
	jitter := time.Duration(rand.Float64()*0.2-0.1) * delay
	return delay + jitter
}
