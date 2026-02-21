# システムアーキテクチャ

## 1. 全体構成

```
                                    ┌─────────────────────────────────────────┐
                                    │           Docker Compose                │
                                    │                                         │
┌──────────┐                        │  ┌─────────────┐    ┌─────────────┐    │
│  Client  │──HTTP──────────────────┼─▶│ API Server  │───▶│ PostgreSQL  │    │
│ (curl等) │                        │  │  :8080      │    │   :5432     │    │
└──────────┘                        │  └─────────────┘    └──────┬──────┘    │
                                    │                            │           │
                                    │  ┌─────────────┐           │           │
                                    │  │   Worker    │───────────┘           │
                                    │  │ (Scheduler) │                       │
                                    │  └──────┬──────┘                       │
                                    │         │                              │
                                    └─────────┼──────────────────────────────┘
                                              │
                                              ▼
                                    ┌─────────────────┐
                                    │  External URLs  │
                                    │ (Target Sites)  │
                                    └─────────────────┘
```

## 2. コンポーネント詳細

### 2.1 API Server (`cmd/api`)

URL管理と収集結果の参照を提供するRESTful APIサーバ。

**責務:**
- URL CRUD操作
- 収集結果の参照（最新・履歴）
- 手動実行のキュー投入
- ヘルスチェック

**技術選定:**
- 標準ライブラリ `net/http` またはルーターライブラリ（chi/gin等）
- JSON形式でリクエスト/レスポンス

### 2.2 Worker (`cmd/worker`)

定期的にURLを収集するバックグラウンドプロセス。

**責務:**
- スケジューリング（`next_run_at`ベース）
- HTTP取得（タイムアウト/リトライ/レート制限）
- HTMLパース（title/OGP/meta）
- 結果保存

**実行モデル:**
- Ticker/Cronベースのポーリング
- Worker Pool（goroutine + channel）
- 並行数制御（semaphore pattern）

### 2.3 Database (PostgreSQL)

URLマスタと収集結果を永続化。

**テーブル:**
- `urls`: URL登録情報
- `url_runs`: 収集結果履歴

## 3. ディレクトリ構成

```
.
├── cmd/
│   ├── api/           # APIサーバのエントリポイント
│   │   └── main.go
│   └── worker/        # ワーカーのエントリポイント
│       └── main.go
├── internal/
│   ├── domain/        # ドメインモデル（Entity/ValueObject）
│   │   ├── url.go
│   │   └── run.go
│   ├── repository/    # データアクセス層
│   │   ├── url_repository.go
│   │   └── run_repository.go
│   ├── service/       # ビジネスロジック
│   │   ├── fetcher.go      # HTTP取得
│   │   ├── parser.go       # HTMLパース
│   │   └── scheduler.go    # スケジューリング
│   ├── handler/       # HTTPハンドラ（API）
│   │   └── url_handler.go
│   ├── httpclient/    # HTTPクライアント設定
│   │   ├── client.go       # カスタムHTTPクライアント
│   │   └── ssrf_guard.go   # SSRF対策
│   ├── config/        # 設定管理
│   │   └── config.go
│   └── observability/ # ログ・メトリクス
│       ├── logger.go
│       └── metrics.go
├── migrations/        # DBマイグレーション
│   └── 001_initial.sql
├── docs/              # ドキュメント
├── docker-compose.yml
├── Dockerfile
├── go.mod
├── go.sum
└── README.md
```

## 4. データフロー

### 4.1 URL登録フロー

```
Client                API Server              Database
  │                       │                       │
  │  POST /urls           │                       │
  │──────────────────────▶│                       │
  │                       │  INSERT urls          │
  │                       │──────────────────────▶│
  │                       │                       │
  │                       │  next_run_at = NOW()  │
  │                       │◀──────────────────────│
  │  201 Created          │                       │
  │◀──────────────────────│                       │
```

### 4.2 収集実行フロー

