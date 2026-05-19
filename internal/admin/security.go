package admin

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type createBanRequest struct {
	ClientIP        string `json:"client_ip"`
	Reason          string `json:"reason"`
	DurationSeconds int64  `json:"duration_seconds"`
}

func (router *Router) securityBans(response http.ResponseWriter, request *http.Request) {
	if router.security == nil {
		writeError(response, http.StatusServiceUnavailable, "security manager is not configured", "configuration_error")
		return
	}

	bans, err := router.security.ListActiveBans(request.Context(), time.Now().UTC())
	if err != nil {
		writeError(response, http.StatusInternalServerError, "failed to query active bans", "database_error")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"bans": bans})
}

func (router *Router) createSecurityBan(response http.ResponseWriter, request *http.Request) {
	if router.security == nil {
		writeError(response, http.StatusServiceUnavailable, "security manager is not configured", "configuration_error")
		return
	}

	var payload createBanRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, "invalid JSON body", "request_error")
		return
	}
	payload.ClientIP = strings.TrimSpace(payload.ClientIP)
	if net.ParseIP(payload.ClientIP) == nil {
		writeError(response, http.StatusBadRequest, "client_ip must be a valid IP address", "request_error")
		return
	}
	if payload.DurationSeconds <= 0 {
		payload.DurationSeconds = int64((30 * time.Minute).Seconds())
	}

	ban, err := router.security.CreateManualBan(
		request.Context(),
		payload.ClientIP,
		strings.TrimSpace(payload.Reason),
		time.Duration(payload.DurationSeconds)*time.Second,
		time.Now().UTC(),
	)
	if err != nil {
		writeError(response, http.StatusBadRequest, err.Error(), "request_error")
		return
	}
	writeJSON(response, http.StatusCreated, ban)
}

func (router *Router) liftSecurityBan(response http.ResponseWriter, request *http.Request) {
	if router.security == nil {
		writeError(response, http.StatusServiceUnavailable, "security manager is not configured", "configuration_error")
		return
	}

	id, ok := liftBanID(request.URL.Path)
	if !ok {
		writeError(response, http.StatusNotFound, "security ban not found", "not_found")
		return
	}
	if err := router.security.LiftBan(request.Context(), id, time.Now().UTC()); err != nil {
		writeError(response, http.StatusNotFound, "security ban not found", "not_found")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"ok": true})
}

func (router *Router) securityEvents(response http.ResponseWriter, request *http.Request) {
	if router.security == nil {
		writeError(response, http.StatusServiceUnavailable, "security manager is not configured", "configuration_error")
		return
	}

	limit, ok := parseLimit(response, request.URL.Query().Get("limit"))
	if !ok {
		return
	}
	events, err := router.security.ListEvents(request.Context(), limit)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "failed to query security events", "database_error")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"events": events})
}

func isLiftBanPath(path string) bool {
	return strings.HasPrefix(path, "/admin/v1/security/bans/") && strings.HasSuffix(path, "/lift")
}

func liftBanID(path string) (int64, bool) {
	raw := strings.TrimSuffix(strings.TrimPrefix(path, "/admin/v1/security/bans/"), "/lift")
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0
}
