package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	server := NewWorkPermitServer(runner, 28, 464)
	rec := postJSON(server, `{"input":"initial work description"}`)

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

	server := NewWorkPermitServer(runner, 28, 464)
	first := postJSON(server, `{"input":"initial work description"}`)
	firstResponse := decodeResponse(t, first)

	rec := postJSON(server, `{"session_id":"`+firstResponse.SessionID+`","input":"start at 08:00"}`)
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

	server := NewWorkPermitServer(runner, 28, 464)
	rec := postJSON(server, `{"input":"complete work description"}`)
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

	server := NewWorkPermitServer(runner, 28, 464)
	first := postJSON(server, `{"input":"complete work description"}`)
	firstResponse := decodeResponse(t, first)

	rec := postJSON(server, `{"session_id":"`+firstResponse.SessionID+`","approved":true,"payloads":[{"id":0,"wptypeid":23}]}`)
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

func postJSON(server *WorkPermitServer, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/work-permits", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	return rec
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
