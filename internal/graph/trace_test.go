package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestBuildWpGraphLogsWithSessionID(t *testing.T) {
	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(originalLogger)

	templateListRaw, err := json.Marshal(map[string]any{
		"success": true,
		"data": []WpMetaInfo{
			{Id: 243, TemplateName: "作业许可证", WpTypeName: "PTW", WpTypeId: 23},
		},
	})
	if err != nil {
		t.Fatalf("marshal template list: %v", err)
	}

	runner, err := BuildWpGraph(
		&recordingWpInfoExtractor{
			results: map[string]WpInfoExtraction{
				"input": {
					AreaName:    "A",
					StartTime:   "2026/08/03-08:00:00",
					EndTime:     "2026/08/03-17:00:00",
					Description: "maintenance",
				},
			},
		},
		stubWpTemplateTool{result: string(templateListRaw)},
		stubWpTypeRecognizer{},
		&recordingWpTemplateDetailTool{
			responses: map[int]string{
				243: `{"success":true,"data":{"id":243,"wptypeid":23,"wptypename":"PTW","templetname":"作业许可证","applycontent_object":{"sqxx":[{"name":"discription","type":"input","value":""}]}}}`,
			},
		},
		stubChatModel{},
		stubStructuredMemoryStore{},
		WpFillRules{},
		&recordingUpdateWpInfoTool{},
		&recordingLinkWpInfoTool{},
	)
	if err != nil {
		t.Fatalf("BuildWpGraph() error = %v", err)
	}

	_, _ = runner.Invoke(context.Background(), &WpGraphState{
		SessionID: "session-123",
		UserInput: "input",
		SiteID:    28,
		UserID:    464,
	})

	output := logs.String()
	if !strings.Contains(output, "trace=session-123") {
		t.Fatalf("logs do not contain session trace id: %s", output)
	}
}

func TestCompactTraceErrorKeepsWholeMessageOnOneLine(t *testing.T) {
	err := fmt.Errorf("first line\nsecond line\twith  spaces %s", strings.Repeat("x", 200))

	got := compactTraceError(err)

	if strings.Contains(got, "\n") {
		t.Fatalf("expected one line, got %q", got)
	}
	if strings.Contains(got, "\t") {
		t.Fatalf("expected tabs to be normalized, got %q", got)
	}
	want := "first line second line with spaces " + strings.Repeat("x", 200)
	if got != want {
		t.Fatalf("unexpected compacted error: %q", got)
	}
}
