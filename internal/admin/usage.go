package admin

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type usageSummaryRow struct {
	Bucket           string `json:"bucket"`
	Requests         int64  `json:"requests"`
	Errors           int64  `json:"errors"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

type usageRequestRow struct {
	RequestID          string  `json:"request_id"`
	APIKeyID           *string `json:"api_key_id,omitempty"`
	APIKeyPrefix       *string `json:"api_key_prefix,omitempty"`
	EndpointID         string  `json:"endpoint_id"`
	Host               string  `json:"host"`
	Method             string  `json:"method"`
	Path               string  `json:"path"`
	Model              *string `json:"model,omitempty"`
	Streaming          bool    `json:"streaming"`
	UsageMissing       bool    `json:"usage_missing"`
	PromptTokens       *int64  `json:"prompt_tokens,omitempty"`
	CompletionTokens   *int64  `json:"completion_tokens,omitempty"`
	TotalTokens        *int64  `json:"total_tokens,omitempty"`
	StatusCode         int     `json:"status_code"`
	UpstreamStatusCode *int    `json:"upstream_status_code,omitempty"`
	LatencyMS          int64   `json:"latency_ms"`
	ClientIP           *string `json:"client_ip,omitempty"`
	UserAgent          *string `json:"user_agent,omitempty"`
	Error              *string `json:"error,omitempty"`
	CreatedAt          string  `json:"created_at"`
}

func (router *Router) usageSummary(response http.ResponseWriter, request *http.Request) {
	from, to, ok := parseTimeRange(response, request)
	if !ok {
		return
	}

	groupBy := strings.TrimSpace(request.URL.Query().Get("group_by"))
	if groupBy == "" {
		groupBy = "day"
	}
	if groupBy != "day" {
		writeError(response, http.StatusBadRequest, "group_by must be day", "request_error")
		return
	}

	rows, err := router.database.QueryContext(
		request.Context(),
		`select date(created_at) as bucket,
		        count(*) as requests,
		        coalesce(sum(case when status_code >= 400 then 1 else 0 end), 0) as errors,
		        coalesce(sum(prompt_tokens), 0) as prompt_tokens,
		        coalesce(sum(completion_tokens), 0) as completion_tokens,
		        coalesce(sum(total_tokens), 0) as total_tokens
		 from usage_logs
		 where created_at >= ? and created_at < ?
		 group by bucket
		 order by bucket`,
		from.Format(time.RFC3339Nano),
		to.Format(time.RFC3339Nano),
	)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "failed to query usage summary", "database_error")
		return
	}
	defer rows.Close()

	summary := []usageSummaryRow{}
	for rows.Next() {
		var row usageSummaryRow
		if err := rows.Scan(&row.Bucket, &row.Requests, &row.Errors, &row.PromptTokens, &row.CompletionTokens, &row.TotalTokens); err != nil {
			writeError(response, http.StatusInternalServerError, "failed to read usage summary", "database_error")
			return
		}
		summary = append(summary, row)
	}
	if err := rows.Err(); err != nil {
		writeError(response, http.StatusInternalServerError, "failed to query usage summary", "database_error")
		return
	}

	writeJSON(response, http.StatusOK, map[string]any{
		"from":     from.Format(time.RFC3339Nano),
		"to":       to.Format(time.RFC3339Nano),
		"group_by": groupBy,
		"rows":     summary,
	})
}

func (router *Router) usageRequests(response http.ResponseWriter, request *http.Request) {
	limit, ok := parseLimit(response, request.URL.Query().Get("limit"))
	if !ok {
		return
	}

	query := `select request_id, api_key_id, api_key_prefix, endpoint_id, host, method, path, model,
	                 streaming, usage_missing, prompt_tokens, completion_tokens, total_tokens,
	                 status_code, upstream_status_code, latency_ms, client_ip, user_agent, error, created_at
	          from usage_logs
	          where 1 = 1`
	args := []any{}

	if endpoint := strings.TrimSpace(request.URL.Query().Get("endpoint")); endpoint != "" {
		query += " and endpoint_id = ?"
		args = append(args, endpoint)
	}
	if keyID := strings.TrimSpace(request.URL.Query().Get("api_key_id")); keyID != "" {
		query += " and api_key_id = ?"
		args = append(args, keyID)
	}

	query += " order by created_at desc limit ?"
	args = append(args, limit)

	rows, err := router.database.QueryContext(request.Context(), query, args...)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "failed to query usage requests", "database_error")
		return
	}
	defer rows.Close()

	requests := []usageRequestRow{}
	for rows.Next() {
		row, err := scanUsageRequest(rows)
		if err != nil {
			writeError(response, http.StatusInternalServerError, "failed to read usage request", "database_error")
			return
		}
		requests = append(requests, row)
	}
	if err := rows.Err(); err != nil {
		writeError(response, http.StatusInternalServerError, "failed to query usage requests", "database_error")
		return
	}

	writeJSON(response, http.StatusOK, map[string]any{"requests": requests})
}

func parseTimeRange(response http.ResponseWriter, request *http.Request) (time.Time, time.Time, bool) {
	query := request.URL.Query()
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -7)
	to := now

	if raw := strings.TrimSpace(query.Get("from")); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeError(response, http.StatusBadRequest, "from must be an RFC3339 timestamp", "request_error")
			return time.Time{}, time.Time{}, false
		}
		from = parsed.UTC()
	}
	if raw := strings.TrimSpace(query.Get("to")); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeError(response, http.StatusBadRequest, "to must be an RFC3339 timestamp", "request_error")
			return time.Time{}, time.Time{}, false
		}
		to = parsed.UTC()
	}
	if !from.Before(to) {
		writeError(response, http.StatusBadRequest, "from must be before to", "request_error")
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

func parseLimit(response http.ResponseWriter, raw string) (int, bool) {
	if strings.TrimSpace(raw) == "" {
		return 100, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		writeError(response, http.StatusBadRequest, "limit must be a positive integer", "request_error")
		return 0, false
	}
	if limit > 500 {
		limit = 500
	}
	return limit, true
}

func scanUsageRequest(scanner interface{ Scan(dest ...any) error }) (usageRequestRow, error) {
	var row usageRequestRow
	var apiKeyID sql.NullString
	var apiKeyPrefix sql.NullString
	var model sql.NullString
	var streaming int
	var usageMissing int
	var promptTokens sql.NullInt64
	var completionTokens sql.NullInt64
	var totalTokens sql.NullInt64
	var upstreamStatusCode sql.NullInt64
	var clientIP sql.NullString
	var userAgent sql.NullString
	var errorText sql.NullString

	err := scanner.Scan(
		&row.RequestID,
		&apiKeyID,
		&apiKeyPrefix,
		&row.EndpointID,
		&row.Host,
		&row.Method,
		&row.Path,
		&model,
		&streaming,
		&usageMissing,
		&promptTokens,
		&completionTokens,
		&totalTokens,
		&row.StatusCode,
		&upstreamStatusCode,
		&row.LatencyMS,
		&clientIP,
		&userAgent,
		&errorText,
		&row.CreatedAt,
	)
	if err != nil {
		return usageRequestRow{}, err
	}

	row.APIKeyID = nullStringPtr(apiKeyID)
	row.APIKeyPrefix = nullStringPtr(apiKeyPrefix)
	row.Model = nullStringPtr(model)
	row.Streaming = streaming == 1
	row.UsageMissing = usageMissing == 1
	row.PromptTokens = nullInt64Ptr(promptTokens)
	row.CompletionTokens = nullInt64Ptr(completionTokens)
	row.TotalTokens = nullInt64Ptr(totalTokens)
	if upstreamStatusCode.Valid {
		value := int(upstreamStatusCode.Int64)
		row.UpstreamStatusCode = &value
	}
	row.ClientIP = nullStringPtr(clientIP)
	row.UserAgent = nullStringPtr(userAgent)
	row.Error = nullStringPtr(errorText)
	return row, nil
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
