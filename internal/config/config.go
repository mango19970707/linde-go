package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	ChatModel     ChatModelConfig     `yaml:"chat_model"`
	WorkPermitAPI WorkPermitAPIConfig `yaml:"work_permit_api"`
}

type ChatModelConfig struct {
	Provider  string                         `yaml:"provider"`
	Defaults  ModelProviderConfig            `yaml:"defaults"`
	Providers map[string]ModelProviderConfig `yaml:"providers"`
}

type ModelProviderConfig struct {
	APIKey       string            `yaml:"api_key"`
	BaseURL      string            `yaml:"base_url"`
	Model        string            `yaml:"model"`
	Timeout      time.Duration     `yaml:"timeout"`
	CustomHeader map[string]string `yaml:"custom_header"`
	APIVersion   string            `yaml:"api_version"`
	MaxTokens    int               `yaml:"max_tokens"`
	Temperature  float32           `yaml:"temperature"`
}

type WorkPermitAPIConfig struct {
	BaseURL string            `yaml:"base_url"`
	SiteID  int               `yaml:"site_id"`
	UserID  int               `yaml:"user_id"`
	Timeout time.Duration     `yaml:"timeout"`
	Headers map[string]string `yaml:"headers"`
}

func Load(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(data))), &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config file %s: %w", path, err)
	}

	return &cfg, nil
}

func (c *AppConfig) GetChatProviderConfig(provider string) (ModelProviderConfig, error) {
	if c == nil {
		return ModelProviderConfig{}, fmt.Errorf("app config is nil")
	}

	selected := strings.TrimSpace(provider)
	if selected == "" {
		selected = strings.TrimSpace(c.ChatModel.Provider)
	}
	if selected == "" {
		return ModelProviderConfig{}, fmt.Errorf("chat_model.provider is required")
	}

	providerCfg, ok := c.ChatModel.Providers[selected]
	if !ok {
		return ModelProviderConfig{}, fmt.Errorf("chat model provider %q not found", selected)
	}

	return mergeProviderConfig(c.ChatModel.Defaults, providerCfg), nil
}

func mergeProviderConfig(defaults, provider ModelProviderConfig) ModelProviderConfig {
	merged := defaults
	if provider.APIKey != "" {
		merged.APIKey = provider.APIKey
	}
	if provider.BaseURL != "" {
		merged.BaseURL = provider.BaseURL
	}
	if provider.Model != "" {
		merged.Model = provider.Model
	}
	if provider.Timeout != 0 {
		merged.Timeout = provider.Timeout
	}
	if provider.APIVersion != "" {
		merged.APIVersion = provider.APIVersion
	}
	if provider.MaxTokens != 0 {
		merged.MaxTokens = provider.MaxTokens
	}
	if provider.Temperature != 0 {
		merged.Temperature = provider.Temperature
	}

	merged.CustomHeader = make(map[string]string, len(defaults.CustomHeader)+len(provider.CustomHeader))
	for key, value := range defaults.CustomHeader {
		merged.CustomHeader[key] = value
	}
	for key, value := range provider.CustomHeader {
		merged.CustomHeader[key] = value
	}
	if len(merged.CustomHeader) == 0 {
		merged.CustomHeader = nil
	}

	return merged
}
