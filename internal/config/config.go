package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Addr                 string
	DataDir              string
	DBPath               string
	BaseURL              string
	SiteName             string
	AdminUser            string
	AdminPass            string
	SessionSecret        string
	DefaultMaxUploadSize int64
	DefaultPreviewLimit  int64
	DefaultPageSize      int
	AccessLogRetention   int
}

type RuntimeSettings struct {
	MaxUploadSize       int64
	PreviewLimit        int64
	PageSize            int
	AccessLogRetention  int
	DefaultExpireOption string
}

const (
	DefaultMaxUploadSize      int64 = 64 << 20
	DefaultPreviewLimit       int64 = 1 << 20
	DefaultPageSize                 = 10
	DefaultAccessLogRetention       = 5000
	DefaultExpireOption             = "7d"
)

func Load() (Config, error) {
	loadDotEnv(".env")

	dataDir := getEnv("FILESERVICE_DATA_DIR", "data")
	dbPath := getEnv("FILESERVICE_DB_PATH", filepath.Join(dataDir, "app.db"))
	sessionSecret, err := loadOrCreateSessionSecret(dataDir)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Addr:                 getEnv("FILESERVICE_ADDR", ":8080"),
		DataDir:              dataDir,
		DBPath:               dbPath,
		BaseURL:              strings.TrimRight(getEnv("FILESERVICE_BASE_URL", ""), "/"),
		SiteName:             getEnv("FILESERVICE_SITE_NAME", "ShareNest"),
		AdminUser:            getEnv("FILESERVICE_ADMIN_USER", "admin"),
		AdminPass:            getEnv("FILESERVICE_ADMIN_PASS", "admin123"),
		SessionSecret:        sessionSecret,
		DefaultMaxUploadSize: DefaultMaxUploadSize,
		DefaultPreviewLimit:  DefaultPreviewLimit,
		DefaultPageSize:      DefaultPageSize,
		AccessLogRetention:   DefaultAccessLogRetention,
	}, nil
}

func DefaultRuntimeConfig() RuntimeSettings {
	return RuntimeSettings{
		MaxUploadSize:       DefaultMaxUploadSize,
		PreviewLimit:        DefaultPreviewLimit,
		PageSize:            DefaultPageSize,
		AccessLogRetention:  DefaultAccessLogRetention,
		DefaultExpireOption: DefaultExpireOption,
	}
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func randomSecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}

func loadOrCreateSessionSecret(dataDir string) (string, error) {
	if value := strings.TrimSpace(os.Getenv("FILESERVICE_SESSION_SECRET")); value != "" {
		return value, nil
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}

	secretPath := filepath.Join(dataDir, "session_secret")
	if data, err := os.ReadFile(secretPath); err == nil {
		if secret := strings.TrimSpace(string(data)); secret != "" {
			return secret, nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	secret := randomSecret()
	if secret == "" {
		return "", fmt.Errorf("generate session secret: crypto/rand unavailable")
	}
	if err := os.WriteFile(secretPath, []byte(secret), 0o600); err != nil {
		return "", err
	}
	return secret, nil
}

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}
