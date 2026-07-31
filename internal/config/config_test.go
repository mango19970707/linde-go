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
  providers:
    anthropic:
      api_key: test-key
      base_url: https://claude.example.com
      model: claude-sonnet
      timeout: 45s
      max_tokens: 2048
      temperature: 0.3
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
	if provider.Timeout != 45*time.Second {
		t.Fatalf("Timeout = %s", provider.Timeout)
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
