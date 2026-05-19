package proxy

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/config"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/security"
)

type Router struct {
	handler *Handler
}

func NewRouter(database *sql.DB, cfg config.Config, logger *slog.Logger, securityManager *security.Manager) *Router {
	endpoints := make(map[string]config.Endpoint, len(cfg.DefaultEndpoints))
	for _, endpoint := range cfg.DefaultEndpoints {
		endpoints[strings.ToLower(endpoint.Host)] = endpoint
	}

	return &Router{
		handler: NewHandler(database, cfg, logger, endpoints, securityManager),
	}
}

func (r *Router) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/health" || request.URL.Path == "/healthz" {
		writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	r.handler.ServeHTTP(response, request)
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}
