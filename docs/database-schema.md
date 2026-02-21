# データベーススキーマ設計

## 1. 概要

PostgreSQLを推奨データベースとして使用。SQLiteでも同等のスキーマで動作可能。

## 2. ER図

```
┌───────────────────────┐       ┌───────────────────────────────┐
│         urls          │       │          url_runs             │
├───────────────────────┤       ├───────────────────────────────┤
│ id (PK)               │──┐    │ id (PK)                       │
│ url                   │  │    │ url_id (FK)                   │──┐
│ normalized_url        │  └───▶│ status                        │  │
│ enabled               │       │ http_status                   │  │
│ interval_seconds      │       │ final_url                     │  │
│ tags                  │       │ content_type                  │  │
│ concurrency_group     │       │ latency_ms                    │  │
│ next_run_at           │       │ title                         │  │
│ created_at            │       │ description                   │  │
│ updated_at            │       │ og_title                      │  │
└───────────────────────┘       │ og_description                │  │
                                │ og_image                      │  │
                                │ og_url                        │  │
                                │ og_site_name                  │  │
                                │ error_code                    │  │
                                │ error_message                 │  │
                                │ attempt                       │  │
                                │ run_at                        │  │
                                │ created_at                    │  │
                                └───────────────────────────────┘  │
                                              │                    │
                                              └────────────────────┘
                                                   1:N relationship
```

## 3. テーブル定義

### 3.1 urls テーブル

収集対象のURL管理テーブル。

```sql
CREATE TABLE urls (
    id              BIGSERIAL PRIMARY KEY,
    url             TEXT NOT NULL,
    normalized_url  TEXT NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    interval_seconds INTEGER NOT NULL,
    tags            JSONB NOT NULL DEFAULT '[]',
    concurrency_group TEXT,
    next_run_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT urls_url_unique UNIQUE (url),
    CONSTRAINT urls_normalized_url_unique UNIQUE (normalized_url),
    CONSTRAINT urls_interval_seconds_positive CHECK (interval_seconds >= 60)
);

-- インデックス
CREATE INDEX idx_urls_enabled_next_run_at ON urls (enabled, next_run_at)
    WHERE enabled = true;
CREATE INDEX idx_urls_concurrency_group_next_run_at ON urls (concurrency_group, next_run_at)
    WHERE enabled = true AND concurrency_group IS NOT NULL;
CREATE INDEX idx_urls_tags ON urls USING GIN (tags);

-- updated_at自動更新トリガー
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_urls_updated_at
    BEFORE UPDATE ON urls
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
```

**カラム説明:**

| カラム | 型 | 説明 |
|--------|-----|------|
| id | BIGSERIAL | 主キー |
| url | TEXT | 元のURL（登録時のまま） |
| normalized_url | TEXT | 正規化URL（重複チェック用） |
| enabled | BOOLEAN | 有効フラグ |
| interval_seconds | INTEGER | 収集間隔（秒） |
| tags | JSONB | タグ配列 |
| concurrency_group | TEXT | 同時実行制御グループ |
| next_run_at | TIMESTAMPTZ | 次回実行予定時刻 |
| created_at | TIMESTAMPTZ | 作成日時 |
| updated_at | TIMESTAMPTZ | 更新日時 |

### 3.2 url_runs テーブル

収集結果履歴テーブル。

```sql
CREATE TYPE run_status AS ENUM ('succeeded', 'failed');

CREATE TABLE url_runs (
    id              BIGSERIAL PRIMARY KEY,
    url_id          BIGINT NOT NULL REFERENCES urls(id) ON DELETE CASCADE,
    status          run_status NOT NULL,
    http_status     INTEGER,
    final_url       TEXT,
    content_type    TEXT,
    latency_ms      INTEGER NOT NULL,
    title           TEXT,
    description     TEXT,
    og_title        TEXT,
    og_description  TEXT,
    og_image        TEXT,
    og_url          TEXT,
    og_site_name    TEXT,
    error_code      TEXT,
    error_message   TEXT,
    attempt         INTEGER NOT NULL DEFAULT 1,
    run_at          TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- インデックス
CREATE INDEX idx_url_runs_url_id_run_at ON url_runs (url_id, run_at DESC);
CREATE INDEX idx_url_runs_status_run_at ON url_runs (status, run_at DESC);
CREATE INDEX idx_url_runs_run_at ON url_runs (run_at DESC);
```

**カラム説明:**

