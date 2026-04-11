package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigKimiProfileDefaults(t *testing.T) {
	resetLLMEnv(t)
	t.Setenv("LLM_PROVIDER", "kimi")
	t.Setenv("LLM_API_KEY", "sk-kimi-test")
	t.Setenv("LLM_AUTH_FILE", filepath.Join(t.TempDir(), "missing-auth.json"))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.LLMProvider != "kimi" {
		t.Fatalf("LLMProvider = %q, want kimi", cfg.LLMProvider)
	}
	if cfg.LLMBaseURL != "https://api.moonshot.cn/v1" {
		t.Fatalf("LLMBaseURL = %q", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "kimi-k2-0905-preview" {
		t.Fatalf("LLMModel = %q", cfg.LLMModel)
	}
	if cfg.RouterPrimaryModel != cfg.LLMModel {
		t.Fatalf("RouterPrimaryModel = %q, want %q", cfg.RouterPrimaryModel, cfg.LLMModel)
	}
	if cfg.RouterFallbackModel != cfg.RouterPrimaryModel {
		t.Fatalf("RouterFallbackModel = %q, want %q", cfg.RouterFallbackModel, cfg.RouterPrimaryModel)
	}
	if cfg.LLMAPIKey != "sk-kimi-test" {
		t.Fatalf("LLMAPIKey = %q", cfg.LLMAPIKey)
	}
	if !cfg.HasUsableLLMConfig() {
		t.Fatal("expected kimi profile to be usable with a real API key")
	}
}

func TestLoadConfigOllamaDefaultsIgnoreLegacyAuthFallback(t *testing.T) {
	resetLLMEnv(t)
	authFile := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(authFile, []byte(`{"OPENAI_API_KEY":"sk-remote-should-not-be-used"}`), 0o644); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OPENAI_AUTH_FILE", authFile)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.LLMProvider != "ollama" {
		t.Fatalf("LLMProvider = %q, want ollama", cfg.LLMProvider)
	}
	if cfg.LLMAuthFile != "" {
		t.Fatalf("LLMAuthFile = %q, want empty", cfg.LLMAuthFile)
	}
	if cfg.LLMBaseURL != "http://127.0.0.1:11434/v1" {
		t.Fatalf("LLMBaseURL = %q", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "qwen3:4b" {
		t.Fatalf("LLMModel = %q", cfg.LLMModel)
	}
	if cfg.RouterPrimaryModel != "qwen3:4b" {
		t.Fatalf("RouterPrimaryModel = %q", cfg.RouterPrimaryModel)
	}
	if cfg.RouterFallbackModel != "gemma4:e4b" {
		t.Fatalf("RouterFallbackModel = %q", cfg.RouterFallbackModel)
	}
	if cfg.RouterRecentMessageCount != 2 {
		t.Fatalf("RouterRecentMessageCount = %d", cfg.RouterRecentMessageCount)
	}
	if !cfg.RouterNoThink {
		t.Fatal("expected RouterNoThink to default to true")
	}
	if cfg.LLMAPIKey != "ollama-local" {
		t.Fatalf("LLMAPIKey = %q, want ollama-local", cfg.LLMAPIKey)
	}
	if !cfg.HasUsableLLMConfig() {
		t.Fatal("expected ollama profile to be usable without a remote API key")
	}
}

func TestLoadConfigNormalizesLegacyOpenAIRuntimeMode(t *testing.T) {
	resetLLMEnv(t)
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("AGENT_RUNTIME_MODE", "openai")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.AgentRuntimeMode != "llm" {
		t.Fatalf("AgentRuntimeMode = %q, want llm", cfg.AgentRuntimeMode)
	}
}

func TestLoadConfigSupportsLangGraphRuntimeMode(t *testing.T) {
	resetLLMEnv(t)
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("AGENT_RUNTIME_MODE", "langgraph")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.AgentRuntimeMode != "langgraph" {
		t.Fatalf("AgentRuntimeMode = %q, want langgraph", cfg.AgentRuntimeMode)
	}
	if !cfg.UseLangGraphRuntime() {
		t.Fatal("expected UseLangGraphRuntime() to return true")
	}
}

func TestLoadConfigInfersOllamaFromQwen3Model(t *testing.T) {
	resetLLMEnv(t)
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:11434/v1")
	t.Setenv("OPENAI_MODEL", "qwen3:4b")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.LLMProvider != "ollama" {
		t.Fatalf("LLMProvider = %q, want ollama", cfg.LLMProvider)
	}
	if cfg.LLMAPIKey != "ollama-local" {
		t.Fatalf("LLMAPIKey = %q, want ollama-local", cfg.LLMAPIKey)
	}
}

func TestValidateLLMConfigRejectsRemoteGPTModel(t *testing.T) {
	cfg := Config{
		LLMProvider: "kimi",
		LLMBaseURL:  "https://api.moonshot.cn/v1",
		LLMModel:    "gpt-5.4",
	}

	if err := cfg.ValidateLLMConfig(); err == nil {
		t.Fatal("expected non-kimi remote GPT model to be rejected")
	}
}

func resetLLMEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"LLM_PROVIDER",
		"LLM_BASE_URL",
		"LLM_API_KEY",
		"LLM_MODEL",
		"LLM_AUTH_FILE",
		"LLM_FALLBACK_MODEL",
		"OPENAI_BASE_URL",
		"OPENAI_API_KEY",
		"OPENAI_MODEL",
		"OPENAI_AUTH_FILE",
		"AGENT_RUNTIME_MODE",
		"ROUTER_PRIMARY_MODEL",
		"ROUTER_FALLBACK_MODEL",
		"ROUTER_NO_THINK",
		"ROUTER_RECENT_MESSAGE_COUNT",
		"ROUTER_CONFIDENCE_CUTOFF",
	} {
		t.Setenv(key, "")
	}
}
