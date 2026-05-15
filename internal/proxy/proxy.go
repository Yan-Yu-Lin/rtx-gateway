package proxy

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/auth"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/config"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/usage"
)

const requestIDAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type Handler struct {
	database  *sql.DB
	cfg       config.Config
	logger    *slog.Logger
	endpoints map[string]config.Endpoint
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func NewHandler(database *sql.DB, cfg config.Config, logger *slog.Logger, endpoints map[string]config.Endpoint) *Handler {
	return &Handler{
		database:  database,
		cfg:       cfg,
		logger:    logger,
		endpoints: endpoints,
	}
}

func (h *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	started := time.Now()
	requestID := requestID()
	host := normalizedHost(request.Host)
	endpoint, ok := h.endpoints[host]
	if !ok {
		writeOpenAIError(response, http.StatusBadGateway, "unknown gateway host", "upstream_error")
		h.logger.Warn("rejected request for unknown host", "request_id", requestID, "host", host, "path", request.URL.RequestURI())
		return
	}

	if h.cfg.MaxBodyBytes > 0 && request.ContentLength > h.cfg.MaxBodyBytes {
		writeOpenAIError(response, http.StatusRequestEntityTooLarge, "request body too large", "request_error")
		h.logUsage(request.Context(), usage.Entry{
			RequestID:  requestID,
			EndpointID: endpoint.ID,
			Host:       host,
			Method:     request.Method,
			Path:       request.URL.RequestURI(),
			StatusCode: http.StatusRequestEntityTooLarge,
			LatencyMS:  time.Since(started).Milliseconds(),
			ClientIP:   clientIP(request),
			UserAgent:  request.UserAgent(),
			Error:      "request body too large",
			CreatedAt:  started,
		})
		return
	}

	principal, err := auth.AuthenticateRequest(request.Context(), h.database, h.cfg.KeyPepper, request)
	if err != nil {
		status, message := authErrorStatus(err)
		writeOpenAIError(response, status, message, "auth_error")
		h.logUsage(request.Context(), usage.Entry{
			RequestID:    requestID,
			APIKeyPrefix: keyPrefixFromHeader(request.Header.Get("Authorization")),
			EndpointID:   endpoint.ID,
			Host:         host,
			Method:       request.Method,
			Path:         request.URL.RequestURI(),
			StatusCode:   status,
			LatencyMS:    time.Since(started).Milliseconds(),
			ClientIP:     clientIP(request),
			UserAgent:    request.UserAgent(),
			Error:        message,
			CreatedAt:    started,
		})
		return
	}

	if !principal.HasScope(endpoint.ID) {
		writeOpenAIError(response, http.StatusForbidden, "API key is not allowed to access this endpoint", "permission_error")
		h.logUsage(request.Context(), usage.Entry{
			RequestID:    requestID,
			APIKeyID:     &principal.ID,
			APIKeyPrefix: principal.Prefix,
			EndpointID:   endpoint.ID,
			Host:         host,
			Method:       request.Method,
			Path:         request.URL.RequestURI(),
			StatusCode:   http.StatusForbidden,
			LatencyMS:    time.Since(started).Milliseconds(),
			ClientIP:     clientIP(request),
			UserAgent:    request.UserAgent(),
			Error:        "forbidden endpoint scope",
			CreatedAt:    started,
		})
		return
	}

	if h.cfg.MaxBodyBytes > 0 {
		request.Body = http.MaxBytesReader(response, request.Body, h.cfg.MaxBodyBytes)
	}

	recorder := &statusRecorder{ResponseWriter: response, status: http.StatusOK}
	var upstreamStatus *int
	var proxyError string
	var usageCapture usage.Capture
	reverseProxy := httputil.NewSingleHostReverseProxy(endpoint.UpstreamURL)
	reverseProxy.Director = director(endpoint.UpstreamURL, requestID)
	reverseProxy.ModifyResponse = func(upstreamResponse *http.Response) error {
		status := upstreamResponse.StatusCode
		upstreamStatus = &status
		return captureUsageFromResponse(upstreamResponse, &usageCapture)
	}
	reverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		status, message := proxyErrorResponse(err)
		proxyError = err.Error()
		writeOpenAIError(w, status, message, "upstream_error")
	}

	reverseProxy.ServeHTTP(recorder, request.WithContext(auth.ContextWithPrincipal(request.Context(), principal)))

	entry := usage.Entry{
		RequestID:          requestID,
		APIKeyID:           &principal.ID,
		APIKeyPrefix:       principal.Prefix,
		EndpointID:         endpoint.ID,
		Host:               host,
		Method:             request.Method,
		Path:               request.URL.RequestURI(),
		Model:              usageCapture.Model,
		Streaming:          usageCapture.Streaming,
		UsageMissing:       usageCapture.UsageMissing,
		PromptTokens:       usageCapture.PromptTokens,
		CompletionTokens:   usageCapture.CompletionTokens,
		TotalTokens:        usageCapture.TotalTokens,
		StatusCode:         recorder.status,
		UpstreamStatusCode: upstreamStatus,
		LatencyMS:          time.Since(started).Milliseconds(),
		ClientIP:           clientIP(request),
		UserAgent:          request.UserAgent(),
		Error:              proxyError,
		CreatedAt:          started,
	}
	h.logUsage(request.Context(), entry)
	h.logger.Info("proxied request",
		"request_id", requestID,
		"endpoint", endpoint.ID,
		"host", host,
		"status", recorder.status,
		"upstream_status", upstreamStatus,
		"model", usageCapture.Model,
		"streaming", usageCapture.Streaming,
		"usage_missing", usageCapture.UsageMissing,
		"latency_ms", entry.LatencyMS,
		"error", proxyError,
	)
}

