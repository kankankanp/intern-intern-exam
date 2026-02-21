# 収集ワーカー設計

## 1. 概要

収集ワーカーは、登録されたURLを定期的に取得し、メタデータを収集するバックグラウンドプロセスである。

## 2. 主要コンポーネント

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Worker Process                             │
│                                                                     │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────────────────┐ │
│  │  Scheduler  │───▶│  Job Queue  │───▶│     Worker Pool         │ │
│  │  (Ticker)   │    │  (Channel)  │    │  (N goroutines)         │ │
│  └─────────────┘    └─────────────┘    └───────────┬─────────────┘ │
│         │                                          │               │
│         │                                          ▼               │
│         │                              ┌─────────────────────────┐ │
│         │                              │       Fetcher           │ │
│         │                              │  ┌─────────────────┐    │ │
│         │                              │  │  Rate Limiter   │    │ │
│         │                              │  ├─────────────────┤    │ │
│         │                              │  │  SSRF Guard     │    │ │
│         │                              │  ├─────────────────┤    │ │
│         │                              │  │  HTTP Client    │    │ │
│         │                              │  └─────────────────┘    │ │
│         │                              └───────────┬─────────────┘ │
│         │                                          │               │
│         │                                          ▼               │
│         │                              ┌─────────────────────────┐ │
│         │                              │       Parser            │ │
│         │                              │  (HTML → Metadata)      │ │
│         │                              └───────────┬─────────────┘ │
│         │                                          │               │
│         ▼                                          ▼               │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                      Repository (DB)                        │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

## 3. スケジューリング

### 3.1 ポーリングベース方式

定期的にDBを参照し、実行対象のURLを取得する。

```go
// スケジューラのメインループ
func (s *Scheduler) Run(ctx context.Context) {
    ticker := time.NewTicker(s.pollInterval) // 例: 5秒
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            s.pollAndDispatch(ctx)
        case <-ctx.Done():
            return
        }
    }
}

func (s *Scheduler) pollAndDispatch(ctx context.Context) {
    // 実行対象URLを取得（排他制御付き）
    urls, err := s.repo.FetchDueURLsWithLock(ctx, s.batchSize)
    if err != nil {
        s.logger.Error("failed to fetch URLs", "error", err)
        return
    }

    for _, url := range urls {
        s.jobQueue <- url
    }
}
```

### 3.2 対象URL取得クエリ

```sql
SELECT id, url, interval_seconds, concurrency_group
FROM urls
WHERE enabled = true
  AND next_run_at <= NOW()
ORDER BY next_run_at ASC
LIMIT :batch_size
FOR UPDATE SKIP LOCKED;
```

**ポイント:**
- `FOR UPDATE SKIP LOCKED`: 他のワーカーがロック中の行をスキップ
- 複数ワーカーインスタンスでも安全に並行実行可能

### 3.3 next_run_at の更新

収集完了後、次回実行時刻を更新:

```sql
UPDATE urls
SET next_run_at = NOW() + (interval_seconds * interval '1 second'),
    updated_at = NOW()
WHERE id = :url_id;
```

## 4. 並行処理

### 4.1 Worker Pool パターン

```go
type WorkerPool struct {
    maxWorkers int
    jobQueue   chan *domain.URL
    fetcher    *Fetcher
    repo       *repository.Repository
}

func (wp *WorkerPool) Start(ctx context.Context) {
    for i := 0; i < wp.maxWorkers; i++ {
        go wp.worker(ctx, i)
    }
}

func (wp *WorkerPool) worker(ctx context.Context, id int) {
    for {
        select {
        case job := <-wp.jobQueue:
            wp.processJob(ctx, job)
        case <-ctx.Done():
            return
        }
    }
}
```

### 4.2 同時実行数制御

**環境変数:** `MAX_WORKERS=10`

goroutine数 = Worker数として制御。channelのバッファサイズで待機キュー長を制御。

### 4.3 Concurrency Group（オプション・上位評価）

同一グループのURLは直列実行することで、特定サイトへの負荷を軽減。

```go
type ConcurrencyController struct {
    groupLocks sync.Map // map[string]*sync.Mutex
}

func (cc *ConcurrencyController) Execute(group string, fn func()) {
    if group == "" {
        fn() // グループ指定なしは即時実行
        return
    }

    lock := cc.getOrCreateLock(group)
    lock.Lock()
    defer lock.Unlock()
    fn()
}
```

