# テスト戦略

## 1. 概要

本システムのテスト戦略を定義する。重要なロジックを保護しつつ、実用的なテストカバレッジを確保する。

## 2. テストピラミッド

```
        ┌───────────┐
        │   E2E     │  ← 少数（Docker Compose全体テスト）
        │  Tests    │
       ─┴───────────┴─
      ┌───────────────┐
      │  Integration  │  ← 中程度（DB/HTTP結合テスト）
      │    Tests      │
     ─┴───────────────┴─
    ┌───────────────────┐
    │    Unit Tests     │  ← 多数（ビジネスロジック）
    │                   │
    └───────────────────┘
```

## 3. ユニットテスト

### 3.1 対象コンポーネント

| コンポーネント | テスト内容 | 優先度 |
|---------------|-----------|--------|
| SSRFGuard | IP/URL検証 | 必須 |
| HTMLParser | title/OGP抽出 | 必須 |
| Retrier | バックオフ計算 | 必須 |
| URLNormalizer | URL正規化 | 必須 |
| Validator | 入力バリデーション | 必須 |

### 3.2 SSRF Guardテスト

```go
package httpclient_test

import (
    "net"
    "testing"

    "url-collector/internal/httpclient"
)

func TestSSRFGuard_ValidateURL(t *testing.T) {
    guard := httpclient.NewSSRFGuard()

    tests := []struct {
        name    string
        url     string
        wantErr bool
    }{
        // 許可されるURL
        {"valid https", "https://example.com", false},
        {"valid http", "http://example.com", false},
        {"valid with path", "https://example.com/page/subpage", false},
        {"valid with query", "https://example.com?foo=bar", false},
        {"valid with port", "https://example.com:8443/page", false},

        // ブロックされるURL - スキーム
        {"ftp scheme", "ftp://example.com/", true},
        {"file scheme", "file:///etc/passwd", true},
        {"data scheme", "data:text/html,<script>alert(1)</script>", true},
        {"javascript scheme", "javascript:alert(1)", true},

        // ブロックされるURL - ループバック
        {"localhost", "http://localhost/", true},
        {"localhost with port", "http://localhost:8080/", true},
        {"127.0.0.1", "http://127.0.0.1/", true},
        {"127.0.0.1 with port", "http://127.0.0.1:80/", true},
        {"127.x.x.x", "http://127.0.0.2/", true},
        {"ipv6 loopback", "http://[::1]/", true},

        // ブロックされるURL - プライベートIP
        {"10.x.x.x", "http://10.0.0.1/", true},
        {"10.255.255.255", "http://10.255.255.255/", true},
        {"172.16.x.x", "http://172.16.0.1/", true},
        {"172.31.x.x", "http://172.31.255.255/", true},
        {"192.168.x.x", "http://192.168.1.1/", true},
        {"192.168.0.1", "http://192.168.0.1/", true},

        // ブロックされるURL - リンクローカル
        {"link-local", "http://169.254.169.254/", true},
        {"aws metadata", "http://169.254.169.254/latest/meta-data/", true},

        // ブロックされるURL - その他
        {"broadcast", "http://255.255.255.255/", true},
        {"0.0.0.0", "http://0.0.0.0/", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := guard.ValidateURL(tt.url)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateURL(%q) error = %v, wantErr %v",
                    tt.url, err, tt.wantErr)
            }
        })
    }
}

func TestSSRFGuard_ValidateIP(t *testing.T) {
    guard := httpclient.NewSSRFGuard()

    tests := []struct {
        name    string
        ip      string
        wantErr bool
    }{
        // 許可されるIP
        {"public ipv4", "8.8.8.8", false},
        {"public ipv4 2", "1.1.1.1", false},
        {"public ipv6", "2001:4860:4860::8888", false},

        // ブロックされるIP
        {"loopback", "127.0.0.1", true},
        {"private 10", "10.0.0.1", true},
        {"private 172", "172.16.0.1", true},
        {"private 192", "192.168.1.1", true},
        {"link-local", "169.254.1.1", true},
        {"ipv6 loopback", "::1", true},
        {"ipv6 link-local", "fe80::1", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ip := net.ParseIP(tt.ip)
            err := guard.ValidateIP(ip)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateIP(%q) error = %v, wantErr %v",
                    tt.ip, err, tt.wantErr)
            }
        })
    }
}
```