```
Worker                  Database              External Site
  │                       │                       │
  │  SELECT ... FOR UPDATE│                       │
  │  SKIP LOCKED          │                       │
  │  WHERE next_run_at <= │                       │
  │        NOW()          │                       │
  │──────────────────────▶│                       │
  │                       │                       │
  │  URLs to process      │                       │
  │◀──────────────────────│                       │
  │                       │                       │
  │  HTTP GET url         │                       │
  │──────────────────────────────────────────────▶│
  │                       │                       │
  │  Response (HTML)      │                       │
  │◀──────────────────────────────────────────────│
  │                       │                       │
  │  Parse title/OGP      │                       │
  │                       │                       │
  │  INSERT url_runs      │                       │
  │──────────────────────▶│                       │
  │                       │                       │
  │  UPDATE urls          │                       │
  │  SET next_run_at =    │                       │
  │      NOW() + interval │                       │
  │──────────────────────▶│                       │
```

## 5. 並行処理モデル

### 5.1 Worker Pool パターン

```go
// 概念的なコード
func (w *Worker) Run(ctx context.Context) {
    jobs := make(chan *domain.URL, w.config.QueueSize)

    // Worker goroutines
    for i := 0; i < w.config.MaxWorkers; i++ {
        go w.worker(ctx, jobs)
    }

    // Scheduler goroutine
    ticker := time.NewTicker(w.config.PollInterval)
    for {
        select {
        case <-ticker.C:
            urls := w.repo.FetchDueURLs(ctx, w.config.BatchSize)
            for _, url := range urls {
                jobs <- url
            }
        case <-ctx.Done():
            return
        }
    }
}

func (w *Worker) worker(ctx context.Context, jobs <-chan *domain.URL) {
    for {
        select {
        case url := <-jobs:
            w.processURL(ctx, url)
        case <-ctx.Done():
            return
        }
    }
}
```

### 5.2 レート制限

```
┌─────────────────────────────────────────┐
│           Rate Limiter                  │
│         (Token Bucket)                  │
│                                         │
│  Tokens: ●●●●●○○○○○ (5/10)             │
│  Refill: 5 tokens/sec                   │
│                                         │
│  Request arrives:                       │
│    - Has token? → Process               │
│    - No token?  → Wait                  │
└─────────────────────────────────────────┘
```

## 6. 設定項目

| 環境変数 | デフォルト | 説明 |
|----------|------------|------|
| `DATABASE_URL` | - | PostgreSQL接続文字列 |
| `API_PORT` | 8080 | APIサーバポート |
| `MAX_WORKERS` | 10 | 最大同時実行数 |
| `RATE_LIMIT_PER_SEC` | 5 | 秒間リクエスト上限 |
| `REQUEST_TIMEOUT` | 8s | HTTPリクエストタイムアウト |
| `MAX_RETRY_ATTEMPTS` | 3 | 最大リトライ回数 |
| `RETRY_BASE_DELAY` | 10s | リトライ基本待機時間 |
| `MAX_REDIRECT` | 5 | 最大リダイレクト回数 |
| `MAX_RESPONSE_SIZE` | 1MB | レスポンスサイズ上限 |
| `POLL_INTERVAL` | 5s | スケジューラポーリング間隔 |
| `BATCH_SIZE` | 100 | 1回の取得バッチサイズ |

## 7. スケーラビリティ考慮

### 7.1 水平スケーリング

- **Worker**: 複数インスタンス可能（`SKIP LOCKED`による排他制御）
- **API**: ステートレス設計で複数インスタンス可能
- **DB**: 単一マスター（将来的にリードレプリカ追加可）

### 7.2 ボトルネック対策

| 箇所 | 対策 |
|------|------|
| DB接続 | コネクションプール |
| 外部アクセス | レート制限 + タイムアウト |
| メモリ | レスポンスサイズ制限 |
| CPU | Worker数制限 |

## 8. 障害対応

### 8.1 Worker障害時

- 処理中のURLは`next_run_at`が未更新のため、次回スケジュール実行時に再取得される
- DBトランザクションによる一貫性保証

### 8.2 外部サイト障害時

- タイムアウトで打ち切り
- リトライ（指数バックオフ）
- 最終的にfailed記録

### 8.3 DB障害時

- API/Workerともにエラーレスポンス
- 再起動後に自動復旧（ステートはDBに集約）
