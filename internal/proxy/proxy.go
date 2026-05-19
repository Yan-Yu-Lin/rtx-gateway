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
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/security"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/usage"
)

const requestIDAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type Handler struct {
	database  *sql.DB
	cfg       config.Config
	logger    *slog.Logger
	endpoints map[string]config.Endpoint
	security  *security.Manager
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func NewHandler(database *sql.DB, cfg config.Config, logger *slog.Logger, endpoints map[string]config.Endpoint, securityManager *security.Manager) *Handler {
	return &Handler{
		database:  database,
		cfg:       cfg,
		logger:    logger,
		endpoints: endpoints,
		security:  securityManager,
	}
}

func (h *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	started := time.Now()
	requestID := requestID()
	host := normalizedHost(request.Host)
	clientIP := clientIP(request)
	endpoint, ok := h.endpoints[host]
	if !ok {
		writeOpenAIError(response, http.StatusBadGateway, "unknown gateway host", "upstream_error")
		h.logger.Warn("rejected request for unknown host", "request_id", requestID, "host", host, "path", request.URL.RequestURI())
		return
	}

	if ban, banned := h.activeBan(clientIP, started); banned {
		writeOpenAIError(response, http.StatusForbidden, "client IP is temporarily banned", "security_error")
		h.logger.Warn("blocked banned client IP", "request_id", requestID, "client_ip", clientIP, "ban_id", ban.ID, "banned_until", ban.BannedUntil)
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
			ClientIP:   clientIP,
			UserAgent:  request.UserAgent(),
			Error:      "request body too large",
			CreatedAt:  started,
		})
		return
	}

	principal, err := auth.AuthenticateRequest(request.Context(), h.database, h.cfg.KeyPepper, request)
	if err != nil {
		if ok, retryAfter := h.allowUnauthed(clientIP, started); !ok {
			status := http.StatusTooManyRequests
			h.recordSecurityEvent(request.Context(), security.EventInput{
				ClientIP:   clientIP,
				EventType:  security.EventRateLimited,
				Host:       host,
				Path:       request.URL.RequestURI(),
				StatusCode: &status,
				Detail:     fmt.Sprintf("unauthenticated rate limit exceeded; retry after %s", retryAfter.Round(time.Second)),
			}, started)
			response.Header().Set("Retry-After", retryAfterHeader(retryAfter))
			writeOpenAIError(response, status, "rate limit exceeded", "rate_limit_error")
			return
		}

		status, message := authErrorStatus(err)
		h.recordAuthFailure(request.Context(), security.EventInput{
			ClientIP:   clientIP,
			Host:       host,
			Path:       request.URL.RequestURI(),
			StatusCode: &status,
			Detail:     message,
		}, started)
		writeOpenAIError(response, status, message, "auth_error")
		return
	}

	if ok, retryAfter := h.allowAuthed(clientIP, principal.ID, started); !ok {
		status := http.StatusTooManyRequests
		h.recordSecurityEvent(request.Context(), security.EventInput{
			ClientIP:   clientIP,
			EventType:  security.EventRateLimited,
			Host:       host,
			Path:       request.URL.RequestURI(),
			StatusCode: &status,
			Detail:     fmt.Sprintf("authenticated rate limit exceeded for key %s; retry after %s", principal.Prefix, retryAfter.Round(time.Second)),
		}, started)
		response.Header().Set("Retry-After", retryAfterHeader(retryAfter))
		writeOpenAIError(response, status, "rate limit exceeded", "rate_limit_error")
		return
	}

	if !principal.HasScope(endpoint.ID) {
		writeOpenAIError(response, http.StatusForbidden, "API key is not allowed to access this endpoint", "permission_error")
		status := http.StatusForbidden
		h.recordSecurityEvent(request.Context(), security.EventInput{
			ClientIP:   clientIP,
			EventType:  security.EventScopeDenied,
			Host:       host,
			Path:       request.URL.RequestURI(),
			StatusCode: &status,
			Detail:     "forbidden endpoint scope",
		}, started)
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
			ClientIP:     clientIP,
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
		ClientIP:           clientIP,
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

func (h *Handler) activeBan(clientIP string, now time.Time) (security.Ban, bool) {
	if h.security == nil {
		return security.Ban{}, false
	}
	return h.security.ActiveBan(clientIP, now)
}

func (h *Handler) allowUnauthed(clientIP string, now time.Time) (bool, time.Duration) {
	if h.security == nil {
		return true, 0
	}
	return h.security.AllowUnauthed(clientIP, now)
}

func (h *Handler) allowAuthed(clientIP string, keyID string, now time.Time) (bool, time.Duration) {
	if h.security == nil {
		return true, 0
	}
	return h.security.AllowAuthed(clientIP, keyID, now)
}

func (h *Handler) recordAuthFailure(ctx context.Context, input security.EventInput, now time.Time) {
	if h.security == nil {
		return
	}
	if ban, err := h.security.RecordAuthFailure(ctx, input, now); err != nil {
		h.logger.Warn("failed to record auth failure", "client_ip", input.ClientIP, "error", err)
	} else if ban != nil {
		h.logger.Warn("auto-banned client IP", "client_ip", input.ClientIP, "ban_id", ban.ID, "banned_until", ban.BannedUntil)
	}
}

func (h *Handler) recordSecurityEvent(ctx context.Context, input security.EventInput, now time.Time) {
	if h.security == nil {
		return
	}
	if err := h.security.RecordEvent(ctx, input, now); err != nil {
		h.logger.Warn("failed to record security event", "client_ip", input.ClientIP, "event_type", input.EventType, "error", err)
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
		request.Header.Del("Authorization")
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
	if realIP := strings.TrimSpace(request.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	if forwardedFor := strings.TrimSpace(request.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return request.RemoteAddr
	}
	return host
}

func retryAfterHeader(duration time.Duration) string {
	seconds := int(duration.Round(time.Second).Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprintf("%d", seconds)
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
