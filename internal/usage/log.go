package usage

import (
	"context"
	"database/sql"
	"time"
)

type Entry struct {
	RequestID          string
	APIKeyID           *string
	APIKeyPrefix       string
	EndpointID         string
	Host               string
	Method             string
	Path               string
	Model              *string
	Streaming          bool
	UsageMissing       bool
	PromptTokens       *int
	CompletionTokens   *int
	TotalTokens        *int
	StatusCode         int
	UpstreamStatusCode *int
	LatencyMS          int64
	ClientIP           string
	UserAgent          string
	Error              string
	CreatedAt          time.Time
}

func Insert(ctx context.Context, database *sql.DB, entry Entry) error {
	_, err := database.ExecContext(
		ctx,
		`insert into usage_logs (
		  request_id, api_key_id, api_key_prefix, endpoint_id, host, method, path, model,
		  streaming, usage_missing, prompt_tokens, completion_tokens, total_tokens,
		  status_code, upstream_status_code, latency_ms, client_ip, user_agent, error, created_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.RequestID,
		entry.APIKeyID,
		entry.APIKeyPrefix,
		entry.EndpointID,
		entry.Host,
		entry.Method,
		entry.Path,
		entry.Model,
		boolInt(entry.Streaming),
		boolInt(entry.UsageMissing),
		entry.PromptTokens,
		entry.CompletionTokens,
		entry.TotalTokens,
		entry.StatusCode,
		entry.UpstreamStatusCode,
		entry.LatencyMS,
		entry.ClientIP,
		entry.UserAgent,
		nullEmpty(entry.Error),
		entry.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
