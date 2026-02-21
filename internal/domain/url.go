package domain

import (
	"net/url"
	"strings"
	"time"
)

type URL struct {
	ID               int64     `json:"id"`
	URL              string    `json:"url"`
	NormalizedURL    string    `json:"normalizedUrl"`
	Enabled          bool      `json:"enabled"`
	IntervalSeconds  int       `json:"intervalSeconds"`
	Tags             []string  `json:"tags"`
	ConcurrencyGroup *string   `json:"maxConcurrencyGroup"`
	NextRunAt        time.Time `json:"nextRunAt"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// NormalizeURL normalizes a URL for deduplication
func NormalizeURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// Lowercase scheme and host
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)

	// Remove default ports
	if (u.Scheme == "http" && u.Port() == "80") ||
		(u.Scheme == "https" && u.Port() == "443") {
		u.Host = u.Hostname()
	}

	// Normalize path
	if u.Path == "" {
		u.Path = "/"
	} else if u.Path != "/" && strings.HasSuffix(u.Path, "/") {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}

	// Remove tracking parameters
	q := u.Query()
	trackingParams := []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"}
	for _, param := range trackingParams {
		q.Del(param)
	}
	u.RawQuery = q.Encode()

	// Remove fragment
	u.Fragment = ""

	return u.String(), nil
}
