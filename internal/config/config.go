package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Addr          string
	DataDir       string
	DBPath        string
	BaseURL       string
	SiteName      string
	AdminUser     string
	AdminPass     string
	SessionSecret string
	MaxUploadSize int64
	PreviewLimit  int64
	PageSize      int
}

func Load() Config {
	loadDotEnv(".env")

	dataDir := getEnv("FILESERVICE_DATA_DIR", "data")
	dbPath := getEnv("FILESERVICE_DB_PATH", filepath.Join(dataDir, "app.db"))

	return Config{
		Addr:          getEnv("FILESERVICE_ADDR", ":8080"),
		DataDir:       dataDir,
		DBPath:        dbPath,
		BaseURL:       strings.TrimRight(getEnv("FILESERVICE_BASE_URL", ""), "/"),
		SiteName:      getEnv("FILESERVICE_SITE_NAME", "File Service"),
		AdminUser:     getEnv("FILESERVICE_ADMIN_USER", "admin"),
		AdminPass:     getEnv("FILESERVICE_ADMIN_PASS", "admin123"),
		SessionSecret: getEnv("FILESERVICE_SESSION_SECRET", randomSecret()),
		MaxUploadSize: getEnvInt64("FILESERVICE_MAX_UPLOAD_SIZE", 64<<20),
		PreviewLimit:  getEnvInt64("FILESERVICE_PREVIEW_LIMIT", 1<<20),
		PageSize:      getEnvInt("FILESERVICE_PAGE_SIZE", 10),
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
		return "file-service-default-secret"
	}
	return hex.EncodeToString(buf)
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