func (h *Handler) logUsage(ctx context.Context, entry usage.Entry) {
	if err := usage.Insert(ctx, h.database, entry); err != nil {
		h.logger.Error("failed to insert usage log", "request_id", entry.RequestID, "error", err)
	}
}

func director(target *url.URL, requestID string) func(*http.Request) {
	return func(request *http.Request) {
		originalHost := request.Host
		request.URL.Scheme = target.Scheme
		request.URL.Host = target.Host
		request.URL.Path = joinURLPath(target.Path, request.URL.Path)
		request.Host = target.Host
		request.Header.Set("X-Request-ID", requestID)
		request.Header.Set("X-Forwarded-Host", originalHost)
	}
}

func joinURLPath(base string, path string) string {
	if base == "" || base == "/" {
		return path
	}
	if path == "" || path == "/" {
		return base
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Flush() {
	if flusher, ok := recorder.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (recorder *statusRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}

func normalizedHost(host string) string {
	value := strings.ToLower(strings.TrimSpace(host))
	if strings.Contains(value, ":") {
		if withoutPort, _, err := net.SplitHostPort(value); err == nil {
			return withoutPort
		}
	}
	return value
}

func clientIP(request *http.Request) string {
	if forwardedFor := strings.TrimSpace(request.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(parts[0])
	}
	if realIP := strings.TrimSpace(request.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return request.RemoteAddr
	}
	return host
}

func authErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, auth.ErrMissingBearer):
		return http.StatusUnauthorized, "missing bearer token"
	case errors.Is(err, auth.ErrDisabledKey):
		return http.StatusForbidden, "API key is disabled"
	default:
		return http.StatusUnauthorized, "invalid API key"
	}
}

func proxyErrorResponse(err error) (int, string) {
	if strings.Contains(err.Error(), "request body too large") {
		return http.StatusRequestEntityTooLarge, "request body too large"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, "upstream timeout"
	}
	var netError net.Error
	if errors.As(err, &netError) && netError.Timeout() {
		return http.StatusGatewayTimeout, "upstream timeout"
	}
	return http.StatusBadGateway, "upstream unavailable"
}

func writeOpenAIError(response http.ResponseWriter, status int, message string, errorType string) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    errorType,
		},
	})
}

func keyPrefixFromHeader(header string) string {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 {
		return ""
	}
	keyParts := strings.Split(strings.TrimPrefix(parts[1], "rtx_live_"), "_")
	if len(keyParts) != 2 || len(keyParts[0]) != 8 {
		return ""
	}
	return keyParts[0]
}

func requestID() string {
	value, err := randomString(16)
	if err != nil {
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return "req_" + value
}

func randomString(length int) (string, error) {
	bytes := make([]byte, length)
	random := make([]byte, length)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for index, value := range random {
		bytes[index] = requestIDAlphabet[int(value)%len(requestIDAlphabet)]
	}
	return string(bytes), nil
}
