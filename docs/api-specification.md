# REST API 仕様

## 1. 概要

URL管理と収集結果参照のためのREST APIを提供する。

**Base URL:** `http://localhost:8080`

**共通ヘッダ:**
- `Content-Type: application/json`

**共通レスポンス形式:**

成功時:
```json
{
  "data": { ... }
}
```

エラー時:
```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human readable message"
  }
}
```

---

## 2. エンドポイント一覧

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

---

## 3. URL管理API

### 3.1 URL登録

**POST /urls**

新しいURLを収集対象として登録する。

**Request Body:**
```json
{
  "url": "https://example.com/page",
  "tags": ["recruitment", "engineering"],
  "intervalSeconds": 86400,
  "enabled": true,
  "maxConcurrencyGroup": "example.com"
}
```

| フィールド | 型 | 必須 | デフォルト | 説明 |
|------------|-----|------|------------|------|
| url | string | ○ | - | 収集対象URL（http/httpsのみ） |
| tags | string[] | - | [] | 分類用タグ |
| intervalSeconds | int | ○ | - | 収集間隔（秒） |
| enabled | bool | - | true | 有効/無効フラグ |
| maxConcurrencyGroup | string | - | null | 同時実行制御グループ |

**Response (201 Created):**
```json
{
  "data": {
    "id": 1,
    "url": "https://example.com/page",
    "tags": ["recruitment", "engineering"],
    "intervalSeconds": 86400,
    "enabled": true,
    "maxConcurrencyGroup": "example.com",
    "nextRunAt": "2024-01-15T10:00:00Z",
    "createdAt": "2024-01-15T10:00:00Z",
    "updatedAt": "2024-01-15T10:00:00Z"
  }
}
```

**Error Responses:**
- `400 Bad Request`: バリデーションエラー
- `409 Conflict`: URL重複

**バリデーションルール:**
- `url`: 有効なURL形式、http/httpsスキームのみ
- `intervalSeconds`: 60以上（1分以上）
- SSRF対策: プライベートIP、localhost等は拒否

**curl例:**
```bash
curl -X POST http://localhost:8080/urls \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "intervalSeconds": 3600,
    "tags": ["main"]
  }'
```

---

### 3.2 URL一覧取得

**GET /urls**

登録済みURLの一覧を取得する。

**Query Parameters:**
| パラメータ | 型 | デフォルト | 説明 |
|------------|-----|------------|------|
| enabled | bool | - | 有効/無効でフィルタ |
| tag | string | - | タグでフィルタ |
| q | string | - | URL部分一致検索 |
| page | int | 1 | ページ番号 |
| pageSize | int | 20 | 1ページあたり件数（max 100） |

**Response (200 OK):**
```json
{
  "data": {
    "items": [
      {
        "id": 1,
        "url": "https://example.com/page",
        "tags": ["recruitment"],
        "intervalSeconds": 86400,
        "enabled": true,
        "maxConcurrencyGroup": "example.com",
        "nextRunAt": "2024-01-16T10:00:00Z",
        "createdAt": "2024-01-15T10:00:00Z",
        "updatedAt": "2024-01-15T10:00:00Z"
      }
    ],
    "pagination": {
      "page": 1,
      "pageSize": 20,
      "totalItems": 150,
      "totalPages": 8
    }
  }
}
```

**curl例:**
```bash
# 有効なURLのみ
curl "http://localhost:8080/urls?enabled=true"

# タグでフィルタ
curl "http://localhost:8080/urls?tag=recruitment"

# キーワード検索
curl "http://localhost:8080/urls?q=example.com"
```

---

### 3.3 URL詳細取得

**GET /urls/{id}**

指定IDのURL詳細を取得する。

**Response (200 OK):**
```json
{
  "data": {
    "id": 1,
    "url": "https://example.com/page",
    "tags": ["recruitment", "engineering"],
    "intervalSeconds": 86400,
    "enabled": true,
    "maxConcurrencyGroup": "example.com",
    "nextRunAt": "2024-01-16T10:00:00Z",
    "createdAt": "2024-01-15T10:00:00Z",
    "updatedAt": "2024-01-15T10:00:00Z"
  }
}
```

**Error Responses:**
- `404 Not Found`: URL未登録

---

### 3.4 URL更新

**PATCH /urls/{id}**

URLの設定を部分更新する。

**Request Body:**
```json
{
  "tags": ["recruitment", "updated"],
  "intervalSeconds": 43200,
  "enabled": false,
  "maxConcurrencyGroup": "new-group"
}
```

すべてのフィールドはオプション。指定されたフィールドのみ更新。

**Response (200 OK):**
```json
{
  "data": {
    "id": 1,
    "url": "https://example.com/page",
    "tags": ["recruitment", "updated"],
    "intervalSeconds": 43200,
    "enabled": false,
    "maxConcurrencyGroup": "new-group",
    "nextRunAt": "2024-01-16T10:00:00Z",
    "createdAt": "2024-01-15T10:00:00Z",
    "updatedAt": "2024-01-15T12:00:00Z"
  }
}
```

**curl例:**
```bash
# 無効化
curl -X PATCH http://localhost:8080/urls/1 \
  -H "Content-Type: application/json" \
  -d '{"enabled": false}'

# 間隔変更
curl -X PATCH http://localhost:8080/urls/1 \
  -H "Content-Type: application/json" \
  -d '{"intervalSeconds": 7200}'
```

---

