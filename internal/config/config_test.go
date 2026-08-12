package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `chat_model:
  provider: anthropic
  defaults:
    timeout: 30s
    max_tokens: 2048
    temperature: 0.7
    custom_header:
      X-Common: common
  providers:
    anthropic:
      api_key: test-key
      base_url: https://claude.example.com
      model: claude-sonnet
      temperature: 0.3
    openai:
      base_url: https://openai.example.com
      model: gpt-5.4
      timeout: 45s
      custom_header:
        X-Provider: provider
work_permit_api:
  base_url: http://127.0.0.1:8083
  site_id: 28
  user_id: 464
  timeout: 15s
  headers:
    X-Test: value
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ChatModel.Provider != "anthropic" {
		t.Fatalf("Provider = %s, want anthropic", cfg.ChatModel.Provider)
	}
	provider, err := cfg.GetChatProviderConfig("")
	if err != nil {
		t.Fatalf("GetChatProviderConfig() error = %v", err)
	}
	if provider.BaseURL != "https://claude.example.com" {
		t.Fatalf("BaseURL = %s", provider.BaseURL)
	}
	if provider.Timeout != 30*time.Second {
		t.Fatalf("Timeout = %s", provider.Timeout)
	}
	if provider.MaxTokens != 2048 {
		t.Fatalf("MaxTokens = %d", provider.MaxTokens)
	}
	if provider.Temperature != 0.3 {
		t.Fatalf("Temperature = %f", provider.Temperature)
	}
	if provider.CustomHeader["X-Common"] != "common" {
		t.Fatalf("Common header = %s", provider.CustomHeader["X-Common"])
	}

	overridden, err := cfg.GetChatProviderConfig("openai")
	if err != nil {
		t.Fatalf("GetChatProviderConfig(openai) error = %v", err)
	}
	if overridden.Timeout != 45*time.Second {
		t.Fatalf("Overridden Timeout = %s", overridden.Timeout)
	}
	if overridden.Temperature != 0.7 {
		t.Fatalf("Inherited Temperature = %f", overridden.Temperature)
	}
	if overridden.CustomHeader["X-Common"] != "common" || overridden.CustomHeader["X-Provider"] != "provider" {
		t.Fatalf("Merged headers = %#v", overridden.CustomHeader)
	}

	if cfg.WorkPermitAPI.BaseURL != "http://127.0.0.1:8083" {
		t.Fatalf("WorkPermitAPI.BaseURL = %s", cfg.WorkPermitAPI.BaseURL)
	}
	if cfg.WorkPermitAPI.SiteID != 28 {
		t.Fatalf("SiteID = %d", cfg.WorkPermitAPI.SiteID)
	}
	if cfg.WorkPermitAPI.Headers["X-Test"] != "value" {
		t.Fatalf("Header X-Test = %s", cfg.WorkPermitAPI.Headers["X-Test"])
	}
}

func TestGetChatProviderConfigErrorsWhenMissing(t *testing.T) {
	cfg := &AppConfig{}

	if _, err := cfg.GetChatProviderConfig("anthropic"); err == nil {
		t.Fatal("expected error for missing provider config")
	}
}

func TestLoadExpandsEnvironmentVariables(t *testing.T) {
	t.Setenv("TEST_OPENAI_API_KEY", "env-openai-key")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `chat_model:
  provider: openai
  providers:
    openai:
      api_key: ${TEST_OPENAI_API_KEY}
      base_url: https://openai.example.com
      model: gpt-5.4
work_permit_api:
  base_url: http://127.0.0.1:8083
  site_id: 28
  user_id: 464
  timeout: 15s
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	provider, err := cfg.GetChatProviderConfig("openai")
	if err != nil {
		t.Fatalf("GetChatProviderConfig(openai) error = %v", err)
	}
	if provider.APIKey != "env-openai-key" {
		t.Fatalf("APIKey = %q", provider.APIKey)
	}
}

func TestLoadResolvesStructuredMemorySQLiteDSNRelativeToConfigFile(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	path := filepath.Join(configDir, "config.yaml")

	content := `chat_model:
  provider: openai
  providers:
    openai:
      api_key: test-key
      base_url: https://openai.example.com
      model: gpt-5.4
work_permit_api:
  base_url: http://127.0.0.1:8083
  site_id: 28
  user_id: 464
  timeout: 15s
structured_memory_db:
  driver: sqlite
  dsn: data/structured_memory.db
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := filepath.Join(dir, "data", "structured_memory.db")
	if cfg.StructuredMemoryDB.DSN != want {
		t.Fatalf("StructuredMemoryDB.DSN = %q, want %q", cfg.StructuredMemoryDB.DSN, want)
	}
}

func TestWorkPermitAPIConfirmWpTypesDefaultsToEnabled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("work_permit_api: {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.WorkPermitAPI.ConfirmWpTypesEnabled() {
		t.Fatal("ConfirmWpTypesEnabled() = false, want true")
	}
}

func TestWorkPermitAPIConfirmWpTypesCanBeDisabled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("work_permit_api:\n  confirm_wp_types: false\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.WorkPermitAPI.ConfirmWpTypesEnabled() {
		t.Fatal("ConfirmWpTypesEnabled() = true, want false")
	}
}

func TestWorkPermitAPIIntentMemoryDefaults(t *testing.T) {
	t.Parallel()

	cfg := WorkPermitAPIConfig{}
	if got := cfg.IntentMemoryScoreThresholdOrDefault(); got != 10 {
		t.Fatalf("IntentMemoryScoreThresholdOrDefault() = %d, want 10", got)
	}
	if got := cfg.IntentMemoryTopNOrDefault(); got != 5 {
		t.Fatalf("IntentMemoryTopNOrDefault() = %d, want 5", got)
	}
}

func TestLoadWorkPermitAPIIntentMemoryConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `work_permit_api:
  intent_memory_score_threshold: 23
  intent_memory_top_n: 2
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.WorkPermitAPI.IntentMemoryScoreThresholdOrDefault(); got != 23 {
		t.Fatalf("threshold = %d, want 23", got)
	}
	if got := cfg.WorkPermitAPI.IntentMemoryTopNOrDefault(); got != 2 {
		t.Fatalf("top_n = %d, want 2", got)
	}
}
