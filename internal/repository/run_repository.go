package repository

import (
	"context"
	"database/sql"
	"time"

	"url-collector/internal/domain"
)

// RunRepository handles Run database operations
type RunRepository struct {
	db *sql.DB
}

// NewRunRepository creates a new Run repository
func NewRunRepository(db *sql.DB) *RunRepository {
	return &RunRepository{db: db}
}

// Create inserts a new run
func (r *RunRepository) Create(ctx context.Context, run *domain.Run) error {
	query := `
		INSERT INTO url_runs (url_id, status, http_status, final_url, content_type, latency_ms, title, description, og_title, og_description, og_image, og_url, og_site_name, error_code, error_message, attempt, run_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING id, created_at`

	run.CreatedAt = time.Now()

	return r.db.QueryRowContext(ctx, query,
		run.URLID,
		run.Status,
		run.HTTPStatus,
		run.FinalURL,
		run.ContentType,
		run.LatencyMs,
		run.Title,
		run.Description,
		run.OGTitle,
		run.OGDescription,
		run.OGImage,
		run.OGURL,
		run.OGSiteName,
		run.ErrorCode,
		run.ErrorMessage,
		run.Attempt,
		run.RunAt,
		run.CreatedAt,
	).Scan(&run.ID, &run.CreatedAt)
}

// GetLatest retrieves the latest run for a URL
func (r *RunRepository) GetLatest(ctx context.Context, urlID int64) (*domain.Run, error) {
	query := `
		SELECT id, url_id, status, http_status, final_url, content_type, latency_ms, title, description, og_title, og_description, og_image, og_url, og_site_name, error_code, error_message, attempt, run_at, created_at
		FROM url_runs
		WHERE url_id = $1
		ORDER BY run_at DESC
		LIMIT 1`

	run := &domain.Run{}
	err := r.db.QueryRowContext(ctx, query, urlID).Scan(
		&run.ID,
		&run.URLID,
		&run.Status,
		&run.HTTPStatus,
		&run.FinalURL,
		&run.ContentType,
		&run.LatencyMs,
		&run.Title,
		&run.Description,
		&run.OGTitle,
		&run.OGDescription,
		&run.OGImage,
		&run.OGURL,
		&run.OGSiteName,
		&run.ErrorCode,
		&run.ErrorMessage,
		&run.Attempt,
		&run.RunAt,
		&run.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return run, nil
}

// RunListParams holds parameters for listing runs
type RunListParams struct {
	URLID    int64
	From     *time.Time
	To       *time.Time
	Status   *domain.RunStatus
	Page     int
	PageSize int
}

// List retrieves runs with pagination and filters
func (r *RunRepository) List(ctx context.Context, params RunListParams) ([]*domain.Run, int, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 || params.PageSize > 100 {
		params.PageSize = 20
	}

	// Build query
	baseQuery := `FROM url_runs WHERE url_id = $1`
	args := []interface{}{params.URLID}
	argIndex := 2

	if params.From != nil {
		baseQuery += ` AND run_at >= $` + string(rune('0'+argIndex))
		args = append(args, *params.From)
		argIndex++
	}

	if params.To != nil {
		baseQuery += ` AND run_at <= $` + string(rune('0'+argIndex))
		args = append(args, *params.To)
		argIndex++
	}

	if params.Status != nil {
		baseQuery += ` AND status = $` + string(rune('0'+argIndex))
		args = append(args, *params.Status)
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
	selectQuery := `SELECT id, url_id, status, http_status, final_url, content_type, latency_ms, title, description, og_title, og_description, og_image, og_url, og_site_name, error_code, error_message, attempt, run_at, created_at ` +
		baseQuery + ` ORDER BY run_at DESC LIMIT $` + string(rune('0'+argIndex)) + ` OFFSET $` + string(rune('0'+argIndex+1))
	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var runs []*domain.Run
	for rows.Next() {
		run := &domain.Run{}
		err := rows.Scan(
			&run.ID,
			&run.URLID,
			&run.Status,
			&run.HTTPStatus,
			&run.FinalURL,
			&run.ContentType,
			&run.LatencyMs,
			&run.Title,
			&run.Description,
			&run.OGTitle,
			&run.OGDescription,
			&run.OGImage,
			&run.OGURL,
			&run.OGSiteName,
			&run.ErrorCode,
			&run.ErrorMessage,
			&run.Attempt,
			&run.RunAt,
			&run.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		runs = append(runs, run)
	}

	return runs, total, nil
}
