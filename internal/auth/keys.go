package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	keyPrefix = "rtx_live_"
	alphabet  = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

var (
	ErrInvalidKeyFormat = errors.New("invalid API key format")
	ErrInvalidKey       = errors.New("invalid API key")
	ErrDisabledKey      = errors.New("API key is disabled")
)

type Principal struct {
	ID     string
	Name   string
	Prefix string
	Scopes []string
}

func (p Principal) HasScope(scope string) bool {
	for _, item := range p.Scopes {
		if item == scope {
			return true
		}
	}
	return false
}

type CreatedKey struct {
	ID        string
	Name      string
	Prefix    string
	RawKey    string
	Scopes    []string
	CreatedAt time.Time
}

func ParseAPIKey(raw string) (string, error) {
	if !strings.HasPrefix(raw, keyPrefix) {
		return "", ErrInvalidKeyFormat
	}

	rest := strings.TrimPrefix(raw, keyPrefix)
	parts := strings.Split(rest, "_")
	if len(parts) != 2 || len(parts[0]) != 8 || len(parts[1]) != 32 {
		return "", ErrInvalidKeyFormat
	}

	if !isAlphaNum(parts[0]) || !isAlphaNum(parts[1]) {
		return "", ErrInvalidKeyFormat
	}

	return parts[0], nil
}

func HashKey(raw string, pepper string) string {
	mac := hmac.New(sha256.New, []byte(pepper))
	_, _ = mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}

func ValidateAPIKey(ctx context.Context, database *sql.DB, pepper string, raw string) (Principal, error) {
	prefix, err := ParseAPIKey(raw)
	if err != nil {
		return Principal{}, err
	}

	var principal Principal
	var storedHash string
	var scopesRaw string
	var enabled int
	var revokedAt sql.NullString
	err = database.QueryRowContext(
		ctx,
		`select id, name, prefix, key_hash, scopes, enabled, revoked_at
		 from api_keys
		 where prefix = ?
		 limit 1`,
		prefix,
	).Scan(&principal.ID, &principal.Name, &principal.Prefix, &storedHash, &scopesRaw, &enabled, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrInvalidKey
	}
	if err != nil {
		return Principal{}, err
	}
	if enabled != 1 || revokedAt.Valid {
		return Principal{}, ErrDisabledKey
	}

	computed := HashKey(raw, pepper)
	if subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) != 1 {
		return Principal{}, ErrInvalidKey
	}

	if err := json.Unmarshal([]byte(scopesRaw), &principal.Scopes); err != nil {
		return Principal{}, fmt.Errorf("parse key scopes: %w", err)
	}

	_, _ = database.ExecContext(ctx, "update api_keys set last_used_at = ?, updated_at = ? where id = ?", nowString(), nowString(), principal.ID)
	return principal, nil
}

func CreateAPIKey(ctx context.Context, database *sql.DB, pepper string, name string, scopes []string) (CreatedKey, error) {
	if strings.TrimSpace(name) == "" {
		return CreatedKey{}, fmt.Errorf("key name is required")
	}
	if len(scopes) == 0 {
		return CreatedKey{}, fmt.Errorf("at least one scope is required")
	}

	prefix, err := randomString(8)
	if err != nil {
		return CreatedKey{}, err
	}
	secret, err := randomString(32)
	if err != nil {
		return CreatedKey{}, err
	}

	rawKey := keyPrefix + prefix + "_" + secret
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return CreatedKey{}, err
	}

	createdAt := time.Now().UTC()
	key := CreatedKey{
		Name:      strings.TrimSpace(name),
		Prefix:    prefix,
		RawKey:    rawKey,
		Scopes:    scopes,
		CreatedAt: createdAt,
	}
	id, err := randomString(20)
	if err != nil {
		return CreatedKey{}, err
	}
	key.ID = "key_" + id

	_, err = database.ExecContext(
		ctx,
		`insert into api_keys (id, name, prefix, key_hash, scopes, enabled, created_at, updated_at)
		 values (?, ?, ?, ?, ?, 1, ?, ?)`,
		key.ID,
		key.Name,
		key.Prefix,
		HashKey(rawKey, pepper),
		string(scopesJSON),
		createdAt.Format(time.RFC3339Nano),
		createdAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return CreatedKey{}, err
	}

	return key, nil
}

func CountAPIKeys(ctx context.Context, database *sql.DB) (int, error) {
	var count int
	if err := database.QueryRowContext(ctx, "select count(*) from api_keys").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func isAlphaNum(value string) bool {
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func randomString(length int) (string, error) {
	bytes := make([]byte, length)
	random := make([]byte, length)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}

	for index, value := range random {
		bytes[index] = alphabet[int(value)%len(alphabet)]
	}
	return string(bytes), nil
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
