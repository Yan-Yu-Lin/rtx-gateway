package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/auth"
)

type keyResponse struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Prefix     string   `json:"prefix"`
	Key        string   `json:"key,omitempty"`
	Scopes     []string `json:"scopes"`
	Enabled    bool     `json:"enabled"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
	LastUsedAt *string  `json:"last_used_at,omitempty"`
	RevokedAt  *string  `json:"revoked_at,omitempty"`
}

func (router *Router) createKey(response http.ResponseWriter, request *http.Request) {
	var payload struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, "invalid JSON body", "request_error")
		return
	}

	scopes := cleanScopes(payload.Scopes)
	if len(scopes) == 0 {
		writeError(response, http.StatusBadRequest, "at least one scope is required", "request_error")
		return
	}
	if !validScopes(scopes) {
		writeError(response, http.StatusBadRequest, "scopes must be llm, ocr, or both", "request_error")
		return
	}

	key, err := auth.CreateAPIKey(request.Context(), router.database, router.cfg.KeyPepper, payload.Name, scopes)
	if err != nil {
		writeError(response, http.StatusBadRequest, err.Error(), "request_error")
		return
	}

	writeJSON(response, http.StatusCreated, keyResponse{
		ID:        key.ID,
		Name:      key.Name,
		Prefix:    key.Prefix,
		Key:       key.RawKey,
		Scopes:    key.Scopes,
		Enabled:   true,
		CreatedAt: key.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt: key.CreatedAt.Format(time.RFC3339Nano),
	})
}

func (router *Router) listKeys(response http.ResponseWriter, request *http.Request) {
	rows, err := router.database.QueryContext(
		request.Context(),
		`select id, name, prefix, scopes, enabled, created_at, updated_at, last_used_at, revoked_at
		 from api_keys
		 order by created_at desc`,
	)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "failed to list keys", "database_error")
		return
	}
	defer rows.Close()

	keys := []keyResponse{}
	for rows.Next() {
		key, err := scanKey(rows)
		if err != nil {
			writeError(response, http.StatusInternalServerError, "failed to read key row", "database_error")
			return
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		writeError(response, http.StatusInternalServerError, "failed to list keys", "database_error")
		return
	}

	writeJSON(response, http.StatusOK, map[string]any{"keys": keys})
}

func (router *Router) revokeKey(response http.ResponseWriter, request *http.Request) {
	id := keyIDFromRevokePath(request.URL.Path)
	if id == "" {
		writeError(response, http.StatusNotFound, "key not found", "not_found")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := router.database.ExecContext(
		request.Context(),
		`update api_keys
		 set enabled = 0, revoked_at = coalesce(revoked_at, ?), updated_at = ?
		 where id = ?`,
		now,
		now,
		id,
	)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "failed to revoke key", "database_error")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(response, http.StatusNotFound, "key not found", "not_found")
		return
	}

	key, err := router.getKey(request.Context(), id)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "failed to read revoked key", "database_error")
		return
	}
	writeJSON(response, http.StatusOK, key)
}

func (router *Router) getKey(ctx context.Context, id string) (keyResponse, error) {
	row := router.database.QueryRowContext(
		ctx,
		`select id, name, prefix, scopes, enabled, created_at, updated_at, last_used_at, revoked_at
		 from api_keys
		 where id = ?`,
		id,
	)
	return scanKey(row)
}

type keyScanner interface {
	Scan(dest ...any) error
}

func scanKey(scanner keyScanner) (keyResponse, error) {
	var key keyResponse
	var scopesRaw string
	var enabled int
	var lastUsedAt sql.NullString
	var revokedAt sql.NullString
	if err := scanner.Scan(&key.ID, &key.Name, &key.Prefix, &scopesRaw, &enabled, &key.CreatedAt, &key.UpdatedAt, &lastUsedAt, &revokedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return keyResponse{}, err
		}
		return keyResponse{}, err
	}
	if err := json.Unmarshal([]byte(scopesRaw), &key.Scopes); err != nil {
		return keyResponse{}, err
	}
	key.Enabled = enabled == 1
	if lastUsedAt.Valid {
		key.LastUsedAt = &lastUsedAt.String
	}
	if revokedAt.Valid {
		key.RevokedAt = &revokedAt.String
	}
	return key, nil
}

func isRevokeKeyPath(path string) bool {
	return strings.HasPrefix(path, "/admin/v1/keys/") && strings.HasSuffix(path, "/revoke")
}

func keyIDFromRevokePath(path string) string {
	if !isRevokeKeyPath(path) {
		return ""
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, "/admin/v1/keys/"), "/revoke")
	return strings.Trim(id, "/")
}

func cleanScopes(raw []string) []string {
	seen := map[string]bool{}
	scopes := []string{}
	for _, scope := range raw {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if scope == "" || seen[scope] {
			continue
		}
		seen[scope] = true
		scopes = append(scopes, scope)
	}
	return scopes
}

func validScopes(scopes []string) bool {
	for _, scope := range scopes {
		if scope != "llm" && scope != "ocr" {
			return false
		}
	}
	return true
}