| カラム | 型 | 説明 |
|--------|-----|------|
| id | BIGSERIAL | 主キー |
| url_id | BIGINT | 外部キー（urls.id） |
| status | ENUM | 結果ステータス（succeeded/failed） |
| http_status | INTEGER | HTTPステータスコード |
| final_url | TEXT | 最終到達URL（リダイレクト後） |
| content_type | TEXT | Content-Typeヘッダ |
| latency_ms | INTEGER | 応答時間（ミリ秒） |
| title | TEXT | `<title>`タグの内容 |
| description | TEXT | meta descriptionの内容 |
| og_title | TEXT | og:title |
| og_description | TEXT | og:description |
| og_image | TEXT | og:image |
| og_url | TEXT | og:url |
| og_site_name | TEXT | og:site_name |
| error_code | TEXT | エラーコード |
| error_message | TEXT | エラーメッセージ |
| attempt | INTEGER | 試行回数 |
| run_at | TIMESTAMPTZ | 収集実行時刻 |
| created_at | TIMESTAMPTZ | レコード作成日時 |

## 4. SQLiteスキーマ

SQLite使用時の代替スキーマ:

```sql
-- urls テーブル
CREATE TABLE urls (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    url             TEXT NOT NULL UNIQUE,
    normalized_url  TEXT NOT NULL UNIQUE,
    enabled         INTEGER NOT NULL DEFAULT 1,
    interval_seconds INTEGER NOT NULL CHECK (interval_seconds >= 60),
    tags            TEXT NOT NULL DEFAULT '[]',  -- JSON配列
    concurrency_group TEXT,
    next_run_at     TEXT NOT NULL DEFAULT (datetime('now')),
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_urls_enabled_next_run_at ON urls (enabled, next_run_at);
CREATE INDEX idx_urls_concurrency_group ON urls (concurrency_group, next_run_at);

-- url_runs テーブル
CREATE TABLE url_runs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    url_id          INTEGER NOT NULL REFERENCES urls(id) ON DELETE CASCADE,
    status          TEXT NOT NULL CHECK (status IN ('succeeded', 'failed')),
    http_status     INTEGER,
    final_url       TEXT,
    content_type    TEXT,
    latency_ms      INTEGER NOT NULL,
    title           TEXT,
    description     TEXT,
    og_title        TEXT,
    og_description  TEXT,
    og_image        TEXT,
    og_url          TEXT,
    og_site_name    TEXT,
    error_code      TEXT,
    error_message   TEXT,
    attempt         INTEGER NOT NULL DEFAULT 1,
    run_at          TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_url_runs_url_id_run_at ON url_runs (url_id, run_at DESC);
CREATE INDEX idx_url_runs_status_run_at ON url_runs (status, run_at DESC);
```

## 5. クエリパターン

### 5.1 実行対象URL取得（排他制御付き）

```sql
-- PostgreSQL
SELECT id, url, interval_seconds, concurrency_group
FROM urls
WHERE enabled = true
  AND next_run_at <= NOW()
ORDER BY next_run_at ASC
LIMIT 100
FOR UPDATE SKIP LOCKED;
```

```sql
-- SQLite（FOR UPDATE非対応のため代替方式）
-- アプリケーション側でトランザクション + 楽観的ロック
BEGIN IMMEDIATE;

SELECT id, url, interval_seconds, concurrency_group
FROM urls
WHERE enabled = 1
  AND datetime(next_run_at) <= datetime('now')
ORDER BY next_run_at ASC
LIMIT 100;

-- 処理後
UPDATE urls SET next_run_at = datetime('now', '+' || interval_seconds || ' seconds')
WHERE id = ?;

COMMIT;
```

### 5.2 最新結果取得

```sql
SELECT *
FROM url_runs
WHERE url_id = :url_id
ORDER BY run_at DESC
LIMIT 1;
```

### 5.3 履歴取得（ページネーション）

```sql
SELECT *
FROM url_runs
WHERE url_id = :url_id
  AND (:from IS NULL OR run_at >= :from)
  AND (:to IS NULL OR run_at <= :to)
  AND (:status IS NULL OR status = :status)
ORDER BY run_at DESC
LIMIT :page_size
OFFSET :offset;
```

### 5.4 URL検索

```sql
-- キーワード検索
SELECT *
FROM urls
WHERE (:q IS NULL OR url ILIKE '%' || :q || '%')
  AND (:enabled IS NULL OR enabled = :enabled)
  AND (:tag IS NULL OR tags ? :tag)
ORDER BY created_at DESC
LIMIT :page_size
OFFSET :offset;
```

