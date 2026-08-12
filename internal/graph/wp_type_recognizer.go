package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const wpTypeRecognitionPrompt = `你是作业票类型识别器。根据作业描述，从候选作业票列表中选择需要开具的作业票类型。
只返回一个合法 JSON，不要返回 Markdown，不要解释或输出额外文本。
返回格式：{"wptypeids":[1,2]}

只能返回候选列表中出现的 wptypeid。作业许可证由系统强制加入，不需要你选择。`

type llmWpTypeRecognizer struct {
	chatModel   model.BaseChatModel
	promptStore PromptStore
}

type wpTypeRecognitionJSON struct {
	WpTypeIDs []int `json:"wptypeids"`
}

func NewLLMWpTypeRecognizer(chatModel model.BaseChatModel, args ...any) (WpTypeRecognizer, error) {
	if chatModel == nil {
		return nil, fmt.Errorf("chat model is nil")
	}

	promptStore := newDefaultPromptStore()
	if len(args) > 0 {
		store, ok := args[0].(PromptStore)
		if !ok {
			return nil, fmt.Errorf("prompt store has invalid type %T", args[0])
		}
		promptStore = store
	}
	return &llmWpTypeRecognizer{
		chatModel:   chatModel,
		promptStore: promptStore,
	}, nil
}

func (r *llmWpTypeRecognizer) Recognize(ctx context.Context, description string, templates []WpMetaInfo, candidates []WpMetaInfo) ([]int, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return nil, fmt.Errorf("work description is empty")
	}

	promptTemplates := templates
	if len(candidates) > 0 {
		promptTemplates = candidates
	}

	candidateJSON, err := json.Marshal(promptTemplates)
	if err != nil {
		return nil, fmt.Errorf("marshal work permit templates: %w", err)
	}

	prompt, err := r.promptStore.Get(ctx, PromptKeyWpTypeRecognition)
	if err != nil {
		return nil, fmt.Errorf("load prompt %q: %w", PromptKeyWpTypeRecognition, err)
	}

	userPrompt := fmt.Sprintf("作业描述：%s\n候选作业票列表：%s", description, candidateJSON)
	response, err := r.chatModel.Generate(ctx, []*schema.Message{
		schema.SystemMessage(prompt.Content),
		schema.UserMessage(userPrompt),
	})
	if err != nil {
		return nil, fmt.Errorf("generate type recognition response: %w", err)
	}
	if response == nil || strings.TrimSpace(response.Content) == "" {
		return nil, fmt.Errorf("type recognition response is empty")
	}

	var result wpTypeRecognitionJSON
	if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
		return nil, fmt.Errorf("decode type recognition response: %w", err)
	}
	return result.WpTypeIDs, nil
}
