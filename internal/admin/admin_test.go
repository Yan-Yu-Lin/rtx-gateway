package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/auth"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/config"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/db"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/health"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/security"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/usage"
)

const testAdminToken = "admin-test-token"
const testPepper = "test-pepper"

func TestValidAdminToken(t *testing.T) {
	t.Parallel()

	if !validAdminToken("Bearer "+testAdminToken, testAdminToken) {
		t.Fatal("expected valid bearer token")
	}
	if validAdminToken("Bearer wrong", testAdminToken) {
		t.Fatal("expected wrong bearer token to fail")
	}
	if validAdminToken(testAdminToken, testAdminToken) {
		t.Fatal("expected missing bearer scheme to fail")
	}
}

func TestAdminRouterRequiresAuth(t *testing.T) {
	t.Parallel()

	router, cleanup := newTestRouter(t, nil)
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/v1/keys", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestKeyCRUD(t *testing.T) {
	t.Parallel()

	router, cleanup := newTestRouter(t, nil)
	defer cleanup()

	createResponse := doAdminRequest(t, router, http.MethodPost, "/admin/v1/keys", `{"name":"PII dev","scopes":["ocr","llm"]}`)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}

	var created keyResponse
	decodeJSON(t, createResponse.Body.Bytes(), &created)
	if created.ID == "" || created.Key == "" || created.Prefix == "" {
		t.Fatalf("created key missing fields: %+v", created)
	}

	var storedHash string
	err := router.database.QueryRow("select key_hash from api_keys where id = ?", created.ID).Scan(&storedHash)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storedHash, created.Key) {
		t.Fatal("raw key appears to be stored in key_hash")
	}

	listResponse := doAdminRequest(t, router, http.MethodGet, "/admin/v1/keys", "")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	var listed struct {
		Keys []keyResponse `json:"keys"`
	}
	decodeJSON(t, listResponse.Body.Bytes(), &listed)
	if len(listed.Keys) != 1 {
		t.Fatalf("listed keys = %d, want 1", len(listed.Keys))
	}
	if listed.Keys[0].Key != "" {
		t.Fatal("list response exposed raw key")
	}

	revokeResponse := doAdminRequest(t, router, http.MethodPost, "/admin/v1/keys/"+created.ID+"/revoke", "")
	if revokeResponse.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, body = %s", revokeResponse.Code, revokeResponse.Body.String())
	}

	_, err = auth.ValidateAPIKey(context.Background(), router.database, testPepper, created.Key)
	if !errors.Is(err, auth.ErrDisabledKey) {
		t.Fatalf("validate revoked key error = %v, want %v", err, auth.ErrDisabledKey)
	}
}

