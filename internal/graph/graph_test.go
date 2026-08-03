package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type stubWpInfoExtractor struct {
	result WpInfoExtraction
	err    error
}

func (s stubWpInfoExtractor) Extract(ctx context.Context, input string) (WpInfoExtraction, error) {
	return s.result, s.err
}

func TestAskWpInfoNodeMapsExtractorResultToState(t *testing.T) {
	t.Parallel()

	node := newAskWpInfoNode(stubWpInfoExtractor{
		result: WpInfoExtraction{
			AreaName:    "A区",
			StartTime:   "2026-08-04 08:00",
			EndTime:     "2026-08-04 17:00",
			Description: "动火作业",
			EquipmentNames: []string{
				"3号焊机",
				"4号焊机",
			},
			CompanyName: "某某公司",
		},
	})

	state := &WpGraphState{
		UserInput: "明天在A区动火作业，使用3号焊机和4号焊机，某某公司施工。",
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newAskWpInfoNode() error = %v", err)
	}

	if got.CurrentNode != nodeAskWpInfo {
		t.Fatalf("CurrentNode = %q", got.CurrentNode)
	}
	if got.Area.Name != "A区" {
		t.Fatalf("Area.Name = %q", got.Area.Name)
	}
	if got.StartTime != "2026-08-04 08:00" {
		t.Fatalf("StartTime = %q", got.StartTime)
	}
	if got.EndTime != "2026-08-04 17:00" {
		t.Fatalf("EndTime = %q", got.EndTime)
	}
	if got.Description != "动火作业" {
		t.Fatalf("Description = %q", got.Description)
	}
	if len(got.Equipments) != 2 {
		t.Fatalf("len(Equipments) = %d", len(got.Equipments))
	}
	if got.Equipments[0].Name != "3号焊机" || got.Equipments[1].Name != "4号焊机" {
		t.Fatalf("Equipments = %#v", got.Equipments)
	}
	if got.WpCompany.Name != "某某公司" {
		t.Fatalf("WpCompany.Name = %q", got.WpCompany.Name)
	}
}

func TestValidateWpInfoNodeRejectsMissingFields(t *testing.T) {
	t.Parallel()

	state := &WpGraphState{
		Area:        AreaInfo{Name: "A区"},
		StartTime:   "2026-08-04 08:00",
		Description: "动火作业",
	}

	got, err := validateWpInfoNode(context.Background(), state)
	if err != nil {
		t.Fatalf("validateWpInfoNode() error = %v", err)
	}

	if got.CurrentNode != nodeValidateWpInfo {
		t.Fatalf("CurrentNode = %q", got.CurrentNode)
	}
	if got.ValidationStatus != validationInvalid {
		t.Fatalf("ValidationStatus = %q", got.ValidationStatus)
	}
	if got.ErrorMsg == "" {
		t.Fatalf("ErrorMsg is empty")
	}
}

func TestBuildWpGraphRequiresExtractor(t *testing.T) {
	t.Parallel()

	if _, err := BuildWpGraph(nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatalf("BuildWpGraph(nil) error = nil")
	}
}

type stubWpTemplateTool struct {
	result string
	err    error
}

func (s stubWpTemplateTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return nil, nil
}

func (s stubWpTemplateTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error) {
	return s.result, s.err
}

type recordingWpTemplateDetailTool struct {
	responses map[int]string
	ids       []int
}

func (s *recordingWpTemplateDetailTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return nil, nil
}

func (s *recordingWpTemplateDetailTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error) {
	var input struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", err
	}
	s.ids = append(s.ids, input.ID)
	return s.responses[input.ID], nil
}

type stubWpTypeRecognizer struct {
	ids []int
	err error
}

func (s stubWpTypeRecognizer) Recognize(ctx context.Context, description string, templates []WpMetaInfo) ([]int, error) {
	return s.ids, s.err
}

