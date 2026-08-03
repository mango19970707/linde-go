package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const wpInfoExtractionPrompt = `你是作业票信息提取器。请从用户输入中提取作业票基础信息，只返回一个合法 JSON 对象，不要返回 Markdown、解释或额外文本。

JSON 字段必须是：
{
  "area_name": "作业区域名称，无法确定时为空字符串",
  "start_time": "开始时间，无法确定时为空字符串",
  "end_time": "结束时间，无法确定时为空字符串",
  "description": "作业描述，无法确定时为空字符串",
  "equipment_names": ["设备名称"],
  "company_name": "作业单位名称，无法确定时为空字符串"
}

不要猜测区域、设备或公司 ID。`

type wpInfoExtractionJSON struct {
	AreaName       string   `json:"area_name"`
	StartTime      string   `json:"start_time"`
	EndTime        string   `json:"end_time"`
	Description    string   `json:"description"`
	EquipmentNames []string `json:"equipment_names"`
	CompanyName    string   `json:"company_name"`
}

type llmWpInfoExtractor struct {
	chatModel model.BaseChatModel
}

func NewLLMWpInfoExtractor(chatModel model.BaseChatModel) (WpInfoExtractor, error) {
	if chatModel == nil {
		return nil, fmt.Errorf("chat model is nil")
	}

	return &llmWpInfoExtractor{chatModel: chatModel}, nil
}

func (e *llmWpInfoExtractor) Extract(ctx context.Context, input string) (WpInfoExtraction, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return WpInfoExtraction{}, fmt.Errorf("user input is empty")
	}

	response, err := e.chatModel.Generate(ctx, []*schema.Message{
		schema.SystemMessage(wpInfoExtractionPrompt),
		schema.UserMessage(input),
	})
	if err != nil {
		return WpInfoExtraction{}, fmt.Errorf("generate extraction response: %w", err)
	}
	if response == nil {
		return WpInfoExtraction{}, fmt.Errorf("extraction response is nil")
	}
	if strings.TrimSpace(response.Content) == "" {
		return WpInfoExtraction{}, fmt.Errorf("extraction response is empty")
	}

	var payload wpInfoExtractionJSON
	if err := json.Unmarshal([]byte(response.Content), &payload); err != nil {
		return WpInfoExtraction{}, fmt.Errorf("decode extraction response: %w", err)
	}

	return WpInfoExtraction{
		AreaName:       strings.TrimSpace(payload.AreaName),
		StartTime:      strings.TrimSpace(payload.StartTime),
		EndTime:        strings.TrimSpace(payload.EndTime),
		Description:    strings.TrimSpace(payload.Description),
		EquipmentNames: payload.EquipmentNames,
		CompanyName:    strings.TrimSpace(payload.CompanyName),
	}, nil
}
