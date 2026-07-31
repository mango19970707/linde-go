package chatmodel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	appconfig "linde-go-v2/internal/config"
)

type ModelConfig struct {
	Provider     string
	APIKey       string
	BaseURL      string
	Model        string
	Timeout      time.Duration
	CustomHeader map[string]string
	APIVersion   string
	MaxTokens    int
	Temperature  float32
}

func BuildModelConfigFromAppConfig(cfg *appconfig.AppConfig, provider string) (*ModelConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("app config is nil")
	}

	selected := provider
	if selected == "" {
		selected = cfg.ChatModel.Provider
	}

	providerCfg, err := cfg.GetChatProviderConfig(selected)
	if err != nil {
		return nil, err
	}

	customHeader := make(map[string]string, len(providerCfg.CustomHeader))
	for key, value := range providerCfg.CustomHeader {
		customHeader[key] = value
	}

	return &ModelConfig{
		Provider:     selected,
		APIKey:       providerCfg.APIKey,
		BaseURL:      providerCfg.BaseURL,
		Model:        providerCfg.Model,
		Timeout:      providerCfg.Timeout,
		CustomHeader: customHeader,
		APIVersion:   providerCfg.APIVersion,
		MaxTokens:    providerCfg.MaxTokens,
		Temperature:  providerCfg.Temperature,
	}, nil
}

func NewChatModelFromAppConfig(ctx context.Context, cfg *appconfig.AppConfig, provider string) (model.BaseChatModel, error) {
	modelCfg, err := BuildModelConfigFromAppConfig(cfg, provider)
	if err != nil {
		return nil, err
	}

	return NewChatModel(ctx, modelCfg)
}

func NewChatModel(ctx context.Context, cfg *ModelConfig) (model.BaseChatModel, error) {
	switch cfg.Provider {
	case "openai":
		return openai.NewChatModel(ctx, &openai.ChatModelConfig{
			APIKey:              cfg.APIKey,
			Timeout:             cfg.Timeout,
			Model:               cfg.Model,
			BaseURL:             cfg.BaseURL,
			APIVersion:          cfg.APIVersion,
			MaxCompletionTokens: &cfg.MaxTokens,
			Temperature:         &cfg.Temperature,
		})
	case "azure_openai":
		return openai.NewChatModel(ctx, &openai.ChatModelConfig{
			APIKey:              cfg.APIKey,
			Timeout:             cfg.Timeout,
			Model:               cfg.Model,
			BaseURL:             cfg.BaseURL,
			APIVersion:          cfg.APIVersion,
			MaxCompletionTokens: &cfg.MaxTokens,
			Temperature:         &cfg.Temperature,
		})
	case "ollama":
		return ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
			BaseURL: cfg.BaseURL, // e.g. http://localhost:11434
			Model:   cfg.Model,   // e.g. "llama3"
			Timeout: cfg.Timeout,
		})
	case "ark":
		return ark.NewChatModel(ctx, &ark.ChatModelConfig{
			APIKey:      cfg.APIKey,
			BaseURL:     cfg.BaseURL,
			Model:       cfg.Model,
			Timeout:     &cfg.Timeout,
			MaxTokens:   &cfg.MaxTokens,
			Temperature: &cfg.Temperature,
		})
	case "anthropic":
		config := &claude.Config{
			APIKey:      cfg.APIKey,
			Model:       cfg.Model,
			MaxTokens:   cfg.MaxTokens,
			Temperature: &cfg.Temperature,
		}
		if cfg.BaseURL != "" {
			config.BaseURL = &cfg.BaseURL
		}
		return claude.NewChatModel(ctx, config)
	default:
		return nil, fmt.Errorf("unsupported llm provider: %s", cfg.Provider)
	}
}

// GenerateOnce 演示一次性消息输出（阻塞直到完整响应返回）
func GenerateOnce(ctx context.Context, chatModel model.BaseChatModel, messages []*schema.Message, opts ...model.Option) error {
	// 调用 Generate 方法，阻塞直到模型返回完整响应
	response, err := chatModel.Generate(ctx, messages, opts...)
	if err != nil {
		return fmt.Errorf("generate failed: %w", err)
	}

	// 打印完整响应
	fmt.Println("=== 一次性输出 ===")
	fmt.Printf("Role: %s\n", response.Role)
	fmt.Printf("Content: %s\n", response.Content)
	if len(response.ToolCalls) > 0 {
		fmt.Printf("Tool Calls: %+v\n", response.ToolCalls)
	}
	fmt.Println()

	return nil
}

// GenerateStream 演示流式消息输出（增量返回消息块）
func GenerateStream(ctx context.Context, chatModel model.BaseChatModel, messages []*schema.Message, opts ...model.Option) error {
	// 调用 Stream 方法，返回一个 StreamReader
	reader, err := chatModel.Stream(ctx, messages, opts...)
	if err != nil {
		return fmt.Errorf("stream failed: %w", err)
	}
	defer reader.Close() // 确保关闭 reader

	fmt.Println("=== 流式输出 ===")

	// 循环读取流式响应块
	for {
		chunk, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			// 流结束
			fmt.Println("\n[流式输出完成]")
			break
		}
		if err != nil {
			return fmt.Errorf("recv chunk failed: %w", err)
		}

		// 打印每个响应块的内容
		fmt.Print(chunk.Content)
	}
	fmt.Println()

	return nil
}