## 6. URL正規化

### 6.1 正規化ルール

重複登録を防ぐため、URLを正規化してから保存:

```go
func NormalizeURL(rawURL string) (string, error) {
    u, err := url.Parse(rawURL)
    if err != nil {
        return "", err
    }

    // スキーム小文字化
    u.Scheme = strings.ToLower(u.Scheme)

    // ホスト小文字化
    u.Host = strings.ToLower(u.Host)

    // デフォルトポート除去
    if (u.Scheme == "http" && u.Port() == "80") ||
       (u.Scheme == "https" && u.Port() == "443") {
        u.Host = u.Hostname()
    }

    // 末尾スラッシュ統一（ルートパス以外は除去）
    if u.Path != "/" && strings.HasSuffix(u.Path, "/") {
        u.Path = strings.TrimSuffix(u.Path, "/")
    }
    if u.Path == "" {
        u.Path = "/"
    }

    // トラッキングパラメータ除去（オプション）
    q := u.Query()
    trackingParams := []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"}
    for _, param := range trackingParams {
        q.Del(param)
    }
    u.RawQuery = q.Encode()

    // フラグメント除去
    u.Fragment = ""

    return u.String(), nil
}
```

### 6.2 正規化例

| 元URL | 正規化後 |
|-------|----------|
| `HTTP://Example.COM/page` | `http://example.com/page` |
| `https://example.com:443/page/` | `https://example.com/page` |
| `https://example.com/page?utm_source=twitter` | `https://example.com/page` |
| `https://example.com` | `https://example.com/` |

## 7. マイグレーション

### 7.1 初期マイグレーション

`migrations/001_initial.sql`:

```sql
-- Up
CREATE TYPE run_status AS ENUM ('succeeded', 'failed');

CREATE TABLE urls (
    id              BIGSERIAL PRIMARY KEY,
    url             TEXT NOT NULL,
    normalized_url  TEXT NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    interval_seconds INTEGER NOT NULL,
    tags            JSONB NOT NULL DEFAULT '[]',
    concurrency_group TEXT,
    next_run_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT urls_url_unique UNIQUE (url),
    CONSTRAINT urls_normalized_url_unique UNIQUE (normalized_url),
    CONSTRAINT urls_interval_seconds_positive CHECK (interval_seconds >= 60)
);

CREATE TABLE url_runs (
    id              BIGSERIAL PRIMARY KEY,
    url_id          BIGINT NOT NULL REFERENCES urls(id) ON DELETE CASCADE,
    status          run_status NOT NULL,
    http_status     INTEGER,
    final_url       TEXT,
    content_type    TEXT,
    latency_ms      INTEGER NOT NULL,
    title           TEXT,
    description     TEXT,
    og_title        TEXT,
    og_description  TEXT,
    og_image        TEXT,
    og_url          TEXT,
    og_site_name    TEXT,
    error_code      TEXT,
    error_message   TEXT,
    attempt         INTEGER NOT NULL DEFAULT 1,
    run_at          TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_urls_enabled_next_run_at ON urls (enabled, next_run_at) WHERE enabled = true;
CREATE INDEX idx_urls_concurrency_group_next_run_at ON urls (concurrency_group, next_run_at) WHERE enabled = true AND concurrency_group IS NOT NULL;
CREATE INDEX idx_urls_tags ON urls USING GIN (tags);
CREATE INDEX idx_url_runs_url_id_run_at ON url_runs (url_id, run_at DESC);
CREATE INDEX idx_url_runs_status_run_at ON url_runs (status, run_at DESC);
CREATE INDEX idx_url_runs_run_at ON url_runs (run_at DESC);

-- Down
DROP TABLE url_runs;
DROP TABLE urls;
DROP TYPE run_status;
```

## 8. 保守運用

### 8.1 古いデータの削除

長期運用では`url_runs`が肥大化するため、定期的なパージを検討:

```sql
-- 90日より古いデータを削除
DELETE FROM url_runs
WHERE run_at < NOW() - INTERVAL '90 days';
```

### 8.2 パーティショニング（オプション）

大量データ時はパーティショニングを検討:

```sql
-- 月次パーティション
CREATE TABLE url_runs (
    ...
) PARTITION BY RANGE (run_at);

CREATE TABLE url_runs_2024_01 PARTITION OF url_runs
    FOR VALUES FROM ('2024-01-01') TO ('2024-02-01');
```
