package chatmodel

import (
	"testing"
	"time"

	appconfig "linde-go-v2/internal/config"
)

func TestBuildModelConfigFromAppConfig(t *testing.T) {
	cfg := &appconfig.AppConfig{
		ChatModel: appconfig.ChatModelConfig{
			Provider: "anthropic",
			Providers: map[string]appconfig.ModelProviderConfig{
				"anthropic": {
					APIKey:      "test-key",
					BaseURL:     "https://claude.example.com",
					Model:       "claude-sonnet-4-5",
					Timeout:     30 * time.Second,
					MaxTokens:   2048,
					Temperature: 0.2,
				},
			},
		},
	}

	modelCfg, err := BuildModelConfigFromAppConfig(cfg, "")
	if err != nil {
		t.Fatalf("BuildModelConfigFromAppConfig() error = %v", err)
	}

	if modelCfg.Provider != "anthropic" {
		t.Fatalf("Provider = %s", modelCfg.Provider)
	}
	if modelCfg.BaseURL != "https://claude.example.com" {
		t.Fatalf("BaseURL = %s", modelCfg.BaseURL)
	}
	if modelCfg.APIKey != "test-key" {
		t.Fatalf("APIKey = %s", modelCfg.APIKey)
	}
	if modelCfg.Timeout != 30*time.Second {
		t.Fatalf("Timeout = %s", modelCfg.Timeout)
	}
}

func TestBuildModelConfigFromAppConfigUsesExplicitProvider(t *testing.T) {
	cfg := &appconfig.AppConfig{
		ChatModel: appconfig.ChatModelConfig{
			Provider: "anthropic",
			Providers: map[string]appconfig.ModelProviderConfig{
				"openai": {
					APIKey:      "openai-key",
					BaseURL:     "https://openai.example.com",
					Model:       "gpt-5.4",
					APIVersion:  "2026-01-01",
					MaxTokens:   1024,
					Temperature: 0.7,
				},
			},
		},
	}

	modelCfg, err := BuildModelConfigFromAppConfig(cfg, "openai")
	if err != nil {
		t.Fatalf("BuildModelConfigFromAppConfig() error = %v", err)
	}

	if modelCfg.Provider != "openai" {
		t.Fatalf("Provider = %s", modelCfg.Provider)
	}
	if modelCfg.APIVersion != "2026-01-01" {
		t.Fatalf("APIVersion = %s", modelCfg.APIVersion)
	}
}