### 3.5 URL削除

**DELETE /urls/{id}**

URLと関連する収集結果を削除する。

**Response (204 No Content)**

**Error Responses:**
- `404 Not Found`: URL未登録

---

## 4. 収集結果API

### 4.1 最新結果取得

**GET /urls/{id}/latest**

指定URLの最新収集結果を取得する。

**Response (200 OK):**
```json
{
  "data": {
    "id": 1001,
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
    "errorCode": null,
    "errorMessage": null,
    "attempt": 1,
    "runAt": "2024-01-15T10:05:00Z",
    "createdAt": "2024-01-15T10:05:00Z"
  }
}
```

**エラー時の例:**
```json
{
  "data": {
    "id": 1002,
    "urlId": 1,
    "status": "failed",
    "httpStatus": 0,
    "finalUrl": null,
    "contentType": null,
    "latencyMs": 8000,
    "title": null,
    "description": null,
    "ogTitle": null,
    "ogDescription": null,
    "ogImage": null,
    "ogUrl": null,
    "ogSiteName": null,
    "errorCode": "timeout",
    "errorMessage": "request timeout after 8000ms",
    "attempt": 3,
    "runAt": "2024-01-15T11:00:00Z",
    "createdAt": "2024-01-15T11:00:00Z"
  }
}
```

**Error Responses:**
- `404 Not Found`: URL未登録または結果なし

---

### 4.2 収集履歴取得

**GET /urls/{id}/runs**

指定URLの収集履歴を取得する。

**Query Parameters:**
| パラメータ | 型 | デフォルト | 説明 |
|------------|-----|------------|------|
| from | datetime | - | 開始日時（ISO8601） |
| to | datetime | - | 終了日時（ISO8601） |
| status | string | - | succeeded/failedでフィルタ |
| page | int | 1 | ページ番号 |
| pageSize | int | 20 | 1ページあたり件数（max 100） |

**Response (200 OK):**
```json
{
  "data": {
    "items": [
      {
        "id": 1002,
        "urlId": 1,
        "status": "succeeded",
        "httpStatus": 200,
        "latencyMs": 230,
        "attempt": 1,
        "runAt": "2024-01-15T11:00:00Z"
      },
      {
        "id": 1001,
        "urlId": 1,
        "status": "succeeded",
        "httpStatus": 200,
        "latencyMs": 245,
        "attempt": 1,
        "runAt": "2024-01-15T10:00:00Z"
      }
    ],
    "pagination": {
      "page": 1,
      "pageSize": 20,
      "totalItems": 45,
      "totalPages": 3
    }
  }
}
```

**curl例:**
```bash
# 期間指定
curl "http://localhost:8080/urls/1/runs?from=2024-01-01T00:00:00Z&to=2024-01-31T23:59:59Z"

# 失敗のみ
curl "http://localhost:8080/urls/1/runs?status=failed"
```

---

### 4.3 手動実行

**POST /urls/{id}/enqueue**

URLを即時キューに投入し、次回のワーカー実行時に収集する。

**Response (202 Accepted):**
```json
{
  "data": {
    "message": "URL enqueued for immediate processing",
    "urlId": 1,
    "enqueuedAt": "2024-01-15T12:00:00Z"
  }
}
```

**動作:**
- `next_run_at`を現在時刻に更新
- 次回のワーカーポーリングで実行される

**curl例:**
```bash
curl -X POST http://localhost:8080/urls/1/enqueue
```

---

## 5. システムAPI

### 5.1 ヘルスチェック

**GET /health**

APIサーバとDBの疎通確認。

**Response (200 OK):**
```json
{
  "data": {
    "status": "healthy",
    "database": "connected",
    "timestamp": "2024-01-15T12:00:00Z"
  }
}
```

**Response (503 Service Unavailable):**
```json
{
  "data": {
    "status": "unhealthy",
    "database": "disconnected",
    "timestamp": "2024-01-15T12:00:00Z"
  }
}
```

---

## 6. エラーコード一覧

### 6.1 HTTPステータスコード

| コード | 説明 |
|--------|------|
| 200 | 成功 |
| 201 | 作成成功 |
| 202 | 受付成功（非同期処理） |
| 204 | 成功（レスポンスボディなし） |
| 400 | リクエスト不正 |
| 404 | リソース未発見 |
| 409 | 競合（重複等） |
| 422 | 処理不可（バリデーションエラー） |
| 500 | サーバ内部エラー |
| 503 | サービス利用不可 |

### 6.2 エラーコード（error.code）

| コード | 説明 |
|--------|------|
| `VALIDATION_ERROR` | 入力値バリデーションエラー |
| `INVALID_URL` | URL形式不正 |
| `SSRF_BLOCKED` | SSRF対策によりブロック |
| `URL_NOT_FOUND` | URL未登録 |
| `URL_DUPLICATE` | URL重複 |
| `INTERNAL_ERROR` | サーバ内部エラー |

---

## 7. 収集結果のerror_code一覧

| コード | 説明 |
|--------|------|
| `timeout` | タイムアウト |
| `dns` | DNS解決失敗 |
| `connection` | 接続エラー |
| `ssrf_blocked` | SSRF対策によりブロック |
| `too_many_redirects` | リダイレクト回数超過 |
| `response_too_large` | レスポンスサイズ超過 |
| `parse_error` | HTML解析エラー |
| `4xx` | クライアントエラー |
| `5xx` | サーバエラー |
