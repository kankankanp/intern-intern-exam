package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"url-collector/internal/domain"
)

// URLRepository handles URL database operations
type URLRepository struct {
	db *sql.DB
}

// NewURLRepository creates a new URL repository
func NewURLRepository(db *sql.DB) *URLRepository {
	return &URLRepository{db: db}
}

// Create inserts a new URL
func (r *URLRepository) Create(ctx context.Context, url *domain.URL) error {
	tagsJSON, err := json.Marshal(url.Tags)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO urls (url, normalized_url, enabled, interval_seconds, tags, concurrency_group, next_run_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`

	now := time.Now()
	url.NextRunAt = now
	url.CreatedAt = now
	url.UpdatedAt = now

	return r.db.QueryRowContext(ctx, query,
		url.URL,
		url.NormalizedURL,
		url.Enabled,
		url.IntervalSeconds,
		tagsJSON,
		url.ConcurrencyGroup,
		url.NextRunAt,
		url.CreatedAt,
		url.UpdatedAt,
	).Scan(&url.ID, &url.CreatedAt, &url.UpdatedAt)
}

// GetByID retrieves a URL by ID
func (r *URLRepository) GetByID(ctx context.Context, id int64) (*domain.URL, error) {
	query := `
		SELECT id, url, normalized_url, enabled, interval_seconds, tags, concurrency_group, next_run_at, created_at, updated_at
		FROM urls
		WHERE id = $1`

	url := &domain.URL{}
	var tagsJSON []byte
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&url.ID,
		&url.URL,
		&url.NormalizedURL,
		&url.Enabled,
		&url.IntervalSeconds,
		&tagsJSON,
		&url.ConcurrencyGroup,
		&url.NextRunAt,
		&url.CreatedAt,
		&url.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(tagsJSON, &url.Tags); err != nil {
		url.Tags = []string{}
	}

	return url, nil
}

// GetByURL retrieves a URL by its URL string
func (r *URLRepository) GetByURL(ctx context.Context, urlStr string) (*domain.URL, error) {
	query := `
		SELECT id, url, normalized_url, enabled, interval_seconds, tags, concurrency_group, next_run_at, created_at, updated_at
		FROM urls
		WHERE url = $1`

	url := &domain.URL{}
	var tagsJSON []byte
	err := r.db.QueryRowContext(ctx, query, urlStr).Scan(
		&url.ID,
		&url.URL,
		&url.NormalizedURL,
		&url.Enabled,
		&url.IntervalSeconds,
		&tagsJSON,
		&url.ConcurrencyGroup,
		&url.NextRunAt,
		&url.CreatedAt,
		&url.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(tagsJSON, &url.Tags); err != nil {
		url.Tags = []string{}
	}

	return url, nil
}

// URLListParams holds parameters for listing URLs
type URLListParams struct {
	Enabled  *bool
	Tag      string
	Query    string
	Page     int
	PageSize int
}

// List retrieves URLs with pagination and filters
func (r *URLRepository) List(ctx context.Context, params URLListParams) ([]*domain.URL, int, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 || params.PageSize > 100 {
		params.PageSize = 20
	}

	// Build query
	baseQuery := `FROM urls WHERE 1=1`
	args := []interface{}{}
	argIndex := 1

	if params.Enabled != nil {
		baseQuery += ` AND enabled = $` + string(rune('0'+argIndex))
		args = append(args, *params.Enabled)
		argIndex++
	}

	if params.Query != "" {
		baseQuery += ` AND url ILIKE $` + string(rune('0'+argIndex))
		args = append(args, "%"+params.Query+"%")
		argIndex++
	}

	if params.Tag != "" {
		baseQuery += ` AND tags @> $` + string(rune('0'+argIndex))
		tagJSON, _ := json.Marshal([]string{params.Tag})
		args = append(args, tagJSON)
		argIndex++
	}

	// Count total
	var total int
	countQuery := `SELECT COUNT(*) ` + baseQuery
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get items
	offset := (params.Page - 1) * params.PageSize
	selectQuery := `SELECT id, url, normalized_url, enabled, interval_seconds, tags, concurrency_group, next_run_at, created_at, updated_at ` +
		baseQuery + ` ORDER BY created_at DESC LIMIT $` + string(rune('0'+argIndex)) + ` OFFSET $` + string(rune('0'+argIndex+1))
	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var urls []*domain.URL
	for rows.Next() {
		url := &domain.URL{}
		var tagsJSON []byte
		err := rows.Scan(
			&url.ID,
			&url.URL,
			&url.NormalizedURL,
			&url.Enabled,
			&url.IntervalSeconds,
			&tagsJSON,
			&url.ConcurrencyGroup,
			&url.NextRunAt,
			&url.CreatedAt,
			&url.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal(tagsJSON, &url.Tags); err != nil {
			url.Tags = []string{}
		}
		urls = append(urls, url)
	}

	return urls, total, nil
}

// Update updates a URL
func (r *URLRepository) Update(ctx context.Context, url *domain.URL) error {
	tagsJSON, err := json.Marshal(url.Tags)
	if err != nil {
		return err
	}

	query := `
		UPDATE urls
		SET enabled = $1, interval_seconds = $2, tags = $3, concurrency_group = $4, updated_at = $5
		WHERE id = $6`

	url.UpdatedAt = time.Now()
	_, err = r.db.ExecContext(ctx, query,
		url.Enabled,
		url.IntervalSeconds,
		tagsJSON,
		url.ConcurrencyGroup,
		url.UpdatedAt,
		url.ID,
	)
	return err
}

// Delete deletes a URL and its runs
func (r *URLRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM urls WHERE id = $1`, id)
	return err
}

// UpdateNextRunAt updates the next run time
func (r *URLRepository) UpdateNextRunAt(ctx context.Context, id int64, nextRunAt time.Time) error {
	query := `UPDATE urls SET next_run_at = $1, updated_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, nextRunAt, time.Now(), id)
	return err
}

// FetchDueURLs retrieves URLs that are due for processing with locking
func (r *URLRepository) FetchDueURLs(ctx context.Context, limit int) ([]*domain.URL, error) {
	query := `
		SELECT id, url, normalized_url, enabled, interval_seconds, tags, concurrency_group, next_run_at, created_at, updated_at
		FROM urls
		WHERE enabled = true AND next_run_at <= NOW()
		ORDER BY next_run_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var urls []*domain.URL
	for rows.Next() {
		url := &domain.URL{}
		var tagsJSON []byte
		err := rows.Scan(
			&url.ID,
			&url.URL,
			&url.NormalizedURL,
			&url.Enabled,
			&url.IntervalSeconds,
			&tagsJSON,
			&url.ConcurrencyGroup,
			&url.NextRunAt,
			&url.CreatedAt,
			&url.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tagsJSON, &url.Tags); err != nil {
			url.Tags = []string{}
		}
		urls = append(urls, url)
	}

	return urls, nil
}

// Enqueue sets next_run_at to now for immediate processing
func (r *URLRepository) Enqueue(ctx context.Context, id int64) error {
	query := `UPDATE urls SET next_run_at = NOW(), updated_at = NOW() WHERE id = $1 AND enabled = true`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
