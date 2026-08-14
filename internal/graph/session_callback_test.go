package graph

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"linde-go-v2/internal/sessionrecorder"
)

func TestBuildWpGraphWritesLLMAndToolTranscriptEvents(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	recorder, err := sessionrecorder.NewRecorder(dir)
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	chatModel := WrapChatModelWithSessionRecorder(&queueChatModel{
		responses: []*schema.Message{
			schema.AssistantMessage(`{"area_name":"A","start_time":"2026/08/14-08:00:00","end_time":"2026/08/14-17:00:00","description":"maintenance","equipment_names":[],"company_name":""}`, nil),
			schema.AssistantMessage(`{"wptypeids":[23]}`, nil),
		},
	}, recorder)
	extractor, err := NewLLMWpInfoExtractor(chatModel)
	if err != nil {
		t.Fatalf("NewLLMWpInfoExtractor() error = %v", err)
	}
	recognizer, err := NewLLMWpTypeRecognizer(chatModel)
	if err != nil {
		t.Fatalf("NewLLMWpTypeRecognizer() error = %v", err)
	}

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
		extractor,
		WrapToolWithSessionRecorder(stubWpTemplateTool{result: string(templateListRaw)}, recorder),
		recognizer,
		WrapToolWithSessionRecorder(&recordingWpTemplateDetailTool{
			responses: map[int]string{
				243: `{"success":true,"data":{"id":243,"wptypeid":23,"wptypename":"PTW","templetname":"作业许可证","applycontent_object":{"id":0,"wptempletid":243,"wptypeid":23,"sqxx":[{"name":"discription","label":"desc","type":"input","value":""}]}}}`,
			},
		}, recorder),
		chatModel,
		stubPromptStore{
			prompts: map[string]PromptDefinition{
				PromptKeyWpFillAreaSelection:   {Key: PromptKeyWpFillAreaSelection, Content: "area prompt"},
				PromptKeyWpFillFieldCompletion: {Key: PromptKeyWpFillFieldCompletion, Content: "fill prompt"},
			},
		},
		stubStructuredMemoryStore{},
		WpFillRules{},
		WrapToolWithSessionRecorder(&recordingUpdateWpInfoTool{}, recorder),
		WrapToolWithSessionRecorder(&recordingLinkWpInfoTool{}, recorder),
		false,
		WithSessionRecorder(recorder),
	)
	if err != nil {
		t.Fatalf("BuildWpGraph() error = %v", err)
	}

	ctx := sessionrecorder.WithSessionID(context.Background(), "session-graph")
	_, err = runner.Invoke(ctx, &WpGraphState{
		SessionID: "session-graph",
		UserInput: "input",
		SiteID:    28,
		UserID:    464,
	})
	if err != nil {
		if _, ok := compose.ExtractInterruptInfo(err); !ok {
			t.Fatalf("runner.Invoke() error = %v", err)
		}
	}

	transcript := loadGraphTranscript(t, filepath.Join(dir, "session-graph.json"))
	assertGraphEventRole(t, transcript.Transcript, "system_message")
	assertGraphEventRole(t, transcript.Transcript, "user_message")
	assertGraphEventRole(t, transcript.Transcript, "assistant_message")
	assertGraphEventRole(t, transcript.Transcript, "tool_call")
	assertGraphEventRole(t, transcript.Transcript, "tool_output")
}

type persistedGraphTranscript struct {
	SessionID  string                `json:"session_id"`
	Status     string                `json:"status"`
	Transcript []persistedGraphEvent `json:"transcript"`
}

type persistedGraphEvent struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func loadGraphTranscript(t *testing.T, path string) persistedGraphTranscript {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	var transcript persistedGraphTranscript
	if err := json.Unmarshal(data, &transcript); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", path, err)
	}
	return transcript
}

func assertGraphEventRole(t *testing.T, events []persistedGraphEvent, want string) {
	t.Helper()

	for _, event := range events {
		if event.Role == want {
			return
		}
	}
	t.Fatalf("role %q not found in events %#v", want, events)
}