## 5. HTTP取得 (Fetcher)

### 5.1 HTTPクライアント設定

```go
type Fetcher struct {
    client      *http.Client
    rateLimiter *rate.Limiter
    ssrfGuard   *SSRFGuard
    config      FetcherConfig
}

type FetcherConfig struct {
    Timeout         time.Duration // 8秒
    MaxRedirects    int           // 5
    MaxResponseSize int64         // 1MB
    UserAgent       string        // "InternInc-MetadataCollector/0.1"
}

func NewFetcher(cfg FetcherConfig) *Fetcher {
    client := &http.Client{
        Timeout: cfg.Timeout,
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            if len(via) >= cfg.MaxRedirects {
                return errors.New("too many redirects")
            }
            // リダイレクト先もSSRFチェック
            return nil
        },
    }
    return &Fetcher{client: client, config: cfg}
}
```

### 5.2 取得フロー

```go
func (f *Fetcher) Fetch(ctx context.Context, targetURL string) (*FetchResult, error) {
    // 1. レート制限
    if err := f.rateLimiter.Wait(ctx); err != nil {
        return nil, fmt.Errorf("rate limit: %w", err)
    }

    // 2. SSRF検証
    if err := f.ssrfGuard.Validate(targetURL); err != nil {
        return nil, fmt.Errorf("ssrf blocked: %w", err)
    }

    // 3. リクエスト作成
    req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("User-Agent", f.config.UserAgent)

    // 4. リクエスト実行
    start := time.Now()
    resp, err := f.client.Do(req)
    latency := time.Since(start)

    if err != nil {
        return nil, f.classifyError(err)
    }
    defer resp.Body.Close()

    // 5. レスポンス読み取り（サイズ制限付き）
    body, err := io.ReadAll(io.LimitReader(resp.Body, f.config.MaxResponseSize))
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
```

### 5.3 レート制限

**グローバルレート制限:**
```go
// 秒間5リクエストまで
rateLimiter := rate.NewLimiter(rate.Limit(5), 5)
```

**ドメイン別レート制限（オプション）:**
```go
type DomainRateLimiter struct {
    limiters sync.Map // map[string]*rate.Limiter
    limit    rate.Limit
    burst    int
}

func (d *DomainRateLimiter) Wait(ctx context.Context, host string) error {
    limiter := d.getOrCreate(host)
    return limiter.Wait(ctx)
}
```

## 6. HTMLパース (Parser)

### 6.1 パーサ実装

```go
type Metadata struct {
    Title         string
    Description   string
    OGTitle       string
    OGDescription string
    OGImage       string
    OGURL         string
    OGSiteName    string
}

func ParseHTML(body []byte) (*Metadata, error) {
    doc, err := html.Parse(bytes.NewReader(body))
    if err != nil {
        return nil, err
    }

    meta := &Metadata{}
    var f func(*html.Node)
    f = func(n *html.Node) {
        if n.Type == html.ElementNode {
            switch n.Data {
            case "title":
                if n.FirstChild != nil {
                    meta.Title = strings.TrimSpace(n.FirstChild.Data)
                }
            case "meta":
                parseMeta(n, meta)
            }
        }
        for c := n.FirstChild; c != nil; c = c.NextSibling {
            f(c)
        }
    }
    f(doc)
    return meta, nil
}

func parseMeta(n *html.Node, meta *Metadata) {
    var property, name, content string
    for _, attr := range n.Attr {
        switch attr.Key {
        case "property":
            property = attr.Val
        case "name":
            name = attr.Val
        case "content":
            content = attr.Val
        }
    }

    switch property {
    case "og:title":
        meta.OGTitle = content
    case "og:description":
        meta.OGDescription = content
    case "og:image":
        meta.OGImage = content
    case "og:url":
        meta.OGURL = content
    case "og:site_name":
        meta.OGSiteName = content
    }

    if name == "description" {
        meta.Description = content
    }
}
```

## 7. リトライ処理

### 7.1 リトライ対象

| 状況 | リトライ |
|------|----------|
| タイムアウト | ○ |
| 一時的ネットワークエラー | ○ |
| DNS解決失敗 | ○ |
| 5xx エラー | ○ |
| 4xx エラー | × |
| SSRFブロック | × |
| パースエラー | × |

