package admin

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

func (router *Router) validAdminRequest(response http.ResponseWriter, request *http.Request) bool {
	if router.cfg.AdminToken == "" {
		writeError(response, http.StatusServiceUnavailable, "admin token is not configured", "configuration_error")
		return false
	}

	if !validAdminToken(request.Header.Get("Authorization"), router.cfg.AdminToken) {
		writeError(response, http.StatusUnauthorized, "missing or invalid admin token", "auth_error")
		return false
	}

	return true
}

func validAdminToken(header string, expected string) bool {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	return constantTimeStringEqual(strings.TrimSpace(parts[1]), expected)
}

func constantTimeStringEqual(actual string, expected string) bool {
	actualHash := sha256.Sum256([]byte(actual))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(actualHash[:], expectedHash[:]) == 1 && len(actual) == len(expected)
}