func TestUsageQueries(t *testing.T) {
	t.Parallel()

	router, cleanup := newTestRouter(t, nil)
	defer cleanup()

	created, err := auth.CreateAPIKey(context.Background(), router.database, testPepper, "usage key", []string{"ocr"})
	if err != nil {
		t.Fatal(err)
	}

	keyID := created.ID
	promptTokens := 10
	completionTokens := 20
	totalTokens := 30
	upstreamStatus := http.StatusOK
	now := time.Now().UTC()
	for _, entry := range []usage.Entry{
		{
			RequestID:          "req_ocr",
			APIKeyID:           &keyID,
			APIKeyPrefix:       created.Prefix,
			EndpointID:         "ocr",
			Host:               "rtx-ocr.arthurlin.dev",
			Method:             http.MethodPost,
			Path:               "/v1/chat/completions",
			Model:              stringPtr("chandra"),
			PromptTokens:       &promptTokens,
			CompletionTokens:   &completionTokens,
			TotalTokens:        &totalTokens,
			StatusCode:         http.StatusOK,
			UpstreamStatusCode: &upstreamStatus,
			LatencyMS:          123,
			CreatedAt:          now,
		},
		{
			RequestID:  "req_llm_error",
			EndpointID: "llm",
			Host:       "rtx-llm.arthurlin.dev",
			Method:     http.MethodGet,
			Path:       "/v1/models",
			StatusCode: http.StatusUnauthorized,
			LatencyMS:  3,
			Error:      "missing bearer token",
			CreatedAt:  now,
		},
	} {
		if err := usage.Insert(context.Background(), router.database, entry); err != nil {
			t.Fatal(err)
		}
	}

	from := now.Add(-time.Hour).Format(time.RFC3339Nano)
	to := now.Add(time.Hour).Format(time.RFC3339Nano)
	summaryResponse := doAdminRequest(t, router, http.MethodGet, "/admin/v1/usage/summary?from="+url.QueryEscape(from)+"&to="+url.QueryEscape(to)+"&group_by=day", "")
	if summaryResponse.Code != http.StatusOK {
		t.Fatalf("summary status = %d, body = %s", summaryResponse.Code, summaryResponse.Body.String())
	}
	var summary struct {
		Rows []usageSummaryRow `json:"rows"`
	}
	decodeJSON(t, summaryResponse.Body.Bytes(), &summary)
	if len(summary.Rows) != 1 {
		t.Fatalf("summary rows = %d, want 1", len(summary.Rows))
	}
	if summary.Rows[0].Requests != 2 || summary.Rows[0].Errors != 1 || summary.Rows[0].TotalTokens != int64(totalTokens) {
		t.Fatalf("unexpected summary row: %+v", summary.Rows[0])
	}

	requestsResponse := doAdminRequest(t, router, http.MethodGet, "/admin/v1/usage/requests?endpoint=ocr&api_key_id="+url.QueryEscape(keyID), "")
	if requestsResponse.Code != http.StatusOK {
		t.Fatalf("requests status = %d, body = %s", requestsResponse.Code, requestsResponse.Body.String())
	}
	var requests struct {
		Requests []usageRequestRow `json:"requests"`
	}
	decodeJSON(t, requestsResponse.Body.Bytes(), &requests)
	if len(requests.Requests) != 1 {
		t.Fatalf("usage requests = %d, want 1", len(requests.Requests))
	}
	if requests.Requests[0].EndpointID != "ocr" || requests.Requests[0].TotalTokens == nil || *requests.Requests[0].TotalTokens != int64(totalTokens) {
		t.Fatalf("unexpected usage request: %+v", requests.Requests[0])
	}
}

func TestHealthCheck(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/health" {
			http.NotFound(response, request)
			return
		}
		_, _ = response.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	endpoints := []config.Endpoint{
		{ID: "llm", Host: "rtx-llm.arthurlin.dev", UpstreamURL: upstreamURL},
	}
	router, cleanup := newTestRouter(t, endpoints)
	defer cleanup()

	checkResponse := doAdminRequest(t, router, http.MethodPost, "/admin/v1/health/check", "")
	if checkResponse.Code != http.StatusOK {
		t.Fatalf("health check status = %d, body = %s", checkResponse.Code, checkResponse.Body.String())
	}

	healthResponse := doAdminRequest(t, router, http.MethodGet, "/admin/v1/health", "")
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", healthResponse.Code, healthResponse.Body.String())
	}
	var payload struct {
		Endpoints []endpointHealthResponse `json:"endpoints"`
	}
	decodeJSON(t, healthResponse.Body.Bytes(), &payload)
	if len(payload.Endpoints) != 1 {
		t.Fatalf("health endpoints = %d, want 1", len(payload.Endpoints))
	}
	if payload.Endpoints[0].LastHealthStatus == nil || *payload.Endpoints[0].LastHealthStatus != "healthy" {
		t.Fatalf("unexpected endpoint health: %+v", payload.Endpoints[0])
	}

	checksResponse := doAdminRequest(t, router, http.MethodGet, "/admin/v1/health/checks?limit=10", "")
	if checksResponse.Code != http.StatusOK {
		t.Fatalf("health checks status = %d, body = %s", checksResponse.Code, checksResponse.Body.String())
	}
	var checksPayload struct {
		Checks []endpointHealthCheckResponse `json:"checks"`
	}
	decodeJSON(t, checksResponse.Body.Bytes(), &checksPayload)
	if len(checksPayload.Checks) != 1 || checksPayload.Checks[0].Status != "healthy" {
		t.Fatalf("unexpected health checks: %+v", checksPayload.Checks)
	}
}

