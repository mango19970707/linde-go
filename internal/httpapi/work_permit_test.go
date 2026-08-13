package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cloudwego/eino/compose"
	"linde-go-v2/internal/graph"
)

type recordingRunner struct {
	inputs []any
	call   func(context.Context, *graph.WpGraphState, ...compose.Option) (*graph.WpGraphState, error)
}

func (r *recordingRunner) Invoke(ctx context.Context, input *graph.WpGraphState, opts ...compose.Option) (*graph.WpGraphState, error) {
	r.inputs = append(r.inputs, input)
	return r.call(ctx, input, opts...)
}

func TestWorkPermitHandlerReturnsNeedMoreInfo(t *testing.T) {
	runner := &recordingRunner{}
	interruptErr := newInterruptError(t, map[string]any{
		"operation":      "collect_wp_info",
		"missing_fields": []string{"start_time", "end_time"},
		"question":       "please provide start and end time",
	})
	runner.call = func(ctx context.Context, input *graph.WpGraphState, opts ...compose.Option) (*graph.WpGraphState, error) {
		return nil, interruptErr
	}

	server := NewWorkPermitServer(runner, nil)
	rec := postJSON(server, `{"input":"initial work description","site_id":28,"user_id":464}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	response := decodeResponse(t, rec)
	if response.Status != "need_more_info" {
		t.Fatalf("status = %q", response.Status)
	}
	if response.SessionID == "" {
		t.Fatal("session_id is empty")
	}
	if len(response.MissingFields) != 2 {
		t.Fatalf("missing_fields = %#v", response.MissingFields)
	}
	if response.Question == "" {
		t.Fatal("question is empty")
	}
}

func TestWorkPermitHandlerResumesWithSupplement(t *testing.T) {
	runner := &recordingRunner{}
	interruptErr := newInterruptError(t, map[string]any{
		"operation":      "collect_wp_info",
		"missing_fields": []string{"start_time"},
		"question":       "please provide start time",
	})
	callCount := 0
	runner.call = func(ctx context.Context, input *graph.WpGraphState, opts ...compose.Option) (*graph.WpGraphState, error) {
		callCount++
		if callCount == 1 {
			return nil, interruptErr
		}
		return &graph.WpGraphState{CurrentNode: "intentRecognition"}, nil
	}

	server := NewWorkPermitServer(runner, nil)
	first := postJSON(server, `{"input":"initial work description","site_id":28,"user_id":464}`)
	firstResponse := decodeResponse(t, first)

	rec := postJSON(server, `{"session_id":"`+firstResponse.SessionID+`","input":"start at 08:00","site_id":28,"user_id":464}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	response := decodeResponse(t, rec)
	if response.Status != "completed" {
		t.Fatalf("status = %q", response.Status)
	}
	if callCount != 2 {
		t.Fatalf("callCount = %d", callCount)
	}
	if len(runner.inputs) != 2 {
		t.Fatalf("inputs = %#v", runner.inputs)
	}
}

func TestWorkPermitHandlerReturnsApprovalPreview(t *testing.T) {
	runner := &recordingRunner{}
	interruptErr := newInterruptError(t, map[string]any{
		"operation":  "submit_work_permits",
		"filled_wps": []graph.FilledWp{{WpType: "一级动火", Name: "permit"}},
	})
	runner.call = func(ctx context.Context, input *graph.WpGraphState, opts ...compose.Option) (*graph.WpGraphState, error) {
		return nil, interruptErr
	}

	server := NewWorkPermitServer(runner, nil)
	rec := postJSON(server, `{"input":"complete work description","site_id":28,"user_id":464}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	response := decodeResponse(t, rec)
	if response.Status != "need_approval" {
		t.Fatalf("status = %q", response.Status)
	}
	if len(response.FilledWps) != 1 || response.FilledWps[0].WpType != "一级动火" {
		t.Fatalf("filled_wps = %#v", response.FilledWps)
	}
}

func TestWorkPermitHandlerReturnsWpTypeConfirmation(t *testing.T) {
	runner := &recordingRunner{}
	interruptErr := newInterruptError(t, map[string]any{
		"operation": "confirm_wp_types",
		"question":  "confirm permit types",
		"intent_wp_types": []graph.WpMetaInfo{
			{Id: 243, TemplateName: "permit", WpTypeName: "PTW800", WpTypeId: 23},
		},
	})
	runner.call = func(ctx context.Context, input *graph.WpGraphState, opts ...compose.Option) (*graph.WpGraphState, error) {
		return nil, interruptErr
	}

	server := NewWorkPermitServer(runner, nil)
	rec := postJSON(server, `{"input":"complete work description","site_id":28,"user_id":464}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	response := decodeResponse(t, rec)
	if response.Status != "need_wp_type_confirmation" {
		t.Fatalf("status = %q", response.Status)
	}
	if len(response.IntentWpTypes) != 1 || response.IntentWpTypes[0].WpTypeId != 23 {
		t.Fatalf("intent_wp_types = %#v", response.IntentWpTypes)
	}
}

func TestWorkPermitHandlerResumesWpTypeConfirmation(t *testing.T) {
	runner := &recordingRunner{}
	interruptErr := newInterruptError(t, map[string]any{
		"operation": "confirm_wp_types",
	})
	callCount := 0
	runner.call = func(ctx context.Context, input *graph.WpGraphState, opts ...compose.Option) (*graph.WpGraphState, error) {
		callCount++
		if callCount == 1 {
			return nil, interruptErr
		}
		return &graph.WpGraphState{CurrentNode: "intentRecognition"}, nil
	}

	server := NewWorkPermitServer(runner, nil)
	first := postJSON(server, `{"input":"complete work description","site_id":28,"user_id":464}`)
	firstResponse := decodeResponse(t, first)

	rec := postJSON(server, `{"session_id":"`+firstResponse.SessionID+`","confirmed":true,"intent_wp_types":[{"id":243,"templetname":"permit","wptypename":"PTW800","wptypeid":23}],"site_id":28,"user_id":464}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	response := decodeResponse(t, rec)
	if response.Status != "completed" {
		t.Fatalf("status = %q", response.Status)
	}
	if callCount != 2 {
		t.Fatalf("callCount = %d", callCount)
	}
}

func TestWorkPermitHandlerResumesApprovalWithFullPayloads(t *testing.T) {
	runner := &recordingRunner{}
	interruptErr := newInterruptError(t, map[string]any{
		"operation": "submit_work_permits",
	})
	callCount := 0
	runner.call = func(ctx context.Context, input *graph.WpGraphState, opts ...compose.Option) (*graph.WpGraphState, error) {
		callCount++
		if callCount == 1 {
			return nil, interruptErr
		}
		return &graph.WpGraphState{CurrentNode: "submitWps"}, nil
	}

	server := NewWorkPermitServer(runner, nil)
	first := postJSON(server, `{"input":"complete work description","site_id":28,"user_id":464}`)
	firstResponse := decodeResponse(t, first)

	rec := postJSON(server, `{"session_id":"`+firstResponse.SessionID+`","approved":true,"payloads":[{"id":0,"wptypeid":23}],"site_id":28,"user_id":464}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	response := decodeResponse(t, rec)
	if response.Status != "completed" {
		t.Fatalf("status = %q", response.Status)
	}
	if callCount != 2 {
		t.Fatalf("callCount = %d", callCount)
	}
}

type recordingStructuredMemoryStore struct {
	entries []graph.StructuredMemoryEntry
	err     error
}

func (s *recordingStructuredMemoryStore) FindByTypeAndArea(ctx context.Context, wpTypeID int, area string) ([]graph.StructuredMemoryEntry, error) {
	if s.err != nil {
		return nil, s.err
	}
	result := make([]graph.StructuredMemoryEntry, 0)
	for _, entry := range s.entries {
		if entry.WpTypeID == wpTypeID && strings.TrimSpace(entry.Area) == strings.TrimSpace(area) {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (s *recordingStructuredMemoryStore) ListAreasByType(ctx context.Context, wpTypeID int) ([]string, error) {
	return nil, s.err
}

func (s *recordingStructuredMemoryStore) FindByType(ctx context.Context, wpTypeID int) ([]graph.StructuredMemoryEntry, error) {
	if s.err != nil {
		return nil, s.err
	}
	result := make([]graph.StructuredMemoryEntry, 0)
	for _, entry := range s.entries {
		if entry.WpTypeID == wpTypeID {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (s *recordingStructuredMemoryStore) Upsert(ctx context.Context, entry graph.StructuredMemoryEntry) error {
	if s.err != nil {
		return s.err
	}
	s.entries = append(s.entries, entry)
	return nil
}

func (s *recordingStructuredMemoryStore) FindIntentMemoryBySite(ctx context.Context, siteID int) ([]graph.StructuredIntentMemoryEntry, error) {
	return nil, s.err
}

func (s *recordingStructuredMemoryStore) UpsertIntentMemory(ctx context.Context, entry graph.StructuredIntentMemoryEntry) error {
	return s.err
}

func TestStructuredMemoryImportHandlerPersistsPayloads(t *testing.T) {
	t.Parallel()

	store := &recordingStructuredMemoryStore{}
	server := NewWorkPermitServer(nil, nil, store)

	rec := postJSONTo(server, "/api/structured-memory/import", `{"payloads":[{"wptempletid":172,"wptypeid":2,"wptypename":"一级动火","sqxx":[{"name":"wpname","label":"作业名称","type":"input","value":"空分东侧动火"},{"name":"discription","label":"作业内容描述","type":"input","value":"焊接支架"},{"name":"areaid","label":"所属区域","type":"selector","value":[{"id":1,"name":"空分东侧"}]}]}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Status       string `json:"status"`
		ImportedWps  int    `json:"imported_wps"`
		ImportedRows int    `json:"imported_rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "completed" {
		t.Fatalf("status = %q", response.Status)
	}
	if response.ImportedWps != 1 {
		t.Fatalf("imported_wps = %d", response.ImportedWps)
	}
	if response.ImportedRows == 0 {
		t.Fatalf("imported_rows = %d", response.ImportedRows)
	}
	if len(store.entries) == 0 {
		t.Fatal("expected structured memory entries")
	}
	if store.entries[0].Source == "" {
		t.Fatal("expected import source to be generated")
	}
}

func TestStructuredMemoryExportHandlerReturnsPayloadsByTypeAndArea(t *testing.T) {
	t.Parallel()

	store := &recordingStructuredMemoryStore{
		entries: []graph.StructuredMemoryEntry{
			{WpTypeID: 2, WpTypeName: "一级动火", Area: "空分东侧", WorkDescription: "焊接支架", FieldDescription: "作业名称", FieldName: "sqxx.0.value", FieldValue: "空分东侧动火", Source: "import-1", UpdatedAt: 1},
			{WpTypeID: 2, WpTypeName: "一级动火", Area: "空分东侧", WorkDescription: "焊接支架", FieldDescription: "作业内容描述", FieldName: "sqxx.1.value", FieldValue: "焊接支架", Source: "import-1", UpdatedAt: 1},
			{WpTypeID: 2, WpTypeName: "一级动火", Area: "空分东侧", WorkDescription: "焊接支架", FieldDescription: "所属区域", FieldName: "sqxx.2.value", FieldValue: []any{map[string]any{"id": 1, "name": "空分东侧"}}, Source: "import-1", UpdatedAt: 1},
		},
	}
	server := NewWorkPermitServer(nil, nil, store)

	values := url.Values{}
	values.Set("wp_type_id", "2")
	values.Set("area", "空分东侧")
	rec := get(server, "/api/structured-memory/export?"+values.Encode())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Status   string           `json:"status"`
		Payloads []map[string]any `json:"payloads"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "completed" {
		t.Fatalf("status = %q", response.Status)
	}
	if len(response.Payloads) != 1 {
		t.Fatalf("payloads = %#v", response.Payloads)
	}
	sqxx, ok := response.Payloads[0]["sqxx"].([]any)
	if !ok || len(sqxx) != 3 {
		t.Fatalf("sqxx = %#v", response.Payloads[0]["sqxx"])
	}
}

func postJSON(server *WorkPermitServer, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/work-permits", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	return rec
}

func postJSONTo(server *WorkPermitServer, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	return rec
}

func get(server *WorkPermitServer, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	return rec
}

func TestWorkPermitHandlerRejectsMissingSiteOrUserID(t *testing.T) {
	t.Parallel()

	server := NewWorkPermitServer(&recordingRunner{}, nil)
	rec := postJSON(server, `{"input":"initial work description"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "site_id and user_id are required") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestWorkPermitHandlerPassesSiteAndUserIDFromRequest(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	runner.call = func(ctx context.Context, input *graph.WpGraphState, opts ...compose.Option) (*graph.WpGraphState, error) {
		return &graph.WpGraphState{SessionID: input.SessionID}, nil
	}

	server := NewWorkPermitServer(runner, nil)
	rec := postJSON(server, `{"input":"initial work description","site_id":123,"user_id":456}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(runner.inputs) != 1 {
		t.Fatalf("inputs = %#v", runner.inputs)
	}

	state, ok := runner.inputs[0].(*graph.WpGraphState)
	if !ok {
		t.Fatalf("input type = %T", runner.inputs[0])
	}
	if state.SiteID != 123 || state.UserID != 456 {
		t.Fatalf("state IDs = (%d, %d)", state.SiteID, state.UserID)
	}
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder) workPermitResponse {
	t.Helper()
	var response workPermitResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}

type inMemoryCheckPointStore struct {
	data map[string][]byte
}

func (s *inMemoryCheckPointStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	data, ok := s.data[checkPointID]
	return data, ok, nil
}

func (s *inMemoryCheckPointStore) Set(ctx context.Context, checkPointID string, data []byte) error {
	s.data[checkPointID] = data
	return nil
}

func newInterruptError(t *testing.T, info any) error {
	t.Helper()

	builder := compose.NewGraph[*graph.WpGraphState, *graph.WpGraphState]()
	builder.AddLambdaNode("interrupt", compose.InvokableLambda(func(ctx context.Context, state *graph.WpGraphState) (*graph.WpGraphState, error) {
		return nil, compose.StatefulInterrupt(ctx, info, state)
	}))
	builder.AddEdge(compose.START, "interrupt")
	builder.AddEdge("interrupt", compose.END)

	runner, err := builder.Compile(
		context.Background(),
		compose.WithCheckPointStore(&inMemoryCheckPointStore{data: map[string][]byte{}}),
		compose.WithGraphName("httpapi-test"),
	)
	if err != nil {
		t.Fatalf("compile interrupt graph: %v", err)
	}
	_, err = runner.Invoke(context.Background(), &graph.WpGraphState{}, compose.WithCheckPointID("interrupt-test"))
	if err == nil {
		t.Fatal("interrupt graph error = nil")
	}
	return err
}
