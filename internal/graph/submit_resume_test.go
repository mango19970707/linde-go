package graph

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/compose"
)

func TestBuildWpGraphResumesApprovalAfterSubmitInterruptWithMarshaledFixtures(t *testing.T) {
	t.Parallel()

	initialInput := "initial"
	supplementInput := "supplement"
	permitName := "作业许可证"
	permitTypeName := "作业许可证"

	templateListRawBytes, err := json.Marshal(map[string]any{
		"success": true,
		"data": []WpMetaInfo{
			{Id: 243, TemplateName: permitName, WpTypeName: permitTypeName, WpTypeId: 23},
		},
	})
	if err != nil {
		t.Fatalf("marshal template list: %v", err)
	}

	templateDetailRawBytes, err := json.Marshal(map[string]any{
		"success": true,
		"data": map[string]any{
			"id":           243,
			"wptypeid":     23,
			"wptypename":   permitTypeName,
			"templetname":  permitName,
			"applycontent": `{"sqxx":[{"name":"discription","type":"input","value":""}]}`,
			"applycontent_object": map[string]any{
				"id":          0,
				"wptempletid": 243,
				"wptypeid":    23,
				"sqxx": []any{
					map[string]any{"name": "discription", "type": "input", "value": ""},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal template detail: %v", err)
	}

	extractor := &recordingWpInfoExtractor{
		results: map[string]WpInfoExtraction{
			initialInput: {
				AreaName:    "A",
				Description: "maintenance",
			},
			supplementInput: {
				StartTime: "2026/08/04-08:00:00",
				EndTime:   "2026/08/04-17:00:00",
			},
		},
	}
	detailTool := &recordingWpTemplateDetailTool{
		responses: map[int]string{243: string(templateDetailRawBytes)},
	}
	updateTool := &recordingUpdateWpInfoTool{}
	linkTool := &recordingLinkWpInfoTool{}
	store := newInMemoryCheckPointStore()

	graph, err := BuildWpGraph(
		extractor,
		stubWpTemplateTool{result: string(templateListRawBytes)},
		stubWpTypeRecognizer{},
		detailTool,
		stubStructuredMemoryStore{},
		WpFillRules{},
		updateTool,
		linkTool,
		compose.WithCheckPointStore(store),
		compose.WithGraphName("work-permit"),
	)
	if err != nil {
		t.Fatalf("BuildWpGraph() error = %v", err)
	}

	checkPointID := "cp-submit-approval"
	_, err = graph.Invoke(context.Background(), &WpGraphState{
		SiteID:    28,
		UserID:    464,
		UserInput: initialInput,
	}, compose.WithCheckPointID(checkPointID))
	if err == nil {
		t.Fatal("first graph.Invoke() error = nil")
	}

	firstInterrupt, ok := compose.ExtractInterruptInfo(err)
	if !ok || len(firstInterrupt.InterruptContexts) != 1 {
		t.Fatalf("first error is not single interrupt: %v", err)
	}

	resumeInfoCtx := compose.ResumeWithData(context.Background(), firstInterrupt.InterruptContexts[0].ID, WpInfoSupplement{
		Input: supplementInput,
	})
	_, err = graph.Invoke(resumeInfoCtx, &WpGraphState{}, compose.WithCheckPointID(checkPointID))
	if err == nil {
		t.Fatal("second graph.Invoke() error = nil")
	}

	secondInterrupt, ok := compose.ExtractInterruptInfo(err)
	if !ok || len(secondInterrupt.InterruptContexts) != 1 {
		t.Fatalf("second error is not single interrupt: %v", err)
	}

	payload := secondInterrupt.InterruptContexts[0].Info.(map[string]any)
	if payload["operation"] != "submit_work_permits" {
		t.Fatalf("second operation = %#v", payload["operation"])
	}

	filledWps, ok := payload["filled_wps"].([]FilledWp)
	if !ok || len(filledWps) != 1 {
		t.Fatalf("filled_wps = %#v", payload["filled_wps"])
	}

	approvedPayload, ok := filledWps[0].Content.(map[string]any)
	if !ok {
		t.Fatalf("approved payload type = %T", filledWps[0].Content)
	}

	resumeApprovalCtx := compose.ResumeWithData(context.Background(), secondInterrupt.InterruptContexts[0].ID, SubmitWpsApproval{
		Approved: true,
		Payloads: []map[string]any{approvedPayload},
	})
	output, err := graph.Invoke(resumeApprovalCtx, &WpGraphState{}, compose.WithCheckPointID(checkPointID))
	if err != nil {
		t.Fatalf("third graph.Invoke() error = %v", err)
	}
	if output == nil {
		t.Fatal("third graph.Invoke() output = nil")
	}
	if output.CurrentNode != nodeSubmitWps {
		t.Fatalf("CurrentNode = %q", output.CurrentNode)
	}
	if len(updateTool.payloads) != 1 {
		t.Fatalf("submitted payloads = %#v", updateTool.payloads)
	}
	if len(linkTool.links) != 0 {
		t.Fatalf("links = %#v", linkTool.links)
	}
}