### 3.3 HTMLパーサテスト

```go
package service_test

import (
    "testing"

    "url-collector/internal/service"
)

func TestParseHTML(t *testing.T) {
    tests := []struct {
        name     string
        html     string
        expected *service.Metadata
    }{
        {
            name: "basic title",
            html: `<!DOCTYPE html>
<html>
<head><title>Test Page Title</title></head>
<body></body>
</html>`,
            expected: &service.Metadata{
                Title: "Test Page Title",
            },
        },
        {
            name: "title with whitespace",
            html: `<html><head><title>
                Title with   spaces
            </title></head></html>`,
            expected: &service.Metadata{
                Title: "Title with   spaces",
            },
        },
        {
            name: "meta description",
            html: `<html><head>
<meta name="description" content="This is the description">
</head></html>`,
            expected: &service.Metadata{
                Description: "This is the description",
            },
        },
        {
            name: "full OGP",
            html: `<html><head>
<title>Page Title</title>
<meta name="description" content="Meta description">
<meta property="og:title" content="OG Title">
<meta property="og:description" content="OG Description">
<meta property="og:image" content="https://example.com/image.png">
<meta property="og:url" content="https://example.com/page">
<meta property="og:site_name" content="Example Site">
</head></html>`,
            expected: &service.Metadata{
                Title:         "Page Title",
                Description:   "Meta description",
                OGTitle:       "OG Title",
                OGDescription: "OG Description",
                OGImage:       "https://example.com/image.png",
                OGURL:         "https://example.com/page",
                OGSiteName:    "Example Site",
            },
        },
        {
            name: "empty html",
            html: `<html><head></head><body></body></html>`,
            expected: &service.Metadata{},
        },
        {
            name:     "malformed html",
            html:     `<html><head><title>Unclosed`,
            expected: &service.Metadata{Title: "Unclosed"},
        },
        {
            name: "og with name attribute (should not match)",
            html: `<html><head>
<meta name="og:title" content="Wrong OG">
<meta property="og:title" content="Correct OG">
</head></html>`,
            expected: &service.Metadata{
                OGTitle: "Correct OG",
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := service.ParseHTML([]byte(tt.html))
            if err != nil {
                t.Fatalf("ParseHTML() error = %v", err)
            }

            if result.Title != tt.expected.Title {
                t.Errorf("Title = %q, want %q", result.Title, tt.expected.Title)
            }
            if result.Description != tt.expected.Description {
                t.Errorf("Description = %q, want %q",
                    result.Description, tt.expected.Description)
            }
            if result.OGTitle != tt.expected.OGTitle {
                t.Errorf("OGTitle = %q, want %q",
                    result.OGTitle, tt.expected.OGTitle)
            }
            if result.OGDescription != tt.expected.OGDescription {
                t.Errorf("OGDescription = %q, want %q",
                    result.OGDescription, tt.expected.OGDescription)
            }
            if result.OGImage != tt.expected.OGImage {
                t.Errorf("OGImage = %q, want %q",
                    result.OGImage, tt.expected.OGImage)
            }
        })
    }
}
```

### 3.4 リトライ計算テスト

