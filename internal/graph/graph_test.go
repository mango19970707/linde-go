package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type stubWpInfoExtractor struct {
	result WpInfoExtraction
	err    error
}

func (s stubWpInfoExtractor) Extract(ctx context.Context, input string) (WpInfoExtraction, error) {
	return s.result, s.err
}

type recordingWpInfoExtractor struct {
	results map[string]WpInfoExtraction
	err     error
	inputs  []string
}

func (s *recordingWpInfoExtractor) Extract(ctx context.Context, input string) (WpInfoExtraction, error) {
	s.inputs = append(s.inputs, input)
	if s.err != nil {
		return WpInfoExtraction{}, s.err
	}
	return s.results[input], nil
}

type inMemoryCheckPointStore struct {
	data map[string][]byte
}

func newInMemoryCheckPointStore() *inMemoryCheckPointStore {
	return &inMemoryCheckPointStore{data: map[string][]byte{}}
}

func (s *inMemoryCheckPointStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	value, ok := s.data[checkPointID]
	return value, ok, nil
}

func (s *inMemoryCheckPointStore) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	s.data[checkPointID] = checkPoint
	return nil
}

func TestAskWpInfoNodeMapsExtractorResultToState(t *testing.T) {
	t.Parallel()

	node := newAskWpInfoNode(stubWpInfoExtractor{
		result: WpInfoExtraction{
			AreaName:    "A区",
			StartTime:   "2026/08/04-08:00:00",
			EndTime:     "2026/08/04-17:00:00",
			Description: "动火作业",
			EquipmentNames: []string{
				"3号焊机",
				"4号焊机",
			},
			CompanyName: "某某公司",
		},
	})

	state := &WpGraphState{
		SiteID:    28,
		UserInput: "明天在A区动火作业，使用3号焊机和4号焊机，某某公司施工。",
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newAskWpInfoNode() error = %v", err)
	}

	if got.CurrentNode != nodeAskWpInfo {
		t.Fatalf("CurrentNode = %q", got.CurrentNode)
	}
	if got.Area != "A区" {
		t.Fatalf("Area = %#v", got.Area)
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

func TestAskWpInfoNodeMergesSupplementInsteadOfClearingExistingFields(t *testing.T) {
	t.Parallel()

	node := newAskWpInfoNode(stubWpInfoExtractor{
		result: WpInfoExtraction{
			StartTime: "2026/08/04-08:00:00",
			EndTime:   "2026/08/04-17:00:00",
		},
	})

	state := &WpGraphState{
		UserInput:        "补充时间",
		Area:             "公共区域",
		Description:      "动火作业",
		StartTime:        "",
		EndTime:          "",
		ValidationStatus: validationInvalid,
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newAskWpInfoNode() error = %v", err)
	}

	if got.Area != "公共区域" {
		t.Fatalf("Area = %#v", got.Area)
	}
	if got.Description != "动火作业" {
		t.Fatalf("Description = %q", got.Description)
	}
	if got.StartTime != "2026/08/04-08:00:00" || got.EndTime != "2026/08/04-17:00:00" {
		t.Fatalf("times = %q - %q", got.StartTime, got.EndTime)
	}
}

func TestValidateWpInfoNodeRejectsMissingFields(t *testing.T) {
	t.Parallel()

	state := &WpGraphState{
		Area:        "公共区域",
		StartTime:   "2026/08/04-08:00:00",
		Description: "动火作业",
	}

	_, err := validateWpInfoNode(context.Background(), state)
	if err == nil {
		t.Fatal("validateWpInfoNode() error = nil")
	}
	if !strings.Contains(err.Error(), "collect_wp_info") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateWpInfoNodeInterruptsWhenRequiredFieldsMissing(t *testing.T) {
	t.Parallel()

	state := &WpGraphState{
		Area:        "公共区域",
		Description: "动火作业",
	}

	_, err := validateWpInfoNode(context.Background(), state)
	if err == nil {
		t.Fatal("validateWpInfoNode() error = nil")
	}
	if !strings.Contains(err.Error(), "start_time") || !strings.Contains(err.Error(), "end_time") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildWpGraphRequiresExtractor(t *testing.T) {
	t.Parallel()

	if _, err := BuildWpGraph(nil, nil, nil, nil, nil, nil, nil, WpFillRules{}, nil, nil); err == nil {
		t.Fatalf("BuildWpGraph(nil) error = nil")
	}
}

func TestBuildWpGraphResumesAfterSupplementalInput(t *testing.T) {
	t.Parallel()

	templateListRaw := `{"success":true,"data":[{"id":243,"templetname":"作业许可证","wptypename":"PTW800作业票","wptypeid":23}]}`
	templateDetailRaw := `{"success":true,"data":{"id":243,"wptypeid":23,"wptypename":"PTW800作业票","templetname":"作业许可证","applycontent":"{\"sqxx\":[{\"name\":\"discription\",\"type\":\"input\",\"value\":\"\"}]}","applycontent_object":{"sqxx":[{"name":"discription","type":"input","value":""}]}}}`

	extractor := &recordingWpInfoExtractor{
		results: map[string]WpInfoExtraction{
			"初始输入": {
				AreaName:    "A区",
				Description: "动火作业",
			},
			"补充时间，2026/08/04-08:00:00 到 2026/08/04-17:00:00": {
				StartTime: "2026/08/04-08:00:00",
				EndTime:   "2026/08/04-17:00:00",
			},
		},
	}
	detailTool := &recordingWpTemplateDetailTool{
		responses: map[int]string{243: templateDetailRaw},
	}
	store := newInMemoryCheckPointStore()

	graph, err := BuildWpGraph(
		extractor,
		stubWpTemplateTool{result: templateListRaw},
		stubWpTypeRecognizer{},
		detailTool,
		stubChatModel{},
		stubPromptStore{},
		stubStructuredMemoryStore{},
		WpFillRules{},
		&recordingUpdateWpInfoTool{},
		&recordingLinkWpInfoTool{},
		false,
		compose.WithCheckPointStore(store),
		compose.WithGraphName("work-permit"),
	)
	if err != nil {
		t.Fatalf("BuildWpGraph() error = %v", err)
	}

	checkPointID := "cp-missing-info"
	_, err = graph.Invoke(context.Background(), &WpGraphState{
		SiteID:    28,
		UserID:    464,
		UserInput: "初始输入",
	}, compose.WithCheckPointID(checkPointID))
	if err == nil {
		t.Fatal("graph.Invoke() error = nil")
	}

	info, ok := compose.ExtractInterruptInfo(err)
	if !ok {
		t.Fatalf("first error is not interrupt: %v", err)
	}
	if len(info.InterruptContexts) != 1 {
		t.Fatalf("len(first InterruptContexts) = %d", len(info.InterruptContexts))
	}

	firstPayload := info.InterruptContexts[0].Info.(map[string]any)
	if firstPayload["operation"] != "collect_wp_info" {
		t.Fatalf("first operation = %#v", firstPayload["operation"])
	}

	resumeCtx := compose.ResumeWithData(context.Background(), info.InterruptContexts[0].ID, WpInfoSupplement{
		Input: "补充时间，2026/08/04-08:00:00 到 2026/08/04-17:00:00",
	})

	_, err = graph.Invoke(resumeCtx, &WpGraphState{}, compose.WithCheckPointID(checkPointID))
	if err == nil {
		t.Fatal("resumed graph.Invoke() error = nil")
	}

	info, ok = compose.ExtractInterruptInfo(err)
	if !ok {
		t.Fatalf("second error is not interrupt: %v", err)
	}
	secondPayload := info.InterruptContexts[0].Info.(map[string]any)
	if secondPayload["operation"] != "submit_work_permits" {
		t.Fatalf("second operation = %#v", secondPayload["operation"])
	}
	if len(extractor.inputs) != 2 {
		t.Fatalf("extractor inputs = %#v", extractor.inputs)
	}
}

func DisabledBuildWpGraphResumesApprovalAfterSubmitInterrupt(t *testing.T) {
	t.Parallel()

	templateListRaw := `{"success":true,"data":[{"id":243,"templetname":"浣滀笟璁稿彲璇?,"wptypename":"PTW800浣滀笟绁?,"wptypeid":23}]}`
	templateDetailRaw := `{"success":true,"data":{"id":243,"wptypeid":23,"wptypename":"PTW800浣滀笟绁?,"templetname":"浣滀笟璁稿彲璇?,"applycontent":"{\"sqxx\":[{\"name\":\"discription\",\"type\":\"input\",\"value\":\"\"}]}","applycontent_object":{"id":0,"wptempletid":243,"wptypeid":23,"sqxx":[{"name":"discription","type":"input","value":""}]}}}`

	extractor := &recordingWpInfoExtractor{
		results: map[string]WpInfoExtraction{
			"鍒濆杈撳叆": {
				AreaName:    "A鍖?",
				Description: "鍔ㄧ伀浣滀笟",
			},
			"琛ュ厖鏃堕棿锛?026/08/04-08:00:00 鍒?2026/08/04-17:00:00": {
				StartTime: "2026/08/04-08:00:00",
				EndTime:   "2026/08/04-17:00:00",
			},
		},
	}
	detailTool := &recordingWpTemplateDetailTool{
		responses: map[int]string{243: templateDetailRaw},
	}
	updateTool := &recordingUpdateWpInfoTool{}
	linkTool := &recordingLinkWpInfoTool{}
	store := newInMemoryCheckPointStore()

	graph, err := BuildWpGraph(
		extractor,
		stubWpTemplateTool{result: templateListRaw},
		stubWpTypeRecognizer{},
		detailTool,
		stubChatModel{},
		stubPromptStore{},
		stubStructuredMemoryStore{},
		WpFillRules{},
		updateTool,
		linkTool,
		false,
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
		UserInput: "鍒濆杈撳叆",
	}, compose.WithCheckPointID(checkPointID))
	if err == nil {
		t.Fatal("first graph.Invoke() error = nil")
	}

	firstInterrupt, ok := compose.ExtractInterruptInfo(err)
	if !ok || len(firstInterrupt.InterruptContexts) != 1 {
		t.Fatalf("first error is not single interrupt: %v", err)
	}

	resumeInfoCtx := compose.ResumeWithData(context.Background(), firstInterrupt.InterruptContexts[0].ID, WpInfoSupplement{
		Input: "琛ュ厖鏃堕棿锛?026/08/04-08:00:00 鍒?2026/08/04-17:00:00",
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

func (s stubWpTypeRecognizer) Recognize(ctx context.Context, description string, templates []WpMetaInfo, candidates []WpMetaInfo) ([]int, error) {
	return s.ids, s.err
}

type recordingWpTypeRecognizer struct {
	ids        []int
	err        error
	candidates []WpMetaInfo
}

func (s *recordingWpTypeRecognizer) Recognize(ctx context.Context, description string, templates []WpMetaInfo, candidates []WpMetaInfo) ([]int, error) {
	s.candidates = append([]WpMetaInfo(nil), candidates...)
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
		stubStructuredMemoryStore{},
		stubWpTypeRecognizer{ids: []int{1}},
		false,
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

func TestIntentRecognitionNodeInterruptsForWpTypeConfirmation(t *testing.T) {
	t.Parallel()

	raw := `{"success":true,"data":[{"id":243,"templetname":"作业许可证","wptypename":"PTW800","wptypeid":23}]}`
	node := newIntentRecognitionNode(stubWpTemplateTool{result: raw}, stubStructuredMemoryStore{}, stubWpTypeRecognizer{ids: []int{23}})

	_, err := node(context.Background(), &WpGraphState{Description: "hot work"})
	if err == nil {
		t.Fatal("newIntentRecognitionNode() error = nil")
	}
	if !strings.Contains(err.Error(), "confirm_wp_types") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateWpTypeConfirmationAppliesEditedWpTypes(t *testing.T) {
	t.Parallel()

	templates := []WpMetaInfo{
		{Id: 243, TemplateName: "作业许可证", WpTypeName: "PTW800", WpTypeId: 23},
		{Id: 180, TemplateName: "临时用电", WpTypeName: "临时用电", WpTypeId: 9},
	}

	got, err := validateWpTypeConfirmation(templates, []WpMetaInfo{
		{Id: 243, TemplateName: "作业许可证", WpTypeName: "PTW800", WpTypeId: 23},
		{Id: 180, TemplateName: "临时用电", WpTypeName: "临时用电", WpTypeId: 9},
	})
	if err != nil {
		t.Fatalf("validateWpTypeConfirmation() error = %v", err)
	}
	if len(got) != 2 || got[1].WpTypeId != 9 {
		t.Fatalf("got = %#v", got)
	}
}

func TestIntentRecognitionNodeCanSkipWpTypeConfirmation(t *testing.T) {
	t.Parallel()

	raw := `{"success":true,"data":[{"id":243,"templetname":"作业许可证","wptypename":"PTW800","wptypeid":23}]}`
	node := newIntentRecognitionNode(stubWpTemplateTool{result: raw}, stubStructuredMemoryStore{}, stubWpTypeRecognizer{ids: []int{23}}, false)

	got, err := node(context.Background(), &WpGraphState{Description: "hot work"})
	if err != nil {
		t.Fatalf("newIntentRecognitionNode() error = %v", err)
	}
	if got.CurrentNode != nodeIntentRecognition {
		t.Fatalf("CurrentNode = %q", got.CurrentNode)
	}
}

func TestIntentRecognitionNodeRejectsUnknownTypeID(t *testing.T) {
	t.Parallel()

	raw := `{"success":true,"data":[{"id":243,"templetname":"作业许可证","wptypename":"PTW800作业票","wptypeid":23}]}`
	node := newIntentRecognitionNode(
		stubWpTemplateTool{result: raw},
		stubStructuredMemoryStore{},
		stubWpTypeRecognizer{ids: []int{999}},
	)

	_, err := node(context.Background(), &WpGraphState{Description: "一般作业"})
	if err == nil {
		t.Fatalf("newIntentRecognitionNode() error = nil")
	}
}

func TestFindIntentMemoryCandidatesAppliesThresholdAndTopN(t *testing.T) {
	t.Parallel()

	templates := []WpMetaInfo{
		{Id: 243, TemplateName: "作业许可证", WpTypeName: "PTW800作业票", WpTypeId: 23},
		{Id: 171, TemplateName: "特级动火", WpTypeName: "特级动火", WpTypeId: 1},
		{Id: 180, TemplateName: "临时用电", WpTypeName: "临时用电", WpTypeId: 9},
	}
	store := stubStructuredMemoryStore{
		intentEntries: []StructuredIntentMemoryEntry{
			{SiteID: 28, Area: "空分装置", WorkDescription: "特级动火作业", WpTypeIDs: []int{1}, UpdatedAt: 2},
			{SiteID: 28, Area: "空分装置", WorkDescription: "临时用电接线", WpTypeIDs: []int{9}, UpdatedAt: 1},
		},
	}

	got, err := findIntentMemoryCandidates(
		context.Background(),
		store,
		28,
		"空分装置",
		"特级动火作业，需要开票",
		templates,
		IntentMemoryRetrievalConfig{ScoreThreshold: 10, TopN: 1},
	)
	if err != nil {
		t.Fatalf("findIntentMemoryCandidates() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].WpTypeId != 1 {
		t.Fatalf("got[0].WpTypeId = %d, want 1", got[0].WpTypeId)
	}
}

func TestIntentRecognitionNodePassesFilteredCandidatesToRecognizer(t *testing.T) {
	t.Parallel()

	templates := []WpMetaInfo{
		{Id: 243, TemplateName: "作业许可证", WpTypeName: "PTW800作业票", WpTypeId: 23},
		{Id: 171, TemplateName: "特级动火", WpTypeName: "特级动火", WpTypeId: 1},
		{Id: 180, TemplateName: "临时用电", WpTypeName: "临时用电", WpTypeId: 9},
	}
	raw, err := json.Marshal(map[string]any{
		"success": true,
		"data":    templates,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	recognizer := &recordingWpTypeRecognizer{ids: []int{1}}
	node := newIntentRecognitionNode(
		stubWpTemplateTool{result: string(raw)},
		stubStructuredMemoryStore{
			intentEntries: []StructuredIntentMemoryEntry{
				{SiteID: 28, Area: "空分装置", WorkDescription: "特级动火作业", WpTypeIDs: []int{1}, UpdatedAt: 2},
				{SiteID: 28, Area: "空分装置", WorkDescription: "临时用电接线", WpTypeIDs: []int{9}, UpdatedAt: 1},
			},
		},
		recognizer,
		IntentMemoryRetrievalConfig{ScoreThreshold: 10, TopN: 1},
		false,
	)

	_, err = node(context.Background(), &WpGraphState{
		SiteID:      28,
		Area:        "空分装置",
		Description: "特级动火作业，需要开票",
	})
	if err != nil {
		t.Fatalf("newIntentRecognitionNode() error = %v", err)
	}
	if len(recognizer.candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(recognizer.candidates))
	}
	if recognizer.candidates[0].WpTypeId != 1 {
		t.Fatalf("candidate WpTypeId = %d, want 1", recognizer.candidates[0].WpTypeId)
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
	entries       []StructuredMemoryEntry
	intentEntries []StructuredIntentMemoryEntry
	err           error
}

func (s stubStructuredMemoryStore) FindByTypeAndArea(ctx context.Context, wpTypeID int, area string) ([]StructuredMemoryEntry, error) {
	if s.err != nil {
		return nil, s.err
	}
	entries := make([]StructuredMemoryEntry, 0)
	for _, entry := range s.entries {
		if entry.WpTypeID == wpTypeID && strings.TrimSpace(entry.Area) == strings.TrimSpace(area) {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (s stubStructuredMemoryStore) ListAreasByType(ctx context.Context, wpTypeID int) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	seen := make(map[string]struct{})
	areas := make([]string, 0)
	for _, entry := range s.entries {
		if entry.WpTypeID != wpTypeID {
			continue
		}
		area := strings.TrimSpace(entry.Area)
		if area == "" {
			continue
		}
		if _, ok := seen[area]; ok {
			continue
		}
		seen[area] = struct{}{}
		areas = append(areas, area)
	}
	return areas, nil
}

func (s stubStructuredMemoryStore) FindByType(ctx context.Context, wpTypeID int) ([]StructuredMemoryEntry, error) {
	return s.entries, s.err
}

func (s stubStructuredMemoryStore) Upsert(ctx context.Context, entry StructuredMemoryEntry) error {
	return s.err
}

func (s stubStructuredMemoryStore) FindIntentMemoryBySite(ctx context.Context, siteID int) ([]StructuredIntentMemoryEntry, error) {
	if s.err != nil {
		return nil, s.err
	}
	entries := make([]StructuredIntentMemoryEntry, 0)
	for _, entry := range s.intentEntries {
		if entry.SiteID == siteID {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (s stubStructuredMemoryStore) UpsertIntentMemory(ctx context.Context, entry StructuredIntentMemoryEntry) error {
	return s.err
}

type recordingStructuredMemoryStore struct {
	entries []StructuredMemoryEntry
	err     error
}

func (s *recordingStructuredMemoryStore) FindByTypeAndArea(ctx context.Context, wpTypeID int, area string) ([]StructuredMemoryEntry, error) {
	return nil, s.err
}

func (s *recordingStructuredMemoryStore) ListAreasByType(ctx context.Context, wpTypeID int) ([]string, error) {
	return nil, s.err
}

func (s *recordingStructuredMemoryStore) FindByType(ctx context.Context, wpTypeID int) ([]StructuredMemoryEntry, error) {
	return nil, s.err
}

func (s *recordingStructuredMemoryStore) Upsert(ctx context.Context, entry StructuredMemoryEntry) error {
	if s.err != nil {
		return s.err
	}
	s.entries = append(s.entries, entry)
	return nil
}

func (s *recordingStructuredMemoryStore) FindIntentMemoryBySite(ctx context.Context, siteID int) ([]StructuredIntentMemoryEntry, error) {
	return nil, s.err
}

func (s *recordingStructuredMemoryStore) UpsertIntentMemory(ctx context.Context, entry StructuredIntentMemoryEntry) error {
	if s.err != nil {
		return s.err
	}
	return nil
}

func TestFillWpsNodePrefersStateThenMemoryThenBlank(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(stubChatModel{
		response: schema.AssistantMessage(`{"fields":{"sqxx.8.value":"13900000000"}}`, nil),
	}, stubPromptStore{
		prompts: map[string]PromptDefinition{
			PromptKeyWpFillAreaSelection:   {Key: PromptKeyWpFillAreaSelection, Content: "area prompt"},
			PromptKeyWpFillFieldCompletion: {Key: PromptKeyWpFillFieldCompletion, Content: "fill prompt"},
		},
	}, stubStructuredMemoryStore{
		entries: []StructuredMemoryEntry{
			{WpTypeID: 23, Area: "空分装置", WorkDescription: "动火检修", FieldName: "sqxx.6.value", FieldValue: "历史区域点位", Source: "structured_memory", UpdatedAt: 2},
			{WpTypeID: 23, Area: "空分装置", WorkDescription: "动火检修", FieldName: "sqxx.7.value", FieldValue: "13800138000", Source: "structured_memory", UpdatedAt: 3},
			{WpTypeID: 23, Area: "空分装置", WorkDescription: "动火检修", FieldName: "sqxx.8.value", FieldValue: "13900000000", Source: "structured_memory", UpdatedAt: 4},
		},
	}, WpFillRules{})

	state := &WpGraphState{
		SiteID:      28,
		UserID:      7,
		Area:        "空分装置",
		StartTime:   "2026/08/03-08:00:00",
		EndTime:     "2026/08/03-17:00:00",
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
						map[string]any{"name": "operationunit", "label": "接收人公司名称", "type": "selector", "value": []any{}},
						map[string]any{"name": "areaid", "label": "所属区域", "type": "selector", "value": []any{}},
						map[string]any{"name": "equipmentid", "label": "所属设备", "type": "selector", "value": []any{}},
						map[string]any{"name": "discription", "label": "作业描述", "type": "input", "value": ""},
						map[string]any{"name": "starttime", "label": "作业计划开始时间", "type": "datepicker", "value": ""},
						map[string]any{"name": "endtime", "label": "作业计划结束时间", "type": "datepicker", "value": ""},
						map[string]any{"name": "wpplace", "label": "作业地点", "type": "input", "value": "", "required": true},
						map[string]any{"name": "qfrdh", "label": "签发人电话", "type": "input", "value": "", "required": true},
						map[string]any{"name": "jsrdh", "label": "接收人电话", "type": "input", "value": "", "required": true},
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
	if filled.WpType != "PTW800作业票" {
		t.Fatalf("WpType = %q", filled.WpType)
	}
	if len(filled.UnfilledFields) != 0 {
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
	if values["jsrdh"] != "13900000000" {
		t.Fatalf("jsrdh = %#v", values["jsrdh"])
	}
}

func TestFillWpsNodeSkipsFieldsConfiguredForWpType(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(stubChatModel{}, stubPromptStore{}, stubStructuredMemoryStore{
		entries: []StructuredMemoryEntry{
			{WpTypeID: 2, Area: "区域", FieldName: "sqxx.0.value", FieldValue: []any{selectorValue(11, "历史单位")}, Source: "structured_memory"},
		},
	}, WpFillRules{ByWpTypeID: map[int]WpFillRule{
		2: {SkipFields: []string{"operationunit"}},
	}})

	state := &WpGraphState{
		Area:      "区域",
		WpCompany: WpCompanyInfo{ID: 12, Name: "当前单位"},
		WpTemplates: []WpTemplate{{
			ID:       172,
			WpTypeID: 2,
			ApplyContentObject: map[string]any{
				"sqxx": []any{
					map[string]any{"name": "operationunit", "label": "作业单位", "type": "selector", "value": []any{}},
					map[string]any{"name": "areaid", "label": "所属区域", "type": "selector", "value": []any{}},
				},
			},
		}},
	}

	got, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("newFillWpsNode() error = %v", err)
	}

	fields := got.FilledWps[0].FilledFields
	if len(fields) != 1 || fields[0].Label != "所属区域" {
		t.Fatalf("FilledFields = %#v", fields)
	}
	if len(got.FilledWps[0].UnfilledFields) != 0 {
		t.Fatalf("UnfilledFields = %#v", got.FilledWps[0].UnfilledFields)
	}
}

func TestFillWpsNodeRejectsMalformedTemplate(t *testing.T) {
	t.Parallel()

	node := newFillWpsNode(stubChatModel{}, stubPromptStore{}, stubStructuredMemoryStore{}, WpFillRules{})
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
	return fmt.Sprintf(`{"success":true,"data":{"id":%d}}`, id), nil
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
	err := submitApprovedWps(context.Background(), updateTool, linkTool, stubStructuredMemoryStore{}, &WpGraphState{}, 23, SubmitWpsApproval{
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
	err := submitApprovedWps(context.Background(), updateTool, linkTool, stubStructuredMemoryStore{}, &WpGraphState{}, 23, SubmitWpsApproval{
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

func TestSubmitWpsNodePersistsStructuredMemoryFromApprovedPayloads(t *testing.T) {
	t.Parallel()

	updateTool := &recordingUpdateWpInfoTool{ids: []int{91001}}
	linkTool := &recordingLinkWpInfoTool{}
	memoryStore := &recordingStructuredMemoryStore{}
	templates := []WpTemplate{
		{
			ID:         172,
			WpTypeID:   2,
			WpTypeName: "????",
			Name:       "????",
			ApplyContentObject: map[string]any{
				"sqxx": []any{
					map[string]any{"name": "wpname", "label": "????", "type": "input", "value": ""},
					map[string]any{"name": "discription", "label": "??????", "type": "input", "value": ""},
					map[string]any{"name": "areaid", "label": "????", "type": "selector", "value": []any{}},
					map[string]any{"name": "wpplace", "label": "????", "type": "input", "value": ""},
					map[string]any{"name": "approvedby", "label": "???", "type": "selector", "value": []any{}},
				},
			},
		},
	}

	err := submitApprovedWps(context.Background(), updateTool, linkTool, memoryStore, &WpGraphState{
		SiteID:      28,
		Area:        "????",
		Description: "????",
		IntentWpTypes: []WpMetaInfo{
			{WpTypeId: 2, WpTypeName: "????"},
		},
		WpTemplates: templates,
	}, 2, SubmitWpsApproval{
		Approved: true,
		Payloads: []map[string]any{
			{
				"id":          0,
				"wptempletid": 172,
				"wptypeid":    2,
				"wptypename":  "????",
				"sqxx": []any{
					map[string]any{"name": "wpname", "label": "????", "type": "input", "value": "??????"},
					map[string]any{"name": "discription", "label": "??????", "type": "input", "value": "????"},
					map[string]any{"name": "areaid", "label": "????", "type": "selector", "value": []any{map[string]any{"id": 1, "name": "????"}}},
					map[string]any{"name": "wpplace", "label": "????", "type": "input", "value": "????"},
					map[string]any{"name": "approvedby", "label": "???", "type": "selector", "value": []any{map[string]any{"id": 7, "name": "??"}}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("submitApprovedWps() error = %v", err)
	}
	if len(memoryStore.entries) == 0 {
		t.Fatal("expected structured memory entries to be persisted")
	}

	byField := make(map[string]StructuredMemoryEntry, len(memoryStore.entries))
	for _, entry := range memoryStore.entries {
		byField[entry.FieldName] = entry
		if entry.Source != "91001" {
			t.Fatalf("entry source = %q, want %q", entry.Source, "91001")
		}
		if entry.WpTypeID != 2 {
			t.Fatalf("entry wp_type_id = %d", entry.WpTypeID)
		}
		if entry.WpTypeName != "????" {
			t.Fatalf("entry wp_type_name = %q", entry.WpTypeName)
		}
		if entry.Area != "????" {
			t.Fatalf("entry area = %q", entry.Area)
		}
		if entry.WorkDescription != "????" {
			t.Fatalf("entry work_description = %q", entry.WorkDescription)
		}
	}

	if got := byField["sqxx.0.value"].FieldDescription; got != "????" {
		t.Fatalf("sqxx.0.value description = %q", got)
	}
	if got := byField["sqxx.3.value"].FieldDescription; got != "????" {
		t.Fatalf("sqxx.3.value description = %q", got)
	}
	if got := byField["sqxx.4.value"].FieldDescription; got != "???" {
		t.Fatalf("sqxx.4.value description = %q", got)
	}
}

func TestSubmitWpsNodeInterruptsBeforeSubmission(t *testing.T) {
	t.Parallel()

	node := newSubmitWpsNode(stubStructuredMemoryStore{}, &recordingUpdateWpInfoTool{}, &recordingLinkWpInfoTool{})
	state := &WpGraphState{
		FilledWps: []FilledWp{{WpType: "PTW800作业票", Name: "作业许可证", Content: map[string]any{"wptypeid": 23}}},
	}

	_, err := node(context.Background(), state)
	if err == nil {
		t.Fatalf("newSubmitWpsNode() error = nil")
	}
	if err == nil {
		t.Fatalf("expected interrupt error")
	}
}
