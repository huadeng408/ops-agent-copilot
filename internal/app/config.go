package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv                   string
	AppName                  string
	Host                     string
	Port                     int
	MySQLHost                string
	MySQLPort                int
	MySQLDB                  string
	MySQLUser                string
	MySQLPassword            string
	RedisHost                string
	RedisPort                int
	OpenAIBaseURL            string
	OpenAIAPIKey             string
	OpenAIModel              string
	OpenAIAuthFile           string
	LogLevel                 string
	DatabaseURL              string
	RedisURL                 string
	AgentRuntimeMode         string
	MetricsEnabled           bool
	OTELEnabled              bool
	OTELServiceName          string
	OTELExporterOTLPEndpoint string
	KeepRecentMessageCount   int
	ReadonlySQLLimit         int
}

func LoadConfig() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		AppEnv:                   getenv("APP_ENV", "dev"),
		AppName:                  getenv("APP_NAME", "ops-agent-copilot"),
		Host:                     getenv("HOST", "0.0.0.0"),
		Port:                     getenvInt("PORT", 18000),
		MySQLHost:                getenv("MYSQL_HOST", "127.0.0.1"),
		MySQLPort:                getenvInt("MYSQL_PORT", 3306),
		MySQLDB:                  getenv("MYSQL_DB", "ops_agent"),
		MySQLUser:                getenv("MYSQL_USER", "root"),
		MySQLPassword:            getenv("MYSQL_PASSWORD", "123456"),
		RedisHost:                getenv("REDIS_HOST", "127.0.0.1"),
		RedisPort:                getenvInt("REDIS_PORT", 6379),
		OpenAIBaseURL:            getenv("OPENAI_BASE_URL", "https://api.moonshot.cn/v1"),
		OpenAIAPIKey:             getenv("OPENAI_API_KEY", "sk-xxx"),
		OpenAIModel:              getenv("OPENAI_MODEL", "kimi-k2-0905-preview"),
		OpenAIAuthFile:           getenv("OPENAI_AUTH_FILE", "auth.json"),
		LogLevel:                 getenv("LOG_LEVEL", "INFO"),
		DatabaseURL:              getenv("DATABASE_URL", ""),
		RedisURL:                 getenv("REDIS_URL", ""),
		AgentRuntimeMode:         getenv("AGENT_RUNTIME_MODE", "auto"),
		MetricsEnabled:           getenvBool("METRICS_ENABLED", true),
		OTELEnabled:              getenvBool("OTEL_ENABLED", false),
		OTELServiceName:          getenv("OTEL_SERVICE_NAME", "ops-agent-copilot"),
		OTELExporterOTLPEndpoint: getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318"),
		KeepRecentMessageCount:   getenvInt("KEEP_RECENT_MESSAGE_COUNT", 8),
		ReadonlySQLLimit:         getenvInt("READONLY_SQL_LIMIT", 200),
	}

	if cfg.OpenAIAuthFile != "" {
		if key, err := readAPIKeyFromAuthFile(cfg.OpenAIAuthFile); err == nil && key != "" {
			cfg.OpenAIAPIKey = key
		}
	}

	return cfg, nil
}

func (c Config) SQLDialect() string {
	if strings.Contains(strings.ToLower(c.SQLDSN()), "sqlite") {
		return "sqlite"
	}
	return "mysql"
}

func (c Config) SQLDriverName() string {
	if strings.HasPrefix(strings.ToLower(c.SQLDSN()), "file:") || strings.HasSuffix(strings.ToLower(c.SQLDSN()), ".sqlite3") || strings.HasSuffix(strings.ToLower(c.SQLDSN()), ".db") {
		return "sqlite"
	}
	if strings.Contains(strings.ToLower(c.SQLDSN()), "sqlite") {
		return "sqlite"
	}
	return "mysql"
}

func (c Config) SQLDSN() string {
	if c.DatabaseURL != "" {
		if dsn, ok := normalizeDatabaseURL(c.DatabaseURL); ok {
			return dsn
		}
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Asia%%2FShanghai",
		c.MySQLUser, c.MySQLPassword, c.MySQLHost, c.MySQLPort, c.MySQLDB)
}

func (c Config) CacheURL() string {
	if c.RedisURL != "" {
		return c.RedisURL
	}
	return fmt.Sprintf("redis://%s:%d/0", c.RedisHost, c.RedisPort)
}

func (c Config) HasRealOpenAIAPIKey() bool {
	key := strings.TrimSpace(c.OpenAIAPIKey)
	switch key {
	case "", "sk-test", "sk-xxx", "your_openai_api_key_here", "your-openai-api-key":
		return false
	default:
		return true
	}
}

func normalizeDatabaseURL(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	lower := strings.ToLower(value)
	switch {
	case strings.HasPrefix(lower, "sqlite+aiosqlite:///"):
		return strings.TrimPrefix(value, "sqlite+aiosqlite:///"), true
	case strings.HasPrefix(lower, "sqlite:///"):
		return strings.TrimPrefix(value, "sqlite:///"), true
	case strings.HasPrefix(lower, "mysql+asyncmy://"):
		return strings.TrimPrefix(value, "mysql+asyncmy://"), true
	case strings.HasPrefix(lower, "mysql+pymysql://"):
		return strings.TrimPrefix(value, "mysql+pymysql://"), true
	case strings.HasPrefix(lower, "mysql://"):
		return strings.TrimPrefix(value, "mysql://"), true
	default:
		return value, value != ""
	}
}

func readAPIKeyFromAuthFile(candidate string) (string, error) {
	paths := []string{candidate}
	if !filepath.IsAbs(candidate) {
		paths = append(paths, filepath.Join(".", candidate))
	}

	for _, path := range paths {
		payload, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var body map[string]any
		if err := json.Unmarshal(payload, &body); err != nil {
			return "", err
		}
		key, _ := body["OPENAI_API_KEY"].(string)
		if strings.TrimSpace(key) != "" {
			return strings.TrimSpace(key), nil
		}
	}
	return "", errors.New("OPENAI_API_KEY not found in auth file")
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getenvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
