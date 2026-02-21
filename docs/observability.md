# 観測性設計（Observability）

## 1. 概要

システムの健全性監視と問題調査のため、構造化ログとメトリクスを設計する。

## 2. ログ設計

### 2.1 ログ形式

JSON形式の構造化ログを採用:

```json
{
  "timestamp": "2024-01-15T10:05:00.123Z",
  "level": "info",
  "message": "url fetch completed",
  "url_id": 123,
  "url": "https://example.com/page",
  "domain": "example.com",
  "status": "succeeded",
  "http_status": 200,
  "latency_ms": 245,
  "attempt": 1,
  "worker_id": 3,
  "trace_id": "abc123"
}
```

### 2.2 ログレベル

| レベル | 用途 | 例 |
|--------|------|-----|
| DEBUG | 開発時の詳細情報 | リクエストヘッダ、パース詳細 |
| INFO | 通常の動作記録 | 収集成功、スケジューラ起動 |
| WARN | 回復可能な問題 | リトライ発生、レート制限 |
| ERROR | 要対応の問題 | DB接続失敗、致命的エラー |

### 2.3 標準フィールド

**共通フィールド:**
| フィールド | 型 | 説明 |
|------------|-----|------|
| timestamp | string | ISO8601形式 |
| level | string | ログレベル |
| message | string | ログメッセージ |
| service | string | サービス名（api/worker） |
| version | string | アプリケーションバージョン |
| trace_id | string | リクエストトレースID |

**収集処理フィールド:**
| フィールド | 型 | 説明 |
|------------|-----|------|
| url_id | int | URL ID |
| url | string | 対象URL |
| domain | string | ドメイン名 |
| status | string | 結果ステータス |
| http_status | int | HTTPステータス |
| latency_ms | int | 応答時間 |
| attempt | int | 試行回数 |
| error_code | string | エラーコード |
| error | string | エラーメッセージ |
| worker_id | int | ワーカーID |

### 2.4 実装例

```go
package observability

import (
    "context"
    "encoding/json"
    "io"
    "os"
    "time"
)

type Logger struct {
    output  io.Writer
    service string
    version string
}

type LogEntry struct {
    Timestamp string         `json:"timestamp"`
    Level     string         `json:"level"`
    Message   string         `json:"message"`
    Service   string         `json:"service"`
    Version   string         `json:"version"`
    Fields    map[string]any `json:"fields,omitempty"`
}

func NewLogger(service, version string) *Logger {
    return &Logger{
        output:  os.Stdout,
        service: service,
        version: version,
    }
}

func (l *Logger) log(level, message string, fields map[string]any) {
    entry := LogEntry{
        Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
        Level:     level,
        Message:   message,
        Service:   l.service,
        Version:   l.version,
        Fields:    fields,
    }

    data, _ := json.Marshal(entry)
    l.output.Write(append(data, '\n'))
}

func (l *Logger) Info(message string, fields map[string]any) {
    l.log("info", message, fields)
}

func (l *Logger) Warn(message string, fields map[string]any) {
    l.log("warn", message, fields)
}

func (l *Logger) Error(message string, fields map[string]any) {
    l.log("error", message, fields)
}

func (l *Logger) Debug(message string, fields map[string]any) {
    l.log("debug", message, fields)
}

// コンテキストからトレースIDを取得
func (l *Logger) WithContext(ctx context.Context) *ContextLogger {
    traceID, _ := ctx.Value("trace_id").(string)
    return &ContextLogger{logger: l, traceID: traceID}
}

type ContextLogger struct {
    logger  *Logger
    traceID string
}

func (cl *ContextLogger) Info(message string, fields map[string]any) {
    if fields == nil {
        fields = make(map[string]any)
    }
    fields["trace_id"] = cl.traceID
    cl.logger.Info(message, fields)
}
```

### 2.5 ログ出力例

