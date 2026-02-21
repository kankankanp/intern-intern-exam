package domain

import (
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "lowercase scheme and host",
			input:    "HTTP://EXAMPLE.COM/Page",
			expected: "http://example.com/Page",
		},
		{
			name:     "remove default http port",
			input:    "http://example.com:80/page",
			expected: "http://example.com/page",
		},
		{
			name:     "remove default https port",
			input:    "https://example.com:443/page",
			expected: "https://example.com/page",
		},
		{
			name:     "keep non-default port",
			input:    "https://example.com:8443/page",
			expected: "https://example.com:8443/page",
		},
		{
			name:     "remove trailing slash from path",
			input:    "https://example.com/page/",
			expected: "https://example.com/page",
		},
		{
			name:     "keep root trailing slash",
			input:    "https://example.com/",
			expected: "https://example.com/",
		},
		{
			name:     "add root path",
			input:    "https://example.com",
			expected: "https://example.com/",
		},
		{
			name:     "remove utm_source",
			input:    "https://example.com/page?utm_source=twitter",
			expected: "https://example.com/page",
		},
		{
			name:     "remove multiple utm parameters",
			input:    "https://example.com/page?utm_source=twitter&utm_medium=social&foo=bar",
			expected: "https://example.com/page?foo=bar",
		},
		{
			name:     "remove all utm parameters",
			input:    "https://example.com/page?utm_source=a&utm_medium=b&utm_campaign=c&utm_term=d&utm_content=e",
			expected: "https://example.com/page",
		},
		{
			name:     "remove fragment",
			input:    "https://example.com/page#section",
			expected: "https://example.com/page",
		},
		{
			name:     "complex normalization",
			input:    "HTTPS://EXAMPLE.COM:443/Page/?utm_source=x#top",
			expected: "https://example.com/Page",
		},
		{
			name:     "preserve query order",
			input:    "https://example.com/page?b=2&a=1",
			expected: "https://example.com/page?a=1&b=2",
		},
		{
			name:     "empty path becomes root",
			input:    "https://example.com",
			expected: "https://example.com/",
		},
		{
			name:    "invalid url",
			input:   "://invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeURL(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
