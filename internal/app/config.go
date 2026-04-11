package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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
	LLMProvider              string
	LLMBaseURL               string
	LLMAPIKey                string
	LLMModel                 string
	RouterPrimaryModel       string
	RouterFallbackModel      string
	RouterNoThink            bool
	RouterRecentMessageCount int
	RouterConfidenceCutoff   float64
	LLMAuthFile              string
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
	LangGraphBaseURL         string
	LangGraphTimeoutMS       int
	InternalAPIKey           string
}

func LoadConfig() (Config, error) {
	_ = godotenv.Load()

	rawLLMBaseURL := getenvAny([]string{"LLM_BASE_URL", "OPENAI_BASE_URL"}, "")
	rawLLMModel := getenvAny([]string{"LLM_MODEL", "OPENAI_MODEL"}, "")

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
		LLMProvider:              resolveLLMProvider(getenvAny([]string{"LLM_PROVIDER"}, ""), rawLLMBaseURL, rawLLMModel),
		LLMBaseURL:               rawLLMBaseURL,
		LLMAPIKey:                getenvAny([]string{"LLM_API_KEY", "OPENAI_API_KEY"}, ""),
		LLMModel:                 rawLLMModel,
		RouterPrimaryModel:       getenvAny([]string{"ROUTER_PRIMARY_MODEL"}, ""),
		RouterFallbackModel:      getenvAny([]string{"ROUTER_FALLBACK_MODEL", "LLM_FALLBACK_MODEL"}, ""),
		RouterNoThink:            getenvBool("ROUTER_NO_THINK", true),
		RouterRecentMessageCount: getenvInt("ROUTER_RECENT_MESSAGE_COUNT", 2),
		RouterConfidenceCutoff:   getenvFloat("ROUTER_CONFIDENCE_CUTOFF", 0.72),
		LogLevel:                 getenv("LOG_LEVEL", "INFO"),
		DatabaseURL:              getenv("DATABASE_URL", ""),
		RedisURL:                 getenv("REDIS_URL", ""),
		AgentRuntimeMode:         normalizeRuntimeMode(getenv("AGENT_RUNTIME_MODE", "auto")),
		MetricsEnabled:           getenvBool("METRICS_ENABLED", true),
		OTELEnabled:              getenvBool("OTEL_ENABLED", false),
		OTELServiceName:          getenv("OTEL_SERVICE_NAME", "ops-agent-copilot"),
		OTELExporterOTLPEndpoint: getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318"),
		KeepRecentMessageCount:   getenvInt("KEEP_RECENT_MESSAGE_COUNT", 8),
		ReadonlySQLLimit:         getenvInt("READONLY_SQL_LIMIT", 200),
		LangGraphBaseURL:         getenv("LANGGRAPH_BASE_URL", "http://127.0.0.1:8001"),
		LangGraphTimeoutMS:       getenvInt("LANGGRAPH_TIMEOUT_MS", 30000),
		InternalAPIKey:           getenv("INTERNAL_API_KEY", ""),
	}

	applyLLMDefaults(&cfg)
	applyRouterDefaults(&cfg)
	cfg.LLMAuthFile = getenvLLMAuthFile(cfg.LLMProvider)
	if cfg.LLMAuthFile != "" {
		if key, err := readAPIKeyFromAuthFile(cfg.LLMAuthFile); err == nil && key != "" {
			cfg.LLMAPIKey = key
		}
	}
	if cfg.LLMProvider == "ollama" && strings.TrimSpace(cfg.LLMAPIKey) == "" {
		cfg.LLMAPIKey = "ollama-local"
	}
	if err := cfg.ValidateLLMConfig(); err != nil {
		return Config{}, err
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

func (c Config) HasUsableLLMConfig() bool {
	if strings.TrimSpace(c.LLMBaseURL) == "" || strings.TrimSpace(c.LLMModel) == "" {
		return false
	}
	switch c.LLMProvider {
	case "kimi":
		return hasRealAPIKey(c.LLMAPIKey)
	case "ollama":
		return true
	default:
		return false
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
		key, _ := body["LLM_API_KEY"].(string)
		if strings.TrimSpace(key) == "" {
			key, _ = body["OPENAI_API_KEY"].(string)
		}
		if strings.TrimSpace(key) != "" {
			return strings.TrimSpace(key), nil
		}
	}
	return "", errors.New("LLM_API_KEY not found in auth file")
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getenvAny(keys []string, fallback string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return fallback
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

func getenvFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
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

func getenvLLMAuthFile(provider string) string {
	switch provider {
	case "ollama":
		return getenvAny([]string{"LLM_AUTH_FILE"}, "")
	default:
		return getenvAny([]string{"LLM_AUTH_FILE", "OPENAI_AUTH_FILE"}, "auth.json")
	}
}

func normalizeLLMProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "kimi":
		return "kimi"
	case "ollama":
		return "ollama"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func resolveLLMProvider(provider string, baseURL string, model string) string {
	if strings.TrimSpace(provider) != "" {
		return normalizeLLMProvider(provider)
	}
	lowerModel := strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(lowerModel, "gemma4") || strings.HasPrefix(lowerModel, "qwen3") {
		return "ollama"
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(baseURL)), ":11434") {
		return "ollama"
	}
	return "kimi"
}

func normalizeRuntimeMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "auto"
	case "heuristic":
		return "heuristic"
	case "langgraph":
		return "langgraph"
	case "llm", "openai":
		return "llm"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func (c Config) UseLangGraphRuntime() bool {
	return normalizeRuntimeMode(c.AgentRuntimeMode) == "langgraph"
}

func applyLLMDefaults(cfg *Config) {
	switch cfg.LLMProvider {
	case "ollama":
		if strings.TrimSpace(cfg.LLMBaseURL) == "" {
			cfg.LLMBaseURL = "http://127.0.0.1:11434/v1"
		}
		if strings.TrimSpace(cfg.LLMModel) == "" {
			cfg.LLMModel = "qwen3:4b"
		}
	default:
		if strings.TrimSpace(cfg.LLMBaseURL) == "" {
			cfg.LLMBaseURL = "https://api.moonshot.cn/v1"
		}
		if strings.TrimSpace(cfg.LLMModel) == "" {
			cfg.LLMModel = "kimi-k2-0905-preview"
		}
	}
}

func applyRouterDefaults(cfg *Config) {
	if cfg.RouterRecentMessageCount <= 0 {
		cfg.RouterRecentMessageCount = 2
	}
	if cfg.RouterConfidenceCutoff <= 0 {
		cfg.RouterConfidenceCutoff = 0.72
	}
	if strings.TrimSpace(cfg.RouterPrimaryModel) == "" {
		cfg.RouterPrimaryModel = cfg.LLMModel
	}
	if strings.TrimSpace(cfg.RouterFallbackModel) == "" {
		switch cfg.LLMProvider {
		case "ollama":
			cfg.RouterFallbackModel = "gemma4:e4b"
		default:
			cfg.RouterFallbackModel = cfg.RouterPrimaryModel
		}
	}
}

func hasRealAPIKey(key string) bool {
	switch strings.TrimSpace(key) {
	case "", "sk-test", "sk-xxx", "your_openai_api_key_here", "your-openai-api-key", "your_llm_api_key_here", "your-llm-api-key":
		return false
	default:
		return true
	}
}

func (c Config) ValidateLLMConfig() error {
	switch c.LLMProvider {
	case "kimi":
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.LLMModel)), "kimi-") {
			return fmt.Errorf("LLM_PROVIDER=kimi requires LLM_MODEL to start with kimi-")
		}
	case "ollama":
		if strings.TrimSpace(c.LLMModel) == "" {
			return fmt.Errorf("LLM_PROVIDER=ollama requires LLM_MODEL to be set")
		}
		if strings.TrimSpace(c.RouterPrimaryModel) == "" {
			return fmt.Errorf("ROUTER_PRIMARY_MODEL cannot be empty when LLM_PROVIDER=ollama")
		}
		if strings.TrimSpace(c.RouterFallbackModel) == "" {
			return fmt.Errorf("ROUTER_FALLBACK_MODEL cannot be empty when LLM_PROVIDER=ollama")
		}
		if !isLocalLLMBaseURL(c.LLMBaseURL) {
			return fmt.Errorf("LLM_PROVIDER=ollama requires LLM_BASE_URL to point to a local Ollama endpoint")
		}
	default:
		return fmt.Errorf("unsupported LLM_PROVIDER=%q: only kimi and ollama are allowed", c.LLMProvider)
	}
	return nil
}

func isLocalLLMBaseURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}
