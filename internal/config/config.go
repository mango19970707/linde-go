package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	ChatModel          ChatModelConfig          `yaml:"chat_model"`
	WorkPermitAPI      WorkPermitAPIConfig      `yaml:"work_permit_api"`
	StructuredMemoryDB StructuredMemoryDBConfig `yaml:"structured_memory_db"`
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
	BaseURL                    string            `yaml:"base_url"`
	SiteID                     int               `yaml:"site_id"`
	UserID                     int               `yaml:"user_id"`
	Timeout                    time.Duration     `yaml:"timeout"`
	Headers                    map[string]string `yaml:"headers"`
	ConfirmWpTypes             *bool             `yaml:"confirm_wp_types"`
	IntentMemoryScoreThreshold int               `yaml:"intent_memory_score_threshold"`
	IntentMemoryTopN           int               `yaml:"intent_memory_top_n"`
}

const (
	defaultIntentMemoryScoreThreshold = 10
	defaultIntentMemoryTopN           = 5
)

func (c WorkPermitAPIConfig) ConfirmWpTypesEnabled() bool {
	return c.ConfirmWpTypes == nil || *c.ConfirmWpTypes
}

func (c WorkPermitAPIConfig) IntentMemoryScoreThresholdOrDefault() int {
	if c.IntentMemoryScoreThreshold > 0 {
		return c.IntentMemoryScoreThreshold
	}
	return defaultIntentMemoryScoreThreshold
}

func (c WorkPermitAPIConfig) IntentMemoryTopNOrDefault() int {
	if c.IntentMemoryTopN > 0 {
		return c.IntentMemoryTopN
	}
	return defaultIntentMemoryTopN
}

type StructuredMemoryDBConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
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
	resolveRelativeStructuredMemoryDSN(path, &cfg)

	return &cfg, nil
}

func resolveRelativeStructuredMemoryDSN(configPath string, cfg *AppConfig) {
	if cfg == nil {
		return
	}

	driver := strings.ToLower(strings.TrimSpace(cfg.StructuredMemoryDB.Driver))
	dsn := strings.TrimSpace(cfg.StructuredMemoryDB.DSN)
	if dsn == "" {
		return
	}
	if driver == "pgx" || isAbsoluteOrNetworkDSN(dsn) {
		return
	}

	configDir := filepath.Dir(configPath)
	baseDir := configDir
	if strings.EqualFold(filepath.Base(configDir), "configs") {
		baseDir = filepath.Dir(configDir)
	}
	cfg.StructuredMemoryDB.DSN = filepath.Join(baseDir, dsn)
}

func isAbsoluteOrNetworkDSN(dsn string) bool {
	if filepath.IsAbs(dsn) {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(dsn))
	return strings.HasPrefix(lower, "file:") ||
		strings.HasPrefix(lower, "postgres://") ||
		strings.HasPrefix(lower, "postgresql://") ||
		strings.Contains(lower, "host=")
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
