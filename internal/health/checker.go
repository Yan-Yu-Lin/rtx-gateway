package health

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/config"
)

type Checker struct {
	database  *sql.DB
	endpoints []config.Endpoint
	client    *http.Client
	logger    *slog.Logger
}

type Result struct {
	EndpointID string `json:"endpoint_id"`
	Status     string `json:"status"`
	StatusCode *int   `json:"status_code,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
	CheckedAt  string `json:"checked_at"`
}

func NewChecker(database *sql.DB, endpoints []config.Endpoint, logger *slog.Logger) *Checker {
	return &Checker{
		database:  database,
		endpoints: endpoints,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
	}
}

func (checker *Checker) Start(ctx context.Context, interval time.Duration) {
	go func() {
		timer := time.NewTimer(0)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if _, err := checker.CheckAll(ctx); err != nil {
					checker.logger.Warn("periodic health check failed", "error", err)
				}
				timer.Reset(interval)
			}
		}
	}()
}

func (checker *Checker) CheckAll(ctx context.Context) ([]Result, error) {
	results := make([]Result, 0, len(checker.endpoints))
	for _, endpoint := range checker.endpoints {
		result := checker.check(ctx, endpoint)
		if err := checker.store(ctx, result); err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (checker *Checker) check(ctx context.Context, endpoint config.Endpoint) Result {
	started := time.Now()
	checkedAt := started.UTC().Format(time.RFC3339Nano)
	healthURL := endpoint.UpstreamURL.ResolveReference(&url.URL{Path: "/health"})

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL.String(), nil)
	if err != nil {
		return Result{
			EndpointID: endpoint.ID,
			Status:     "unhealthy",
			LatencyMS:  time.Since(started).Milliseconds(),
			Error:      err.Error(),
			CheckedAt:  checkedAt,
		}
	}

	response, err := checker.client.Do(request)
	if err != nil {
		return Result{
			EndpointID: endpoint.ID,
			Status:     "unhealthy",
			LatencyMS:  time.Since(started).Milliseconds(),
			Error:      err.Error(),
			CheckedAt:  checkedAt,
		}
	}
	defer response.Body.Close()

	statusCode := response.StatusCode
	status := "unhealthy"
	if statusCode >= 200 && statusCode < 300 {
		status = "healthy"
	}

	return Result{
		EndpointID: endpoint.ID,
		Status:     status,
		StatusCode: &statusCode,
		LatencyMS:  time.Since(started).Milliseconds(),
		CheckedAt:  checkedAt,
	}
}

func (checker *Checker) store(ctx context.Context, result Result) error {
	_, err := checker.database.ExecContext(
		ctx,
		`update endpoints
		 set last_health_status = ?,
		     last_health_status_code = ?,
		     last_health_latency_ms = ?,
		     last_health_error = ?,
		     last_health_checked_at = ?,
		     updated_at = ?
		 where id = ?`,
		result.Status,
		result.StatusCode,
		result.LatencyMS,
		nullEmpty(result.Error),
		result.CheckedAt,
		result.CheckedAt,
		result.EndpointID,
	)
	if err != nil {
		return fmt.Errorf("update endpoint health %s: %w", result.EndpointID, err)
	}

	_, err = checker.database.ExecContext(
		ctx,
		`insert into endpoint_health_checks (endpoint_id, status, status_code, latency_ms, error, checked_at)
		 values (?, ?, ?, ?, ?, ?)`,
		result.EndpointID,
		result.Status,
		result.StatusCode,
		result.LatencyMS,
		nullEmpty(result.Error),
		result.CheckedAt,
	)
	if err != nil {
		return fmt.Errorf("insert endpoint health %s: %w", result.EndpointID, err)
	}

	return nil
}

func nullEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
