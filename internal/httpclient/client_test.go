package httpclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestHTTPClient_Success tests successful HTTP fetch with httptest.Server
func TestHTTPClient_Success(t *testing.T) {
	// Create test server (200 OK)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><head><title>Test Page</title></head><body>Hello</body></html>`))
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	start := time.Now()
	resp, err := client.Get(ts.URL)
	latency := time.Since(start)

	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("ContentType = %q, want %q", resp.Header.Get("Content-Type"), "text/html; charset=utf-8")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if len(body) == 0 {
		t.Error("Body is empty")
	}

	if latency <= 0 {
		t.Error("Latency should be positive")
	}
}

// TestHTTPClient_Redirect tests redirect handling (301)
func TestHTTPClient_Redirect(t *testing.T) {
	// Final destination
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><head><title>Final</title></head></html>`))
	}))
	defer final.Close()

	// Redirect source
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusMovedPermanently)
	}))
	defer redirect.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(redirect.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()

	// FinalURL should be the destination (may or may not have trailing slash)
	finalURL := resp.Request.URL.String()
	if finalURL != final.URL && finalURL != final.URL+"/" {
		t.Errorf("FinalURL = %q, want %q or %q", finalURL, final.URL, final.URL+"/")
	}
}

// TestHTTPClient_ServerError tests 500 error handling
func TestHTTPClient_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("Get() error = %v (should not error on 5xx)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", resp.StatusCode)
	}
}

// TestHTTPClient_Timeout tests timeout handling
func TestHTTPClient_Timeout(t *testing.T) {
	// Slow server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	_, err := client.Get(ts.URL)

	if err == nil {
		t.Fatal("Get() expected timeout error, got nil")
	}
}