```go
package service_test

import (
    "testing"
    "time"

    "url-collector/internal/service"
)

func TestRetrier_CalculateDelay(t *testing.T) {
    retrier := service.NewRetrier(service.RetryConfig{
        MaxAttempts: 3,
        BaseDelay:   10 * time.Second,
        MaxDelay:    2 * time.Minute,
    })

    tests := []struct {
        attempt  int
        minDelay time.Duration
        maxDelay time.Duration
    }{
        {1, 9 * time.Second, 11 * time.Second},   // 10s ± 10%
        {2, 18 * time.Second, 22 * time.Second},  // 20s ± 10%
        {3, 36 * time.Second, 44 * time.Second},  // 40s ± 10%
        {4, 72 * time.Second, 88 * time.Second},  // 80s ± 10%
        {5, 108 * time.Second, 132 * time.Second}, // capped at 120s ± 10%
    }

    for _, tt := range tests {
        t.Run("", func(t *testing.T) {
            delay := retrier.CalculateDelay(tt.attempt)
            if delay < tt.minDelay || delay > tt.maxDelay {
                t.Errorf("CalculateDelay(%d) = %v, want between %v and %v",
                    tt.attempt, delay, tt.minDelay, tt.maxDelay)
            }
        })
    }
}

func TestIsRetryable(t *testing.T) {
    tests := []struct {
        name     string
        err      error
        expected bool
    }{
        {"timeout error", service.ErrTimeout, true},
        {"connection error", service.ErrConnection, true},
        {"5xx error", service.NewHTTPError(500), true},
        {"503 error", service.NewHTTPError(503), true},
        {"4xx error", service.NewHTTPError(404), false},
        {"400 error", service.NewHTTPError(400), false},
        {"ssrf blocked", httpclient.ErrPrivateIP, false},
        {"parse error", service.ErrParseHTML, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := service.IsRetryable(tt.err)
            if result != tt.expected {
                t.Errorf("IsRetryable(%v) = %v, want %v",
                    tt.err, result, tt.expected)
            }
        })
    }
}
```

### 3.5 URL正規化テスト

```go
package domain_test

import (
    "testing"

    "url-collector/internal/domain"
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
            name:     "remove trailing slash",
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
            name:     "remove utm parameters",
            input:    "https://example.com/page?utm_source=twitter&utm_medium=social&foo=bar",
            expected: "https://example.com/page?foo=bar",
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
            name:    "invalid url",
            input:   "://invalid",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := domain.NormalizeURL(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("NormalizeURL(%q) error = %v, wantErr %v",
                    tt.input, err, tt.wantErr)
            }
            if result != tt.expected {
                t.Errorf("NormalizeURL(%q) = %q, want %q",
                    tt.input, result, tt.expected)
            }
        })
    }
}
```

## 4. 結合テスト

### 4.1 HTTPテスト（httptest使用）

```go
package service_test

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "url-collector/internal/service"
)

func TestFetcher_Fetch_Success(t *testing.T) {
    // テストサーバー
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html")
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`<html><head><title>Test</title></head></html>`))
    }))
    defer ts.Close()

    fetcher := service.NewFetcher(service.FetcherConfig{
        Timeout: 5 * time.Second,
    })

    result, err := fetcher.Fetch(context.Background(), ts.URL)
    if err != nil {
        t.Fatalf("Fetch() error = %v", err)
    }

    if result.StatusCode != 200 {
        t.Errorf("StatusCode = %d, want 200", result.StatusCode)
    }
    if result.ContentType != "text/html" {
        t.Errorf("ContentType = %q, want %q", result.ContentType, "text/html")
    }
}

func TestFetcher_Fetch_Redirect(t *testing.T) {
    // リダイレクト先
    final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`<html><head><title>Final</title></head></html>`))
    }))
    defer final.Close()

    // リダイレクト元
    redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        http.Redirect(w, r, final.URL, http.StatusMovedPermanently)
    }))
    defer redirect.Close()

    fetcher := service.NewFetcher(service.FetcherConfig{
        Timeout:      5 * time.Second,
        MaxRedirects: 5,
    })

    result, err := fetcher.Fetch(context.Background(), redirect.URL)
    if err != nil {
        t.Fatalf("Fetch() error = %v", err)
    }

    if result.FinalURL != final.URL+"/" {
        t.Errorf("FinalURL = %q, want %q", result.FinalURL, final.URL+"/")
    }
}

func TestFetcher_Fetch_Timeout(t *testing.T) {
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(2 * time.Second)
        w.WriteHeader(http.StatusOK)
    }))
    defer ts.Close()

    fetcher := service.NewFetcher(service.FetcherConfig{
        Timeout: 500 * time.Millisecond,
    })

    _, err := fetcher.Fetch(context.Background(), ts.URL)
    if err == nil {
        t.Fatal("Fetch() expected timeout error")
    }
}

func TestFetcher_Fetch_TooManyRedirects(t *testing.T) {
    var redirectCount int
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        redirectCount++
        http.Redirect(w, r, r.URL.String()+"x", http.StatusMovedPermanently)
    }))
    defer ts.Close()

    fetcher := service.NewFetcher(service.FetcherConfig{
        Timeout:      5 * time.Second,
        MaxRedirects: 3,
    })

    _, err := fetcher.Fetch(context.Background(), ts.URL)
    if err == nil {
        t.Fatal("Fetch() expected too many redirects error")
    }
}

func TestFetcher_Fetch_ServerError(t *testing.T) {
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
    }))
    defer ts.Close()

    fetcher := service.NewFetcher(service.FetcherConfig{
        Timeout: 5 * time.Second,
    })

    result, err := fetcher.Fetch(context.Background(), ts.URL)
    if err != nil {
        t.Fatalf("Fetch() error = %v", err)
    }

    if result.StatusCode != 500 {
        t.Errorf("StatusCode = %d, want 500", result.StatusCode)
    }
}
```

