package chatmodel

import (
	"context"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestNewChatModel(t *testing.T) {
	// 配置模型
	cfg := &ModelConfig{
		Provider:    "anthropic",
		APIKey:      "",
		BaseURL:     "", // 留空使用官方 API，或设置中转地址
		Model:       "claude-opus-4-6",
		MaxTokens:   1024,
		Temperature: 0.7,
	}

	ctx := context.Background()

	// 创建聊天模型
	chatModel, err := NewChatModel(ctx, cfg)
	if err != nil {
		panic(err)
	}

	// 准备消息
	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: "请用一句话介绍什么是人工智能？",
		},
	}

	// 演示一次性输出
	fmt.Println("【示例1：一次性输出】")
	if err := GenerateOnce(ctx, chatModel, messages); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	// 演示流式输出
	fmt.Println("\n【示例2：流式输出】")
	if err := GenerateStream(ctx, chatModel, messages); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
