package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const defaultMaxBodyBytes int64 = 64 * 1024 * 1024

type Endpoint struct {
	ID          string
	Host        string
	UpstreamURL *url.URL
}

type Config struct {
	PublicAddr       string
	AdminAddr        string
	DBPath           string
	KeyPepper        string
	AdminToken       string
	MaxBodyBytes     int64
	SeedTestKey      bool
	DefaultEndpoints []Endpoint
}

func Load() (Config, error) {
	llmURL, err := parseURL(env("RTX_GATEWAY_LLM_UPSTREAM", "http://127.0.0.1:9180"))
	if err != nil {
		return Config{}, fmt.Errorf("parse RTX_GATEWAY_LLM_UPSTREAM: %w", err)
	}

	ocrURL, err := parseURL(env("RTX_GATEWAY_OCR_UPSTREAM", "http://127.0.0.1:9183"))
	if err != nil {
		return Config{}, fmt.Errorf("parse RTX_GATEWAY_OCR_UPSTREAM: %w", err)
	}

	maxBodyBytes, err := parseInt64(env("RTX_GATEWAY_MAX_BODY_BYTES", strconv.FormatInt(defaultMaxBodyBytes, 10)))
	if err != nil {
		return Config{}, fmt.Errorf("parse RTX_GATEWAY_MAX_BODY_BYTES: %w", err)
	}

	return Config{
		PublicAddr:   env("RTX_GATEWAY_PUBLIC_ADDR", "127.0.0.1:9188"),
		AdminAddr:    env("RTX_GATEWAY_ADMIN_ADDR", "127.0.0.1:9189"),
		DBPath:       env("RTX_GATEWAY_DB_PATH", "./rtx-gateway.db"),
		KeyPepper:    env("RTX_GATEWAY_KEY_PEPPER", "dev-insecure-change-me"),
		AdminToken:   os.Getenv("RTX_GATEWAY_ADMIN_TOKEN"),
		MaxBodyBytes: maxBodyBytes,
		SeedTestKey:  parseBool(os.Getenv("RTX_GATEWAY_SEED_TEST_KEY")),
		DefaultEndpoints: []Endpoint{
			{
				ID:          "llm",
				Host:        strings.ToLower(env("RTX_GATEWAY_LLM_HOST", "rtx-llm.arthurlin.dev")),
				UpstreamURL: llmURL,
			},
			{
				ID:          "ocr",
				Host:        strings.ToLower(env("RTX_GATEWAY_OCR_HOST", "rtx-ocr.arthurlin.dev")),
				UpstreamURL: ocrURL,
			},
		},
	}, nil
}

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func parseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("must include scheme and host")
	}
	return parsed, nil
}

func parseInt64(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return value, nil
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