### 7.2 指数バックオフ

```go
type RetryConfig struct {
    MaxAttempts int           // 3
    BaseDelay   time.Duration // 10秒
    MaxDelay    time.Duration // 2分
}

func (r *Retrier) CalculateDelay(attempt int) time.Duration {
    delay := r.config.BaseDelay * time.Duration(1<<uint(attempt-1)) // 2^(attempt-1)
    if delay > r.config.MaxDelay {
        delay = r.config.MaxDelay
    }
    // ジッター追加（±10%）
    jitter := time.Duration(rand.Float64()*0.2-0.1) * delay
    return delay + jitter
}
```

**バックオフ例:**
| 試行 | 待機時間 |
|------|----------|
| 1回目 | 即時 |
| 2回目 | 10秒 |
| 3回目 | 20秒 |

### 7.3 リトライ実装

```go
func (w *Worker) processWithRetry(ctx context.Context, url *domain.URL) *domain.Run {
    var lastErr error
    for attempt := 1; attempt <= w.retryConfig.MaxAttempts; attempt++ {
        result, err := w.fetcher.Fetch(ctx, url.URL)
        if err == nil {
            return w.createSuccessRun(url, result, attempt)
        }

        lastErr = err
        if !isRetryable(err) {
            break
        }

        if attempt < w.retryConfig.MaxAttempts {
            delay := w.retrier.CalculateDelay(attempt)
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                return w.createFailedRun(url, ctx.Err(), attempt)
            }
        }
    }
    return w.createFailedRun(url, lastErr, w.retryConfig.MaxAttempts)
}

func isRetryable(err error) bool {
    var ssrfErr *SSRFError
    if errors.As(err, &ssrfErr) {
        return false
    }
    var httpErr *HTTPError
    if errors.As(err, &httpErr) && httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
        return false
    }
    return true
}
```

## 8. 結果保存

### 8.1 トランザクション処理

```go
func (w *Worker) saveResult(ctx context.Context, url *domain.URL, run *domain.Run) error {
    return w.repo.WithTransaction(ctx, func(tx *sql.Tx) error {
        // 1. 収集結果を保存
        if err := w.repo.InsertRun(ctx, tx, run); err != nil {
            return err
        }

        // 2. 次回実行時刻を更新
        nextRunAt := time.Now().Add(time.Duration(url.IntervalSeconds) * time.Second)
        if err := w.repo.UpdateNextRunAt(ctx, tx, url.ID, nextRunAt); err != nil {
            return err
        }

        return nil
    })
}
```

## 9. グレースフルシャットダウン

```go
func (w *Worker) Run(ctx context.Context) error {
    // シグナルハンドリング
    ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
    defer cancel()

    // Worker Pool起動
    w.pool.Start(ctx)

    // Scheduler起動
    w.scheduler.Run(ctx)

    // コンテキストキャンセル待ち
    <-ctx.Done()

    // グレースフル停止
    w.logger.Info("shutting down, waiting for active jobs...")
    w.pool.Wait() // 実行中のジョブ完了待ち
    w.logger.Info("shutdown complete")

    return nil
}
```

## 10. 設定項目

| 環境変数 | デフォルト | 説明 |
|----------|------------|------|
| `MAX_WORKERS` | 10 | 最大同時実行Worker数 |
| `RATE_LIMIT_PER_SEC` | 5 | 秒間リクエスト上限 |
| `REQUEST_TIMEOUT` | 8s | HTTPリクエストタイムアウト |
| `MAX_RETRY_ATTEMPTS` | 3 | 最大リトライ回数 |
| `RETRY_BASE_DELAY` | 10s | リトライ基本待機時間 |
| `RETRY_MAX_DELAY` | 2m | リトライ最大待機時間 |
| `MAX_REDIRECTS` | 5 | 最大リダイレクト回数 |
| `MAX_RESPONSE_SIZE` | 1MB | レスポンスサイズ上限 |
| `POLL_INTERVAL` | 5s | スケジューラポーリング間隔 |
| `BATCH_SIZE` | 100 | 1回のポーリングで取得するURL数 |
| `USER_AGENT` | `InternInc-MetadataCollector/0.1` | User-Agent文字列 |
