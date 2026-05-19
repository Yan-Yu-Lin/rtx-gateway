package admin

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/config"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/health"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/security"
)

type Router struct {
	database *sql.DB
	cfg      config.Config
	checker  *health.Checker
	security *security.Manager
	logger   *slog.Logger
}

func NewRouter(database *sql.DB, cfg config.Config, checker *health.Checker, securityManager *security.Manager, logger *slog.Logger) *Router {
	return &Router{database: database, cfg: cfg, checker: checker, security: securityManager, logger: logger}
}

func (router *Router) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if !router.validAdminRequest(response, request) {
		return
	}

	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/admin/v1/keys":
		router.createKey(response, request)
	case request.Method == http.MethodGet && request.URL.Path == "/admin/v1/keys":
		router.listKeys(response, request)
	case request.Method == http.MethodPost && isRevokeKeyPath(request.URL.Path):
		router.revokeKey(response, request)
	case request.Method == http.MethodGet && request.URL.Path == "/admin/v1/usage/summary":
		router.usageSummary(response, request)
	case request.Method == http.MethodGet && request.URL.Path == "/admin/v1/usage/requests":
		router.usageRequests(response, request)
	case request.Method == http.MethodGet && request.URL.Path == "/admin/v1/health":
		router.health(response, request)
	case request.Method == http.MethodGet && request.URL.Path == "/admin/v1/health/checks":
		router.healthChecks(response, request)
	case request.Method == http.MethodPost && request.URL.Path == "/admin/v1/health/check":
		router.checkHealth(response, request)
	case request.Method == http.MethodGet && request.URL.Path == "/admin/v1/security/bans":
		router.securityBans(response, request)
	case request.Method == http.MethodPost && request.URL.Path == "/admin/v1/security/bans":
		router.createSecurityBan(response, request)
	case request.Method == http.MethodPost && isLiftBanPath(request.URL.Path):
		router.liftSecurityBan(response, request)
	case request.Method == http.MethodGet && request.URL.Path == "/admin/v1/security/events":
		router.securityEvents(response, request)
	default:
		writeError(response, http.StatusNotFound, "admin endpoint not found", "not_found")
	}
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}

func writeError(response http.ResponseWriter, status int, message string, errorType string) {
	writeJSON(response, status, map[string]any{
		"error": map[string]string{
			"message": message,
			"type":    errorType,
		},
	})
}