**収集成功:**
```json
{
  "timestamp": "2024-01-15T10:05:00.123Z",
  "level": "info",
  "message": "url fetch completed",
  "service": "worker",
  "version": "0.1.0",
  "fields": {
    "url_id": 123,
    "url": "https://example.com/page",
    "domain": "example.com",
    "status": "succeeded",
    "http_status": 200,
    "latency_ms": 245,
    "attempt": 1,
    "title": "Example Page",
    "worker_id": 3
  }
}
```

**リトライ発生:**
```json
{
  "timestamp": "2024-01-15T10:05:08.456Z",
  "level": "warn",
  "message": "url fetch failed, will retry",
  "service": "worker",
  "version": "0.1.0",
  "fields": {
    "url_id": 124,
    "url": "https://slow-site.com/page",
    "domain": "slow-site.com",
    "error_code": "timeout",
    "error": "request timeout after 8000ms",
    "attempt": 1,
    "next_attempt_in_sec": 10,
    "worker_id": 5
  }
}
```

**SSRF ブロック:**
```json
{
  "timestamp": "2024-01-15T10:05:10.789Z",
  "level": "warn",
  "message": "url fetch blocked by ssrf guard",
  "service": "worker",
  "version": "0.1.0",
  "fields": {
    "url_id": 125,
    "url": "http://169.254.169.254/",
    "error_code": "ssrf_blocked",
    "error": "link-local address not allowed",
    "worker_id": 2
  }
}
```

## 3. メトリクス設計

### 3.1 収集メトリクス

| メトリクス名 | 型 | ラベル | 説明 |
|-------------|-----|--------|------|
| `fetch_total` | Counter | status, error_code | 収集試行総数 |
| `fetch_duration_ms` | Histogram | status | 収集所要時間 |
| `fetch_response_size_bytes` | Histogram | - | レスポンスサイズ |
| `retry_total` | Counter | attempt | リトライ発生数 |
| `queue_size` | Gauge | - | 待機キュー長 |
| `active_workers` | Gauge | - | 稼働中Worker数 |
| `rate_limit_wait_ms` | Histogram | - | レート制限待機時間 |
| `db_query_duration_ms` | Histogram | operation | DBクエリ所要時間 |

### 3.2 Prometheus形式

```go
package observability

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    FetchTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "url_collector_fetch_total",
            Help: "Total number of URL fetch attempts",
        },
        []string{"status", "error_code"},
    )

    FetchDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "url_collector_fetch_duration_ms",
            Help:    "URL fetch duration in milliseconds",
            Buckets: []float64{50, 100, 250, 500, 1000, 2500, 5000, 10000},
        },
        []string{"status"},
    )

    FetchResponseSize = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "url_collector_fetch_response_size_bytes",
            Help:    "Response size in bytes",
            Buckets: prometheus.ExponentialBuckets(1024, 2, 10), // 1KB to 1MB
        },
    )

    RetryTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "url_collector_retry_total",
            Help: "Total number of retries",
        },
        []string{"attempt"},
    )

    QueueSize = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "url_collector_queue_size",
            Help: "Current number of URLs in queue",
        },
    )

    ActiveWorkers = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "url_collector_active_workers",
            Help: "Number of active workers",
        },
    )

    RateLimitWait = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "url_collector_rate_limit_wait_ms",
            Help:    "Rate limit wait time in milliseconds",
            Buckets: []float64{0, 10, 50, 100, 250, 500, 1000},
        },
    )

    DBQueryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "url_collector_db_query_duration_ms",
            Help:    "Database query duration in milliseconds",
            Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500},
        },
        []string{"operation"},
    )
)

// 使用例
func (w *Worker) recordFetchMetrics(status string, errorCode string, latencyMs int, responseSize int) {
    FetchTotal.WithLabelValues(status, errorCode).Inc()
    FetchDuration.WithLabelValues(status).Observe(float64(latencyMs))
    if responseSize > 0 {
        FetchResponseSize.Observe(float64(responseSize))
    }
}
```

### 3.3 メトリクスエンドポイント

```go
package main

import (
    "net/http"

    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    // /metrics エンドポイントを公開
    http.Handle("/metrics", promhttp.Handler())
    http.ListenAndServe(":9090", nil)
}
```

### 3.4 Prometheus設定例

