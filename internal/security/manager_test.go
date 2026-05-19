package security

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/config"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/db"
)

func TestRecordAuthFailureAutoBans(t *testing.T) {
	t.Parallel()

	manager, cleanup := newTestManager(t)
	defer cleanup()
	manager.policy.FailureThreshold = 2
	manager.policy.FailureWindow = time.Minute
	manager.policy.BanDurations = []time.Duration{5 * time.Minute}

	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		ban, err := manager.RecordAuthFailure(context.Background(), EventInput{
			ClientIP: "203.0.113.44",
			Host:     "rtx-llm.arthurlin.dev",
			Path:     "/v1/models",
			Detail:   "missing bearer token",
		}, now.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && ban != nil {
			t.Fatal("first failure should not ban")
		}
		if i == 1 && ban == nil {
			t.Fatal("second failure should auto-ban")
		}
	}

	if _, ok := manager.ActiveBan("203.0.113.44", now.Add(2*time.Second)); !ok {
		t.Fatal("expected active ban")
	}
}

func TestRateLimiter(t *testing.T) {
	t.Parallel()

	manager, cleanup := newTestManager(t)
	defer cleanup()
	manager.policy.UnauthLimit = 2
	manager.policy.RateLimitWindow = time.Minute

	now := time.Now().UTC()
	if ok, _ := manager.AllowUnauthed("203.0.113.45", now); !ok {
		t.Fatal("first request should be allowed")
	}
	if ok, _ := manager.AllowUnauthed("203.0.113.45", now.Add(time.Second)); !ok {
		t.Fatal("second request should be allowed")
	}
	if ok, _ := manager.AllowUnauthed("203.0.113.45", now.Add(2*time.Second)); ok {
		t.Fatal("third request should be rate limited")
	}
}

func newTestManager(t *testing.T) (*Manager, func()) {
	t.Helper()

	database, err := db.Open(context.Background(), t.TempDir()+"/security.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(context.Background(), database, []config.Endpoint{}); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(context.Background(), database, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return manager, func() {
		_ = database.Close()
	}
}