### 4.2 DBテスト

```go
package repository_test

import (
    "context"
    "database/sql"
    "testing"
    "time"

    _ "github.com/mattn/go-sqlite3"
    "url-collector/internal/domain"
    "url-collector/internal/repository"
)

func setupTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil {
        t.Fatalf("failed to open db: %v", err)
    }

    // スキーマ作成
    _, err = db.Exec(`
        CREATE TABLE urls (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            url TEXT NOT NULL UNIQUE,
            normalized_url TEXT NOT NULL UNIQUE,
            enabled INTEGER NOT NULL DEFAULT 1,
            interval_seconds INTEGER NOT NULL,
            tags TEXT NOT NULL DEFAULT '[]',
            concurrency_group TEXT,
            next_run_at TEXT NOT NULL,
            created_at TEXT NOT NULL,
            updated_at TEXT NOT NULL
        );

        CREATE TABLE url_runs (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            url_id INTEGER NOT NULL,
            status TEXT NOT NULL,
            http_status INTEGER,
            final_url TEXT,
            content_type TEXT,
            latency_ms INTEGER NOT NULL,
            title TEXT,
            description TEXT,
            og_title TEXT,
            og_description TEXT,
            og_image TEXT,
            og_url TEXT,
            og_site_name TEXT,
            error_code TEXT,
            error_message TEXT,
            attempt INTEGER NOT NULL DEFAULT 1,
            run_at TEXT NOT NULL,
            created_at TEXT NOT NULL
        );
    `)
    if err != nil {
        t.Fatalf("failed to create schema: %v", err)
    }

    return db
}

func TestURLRepository_Create(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    repo := repository.NewURLRepository(db)
    ctx := context.Background()

    url := &domain.URL{
        URL:             "https://example.com/page",
        NormalizedURL:   "https://example.com/page",
        Enabled:         true,
        IntervalSeconds: 3600,
        Tags:            []string{"test"},
    }

    err := repo.Create(ctx, url)
    if err != nil {
        t.Fatalf("Create() error = %v", err)
    }

    if url.ID == 0 {
        t.Error("Create() should set ID")
    }
}

func TestURLRepository_FetchDueURLs(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    repo := repository.NewURLRepository(db)
    ctx := context.Background()

    // 過去のnext_run_atを持つURL
    _, _ = db.Exec(`
        INSERT INTO urls (url, normalized_url, enabled, interval_seconds, tags, next_run_at, created_at, updated_at)
        VALUES ('https://due.com', 'https://due.com/', 1, 3600, '[]', datetime('now', '-1 hour'), datetime('now'), datetime('now'))
    `)

    // 未来のnext_run_atを持つURL
    _, _ = db.Exec(`
        INSERT INTO urls (url, normalized_url, enabled, interval_seconds, tags, next_run_at, created_at, updated_at)
        VALUES ('https://future.com', 'https://future.com/', 1, 3600, '[]', datetime('now', '+1 hour'), datetime('now'), datetime('now'))
    `)

    // 無効なURL
    _, _ = db.Exec(`
        INSERT INTO urls (url, normalized_url, enabled, interval_seconds, tags, next_run_at, created_at, updated_at)
        VALUES ('https://disabled.com', 'https://disabled.com/', 0, 3600, '[]', datetime('now', '-1 hour'), datetime('now'), datetime('now'))
    `)

    urls, err := repo.FetchDueURLs(ctx, 10)
    if err != nil {
        t.Fatalf("FetchDueURLs() error = %v", err)
    }

    if len(urls) != 1 {
        t.Errorf("FetchDueURLs() returned %d URLs, want 1", len(urls))
    }

    if len(urls) > 0 && urls[0].URL != "https://due.com" {
        t.Errorf("FetchDueURLs() returned wrong URL: %s", urls[0].URL)
    }
}
```

