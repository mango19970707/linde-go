package graph

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type queueChatModel struct {
	responses []*schema.Message
	err       error
}

func (s *queueChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if s.err != nil {
		return nil, s.err
	}
	if len(s.responses) == 0 {
		return nil, nil
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response, nil
}

func (s *queueChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("not implemented")
}

func TestFillWpsNodeUsesLLMFallbackForRequiredSelectorFields(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(&queueChatModel{
		responses: []*schema.Message{
			schema.AssistantMessage(`{"fields":{"sqxx.0.value":[{"id":501,"name":"李文涛"}]}}`, nil),
		},
	}, stubPromptStore{
		prompts: map[string]PromptDefinition{
			PromptKeyWpFillFieldCompletion: {Key: PromptKeyWpFillFieldCompletion, Content: "fill prompt"},
		},
	}, stubStructuredMemoryStore{}, WpFillRules{})

	state := &WpGraphState{
		Area:        "空分装置",
		Description: "动火检修",
		WpTemplates: []WpTemplate{{
			ID:       243,
			WpTypeID: 23,
			Name:     "作业许可证",
			ApplyContentObject: map[string]any{
				"sqxx": []any{
					map[string]any{
						"name":     "qfr",
						"label":    "许可证签发人",
						"type":     "selector",
						"value":    []any{},
						"required": true,
					},
				},
			},
		}},
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newFillWpsNode() error = %v", err)
	}

	value := got.FilledWps[0].Content.(map[string]any)["sqxx"].([]any)[0].(map[string]any)["value"]
	items, ok := value.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("sqxx.0.value = %#v", value)
	}

	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("sqxx.0.value[0] type = %T", items[0])
	}
	if item["name"] != "李文涛" {
		t.Fatalf("sqxx.0.value[0] = %#v", item)
	}
	if len(got.FilledWps[0].UnfilledFields) != 0 {
		t.Fatalf("UnfilledFields = %#v", got.FilledWps[0].UnfilledFields)
	}
}

