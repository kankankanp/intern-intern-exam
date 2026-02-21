package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

var (
	ErrTooManyRedirects  = errors.New("too many redirects")
	ErrResponseTooLarge  = errors.New("response body too large")
	ErrRedirectBlocked   = errors.New("redirect blocked by SSRF guard")
)

// ClientConfig holds HTTP client configuration
type ClientConfig struct {
	Timeout         time.Duration
	MaxRedirects    int
	MaxResponseSize int64
	UserAgent       string
}

// DefaultClientConfig returns default client configuration
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Timeout:         8 * time.Second,
		MaxRedirects:    5,
		MaxResponseSize: 1 * 1024 * 1024, // 1MB
		UserAgent:       "InternInc-MetadataCollector/0.1",
	}
}

// SafeClient is an HTTP client with SSRF protection
type SafeClient struct {
	client *http.Client
	guard  *SSRFGuard
	config ClientConfig
}

// NewSafeClient creates a new safe HTTP client
func NewSafeClient(config ClientConfig) *SafeClient {
	guard := NewSSRFGuard()

	dialer := &safeDialer{
		guard:   guard,
		timeout: config.Timeout,
	}

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}

	sc := &SafeClient{
		client: client,
		guard:  guard,
		config: config,
	}

	// Set redirect policy
	client.CheckRedirect = sc.checkRedirect

	return sc
}

// FetchResult holds the result of a fetch operation
type FetchResult struct {
	StatusCode  int
	FinalURL    string
	ContentType string
	Body        []byte
	LatencyMs   int
}

// Fetch fetches a URL with SSRF protection
func (c *SafeClient) Fetch(ctx context.Context, targetURL string) (*FetchResult, error) {
	// Validate URL before request
	if err := c.guard.ValidateURL(targetURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.config.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ja,en;q=0.9")

	start := time.Now()
	resp, err := c.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read body with size limit
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.config.MaxResponseSize))
	if err != nil {
		return nil, err
	}

	return &FetchResult{
		StatusCode:  resp.StatusCode,
		FinalURL:    resp.Request.URL.String(),
		ContentType: resp.Header.Get("Content-Type"),
		Body:        body,
		LatencyMs:   int(latency.Milliseconds()),
	}, nil
}

func (c *SafeClient) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= c.config.MaxRedirects {
		return ErrTooManyRedirects
	}

	// Validate redirect target
	if err := c.guard.ValidateURL(req.URL.String()); err != nil {
		return fmt.Errorf("%w: %v", ErrRedirectBlocked, err)
	}

	return nil
}

// safeDialer performs DNS resolution and validates the resolved IP
type safeDialer struct {
	guard   *SSRFGuard
	timeout time.Duration
}

func (d *safeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	// Resolve DNS
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}

	// Find a valid IP
	var validIP net.IP
	for _, ip := range ips {
		if err := d.guard.ValidateIP(ip); err == nil {
			validIP = ip
			break
		}
	}

	if validIP == nil {
		return nil, ErrPrivateIP
	}

	// Connect using validated IP
	dialer := &net.Dialer{Timeout: d.timeout}
	return dialer.DialContext(ctx, network, net.JoinHostPort(validIP.String(), port))
}
