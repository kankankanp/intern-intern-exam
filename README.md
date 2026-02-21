# URL Metadata Collector

URLメタデータ収集基盤 - 指定したURLのタイトル、OGP、HTTPステータスなどを定期的に収集するシステム。

## 機能

- **URL管理API**: URLの登録・更新・削除・一覧取得
- **定期収集**: 設定した間隔でURLのメタデータを自動収集
- **メタデータ抽出**: title、meta description、OGP情報を抽出
- **並行処理**: Worker Poolによる効率的な並行収集
- **レート制限**: 外部サイトへの負荷軽減
- **リトライ**: 指数バックオフによる失敗時の再試行
- **SSRF対策**: プライベートIP/ローカルホストへのアクセス防止

## クイックスタート

### Docker Composeで起動

```bash
# 起動
docker compose up -d

# ログ確認
docker compose logs -f

# 停止
docker compose down
```

### 動作確認

```bash
# URL登録
curl -X POST http://localhost:8080/urls \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "intervalSeconds": 300,
    "tags": ["test"]
  }'

# URL一覧
curl http://localhost:8080/urls

# 手動実行（即時収集）
curl -X POST http://localhost:8080/urls/1/enqueue

# 最新結果確認
curl http://localhost:8080/urls/1/latest

# 収集履歴
curl http://localhost:8080/urls/1/runs
```

## API仕様

### エンドポイント一覧

| メソッド | パス | 説明 |
|----------|------|------|
| POST | `/urls` | URL登録 |
| GET | `/urls` | URL一覧取得 |
| GET | `/urls/{id}` | URL詳細取得 |
| PATCH | `/urls/{id}` | URL更新 |
| DELETE | `/urls/{id}` | URL削除 |
| GET | `/urls/{id}/latest` | 最新収集結果 |
| GET | `/urls/{id}/runs` | 収集履歴 |
| POST | `/urls/{id}/enqueue` | 手動実行 |
| GET | `/health` | ヘルスチェック |

### URL登録リクエスト

```json
{
  "url": "https://example.com/page",
  "intervalSeconds": 3600,
  "tags": ["recruitment", "engineering"],
  "enabled": true,
  "maxConcurrencyGroup": "example.com"
}
```

### 収集結果レスポンス

```json
{
  "data": {
    "id": 1,
    "urlId": 1,
    "status": "succeeded",
    "httpStatus": 200,
    "finalUrl": "https://example.com/page/",
    "contentType": "text/html; charset=utf-8",
    "latencyMs": 245,
    "title": "Example Page Title",
    "description": "This is the meta description",
    "ogTitle": "Example OG Title",
    "ogDescription": "This is the OG description",
    "ogImage": "https://example.com/og-image.png",
    "ogUrl": "https://example.com/page",
    "ogSiteName": "Example Site",
    "attempt": 1,
    "runAt": "2024-01-15T10:05:00Z"
  }
}
```

## 環境変数

| 変数 | デフォルト | 説明 |
|------|------------|------|
| `DATABASE_URL` | - | PostgreSQL接続文字列 |
| `API_PORT` | 8080 | APIサーバポート |
| `MAX_WORKERS` | 10 | 最大同時実行Worker数 |
| `RATE_LIMIT_PER_SEC` | 5 | 秒間リクエスト上限（グローバル） |
| `DOMAIN_RATE_LIMIT_PER_SEC` | 2 | ドメイン別秒間リクエスト上限 |
| `REQUEST_TIMEOUT` | 8s | HTTPリクエストタイムアウト |
| `MAX_RETRY_ATTEMPTS` | 3 | 最大リトライ回数 |
| `RETRY_BASE_DELAY` | 10s | リトライ基本待機時間 |
| `POLL_INTERVAL` | 5s | スケジューラポーリング間隔 |
| `BATCH_SIZE` | 100 | 1回のポーリングで取得するURL数 |

## アーキテクチャ

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ API Server  │     │   Worker    │     │ PostgreSQL  │
│   :8080     │────▶│ (Scheduler) │────▶│   :5432     │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │ External    │
                    │ URLs        │
                    └─────────────┘
```

### コンポーネント

- **API Server** (`cmd/api`): URL管理と結果参照のREST API
- **Worker** (`cmd/worker`): 定期収集を行うバックグラウンドプロセス
- **PostgreSQL**: URLマスタと収集結果を永続化

## 設計判断

### 排他制御

`SELECT ... FOR UPDATE SKIP LOCKED`を使用し、複数Workerインスタンスでも安全に並行処理可能。

### リトライ戦略

- 対象: タイムアウト、一時的ネットワークエラー、5xx
- 非対象: SSRFブロック、4xx
- バックオフ: `base * 2^(attempt-1)`（例: 10s→20s→40s）

### SSRF対策

多層防御アプローチ:
1. スキーム検証（http/httpsのみ）
2. ホスト検証（プライベートIP/localhost拒否）
3. DNS解決後のIP検証
4. リダイレクト先の再検証

ブロック対象:
- `127.0.0.0/8` (ループバック)
- `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16` (プライベート)
- `169.254.0.0/16` (リンクローカル/AWSメタデータ)

### ドメイン別レート制限

各ドメインに対して個別にレート制限を適用（デフォルト: 2件/秒）。同一ドメインへの過度な負荷を防ぎつつ、異なるドメインは並行処理可能。

### 同時実行制御

`maxConcurrencyGroup`を指定すると、同一グループのURLは完全に直列実行され、より厳格な負荷軽減が可能。

## 開発

### ローカル環境

```bash
# 依存関係インストール
go mod download

# テスト実行
go test ./...

# ビルド
go build -o bin/api ./cmd/api
go build -o bin/worker ./cmd/worker

# 実行（要PostgreSQL）
./bin/api
./bin/worker
```

### プロジェクト構造

```
.
├── cmd/
│   ├── api/           # APIサーバエントリポイント
│   └── worker/        # ワーカーエントリポイント
├── internal/
│   ├── config/        # 設定管理
│   ├── domain/        # ドメインモデル
│   ├── handler/       # HTTPハンドラ
│   ├── httpclient/    # HTTPクライアント/SSRF対策
│   ├── observability/ # ログ
│   ├── repository/    # データアクセス
│   └── service/       # ビジネスロジック
├── migrations/        # DBマイグレーション
├── docs/              # 設計ドキュメント
├── docker-compose.yml
├── Dockerfile
└── README.md
```

## ドキュメント

詳細な設計ドキュメントは`docs/`ディレクトリを参照:

- [アーキテクチャ](docs/architecture.md)
- [API仕様](docs/api-specification.md)
- [ワーカー設計](docs/worker-design.md)
- [データベーススキーマ](docs/database-schema.md)
- [セキュリティ](docs/security.md)
- [観測性](docs/observability.md)
- [テスト戦略](docs/testing-strategy.md)

## ライセンス

MIT
