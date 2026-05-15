package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/config"
)

type Router struct {
	database *sql.DB
	cfg      config.Config
}

func NewRouter(database *sql.DB, cfg config.Config) *Router {
	return &Router{database: database, cfg: cfg}
}

func (router *Router) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/health" || request.URL.Path == "/healthz" || request.URL.Path == "/admin/v1/health" {
		writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	if router.cfg.AdminToken != "" && !validAdminToken(request.Header.Get("Authorization"), router.cfg.AdminToken) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{
			"error": map[string]string{
				"message": "missing or invalid admin token",
				"type":    "auth_error",
			},
		})
		return
	}

	writeJSON(response, http.StatusNotImplemented, map[string]any{
		"error": map[string]string{
			"message": "admin API endpoint is not implemented in Phase 1",
			"type":    "not_implemented",
		},
	})
}

func validAdminToken(header string, expected string) bool {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	return len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && parts[1] == expected
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}