## 5. E2Eテスト

### 5.1 Docker Composeテスト

```yaml
# docker-compose.test.yml
version: '3.8'

services:
  test:
    build: .
    command: go test -v ./... -tags=e2e
    depends_on:
      - db
    environment:
      - DATABASE_URL=postgres://test:test@db:5432/test?sslmode=disable
      - TEST_MODE=true

  db:
    image: postgres:15
    environment:
      - POSTGRES_USER=test
      - POSTGRES_PASSWORD=test
      - POSTGRES_DB=test
```

### 5.2 E2Eテストコード

```go
//go:build e2e

package e2e_test

import (
    "bytes"
    "encoding/json"
    "net/http"
    "testing"
    "time"
)

const baseURL = "http://localhost:8080"

func TestE2E_URLWorkflow(t *testing.T) {
    // 1. URL登録
    createBody := map[string]any{
        "url":             "https://example.com",
        "intervalSeconds": 60,
        "tags":            []string{"test"},
    }
    body, _ := json.Marshal(createBody)

    resp, err := http.Post(baseURL+"/urls", "application/json", bytes.NewReader(body))
    if err != nil {
        t.Fatalf("POST /urls error: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        t.Fatalf("POST /urls status = %d, want 201", resp.StatusCode)
    }

    var createResp map[string]any
    json.NewDecoder(resp.Body).Decode(&createResp)
    urlID := createResp["data"].(map[string]any)["id"]

    // 2. 手動実行
    resp, err = http.Post(baseURL+"/urls/"+urlID+"/enqueue", "", nil)
    if err != nil {
        t.Fatalf("POST /urls/{id}/enqueue error: %v", err)
    }
    resp.Body.Close()

    // 3. 収集完了待ち
    time.Sleep(10 * time.Second)

    // 4. 結果確認
    resp, err = http.Get(baseURL + "/urls/" + urlID + "/latest")
    if err != nil {
        t.Fatalf("GET /urls/{id}/latest error: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Fatalf("GET /urls/{id}/latest status = %d, want 200", resp.StatusCode)
    }

    var latestResp map[string]any
    json.NewDecoder(resp.Body).Decode(&latestResp)
    data := latestResp["data"].(map[string]any)

    if data["status"] != "succeeded" {
        t.Errorf("run status = %s, want succeeded", data["status"])
    }
}
```

## 6. テスト実行

### 6.1 コマンド

```bash
# 全テスト実行
go test ./...

# カバレッジ付き
go test -cover ./...

# 特定パッケージ
go test ./internal/httpclient/...

# 詳細出力
go test -v ./...

# E2Eテスト（Docker Compose必要）
docker compose -f docker-compose.test.yml up --abort-on-container-exit
```

### 6.2 CI設定例

```yaml
# .github/workflows/test.yml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest

    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
          POSTGRES_DB: test
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Run tests
        run: go test -v -race -coverprofile=coverage.txt ./...
        env:
          DATABASE_URL: postgres://test:test@localhost:5432/test?sslmode=disable

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage.txt
```

## 7. テストカバレッジ目標

| パッケージ | 目標 | 理由 |
|-----------|------|------|
| httpclient (SSRFGuard) | 90%+ | セキュリティ重要 |
| service (Parser) | 85%+ | ビジネスロジック |
| service (Retrier) | 85%+ | 信頼性 |
| domain | 80%+ | コアロジック |
| repository | 70%+ | DB操作 |
| handler | 60%+ | HTTP層 |
