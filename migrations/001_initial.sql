-- URL metadata collector initial schema

-- Create run status enum
DO $$ BEGIN
    CREATE TYPE run_status AS ENUM ('succeeded', 'failed');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- URLs table
CREATE TABLE IF NOT EXISTS urls (
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

-- URL runs table
CREATE TABLE IF NOT EXISTS url_runs (
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

-- Indexes
CREATE INDEX IF NOT EXISTS idx_urls_enabled_next_run_at ON urls (enabled, next_run_at) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_urls_concurrency_group_next_run_at ON urls (concurrency_group, next_run_at) WHERE enabled = true AND concurrency_group IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_urls_tags ON urls USING GIN (tags);
CREATE INDEX IF NOT EXISTS idx_url_runs_url_id_run_at ON url_runs (url_id, run_at DESC);
CREATE INDEX IF NOT EXISTS idx_url_runs_status_run_at ON url_runs (status, run_at DESC);
CREATE INDEX IF NOT EXISTS idx_url_runs_run_at ON url_runs (run_at DESC);

-- Trigger function for updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger for urls table
DROP TRIGGER IF EXISTS update_urls_updated_at ON urls;
CREATE TRIGGER update_urls_updated_at
    BEFORE UPDATE ON urls
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