func TestFillWpsNodeTraversesGrfhFields(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(&queueChatModel{}, stubStructuredMemoryStore{
		entries: []StructuredMemoryEntry{
			{
				WpTypeID:        23,
				Area:            "空分装置",
				WorkDescription: "动火检修",
				FieldName:       "grfh.0.value",
				FieldValue:      []any{map[string]any{"id": 1, "name": "是"}, map[string]any{"id": 2, "name": "全棉工作服"}},
				Source:          "memory-sample",
				UpdatedAt:       100,
			},
		},
	}, WpFillRules{})

	state := &WpGraphState{
		Area:        "空分装置",
		Description: "动火检修",
		WpTemplates: []WpTemplate{{
			ID:       243,
			WpTypeID: 23,
			Name:     "作业许可证",
			ApplyContentObject: map[string]any{
				"grfh": []any{
					map[string]any{
						"name":     "sq",
						"label":    "身体",
						"type":     "checkbox",
						"value":    []any{},
						"required": false,
					},
				},
			},
		}},
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newFillWpsNode() error = %v", err)
	}

	content := got.FilledWps[0].Content.(map[string]any)
	grfh := content["grfh"].([]any)
	value := grfh[0].(map[string]any)["value"]
	items, ok := value.([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("grfh.0.value = %#v", value)
	}
	if got.FilledWps[0].FilledFields[0].Source != structuredMemorySourceExact {
		t.Fatalf("source = %q", got.FilledWps[0].FilledFields[0].Source)
	}
}

func TestFillWpsNodeUsesAreaGuessStructuredMemory(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(&queueChatModel{
		responses: []*schema.Message{
			schema.AssistantMessage(`{"area":"压缩机厂房","confidence":0.91,"reason":"作业描述更贴近压缩机厂房"}`, nil),
		},
	}, stubStructuredMemoryStore{
		entries: []StructuredMemoryEntry{
			{
				WpTypeID:        23,
				Area:            "空分装置",
				WorkDescription: "空分巡检",
				FieldName:       "sqxx.0.value",
				FieldValue:      "空分装置北侧平台",
				Source:          "sample-a",
				UpdatedAt:       10,
			},
			{
				WpTypeID:        23,
				Area:            "压缩机厂房",
				WorkDescription: "压缩机检修动火",
				FieldName:       "sqxx.0.value",
				FieldValue:      "压缩机厂房二层平台",
				Source:          "sample-b",
				UpdatedAt:       11,
			},
		},
	}, WpFillRules{})

	state := &WpGraphState{
		Area:        "压缩机区域",
		Description: "压缩机检修动火",
		WpTemplates: []WpTemplate{{
			ID:       243,
			WpTypeID: 23,
			Name:     "作业许可证",
			ApplyContentObject: map[string]any{
				"sqxx": []any{
					map[string]any{
						"name":     "wpplace",
						"label":    "作业地点",
						"type":     "input",
						"value":    "",
						"required": true,
					},
				},
			},
		}},
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newFillWpsNode() error = %v", err)
	}

	content := got.FilledWps[0].Content.(map[string]any)
	sqxx := content["sqxx"].([]any)
	if value := sqxx[0].(map[string]any)["value"]; value != "压缩机厂房二层平台" {
		t.Fatalf("sqxx.0.value = %#v", value)
	}
	if got.FilledWps[0].FilledFields[0].Source != structuredMemorySourceAreaGuess {
		t.Fatalf("source = %q", got.FilledWps[0].FilledFields[0].Source)
	}
}

func TestFillWpsNodeUsesMostSimilarDescriptionWithinExactArea(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(&queueChatModel{}, stubStructuredMemoryStore{
		entries: []StructuredMemoryEntry{
			{
				WpTypeID:        23,
				Area:            "空分装置",
				WorkDescription: "空分巡检",
				FieldName:       "sqxx.0.value",
				FieldValue:      "巡检地点",
				UpdatedAt:       20,
			},
			{
				WpTypeID:        23,
				Area:            "空分装置",
				WorkDescription: "空分装置动火检修",
				FieldName:       "sqxx.0.value",
				FieldValue:      "动火检修地点",
				UpdatedAt:       10,
			},
		},
	}, WpFillRules{})

	state := &WpGraphState{
		Area:        "空分装置",
		Description: "空分装置动火检修",
		WpTemplates: []WpTemplate{{
			ID:       243,
			WpTypeID: 23,
			ApplyContentObject: map[string]any{
				"sqxx": []any{
					map[string]any{
						"name":  "wpplace",
						"label": "作业地点",
						"type":  "input",
						"value": "",
					},
				},
			},
		}},
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newFillWpsNode() error = %v", err)
	}

	value := got.FilledWps[0].Content.(map[string]any)["sqxx"].([]any)[0].(map[string]any)["value"]
	if value != "动火检修地点" {
		t.Fatalf("sqxx.0.value = %#v", value)
	}
}

func TestFillWpsNodeDoesNotUseDescriptionMismatchedMemory(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(&queueChatModel{}, stubStructuredMemoryStore{
		entries: []StructuredMemoryEntry{
			{
				WpTypeID:        23,
				Area:            "空分装置",
				WorkDescription: "空分巡检",
				FieldName:       "sqxx.0.value",
				FieldValue:      "巡检地点",
				UpdatedAt:       20,
			},
		},
	}, WpFillRules{})

	state := &WpGraphState{
		Area:        "空分装置",
		Description: "压缩机大修",
		WpTemplates: []WpTemplate{{
			ID:       243,
			WpTypeID: 23,
			ApplyContentObject: map[string]any{
				"sqxx": []any{
					map[string]any{
						"name":     "wpplace",
						"label":    "作业地点",
						"type":     "input",
						"value":    "",
						"required": true,
					},
				},
			},
		}},
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newFillWpsNode() error = %v", err)
	}

	value := got.FilledWps[0].Content.(map[string]any)["sqxx"].([]any)[0].(map[string]any)["value"]
	if value != "" {
		t.Fatalf("sqxx.0.value = %#v, want blank", value)
	}
	if len(got.FilledWps[0].UnfilledFields) != 1 {
		t.Fatalf("UnfilledFields = %#v", got.FilledWps[0].UnfilledFields)
	}
}

func TestFillWpsNodeUsesMostSimilarDescriptionForRequiredFallback(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(&queueChatModel{}, stubStructuredMemoryStore{
		entries: []StructuredMemoryEntry{
			{
				WpTypeID:        23,
				Area:            "空分装置",
				WorkDescription: "空分巡检",
				FieldName:       "sqxx.0.value",
				FieldValue:      "巡检联系人",
				UpdatedAt:       20,
			},
			{
				WpTypeID:        23,
				Area:            "压缩机区域",
				WorkDescription: "压缩机大修",
				FieldName:       "sqxx.0.value",
				FieldValue:      "大修联系人",
				UpdatedAt:       10,
			},
		},
	}, WpFillRules{})

	state := &WpGraphState{
		Area:        "未知区域",
		Description: "压缩机大修",
		WpTemplates: []WpTemplate{{
			ID:       243,
			WpTypeID: 23,
			ApplyContentObject: map[string]any{
				"sqxx": []any{
					map[string]any{
						"name":     "contact",
						"label":    "联系人",
						"type":     "input",
						"value":    "",
						"required": true,
					},
				},
			},
		}},
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newFillWpsNode() error = %v", err)
	}

	value := got.FilledWps[0].Content.(map[string]any)["sqxx"].([]any)[0].(map[string]any)["value"]
	if value != "大修联系人" {
		t.Fatalf("sqxx.0.value = %#v", value)
	}
}

func TestFillWpsNodeIgnoresLLMFillTimeout(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(&queueChatModel{
		err: context.DeadlineExceeded,
	}, stubStructuredMemoryStore{}, WpFillRules{})

	state := &WpGraphState{
		Area:        "压缩机厂房",
		Description: "压缩机检修动火",
		WpTemplates: []WpTemplate{{
			ID:       245,
			WpTypeID: 23,
			Name:     "作业许可证",
			ApplyContentObject: map[string]any{
				"sqxx": []any{
					map[string]any{
						"name":     "wpplace",
						"label":    "作业地点",
						"type":     "input",
						"value":    "",
						"required": true,
					},
				},
			},
		}},
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newFillWpsNode() error = %v", err)
	}

	if len(got.FilledWps) != 1 {
		t.Fatalf("len(FilledWps) = %d", len(got.FilledWps))
	}
	if len(got.FilledWps[0].FilledFields) != 0 {
		t.Fatalf("FilledFields = %#v", got.FilledWps[0].FilledFields)
	}
	if len(got.FilledWps[0].UnfilledFields) != 1 {
		t.Fatalf("UnfilledFields = %#v", got.FilledWps[0].UnfilledFields)
	}
	if got.FilledWps[0].UnfilledFields[0].Label != "作业地点" {
		t.Fatalf("UnfilledFields[0] = %#v", got.FilledWps[0].UnfilledFields[0])
	}
}

func TestFillWpsNodeIgnoresAreaSelectionTimeout(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(&queueChatModel{
		err: context.DeadlineExceeded,
	}, stubStructuredMemoryStore{
		entries: []StructuredMemoryEntry{
			{
				WpTypeID:        23,
				Area:            "空分装置",
				WorkDescription: "空分巡检",
				FieldName:       "sqxx.0.value",
				FieldValue:      "空分装置北侧平台",
				Source:          "sample-a",
				UpdatedAt:       10,
			},
		},
	}, WpFillRules{})

	state := &WpGraphState{
		Area:        "压缩机区域",
		Description: "压缩机检修动火",
		WpTemplates: []WpTemplate{{
			ID:       243,
			WpTypeID: 23,
			Name:     "作业许可证",
			ApplyContentObject: map[string]any{
				"sqxx": []any{
					map[string]any{
						"name":     "wpplace",
						"label":    "作业地点",
						"type":     "input",
						"value":    "",
						"required": true,
					},
				},
			},
		}},
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newFillWpsNode() error = %v", err)
	}

	if len(got.FilledWps) != 1 {
		t.Fatalf("len(FilledWps) = %d", len(got.FilledWps))
	}
	if len(got.FilledWps[0].FilledFields) != 0 {
		t.Fatalf("FilledFields = %#v", got.FilledWps[0].FilledFields)
	}
	if len(got.FilledWps[0].UnfilledFields) != 1 {
		t.Fatalf("UnfilledFields = %#v", got.FilledWps[0].UnfilledFields)
	}
}

func TestFillWpsNodeIgnoresMalformedLLMFillResponse(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(&queueChatModel{
		responses: []*schema.Message{
			schema.AssistantMessage(`{"fields":`, nil),
		},
	}, stubStructuredMemoryStore{}, WpFillRules{})

	state := &WpGraphState{
		Area:        "压缩机厂房",
		Description: "压缩机检修动火",
		WpTemplates: []WpTemplate{{
			ID:       245,
			WpTypeID: 23,
			Name:     "作业许可证",
			ApplyContentObject: map[string]any{
				"sqxx": []any{
					map[string]any{
						"name":     "wpplace",
						"label":    "作业地点",
						"type":     "input",
						"value":    "",
						"required": true,
					},
				},
			},
		}},
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newFillWpsNode() error = %v", err)
	}

	if len(got.FilledWps) != 1 {
		t.Fatalf("len(FilledWps) = %d", len(got.FilledWps))
	}
	if len(got.FilledWps[0].FilledFields) != 0 {
		t.Fatalf("FilledFields = %#v", got.FilledWps[0].FilledFields)
	}
	if len(got.FilledWps[0].UnfilledFields) != 1 {
		t.Fatalf("UnfilledFields = %#v", got.FilledWps[0].UnfilledFields)
	}
}
