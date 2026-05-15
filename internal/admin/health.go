package admin

import (
	"database/sql"
	"net/http"
)

type endpointHealthResponse struct {
	ID                   string  `json:"id"`
	Host                 string  `json:"host"`
	UpstreamURL          string  `json:"upstream_url"`
	Enabled              bool    `json:"enabled"`
	LastHealthStatus     *string `json:"last_health_status,omitempty"`
	LastHealthStatusCode *int    `json:"last_health_status_code,omitempty"`
	LastHealthLatencyMS  *int64  `json:"last_health_latency_ms,omitempty"`
	LastHealthError      *string `json:"last_health_error,omitempty"`
	LastHealthCheckedAt  *string `json:"last_health_checked_at,omitempty"`
}

func (router *Router) health(response http.ResponseWriter, request *http.Request) {
	rows, err := router.database.QueryContext(
		request.Context(),
		`select id, host, upstream_url, enabled, last_health_status, last_health_status_code,
		        last_health_latency_ms, last_health_error, last_health_checked_at
		 from endpoints
		 order by id`,
	)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "failed to query endpoint health", "database_error")
		return
	}
	defer rows.Close()

	endpoints := []endpointHealthResponse{}
	for rows.Next() {
		endpoint, err := scanEndpointHealth(rows)
		if err != nil {
			writeError(response, http.StatusInternalServerError, "failed to read endpoint health", "database_error")
			return
		}
		endpoints = append(endpoints, endpoint)
	}
	if err := rows.Err(); err != nil {
		writeError(response, http.StatusInternalServerError, "failed to query endpoint health", "database_error")
		return
	}

	writeJSON(response, http.StatusOK, map[string]any{"endpoints": endpoints})
}

func (router *Router) checkHealth(response http.ResponseWriter, request *http.Request) {
	if router.checker == nil {
		writeError(response, http.StatusServiceUnavailable, "health checker is not configured", "configuration_error")
		return
	}

	results, err := router.checker.CheckAll(request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, "health check failed", "upstream_error")
		return
	}

	writeJSON(response, http.StatusOK, map[string]any{"results": results})
}

func scanEndpointHealth(scanner interface{ Scan(dest ...any) error }) (endpointHealthResponse, error) {
	var endpoint endpointHealthResponse
	var enabled int
	var status sql.NullString
	var statusCode sql.NullInt64
	var latencyMS sql.NullInt64
	var errorText sql.NullString
	var checkedAt sql.NullString

	err := scanner.Scan(
		&endpoint.ID,
		&endpoint.Host,
		&endpoint.UpstreamURL,
		&enabled,
		&status,
		&statusCode,
		&latencyMS,
		&errorText,
		&checkedAt,
	)
	if err != nil {
		return endpointHealthResponse{}, err
	}

	endpoint.Enabled = enabled == 1
	endpoint.LastHealthStatus = nullStringPtr(status)
	if statusCode.Valid {
		value := int(statusCode.Int64)
		endpoint.LastHealthStatusCode = &value
	}
	endpoint.LastHealthLatencyMS = nullInt64Ptr(latencyMS)
	endpoint.LastHealthError = nullStringPtr(errorText)
	endpoint.LastHealthCheckedAt = nullStringPtr(checkedAt)
	return endpoint, nil
}