`prometheus.yml`:
```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'url-collector-api'
    static_configs:
      - targets: ['api:8080']

  - job_name: 'url-collector-worker'
    static_configs:
      - targets: ['worker:9090']
```

## 4. ダッシュボード設計

### 4.1 主要パネル

**概要ダッシュボード:**
- 収集成功率（過去1時間）
- 平均応答時間（過去1時間）
- エラー内訳（円グラフ）
- 処理スループット（時系列）

**詳細ダッシュボード:**
- レイテンシ分布（ヒストグラム）
- Worker稼働状況
- キュー長推移
- DBクエリ性能

### 4.2 Grafanaクエリ例

**成功率:**
```promql
sum(rate(url_collector_fetch_total{status="succeeded"}[5m])) /
sum(rate(url_collector_fetch_total[5m])) * 100
```

**P95レイテンシ:**
```promql
histogram_quantile(0.95,
  sum(rate(url_collector_fetch_duration_ms_bucket[5m])) by (le)
)
```

**エラー率（種類別）:**
```promql
sum(rate(url_collector_fetch_total{status="failed"}[5m])) by (error_code)
```

## 5. アラート設計

### 5.1 アラートルール

```yaml
groups:
  - name: url-collector
    rules:
      # 成功率低下
      - alert: LowSuccessRate
        expr: |
          sum(rate(url_collector_fetch_total{status="succeeded"}[5m])) /
          sum(rate(url_collector_fetch_total[5m])) < 0.9
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "URL収集成功率が90%を下回っています"

      # 高レイテンシ
      - alert: HighLatency
        expr: |
          histogram_quantile(0.95,
            sum(rate(url_collector_fetch_duration_ms_bucket[5m])) by (le)
          ) > 5000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P95レイテンシが5秒を超えています"

      # Worker停止
      - alert: NoActiveWorkers
        expr: url_collector_active_workers == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "稼働中のWorkerがありません"

      # キュー滞留
      - alert: QueueBacklog
        expr: url_collector_queue_size > 1000
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "キューが滞留しています"
```

## 6. トレーシング（オプション）

### 6.1 トレースID伝播

```go
package observability

import (
    "context"
    "crypto/rand"
    "encoding/hex"
)

type contextKey string

const traceIDKey contextKey = "trace_id"

func GenerateTraceID() string {
    b := make([]byte, 16)
    rand.Read(b)
    return hex.EncodeToString(b)
}

func ContextWithTraceID(ctx context.Context) context.Context {
    return context.WithValue(ctx, traceIDKey, GenerateTraceID())
}

func TraceIDFromContext(ctx context.Context) string {
    if id, ok := ctx.Value(traceIDKey).(string); ok {
        return id
    }
    return ""
}
```

### 6.2 HTTPミドルウェア

```go
func TraceMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        traceID := r.Header.Get("X-Trace-ID")
        if traceID == "" {
            traceID = GenerateTraceID()
        }

        ctx := context.WithValue(r.Context(), traceIDKey, traceID)
        w.Header().Set("X-Trace-ID", traceID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

## 7. 設定

| 環境変数 | デフォルト | 説明 |
|----------|------------|------|
| `LOG_LEVEL` | info | ログレベル |
| `LOG_FORMAT` | json | ログ形式（json/text） |
| `METRICS_ENABLED` | true | メトリクス有効化 |
| `METRICS_PORT` | 9090 | メトリクスポート |

## 8. 運用Tips

### 8.1 問題調査フロー

1. **アラート発生** → Grafanaダッシュボードで傾向確認
2. **エラー率上昇** → `error_code`ラベルで内訳確認
3. **詳細調査** → ログからトレースIDで関連ログ抽出
4. **根本原因特定** → 特定URLやドメインに問題が集中していないか確認

### 8.2 ログ検索例（jq）

```bash
# 失敗ログのみ抽出
cat logs.json | jq 'select(.fields.status == "failed")'

# 特定ドメインのログ
cat logs.json | jq 'select(.fields.domain == "slow-site.com")'

# タイムアウトエラー
cat logs.json | jq 'select(.fields.error_code == "timeout")'
```
