package domain

import "time"

type RunStatus string

const (
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
)

type Run struct {
	ID            int64     `json:"id"`
	URLID         int64     `json:"urlId"`
	Status        RunStatus `json:"status"`
	HTTPStatus    *int      `json:"httpStatus"`
	FinalURL      *string   `json:"finalUrl"`
	ContentType   *string   `json:"contentType"`
	LatencyMs     int       `json:"latencyMs"`
	Title         *string   `json:"title"`
	Description   *string   `json:"description"`
	OGTitle       *string   `json:"ogTitle"`
	OGDescription *string   `json:"ogDescription"`
	OGImage       *string   `json:"ogImage"`
	OGURL         *string   `json:"ogUrl"`
	OGSiteName    *string   `json:"ogSiteName"`
	ErrorCode     *string   `json:"errorCode"`
	ErrorMessage  *string   `json:"errorMessage"`
	Attempt       int       `json:"attempt"`
	RunAt         time.Time `json:"runAt"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Metadata holds parsed HTML metadata
type Metadata struct {
	Title         string
	Description   string
	OGTitle       string
	OGDescription string
	OGImage       string
	OGURL         string
	OGSiteName    string
}