func TestIntentRecognitionNodeSelectsTypesAndAlwaysIncludesPermit(t *testing.T) {
	t.Parallel()

	templates := []WpMetaInfo{
		{Id: 243, TemplateName: "作业许可证", WpTypeName: "PTW800作业票", WpTypeId: 23},
		{Id: 171, TemplateName: "特级动火", WpTypeName: "特级动火", WpTypeId: 1},
		{Id: 172, TemplateName: "一级动火", WpTypeName: "一级动火", WpTypeId: 1},
		{Id: 180, TemplateName: "临时用电", WpTypeName: "临时用电", WpTypeId: 9},
	}
	raw, err := json.Marshal(map[string]any{
		"success": true,
		"data":    templates,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	state := &WpGraphState{
		Description: "在装置区进行特级动火作业",
	}
	node := newIntentRecognitionNode(
		stubWpTemplateTool{result: string(raw)},
		stubWpTypeRecognizer{ids: []int{1}},
	)

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newIntentRecognitionNode() error = %v", err)
	}

	if got.CurrentNode != nodeIntentRecognition {
		t.Fatalf("CurrentNode = %q", got.CurrentNode)
	}
	if len(got.IntentWpTypes) != 2 {
		t.Fatalf("len(IntentWpTypes) = %d, want 2", len(got.IntentWpTypes))
	}
	if got.IntentWpTypes[0].WpTypeId != 23 || got.IntentWpTypes[1].WpTypeId != 1 {
		t.Fatalf("IntentWpTypes = %#v", got.IntentWpTypes)
	}
}

func TestIntentRecognitionNodeRejectsUnknownTypeID(t *testing.T) {
	t.Parallel()

	raw := `{"success":true,"data":[{"id":243,"templetname":"作业许可证","wptypename":"PTW800作业票","wptypeid":23}]}`
	node := newIntentRecognitionNode(
		stubWpTemplateTool{result: raw},
		stubWpTypeRecognizer{ids: []int{999}},
	)

	_, err := node(context.Background(), &WpGraphState{Description: "一般作业"})
	if err == nil {
		t.Fatalf("newIntentRecognitionNode() error = nil")
	}
}

func TestSearchWpTemplateNodeLoadsTemplatesByTemplateID(t *testing.T) {
	t.Parallel()

	tool := &recordingWpTemplateDetailTool{
		responses: map[int]string{
			243: `{"success":true,"data":{"id":243,"wptypeid":23,"wptypename":"PTW800作业票","templetname":"作业许可证","isdefault":false,"idprefix":"800","updatedby":"admin","updatetime":1681351492000,"version":0,"state":0,"siteids":"28","handwrittensignature":0,"applycontent":"{\"name\":\"permit-raw\"}","applycontent_object":{"name":"permit"}}}`,
			171: `{"success":true,"data":{"id":171,"wptypeid":1,"wptypename":"特级动火","templetname":"特级动火","isdefault":true,"idprefix":"100","updatedby":"ops","updatetime":1681351492001,"version":2,"state":1,"siteids":"28,29","handwrittensignature":1,"applycontent":"{\"name\":\"fire-raw\"}","applycontent_object":{"name":"fire"}}}`,
		},
	}
	node := newSearchWpTemplateNode(tool)
	state := &WpGraphState{
		IntentWpTypes: []WpMetaInfo{
			{Id: 243, TemplateName: "作业许可证"},
			{Id: 171, TemplateName: "特级动火"},
		},
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newSearchWpTemplateNode() error = %v", err)
	}

	if got.CurrentNode != nodeSearchWpTemplate {
		t.Fatalf("CurrentNode = %q", got.CurrentNode)
	}
	if len(tool.ids) != 2 || tool.ids[0] != 243 || tool.ids[1] != 171 {
		t.Fatalf("queried template IDs = %#v", tool.ids)
	}
	if len(got.WpTemplates) != 2 {
		t.Fatalf("len(WpTemplates) = %d", len(got.WpTemplates))
	}
	if got.WpTemplates[0].ID != 243 || got.WpTemplates[0].Name != "作业许可证" {
		t.Fatalf("WpTemplates[0] = %#v", got.WpTemplates[0])
	}
	if got.WpTemplates[0].WpTypeID != 23 || got.WpTemplates[0].WpTypeName != "PTW800作业票" {
		t.Fatalf("WpTemplates[0] type = %#v", got.WpTemplates[0])
	}
	if got.WpTemplates[0].IDPrefix != "800" || got.WpTemplates[0].UpdatedBy != "admin" {
		t.Fatalf("WpTemplates[0] metadata = %#v", got.WpTemplates[0])
	}
	if got.WpTemplates[0].ApplyContent != "{\"name\":\"permit-raw\"}" {
		t.Fatalf("WpTemplates[0].ApplyContent = %q", got.WpTemplates[0].ApplyContent)
	}
	if got.WpTemplates[0].ApplyContentObject["name"] != "permit" {
		t.Fatalf("WpTemplates[0].ApplyContentObject = %#v", got.WpTemplates[0].ApplyContentObject)
	}
}

type stubStructuredMemoryStore struct {
	entries []StructuredMemoryEntry
	err     error
}

func (s stubStructuredMemoryStore) Lookup(ctx context.Context, wpTypeID, areaID int) ([]StructuredMemoryEntry, error) {
	return s.entries, s.err
}

func TestFillWpsNodePrefersStateThenMemoryThenBlank(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(stubStructuredMemoryStore{
		entries: []StructuredMemoryEntry{
			{WpTypeID: 23, AreaID: 101, FieldName: "wpplace", FieldValue: "历史区域点位", Source: "structured_memory", UpdatedAt: 2},
			{WpTypeID: 23, AreaID: 101, FieldName: "qfrdh", FieldValue: "13800138000", Source: "structured_memory", UpdatedAt: 3},
		},
	})

	state := &WpGraphState{
		SiteID:      28,
		UserID:      7,
		Area:        AreaInfo{ID: 101, Name: "空分装置"},
		StartTime:   "2026-08-03 08:00",
		EndTime:     "2026-08-03 17:00",
		Description: "动火检修",
		Equipments:  []EquipmentInfo{{ID: 9, Name: "1号压缩机"}},
		WpCompany:   WpCompanyInfo{ID: 5, Name: "检修公司"},
		WpTemplates: []WpTemplate{
			{
				ID:         243,
				WpTypeID:   23,
				WpTypeName: "PTW800作业票",
				Name:       "作业许可证",
				ApplyContentObject: map[string]any{
					"id":          0,
					"wptempletid": 243,
					"wptypeid":    23,
					"siteid":      0,
					"userid":      0,
					"sqxx": []any{
						map[string]any{"name": "operationunit", "type": "selector", "value": []any{}},
						map[string]any{"name": "areaid", "type": "selector", "value": []any{}},
						map[string]any{"name": "equipmentid", "type": "selector", "value": []any{}},
						map[string]any{"name": "discription", "type": "input", "value": ""},
						map[string]any{"name": "starttime", "type": "datepicker", "value": ""},
						map[string]any{"name": "endtime", "type": "datepicker", "value": ""},
						map[string]any{"name": "wpplace", "type": "input", "value": ""},
						map[string]any{"name": "qfrdh", "type": "input", "value": ""},
						map[string]any{"name": "jsrdh", "type": "input", "value": ""},
					},
				},
			},
		},
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newFillWpsNode() error = %v", err)
	}

	if got.CurrentNode != nodeFillWps {
		t.Fatalf("CurrentNode = %q", got.CurrentNode)
	}
	if len(got.FilledWps) != 1 {
		t.Fatalf("len(FilledWps) = %d", len(got.FilledWps))
	}
	filled := got.FilledWps[0]
	if len(filled.FilledFields) == 0 {
		t.Fatalf("FilledFields is empty")
	}
	if len(filled.UnfilledFields) != 1 || filled.UnfilledFields[0].Name != "jsrdh" {
		t.Fatalf("UnfilledFields = %#v", filled.UnfilledFields)
	}

	content, ok := filled.Content.(map[string]any)
	if !ok {
		t.Fatalf("Content type = %T", filled.Content)
	}
	if content["siteid"] != float64(28) && content["siteid"] != 28 {
		t.Fatalf("siteid = %#v", content["siteid"])
	}
	if content["userid"] != float64(7) && content["userid"] != 7 {
		t.Fatalf("userid = %#v", content["userid"])
	}

	sqxx, ok := content["sqxx"].([]any)
	if !ok {
		t.Fatalf("sqxx type = %T", content["sqxx"])
	}

	values := make(map[string]any, len(sqxx))
	for _, rawField := range sqxx {
		field := rawField.(map[string]any)
		values[field["name"].(string)] = field["value"]
	}
	if values["discription"] != "动火检修" {
		t.Fatalf("discription = %#v", values["discription"])
	}
	if values["wpplace"] != "历史区域点位" {
		t.Fatalf("wpplace = %#v", values["wpplace"])
	}
	if values["qfrdh"] != "13800138000" {
		t.Fatalf("qfrdh = %#v", values["qfrdh"])
	}
	if values["jsrdh"] != "" {
		t.Fatalf("jsrdh = %#v", values["jsrdh"])
	}
}

func TestFillWpsNodeRejectsMalformedTemplate(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(stubStructuredMemoryStore{})
	state := &WpGraphState{
		WpTemplates: []WpTemplate{
			{ID: 243, Name: "作业许可证", ApplyContentObject: map[string]any{"bad": "shape"}},
		},
	}

	if _, err := node(context.Background(), state); err == nil {
		t.Fatalf("newFillWpsNode() error = nil")
	}
}

type recordingUpdateWpInfoTool struct {
	payloads []map[string]any
	ids      []int
}

func (s *recordingUpdateWpInfoTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return nil, nil
}

func (s *recordingUpdateWpInfoTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error) {
	var input struct {
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", err
	}
	s.payloads = append(s.payloads, input.Payload)
	id := 90001
	if len(s.ids) > 0 {
		id = s.ids[0]
		s.ids = s.ids[1:]
	}
	return fmt.Sprintf(`{"success":true,"data":%d}`, id), nil
}

type recordingLinkWpInfoTool struct {
	links []map[string]int
}

func (s *recordingLinkWpInfoTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return nil, nil
}

func (s *recordingLinkWpInfoTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error) {
	var input struct {
		ID  int `json:"id"`
		FID int `json:"fid"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", err
	}
	s.links = append(s.links, map[string]int{"id": input.ID, "fid": input.FID})
	return `{"success":true,"data":true}`, nil
}

func TestSubmitWpsNodeSubmitsApprovedFullPayloads(t *testing.T) {
	t.Parallel()

	updateTool := &recordingUpdateWpInfoTool{}
	linkTool := &recordingLinkWpInfoTool{}
	err := submitApprovedWps(context.Background(), updateTool, linkTool, 23, SubmitWpsApproval{
		Approved: true,
		Payloads: []map[string]any{{"id": 0, "wptempletid": 243, "wptypeid": 23}},
	})
	if err != nil {
		t.Fatalf("submitApprovedWps() error = %v", err)
	}
	if len(updateTool.payloads) != 1 || updateTool.payloads[0]["wptempletid"] != float64(243) && updateTool.payloads[0]["wptempletid"] != 243 {
		t.Fatalf("payloads = %#v", updateTool.payloads)
	}
	if len(linkTool.links) != 0 {
		t.Fatalf("links = %#v", linkTool.links)
	}
}

func TestSubmitWpsNodeLinksChildrenToPermitParent(t *testing.T) {
	t.Parallel()

	updateTool := &recordingUpdateWpInfoTool{ids: []int{91001, 91002, 91003}}
	linkTool := &recordingLinkWpInfoTool{}
	err := submitApprovedWps(context.Background(), updateTool, linkTool, 23, SubmitWpsApproval{
		Approved: true,
		Payloads: []map[string]any{
			{"id": 0, "wptempletid": 243, "wptypeid": 23},
			{"id": 0, "wptempletid": 171, "wptypeid": 1},
			{"id": 0, "wptempletid": 180, "wptypeid": 9},
		},
	})
	if err != nil {
		t.Fatalf("submitApprovedWps() error = %v", err)
	}
	if len(linkTool.links) != 2 {
		t.Fatalf("len(links) = %d", len(linkTool.links))
	}
	if linkTool.links[0]["fid"] != 91001 || linkTool.links[0]["id"] != 91002 {
		t.Fatalf("links[0] = %#v", linkTool.links[0])
	}
	if linkTool.links[1]["fid"] != 91001 || linkTool.links[1]["id"] != 91003 {
		t.Fatalf("links[1] = %#v", linkTool.links[1])
	}
}

func TestSubmitWpsNodeInterruptsBeforeSubmission(t *testing.T) {
	t.Parallel()

	node := newSubmitWpsNode(&recordingUpdateWpInfoTool{}, &recordingLinkWpInfoTool{})
	state := &WpGraphState{
		FilledWps: []FilledWp{{TemplateID: 243, Name: "作业许可证", Content: map[string]any{"wptypeid": 23}}},
	}

	_, err := node(context.Background(), state)
	if err == nil {
		t.Fatalf("newSubmitWpsNode() error = nil")
	}
	if err == nil {
		t.Fatalf("expected interrupt error")
	}
}