func TestSecurityAdminEndpoints(t *testing.T) {
	t.Parallel()

	router, cleanup := newTestRouter(t, nil)
	defer cleanup()

	createResponse := doAdminRequest(t, router, http.MethodPost, "/admin/v1/security/bans", `{"client_ip":"203.0.113.55","reason":"test ban","duration_seconds":300}`)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create ban status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}

	var created struct {
		ID       int64  `json:"id"`
		ClientIP string `json:"client_ip"`
	}
	decodeJSON(t, createResponse.Body.Bytes(), &created)
	if created.ID == 0 || created.ClientIP != "203.0.113.55" {
		t.Fatalf("unexpected created ban: %+v", created)
	}

	listResponse := doAdminRequest(t, router, http.MethodGet, "/admin/v1/security/bans", "")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list bans status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	var listed struct {
		Bans []struct {
			ID int64 `json:"id"`
		} `json:"bans"`
	}
	decodeJSON(t, listResponse.Body.Bytes(), &listed)
	if len(listed.Bans) != 1 {
		t.Fatalf("listed bans = %d, want 1", len(listed.Bans))
	}

	eventsResponse := doAdminRequest(t, router, http.MethodGet, "/admin/v1/security/events?limit=10", "")
	if eventsResponse.Code != http.StatusOK {
		t.Fatalf("events status = %d, body = %s", eventsResponse.Code, eventsResponse.Body.String())
	}
	var events struct {
		Events []struct {
			EventType string `json:"event_type"`
		} `json:"events"`
	}
	decodeJSON(t, eventsResponse.Body.Bytes(), &events)
	if len(events.Events) == 0 || events.Events[0].EventType != security.EventManualBan {
		t.Fatalf("unexpected events: %+v", events.Events)
	}

	liftResponse := doAdminRequest(t, router, http.MethodPost, "/admin/v1/security/bans/"+strconv.FormatInt(created.ID, 10)+"/lift", "")
	if liftResponse.Code != http.StatusOK {
		t.Fatalf("lift status = %d, body = %s", liftResponse.Code, liftResponse.Body.String())
	}

	listResponse = doAdminRequest(t, router, http.MethodGet, "/admin/v1/security/bans", "")
	decodeJSON(t, listResponse.Body.Bytes(), &listed)
	if len(listed.Bans) != 0 {
		t.Fatalf("listed bans after lift = %d, want 0", len(listed.Bans))
	}
}

func newTestRouter(t *testing.T, endpoints []config.Endpoint) (*Router, func()) {
	t.Helper()

	if endpoints == nil {
		endpoints = defaultTestEndpoints(t)
	}
	cfg := config.Config{
		AdminToken:       testAdminToken,
		KeyPepper:        testPepper,
		DefaultEndpoints: endpoints,
	}
	database, err := db.Open(context.Background(), t.TempDir()+"/test.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(context.Background(), database, endpoints); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	checker := health.NewChecker(database, endpoints, logger)
	securityManager, err := security.NewManager(context.Background(), database, logger)
	if err != nil {
		t.Fatal(err)
	}
	return NewRouter(database, cfg, checker, securityManager, logger), func() {
		_ = database.Close()
	}
}

func defaultTestEndpoints(t *testing.T) []config.Endpoint {
	t.Helper()

	llmURL, err := url.Parse("http://127.0.0.1:19180")
	if err != nil {
		t.Fatal(err)
	}
	ocrURL, err := url.Parse("http://127.0.0.1:19183")
	if err != nil {
		t.Fatal(err)
	}
	return []config.Endpoint{
		{ID: "llm", Host: "rtx-llm.arthurlin.dev", UpstreamURL: llmURL},
		{ID: "ocr", Host: "rtx-ocr.arthurlin.dev", UpstreamURL: ocrURL},
	}
}

func doAdminRequest(t *testing.T, router *Router, method string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func decodeJSON(t *testing.T, body []byte, target any) {
	t.Helper()

	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode JSON %q: %v", string(body), err)
	}
}

func stringPtr(value string) *string {
	return &value
}
