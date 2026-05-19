package security

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	EventAuthFailure = "auth_failure"
	EventRateLimited = "rate_limited"
	EventAutoBanned  = "auto_banned"
	EventManualBan   = "manual_ban"
	EventBanLifted   = "ban_lifted"
	EventScopeDenied = "scope_denied"
)

type Policy struct {
	UnauthLimit      int
	AuthIPLimit      int
	AuthKeyLimit     int
	RateLimitWindow  time.Duration
	FailureThreshold int
	FailureWindow    time.Duration
	BanDurations     []time.Duration
}

func DefaultPolicy() Policy {
	return Policy{
		UnauthLimit:      30,
		AuthIPLimit:      300,
		AuthKeyLimit:     600,
		RateLimitWindow:  time.Minute,
		FailureThreshold: 20,
		FailureWindow:    10 * time.Minute,
		BanDurations: []time.Duration{
			5 * time.Minute,
			30 * time.Minute,
			2 * time.Hour,
			24 * time.Hour,
		},
	}
}

type Manager struct {
	database *sql.DB
	logger   *slog.Logger
	policy   Policy

	mu       sync.Mutex
	bans     map[string]Ban
	failures map[string][]time.Time
	limits   map[string][]time.Time
}

type Ban struct {
	ID          int64      `json:"id"`
	ClientIP    string     `json:"client_ip"`
	Reason      string     `json:"reason"`
	Strikes     int        `json:"strikes"`
	BannedUntil time.Time  `json:"banned_until"`
	Manual      bool       `json:"manual"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	LiftedAt    *time.Time `json:"lifted_at,omitempty"`
}

type Event struct {
	ID         int64     `json:"id"`
	ClientIP   string    `json:"client_ip"`
	EventType  string    `json:"event_type"`
	Host       string    `json:"host,omitempty"`
	Path       string    `json:"path,omitempty"`
	StatusCode *int      `json:"status_code,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type EventInput struct {
	ClientIP   string
	EventType  string
	Host       string
	Path       string
	StatusCode *int
	Detail     string
}

func NewManager(ctx context.Context, database *sql.DB, logger *slog.Logger) (*Manager, error) {
	manager := &Manager{
		database: database,
		logger:   logger,
		policy:   DefaultPolicy(),
		bans:     map[string]Ban{},
		failures: map[string][]time.Time{},
		limits:   map[string][]time.Time{},
	}
	if err := manager.LoadActiveBans(ctx); err != nil {
		return nil, err
	}
	return manager, nil
}

func (manager *Manager) LoadActiveBans(ctx context.Context) error {
	now := time.Now().UTC()
	bans, err := manager.ListActiveBans(ctx, now)
	if err != nil {
		return err
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.bans = map[string]Ban{}
	for _, ban := range bans {
		manager.bans[ban.ClientIP] = ban
	}
	return nil
}

func (manager *Manager) ActiveBan(clientIP string, now time.Time) (Ban, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	ban, ok := manager.bans[clientIP]
	if !ok {
		return Ban{}, false
	}
	if ban.LiftedAt != nil || !ban.BannedUntil.After(now.UTC()) {
		delete(manager.bans, clientIP)
		return Ban{}, false
	}
	return ban, true
}

func (manager *Manager) AllowUnauthed(clientIP string, now time.Time) (bool, time.Duration) {
	return manager.allow("unauth:"+clientIP, manager.policy.UnauthLimit, manager.policy.RateLimitWindow, now)
}

func (manager *Manager) AllowAuthed(clientIP string, apiKeyID string, now time.Time) (bool, time.Duration) {
	if ok, retryAfter := manager.allow("auth-ip:"+clientIP, manager.policy.AuthIPLimit, manager.policy.RateLimitWindow, now); !ok {
		return false, retryAfter
	}
	return manager.allow("auth-key:"+apiKeyID, manager.policy.AuthKeyLimit, manager.policy.RateLimitWindow, now)
}

func (manager *Manager) RecordAuthFailure(ctx context.Context, input EventInput, now time.Time) (*Ban, error) {
	input.EventType = EventAuthFailure
	if err := manager.RecordEvent(ctx, input, now); err != nil {
		return nil, err
	}

	windowStart := now.UTC().Add(-manager.policy.FailureWindow)
	manager.mu.Lock()
	failures := append(manager.failures[input.ClientIP], now.UTC())
	failures = pruneTimes(failures, windowStart)
	manager.failures[input.ClientIP] = failures
	shouldBan := len(failures) >= manager.policy.FailureThreshold
	manager.mu.Unlock()

	if !shouldBan {
		return nil, nil
	}

	ban, err := manager.CreateAutoBan(ctx, input.ClientIP, "too many authentication failures", now)
	if err != nil {
		return nil, err
	}

	manager.mu.Lock()
	delete(manager.failures, input.ClientIP)
	manager.mu.Unlock()

	status := 403
	_ = manager.RecordEvent(ctx, EventInput{
		ClientIP:   input.ClientIP,
		EventType:  EventAutoBanned,
		Host:       input.Host,
		Path:       input.Path,
		StatusCode: &status,
		Detail:     fmt.Sprintf("banned until %s after %d strikes", ban.BannedUntil.Format(time.RFC3339), ban.Strikes),
	}, now)
	return &ban, nil
}

func (manager *Manager) CreateAutoBan(ctx context.Context, clientIP string, reason string, now time.Time) (Ban, error) {
	strikes, err := manager.nextStrikeCount(ctx, clientIP)
	if err != nil {
		return Ban{}, err
	}
	duration := manager.banDuration(strikes)
	return manager.createBan(ctx, clientIP, reason, strikes, duration, false, now)
}

func (manager *Manager) CreateManualBan(ctx context.Context, clientIP string, reason string, duration time.Duration, now time.Time) (Ban, error) {
	clientIP = strings.TrimSpace(clientIP)
	if net.ParseIP(clientIP) == nil {
		return Ban{}, fmt.Errorf("client_ip must be a valid IP address")
	}
	if duration <= 0 {
		return Ban{}, fmt.Errorf("duration must be positive")
	}
	if strings.TrimSpace(reason) == "" {
		reason = "manual ban"
	}

	ban, err := manager.createBan(ctx, clientIP, reason, 1, duration, true, now)
	if err != nil {
		return Ban{}, err
	}
	status := 403
	_ = manager.RecordEvent(ctx, EventInput{
		ClientIP:   clientIP,
		EventType:  EventManualBan,
		StatusCode: &status,
		Detail:     reason,
	}, now)
	return ban, nil
}

func (manager *Manager) LiftBan(ctx context.Context, id int64, now time.Time) error {
	var clientIP string
	err := manager.database.QueryRowContext(ctx, "select client_ip from ip_bans where id = ?", id).Scan(&clientIP)
	if err != nil {
		return err
	}

	nowString := now.UTC().Format(time.RFC3339Nano)
	result, err := manager.database.ExecContext(
		ctx,
		"update ip_bans set lifted_at = ?, updated_at = ? where id = ? and lifted_at is null",
		nowString,
		nowString,
		id,
	)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}

	manager.mu.Lock()
	delete(manager.bans, clientIP)
	manager.mu.Unlock()

	_ = manager.RecordEvent(ctx, EventInput{
		ClientIP:  clientIP,
		EventType: EventBanLifted,
		Detail:    fmt.Sprintf("ban %d lifted", id),
	}, now)
	return nil
}

