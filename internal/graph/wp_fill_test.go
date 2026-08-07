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
