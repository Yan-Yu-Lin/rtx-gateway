package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestClientIPPrefersXRealIP(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("X-Forwarded-For", "198.51.100.20, 127.0.0.1")
	request.Header.Set("X-Real-IP", "203.0.113.10")

	if got := clientIP(request); got != "203.0.113.10" {
		t.Fatalf("clientIP = %q, want X-Real-IP", got)
	}
}

func TestDirectorStripsAuthorization(t *testing.T) {
	t.Parallel()

	target, err := url.Parse("http://127.0.0.1:9180")
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "https://rtx-llm.arthurlin.dev/v1/models", nil)
	request.Host = "rtx-llm.arthurlin.dev"
	request.Header.Set("Authorization", "Bearer rtx_live_abcdefgh_secret")

	director(target, "req_test")(request)

	if got := request.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization header forwarded as %q", got)
	}
	if got := request.Header.Get("X-Forwarded-Host"); got != "rtx-llm.arthurlin.dev" {
		t.Fatalf("X-Forwarded-Host = %q", got)
	}
}