func (manager *Manager) ListActiveBans(ctx context.Context, now time.Time) ([]Ban, error) {
	rows, err := manager.database.QueryContext(
		ctx,
		`select id, client_ip, reason, strikes, banned_until, manual, created_at, updated_at, lifted_at
		 from ip_bans
		 where lifted_at is null and banned_until > ?
		 order by banned_until desc`,
		now.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bans []Ban
	for rows.Next() {
		ban, err := scanBan(rows)
		if err != nil {
			return nil, err
		}
		bans = append(bans, ban)
	}
	return bans, rows.Err()
}

func (manager *Manager) ListEvents(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	rows, err := manager.database.QueryContext(
		ctx,
		`select id, client_ip, event_type, host, path, status_code, detail, created_at
		 from security_events
		 order by created_at desc
		 limit ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (manager *Manager) RecordEvent(ctx context.Context, input EventInput, now time.Time) error {
	if strings.TrimSpace(input.ClientIP) == "" {
		return nil
	}
	_, err := manager.database.ExecContext(
		ctx,
		`insert into security_events (client_ip, event_type, host, path, status_code, detail, created_at)
		 values (?, ?, ?, ?, ?, ?, ?)`,
		input.ClientIP,
		input.EventType,
		nullEmpty(input.Host),
		nullEmpty(input.Path),
		input.StatusCode,
		nullEmpty(input.Detail),
		now.UTC().Format(time.RFC3339Nano),
	)
	if err != nil && manager.logger != nil {
		manager.logger.Warn("failed to record security event", "client_ip", input.ClientIP, "event_type", input.EventType, "error", err)
	}
	return err
}

func (manager *Manager) allow(bucket string, limit int, window time.Duration, now time.Time) (bool, time.Duration) {
	if limit <= 0 {
		return true, 0
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	now = now.UTC()
	windowStart := now.Add(-window)
	hits := pruneTimes(manager.limits[bucket], windowStart)
	if len(hits) >= limit {
		return false, hits[0].Add(window).Sub(now)
	}
	hits = append(hits, now)
	manager.limits[bucket] = hits
	return true, 0
}

func (manager *Manager) createBan(ctx context.Context, clientIP string, reason string, strikes int, duration time.Duration, manual bool, now time.Time) (Ban, error) {
	now = now.UTC()
	bannedUntil := now.Add(duration)
	result, err := manager.database.ExecContext(
		ctx,
		`insert into ip_bans (client_ip, reason, strikes, banned_until, manual, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?)`,
		clientIP,
		reason,
		strikes,
		bannedUntil.Format(time.RFC3339Nano),
		boolInt(manual),
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Ban{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Ban{}, err
	}

	ban := Ban{
		ID:          id,
		ClientIP:    clientIP,
		Reason:      reason,
		Strikes:     strikes,
		BannedUntil: bannedUntil,
		Manual:      manual,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	manager.mu.Lock()
	manager.bans[clientIP] = ban
	manager.mu.Unlock()
	return ban, nil
}

func (manager *Manager) nextStrikeCount(ctx context.Context, clientIP string) (int, error) {
	var strikes sql.NullInt64
	err := manager.database.QueryRowContext(ctx, "select max(strikes) from ip_bans where client_ip = ?", clientIP).Scan(&strikes)
	if err != nil {
		return 0, err
	}
	if !strikes.Valid {
		return 1, nil
	}
	return int(strikes.Int64) + 1, nil
}

func (manager *Manager) banDuration(strikes int) time.Duration {
	if len(manager.policy.BanDurations) == 0 {
		return 5 * time.Minute
	}
	index := strikes - 1
	if index < 0 {
		index = 0
	}
	if index >= len(manager.policy.BanDurations) {
		index = len(manager.policy.BanDurations) - 1
	}
	return manager.policy.BanDurations[index]
}

func pruneTimes(values []time.Time, after time.Time) []time.Time {
	index := 0
	for _, value := range values {
		if value.After(after) {
			values[index] = value
			index++
		}
	}
	return values[:index]
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullEmpty(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}
