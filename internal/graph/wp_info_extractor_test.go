package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type stubChatModel struct {
	response *schema.Message
	err      error
}

func (s stubChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.response, nil
}

func (s stubChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("not implemented")
}

func TestLLMWpInfoExtractorExtractParsesJSON(t *testing.T) {
	t.Parallel()

	extractor, err := newLLMWpInfoExtractor(
		stubChatModel{
			response: schema.AssistantMessage(`{"area_name":"A区","start_time":"2026-08-04 08:00","end_time":"2026-08-04 17:00","description":"动火作业","equipment_names":["3号焊机"],"company_name":"某某公司"}`, nil),
		},
		time.Date(2026, 8, 3, 9, 0, 0, 0, mustLoadLocation(t, "Asia/Shanghai")),
	)
	if err != nil {
		t.Fatalf("newLLMWpInfoExtractor() error = %v", err)
	}

	got, err := extractor.Extract(context.Background(), "明天在A区动火作业")
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if got.AreaName != "A区" {
		t.Fatalf("AreaName = %q", got.AreaName)
	}
	if got.StartTime != "2026/08/04-08:00:00" {
		t.Fatalf("StartTime = %q", got.StartTime)
	}
	if got.EndTime != "2026/08/04-17:00:00" {
		t.Fatalf("EndTime = %q", got.EndTime)
	}
	if got.Description != "动火作业" {
		t.Fatalf("Description = %q", got.Description)
	}
}

func TestLLMWpInfoExtractorExtractNormalizesRelativeTimes(t *testing.T) {
	t.Parallel()

	extractor, err := newLLMWpInfoExtractor(
		stubChatModel{
			response: schema.AssistantMessage(`{"area_name":"A区","start_time":"明天8点","end_time":"明天12点","description":"动火作业","equipment_names":[],"company_name":"某某公司"}`, nil),
		},
		time.Date(2026, 8, 3, 9, 0, 0, 0, mustLoadLocation(t, "Asia/Shanghai")),
	)
	if err != nil {
		t.Fatalf("newLLMWpInfoExtractor() error = %v", err)
	}

	got, err := extractor.Extract(context.Background(), "明天8-12点，在A区动火作业")
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if got.StartTime != "2026/08/04-08:00:00" {
		t.Fatalf("StartTime = %q", got.StartTime)
	}
	if got.EndTime != "2026/08/04-12:00:00" {
		t.Fatalf("EndTime = %q", got.EndTime)
	}
}

func TestLLMWpInfoExtractorExtractRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	extractor, err := NewLLMWpInfoExtractor(stubChatModel{
		response: schema.AssistantMessage("not json", nil),
	})
	if err != nil {
		t.Fatalf("NewLLMWpInfoExtractor() error = %v", err)
	}

	if _, err := extractor.Extract(context.Background(), "明天在A区动火作业"); err == nil {
		t.Fatalf("Extract() error = nil")
	}
}

func TestLLMWpTypeRecognizerRecognizeParsesTypeIDs(t *testing.T) {
	t.Parallel()

	recognizer, err := NewLLMWpTypeRecognizer(stubChatModel{
		response: schema.AssistantMessage(`{"wptypeids":[1,9]}`, nil),
	})
	if err != nil {
		t.Fatalf("NewLLMWpTypeRecognizer() error = %v", err)
	}

	got, err := recognizer.Recognize(context.Background(), "进行动火和临时用电作业", []WpMetaInfo{
		{WpTypeId: 1, WpTypeName: "动火作业"},
		{WpTypeId: 9, WpTypeName: "临时用电"},
	}, nil)
	if err != nil {
		t.Fatalf("Recognize() error = %v", err)
	}

	if len(got) != 2 || got[0] != 1 || got[1] != 9 {
		t.Fatalf("Recognize() = %#v", got)
	}
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()

	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("time.LoadLocation(%q): %v", name, err)
	}
	return location
}
