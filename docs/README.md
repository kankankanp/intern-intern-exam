# URL Metadata Collector - 設計ドキュメント

URLメタデータ収集基盤の設計ドキュメント一覧です。

## ドキュメント構成

| ドキュメント | 内容 |
|------------|------|
| [architecture.md](./architecture.md) | システムアーキテクチャ全体像 |
| [api-specification.md](./api-specification.md) | REST API仕様 |
| [worker-design.md](./worker-design.md) | 収集ワーカーの設計 |
| [database-schema.md](./database-schema.md) | データベーススキーマ |
| [security.md](./security.md) | セキュリティ設計（SSRF対策等） |
| [observability.md](./observability.md) | ログ・メトリクス設計 |
| [testing-strategy.md](./testing-strategy.md) | テスト戦略 |

## プロジェクト概要

### 目的

社内で扱う「募集ページ / 記事 / LP」などのURLに対し、以下のメタ情報を定期収集し、品質監視・SEO運用・リンク切れ検知に活用できる基盤を構築する。

### 収集対象

- HTTP status code
- 最終到達URL（リダイレクト後）
- 応答時間（ms）
- Content-Type
- HTMLの場合：
  - `<title>`
  - OGP: `og:title`, `og:description`, `og:image`, `og:url`, `og:site_name`
  - meta description

### 主要コンポーネント

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   API Server    │     │     Worker      │     │   PostgreSQL    │
│   (REST API)    │────▶│  (Scheduler +   │────▶│   (Database)    │
│                 │     │   Fetcher)      │     │                 │
└─────────────────┘     └─────────────────┘     └─────────────────┘
         │                       │
         │                       ▼
         │              ┌─────────────────┐
         └─────────────▶│  External URLs  │
                        │  (Target Sites) │
                        └─────────────────┘
```

### 技術スタック

- **言語**: Go 1.21+
- **データベース**: PostgreSQL（推奨）/ SQLite
- **コンテナ**: Docker / Docker Compose
- **HTTPクライアント**: 標準ライブラリ + カスタムガード
- **HTMLパーサ**: `golang.org/x/net/html`

## クイックスタート

```bash
# 起動
docker compose up -d

# URL登録
curl -X POST http://localhost:8080/urls \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com", "intervalSeconds": 3600}'

# 結果確認
curl http://localhost:8080/urls/1/latest
```

## 設計判断サマリ

| 項目 | 判断 | 理由 |
|------|------|------|
| DBロック方式 | `SELECT ... FOR UPDATE SKIP LOCKED` | シンプルで十分な並行制御 |
| リトライ戦略 | 指数バックオフ（最大3回） | サーバ負荷軽減 |
| レート制限 | トークンバケット | バースト許容しつつ平均制限 |
| SSRF対策 | IP検証 + スキーム制限 + リダイレクト再検証 | 多層防御 |
| ログ形式 | JSON構造化ログ | 検索・分析の容易さ |
