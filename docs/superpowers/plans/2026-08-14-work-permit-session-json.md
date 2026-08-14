# Work Permit Session JSON Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist each `/api/work-permits` session as a JSON transcript in `D:/code/linde-go-v2/session`, including HTTP events, interrupt/resume state, system messages, user messages, assistant messages, tool calls, and tool outputs.

**Architecture:** Add a dedicated `internal/sessionrecorder` package that owns transcript state, context propagation, and atomic JSON writes. Wire the recorder into `internal/httpapi/work_permit.go` for HTTP/session events and into the existing `internal/graph/graph.go` callback handler so LLM/tool events append into the same session file using `session_id` from `context.Context`.

**Tech Stack:** Go, Gin, Eino compose callbacks, JSON file persistence, Go tests.

---

### Task 1: Add failing recorder unit tests

**Files:**
- Create: `internal/sessionrecorder/recorder_test.go`
- Create: `internal/sessionrecorder/context_test.go`

- [ ] **Step 1: Write the failing transcript persistence tests**

```go
func TestRecorderAppendEventPersistsJSONTranscript(t *testing.T) {
	dir := t.TempDir()
	recorder := NewRecorder(dir)

	err := recorder.AppendEvent("session-1", Event{
		Source:  SourceHTTP,
		Type:    EventTypeHTTPRequest,
		Name:    "POST /api/work-permits",
		Content: map[string]any{"input": "job"},
	})
	if err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	path := filepath.Join(dir, "session-1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var transcript Transcript
	if err := json.Unmarshal(data, &transcript); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if transcript.SessionID != "session-1" || len(transcript.Events) != 1 {
		t.Fatalf("transcript = %#v", transcript)
	}
}

func TestRecorderMarkStatusUpdatesExistingTranscript(t *testing.T) {
	dir := t.TempDir()
	recorder := NewRecorder(dir)

	if err := recorder.AppendEvent("session-1", Event{Source: SourceHTTP, Type: EventTypeHTTPRequest}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	if err := recorder.MarkStatus("session-1", StatusInterrupted); err != nil {
		t.Fatalf("MarkStatus() error = %v", err)
	}

	transcript := loadTranscriptForTest(t, filepath.Join(dir, "session-1.json"))
	if transcript.Status != StatusInterrupted {
		t.Fatalf("status = %q", transcript.Status)
	}
}
```

- [ ] **Step 2: Write the failing context propagation test**

```go
func TestContextRoundTripSessionID(t *testing.T) {
	ctx := WithSessionID(context.Background(), "session-ctx")

	sessionID, ok := SessionIDFromContext(ctx)
	if !ok {
		t.Fatal("SessionIDFromContext() ok = false")
	}
	if sessionID != "session-ctx" {
		t.Fatalf("sessionID = %q", sessionID)
	}
}
```

- [ ] **Step 3: Run the focused tests to verify they fail**

Run:

```bash
go test ./internal/sessionrecorder -run 'TestRecorder|TestContext' -count=1
```

Expected: FAIL because `NewRecorder`, `AppendEvent`, `MarkStatus`, `Transcript`, `WithSessionID`, and `SessionIDFromContext` do not exist yet.

- [ ] **Step 4: Commit**

```bash
git add internal/sessionrecorder/recorder_test.go internal/sessionrecorder/context_test.go
git commit -m "test: add failing session recorder tests"
```

### Task 2: Implement the session recorder package

**Files:**
- Create: `internal/sessionrecorder/context.go`
- Create: `internal/sessionrecorder/recorder.go`
- Create: `internal/sessionrecorder/file_store.go`
- Test: `internal/sessionrecorder/recorder_test.go`
- Test: `internal/sessionrecorder/context_test.go`

- [ ] **Step 1: Add the minimal context helper implementation**

```go
type sessionIDKey struct{}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, strings.TrimSpace(sessionID))
}

func SessionIDFromContext(ctx context.Context) (string, bool) {
	value, _ := ctx.Value(sessionIDKey{}).(string)
	value = strings.TrimSpace(value)
	return value, value != ""
}
```

- [ ] **Step 2: Add transcript, event, and recorder types**

```go
type Status string

const (
	StatusRunning     Status = "running"
	StatusInterrupted Status = "interrupted"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
)

type Event struct {
	ID        string         `json:"id"`
	SessionID string         `json:"session_id"`
	Timestamp time.Time      `json:"timestamp"`
	Source    string         `json:"source"`
	Type      string         `json:"type"`
	Name      string         `json:"name,omitempty"`
	Content   any            `json:"content,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type Transcript struct {
	SessionID string    `json:"session_id"`
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Events    []Event   `json:"events"`
}

type Recorder struct {
	rootDir  string
	mu       sync.Mutex
	sessions map[string]*sessionTranscript
}
```

- [ ] **Step 3: Implement append, status transitions, and atomic file writes**

```go
func (r *Recorder) AppendEvent(sessionID string, event Event) error {
	transcript := r.getOrCreate(sessionID)

	transcript.mu.Lock()
	defer transcript.mu.Unlock()

	now := time.Now().UTC()
	if transcript.data.CreatedAt.IsZero() {
		transcript.data.CreatedAt = now
		transcript.data.Status = StatusRunning
	}
	event.ID = newEventID()
	event.SessionID = sessionID
	event.Timestamp = now
	transcript.data.SessionID = sessionID
	transcript.data.UpdatedAt = now
	transcript.data.Events = append(transcript.data.Events, event)
	return writeTranscriptFile(r.rootDir, transcript.data)
}

func (r *Recorder) MarkStatus(sessionID string, status Status) error {
	transcript := r.getOrCreate(sessionID)

	transcript.mu.Lock()
	defer transcript.mu.Unlock()

	now := time.Now().UTC()
	if transcript.data.CreatedAt.IsZero() {
		transcript.data.CreatedAt = now
		transcript.data.SessionID = sessionID
	}
	transcript.data.Status = status
	transcript.data.UpdatedAt = now
	return writeTranscriptFile(r.rootDir, transcript.data)
}

func writeTranscriptFile(rootDir string, transcript Transcript) error {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(rootDir, transcript.SessionID+".json")
	tmp := target + ".tmp"
	data, err := json.MarshalIndent(transcript, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}
```

- [ ] **Step 4: Run the recorder tests and verify they pass**

Run:

```bash
go test ./internal/sessionrecorder -run 'TestRecorder|TestContext' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sessionrecorder/context.go internal/sessionrecorder/recorder.go internal/sessionrecorder/file_store.go internal/sessionrecorder/recorder_test.go internal/sessionrecorder/context_test.go
git commit -m "feat: add session transcript recorder"
```

### Task 3: Add failing HTTP transcript tests

**Files:**
- Modify: `internal/httpapi/work_permit_test.go`

- [ ] **Step 1: Add the failing interrupted-session transcript test**

```go
func TestWorkPermitHandlerWritesInterruptedTranscriptJSON(t *testing.T) {
	dir := t.TempDir()
	runner := &recordingRunner{}
	interruptErr := newInterruptError(t, map[string]any{
		"operation":      "collect_wp_info",
		"missing_fields": []string{"start_time"},
		"question":       "please provide start time",
	})
	runner.call = func(ctx context.Context, input *graph.WpGraphState, opts ...compose.Option) (*graph.WpGraphState, error) {
		return nil, interruptErr
	}

	server := NewWorkPermitServer(runner, nil, nil, NewRecorder(dir))
	rec := postJSON(server, `{"input":"initial work description","site_id":28,"user_id":464}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	response := decodeResponse(t, rec)
	transcript := loadHTTPTranscript(t, filepath.Join(dir, response.SessionID+".json"))
	if transcript.Status != "interrupted" {
		t.Fatalf("status = %q", transcript.Status)
	}
	if !containsEventType(transcript.Events, "http_request") || !containsEventType(transcript.Events, "interrupt") {
		t.Fatalf("events = %#v", transcript.Events)
	}
}
```

- [ ] **Step 2: Add the failing resume-and-complete transcript test**

```go
func TestWorkPermitHandlerUpdatesTranscriptAcrossResume(t *testing.T) {
	dir := t.TempDir()
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
		return &graph.WpGraphState{SessionID: input.SessionID}, nil
	}

	server := NewWorkPermitServer(runner, nil, nil, NewRecorder(dir))
	first := postJSON(server, `{"input":"initial work description","site_id":28,"user_id":464}`)
	sessionID := decodeResponse(t, first).SessionID
	second := postJSON(server, `{"session_id":"`+sessionID+`","input":"start at 08:00","site_id":28,"user_id":464}`)

	if second.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", second.Code, second.Body.String())
	}
	transcript := loadHTTPTranscript(t, filepath.Join(dir, sessionID+".json"))
	if transcript.Status != "completed" {
		t.Fatalf("status = %q", transcript.Status)
	}
	if !containsEventType(transcript.Events, "resume") || !containsEventType(transcript.Events, "http_response") {
		t.Fatalf("events = %#v", transcript.Events)
	}
}
```

- [ ] **Step 3: Run the HTTP-focused tests to verify they fail**

Run:

```bash
go test ./internal/httpapi -run 'TestWorkPermitHandlerWritesInterruptedTranscriptJSON|TestWorkPermitHandlerUpdatesTranscriptAcrossResume' -count=1
```

Expected: FAIL because `NewRecorder` is not in scope for the server yet, `NewWorkPermitServer` does not accept a recorder, and no transcript file is written.

- [ ] **Step 4: Commit**

```bash
git add internal/httpapi/work_permit_test.go
git commit -m "test: add failing http transcript tests"
```

### Task 4: Wire the recorder into the HTTP work permit server

**Files:**
- Modify: `internal/httpapi/work_permit.go`
- Modify: `internal/httpapi/work_permit_test.go`
- Modify: `main.go`
- Test: `internal/httpapi/work_permit_test.go`

- [ ] **Step 1: Extend the server to accept and store a recorder**

```go
type WorkPermitServer struct {
	runner      WorkPermitRunner
	promptStore graph.PromptStore
	memoryStore graph.StructuredMemoryStore
	recorder    *sessionrecorder.Recorder

	mu       sync.Mutex
	sessions map[string]sessionState
}

func NewWorkPermitServer(runner WorkPermitRunner, args ...any) *WorkPermitServer {
	var promptStore graph.PromptStore
	var memoryStore graph.StructuredMemoryStore
	var recorder *sessionrecorder.Recorder

	for _, arg := range args {
		switch value := arg.(type) {
		case graph.PromptStore:
			promptStore = value
		case graph.StructuredMemoryStore:
			memoryStore = value
		case *sessionrecorder.Recorder:
			recorder = value
		}
	}

	return &WorkPermitServer{
		runner:      runner,
		promptStore: promptStore,
		memoryStore: memoryStore,
		recorder:    recorder,
		sessions:    make(map[string]sessionState),
	}
}
```

- [ ] **Step 2: Record request, resume, response, interrupt, and error events**

```go
func (s *WorkPermitServer) recordEvent(sessionID string, event sessionrecorder.Event) {
	if s.recorder == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	if err := s.recorder.AppendEvent(sessionID, event); err != nil {
		slog.Error("wp_session_record_error", slog.String("session_id", sessionID), slog.String("err", err.Error()))
	}
}

func (s *WorkPermitServer) handleInitialRequest(c *gin.Context, request workPermitRequest) {
	// ...
	s.recordEvent(sessionID, sessionrecorder.Event{
		Source:  sessionrecorder.SourceHTTP,
		Type:    sessionrecorder.EventTypeHTTPRequest,
		Name:    "POST /api/work-permits",
		Content: request,
	})
	ctx := sessionrecorder.WithSessionID(c.Request.Context(), sessionID)
	output, err := s.runner.Invoke(ctx, state, compose.WithCheckPointID(checkpointID))
	s.writeRunResult(c, sessionID, checkpointID, output, err)
}
```

- [ ] **Step 3: Mark transcript status transitions**

```go
if info, ok := compose.ExtractInterruptInfo(runErr); ok && len(info.InterruptContexts) > 0 {
	// ...
	s.recordEvent(sessionID, sessionrecorder.Event{
		Source: sessionrecorder.SourceGraph,
		Type:   sessionrecorder.EventTypeInterrupt,
		Name:   operation,
		Content: map[string]any{
			"question":       question,
			"missing_fields": missingFields,
			"intent_wp_types": intentWpTypes,
		},
	})
	_ = s.recorder.MarkStatus(sessionID, sessionrecorder.StatusInterrupted)
	// ...
}

if runErr != nil {
	s.recordEvent(sessionID, sessionrecorder.Event{
		Source:  sessionrecorder.SourceHTTP,
		Type:    sessionrecorder.EventTypeError,
		Name:    "runner.invoke",
		Content: runErr.Error(),
	})
	_ = s.recorder.MarkStatus(sessionID, sessionrecorder.StatusFailed)
}

s.recordEvent(sessionID, sessionrecorder.Event{
	Source:  sessionrecorder.SourceHTTP,
	Type:    sessionrecorder.EventTypeHTTPResponse,
	Name:    "POST /api/work-permits",
	Content: response,
})
_ = s.recorder.MarkStatus(sessionID, sessionrecorder.StatusCompleted)
```

- [ ] **Step 4: Construct the recorder in `main.go` and pass it to the server**

```go
transcriptRecorder := sessionrecorder.NewRecorder("session")
server := httpapi.NewWorkPermitServer(runner, promptStore, structuredMemoryStore, transcriptRecorder)
```

- [ ] **Step 5: Run the HTTP tests and verify they pass**

Run:

```bash
go test ./internal/httpapi -run 'TestWorkPermitHandlerWritesInterruptedTranscriptJSON|TestWorkPermitHandlerUpdatesTranscriptAcrossResume|TestWorkPermitHandlerReturnsNeedMoreInfo|TestWorkPermitHandlerResumesWithSupplement' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/work_permit.go internal/httpapi/work_permit_test.go main.go
git commit -m "feat: persist http work permit session transcripts"
```

### Task 5: Add failing graph callback transcript tests

**Files:**
- Create: `internal/graph/session_callback_test.go`

- [ ] **Step 1: Add a failing callback transcript test**

```go
func TestBuildWpGraphWritesLLMAndToolTranscriptEvents(t *testing.T) {
	dir := t.TempDir()
	recorder := sessionrecorder.NewRecorder(dir)

	runner, err := BuildWpGraph(
		&recordingWpInfoExtractor{results: map[string]WpInfoExtraction{
			"input": {
				AreaName: "A", StartTime: "2026/08/14-08:00:00", EndTime: "2026/08/14-17:00:00", Description: "maintenance",
			},
		}},
		stubWpTemplateTool{result: `{"success":true,"data":[{"id":243,"templetname":"permit","wptypename":"PTW","wptypeid":23}]}`},
		stubWpTypeRecognizer{},
		&recordingWpTemplateDetailTool{responses: map[int]string{
			243: `{"success":true,"data":{"id":243,"wptypeid":23,"wptypename":"PTW","templetname":"permit","applycontent_object":{"sqxx":[{"name":"discription","type":"input","value":""}]}}}`,
		}},
		stubChatModel{},
		stubPromptStore{},
		stubStructuredMemoryStore{},
		WpFillRules{},
		&recordingUpdateWpInfoTool{},
		&recordingLinkWpInfoTool{},
		false,
		WithSessionRecorder(recorder),
	)
	if err != nil {
		t.Fatalf("BuildWpGraph() error = %v", err)
	}

	ctx := sessionrecorder.WithSessionID(context.Background(), "session-graph")
	_, _ = runner.Invoke(ctx, &WpGraphState{SessionID: "session-graph", UserInput: "input", SiteID: 28, UserID: 464})

	transcript := loadGraphTranscript(t, filepath.Join(dir, "session-graph.json"))
	if !containsGraphEvent(transcript.Events, "assistant_message") || !containsGraphEvent(transcript.Events, "tool_output") {
		t.Fatalf("events = %#v", transcript.Events)
	}
}
```

- [ ] **Step 2: Run the focused graph test to verify it fails**

Run:

```bash
go test ./internal/graph -run TestBuildWpGraphWritesLLMAndToolTranscriptEvents -count=1
```

Expected: FAIL because `WithSessionRecorder` and transcript callback emission do not exist yet.

- [ ] **Step 3: Commit**

```bash
git add internal/graph/session_callback_test.go
git commit -m "test: add failing graph transcript callback test"
```

### Task 6: Extend graph callbacks to emit transcript events

**Files:**
- Modify: `internal/graph/graph.go`
- Create: `internal/graph/session_callback_test.go`
- Test: `internal/graph/trace_test.go`
- Test: `internal/graph/session_callback_test.go`

- [ ] **Step 1: Add a graph option for session recorder injection**

```go
type sessionRecorderOption struct {
	recorder *sessionrecorder.Recorder
}

func WithSessionRecorder(recorder *sessionrecorder.Recorder) any {
	return sessionRecorderOption{recorder: recorder}
}
```

- [ ] **Step 2: Thread the recorder into the existing traceable runner**

```go
type traceableWpGraphRunnable struct {
	inner    compose.Runnable[*WpGraphState, *WpGraphState]
	handler  callbacks.Handler
	recorder *sessionrecorder.Recorder
}

return &traceableWpGraphRunnable{
	inner:    runnable,
	handler:  newWpGraphTraceHandler(recorder),
	recorder: recorder,
}, nil
```

- [ ] **Step 3: Extend the callback handler to append transcript events without breaking the graph**

```go
func newWpGraphTraceHandler(recorder *sessionrecorder.Recorder) callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			recordGraphCallbackEvent(ctx, recorder, info, input, "start")
			return context.WithValue(ctx, wpTraceKey{}, wpTraceMeta{
				traceID: extractTraceID(input),
				startAt: time.Now(),
			})
		}).
		OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			recordGraphOutputEvent(ctx, recorder, info, output)
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			recordGraphErrorEvent(ctx, recorder, info, err)
			return ctx
		}).
		Build()
}
```

- [ ] **Step 4: Map callback payloads into transcript event types**

```go
func recordGraphOutputEvent(ctx context.Context, recorder *sessionrecorder.Recorder, info *callbacks.RunInfo, output callbacks.CallbackOutput) {
	sessionID, ok := sessionrecorder.SessionIDFromContext(ctx)
	if !ok || recorder == nil || info == nil {
		return
	}

	event := sessionrecorder.Event{
		Source: sessionrecorder.SourceGraph,
		Name:   info.Name,
	}
	switch {
	case strings.Contains(strings.ToLower(info.Name), "tool"):
		event.Type = sessionrecorder.EventTypeToolOutput
		event.Source = sessionrecorder.SourceTool
	case strings.Contains(strings.ToLower(info.Name), "chat"):
		event.Type = sessionrecorder.EventTypeAssistantMessage
		event.Source = sessionrecorder.SourceLLM
	default:
		event.Type = sessionrecorder.EventTypeSystemMessage
	}
	event.Content = output
	_ = recorder.AppendEvent(sessionID, event)
}
```

- [ ] **Step 5: Run the graph tests and verify they pass**

Run:

```bash
go test ./internal/graph -run 'TestBuildWpGraphWritesLLMAndToolTranscriptEvents|TestBuildWpGraphLogsWithSessionID' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/graph/graph.go internal/graph/session_callback_test.go internal/graph/trace_test.go
git commit -m "feat: record graph transcript callback events"
```

### Task 7: Verify and finish the full change

**Files:**
- Modify: `docs/superpowers/specs/2026-08-14-work-permit-session-json-design.md`
- Modify: `docs/superpowers/plans/2026-08-14-work-permit-session-json.md`

- [ ] **Step 1: Run formatting on all touched Go files**

Run:

```bash
gofmt -w internal/sessionrecorder/context.go internal/sessionrecorder/file_store.go internal/sessionrecorder/recorder.go internal/sessionrecorder/context_test.go internal/sessionrecorder/recorder_test.go internal/httpapi/work_permit.go internal/httpapi/work_permit_test.go internal/graph/graph.go internal/graph/session_callback_test.go main.go
```

Expected: files are formatted with no output.

- [ ] **Step 2: Run the focused package tests**

Run:

```bash
go test ./internal/sessionrecorder ./internal/httpapi ./internal/graph -count=1
```

Expected: PASS.

- [ ] **Step 3: Run the full test suite**

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 4: Inspect the resulting diff and verify acceptance criteria**

Run:

```bash
git diff -- internal/sessionrecorder internal/httpapi/work_permit.go internal/httpapi/work_permit_test.go internal/graph/graph.go internal/graph/session_callback_test.go main.go
```

Expected:
- only transcript-related changes are present
- `D:/code/linde-go-v2/session/<session_id>.json` is written by the recorder
- interrupted sessions remain readable
- completed sessions retain the same file and final status

- [ ] **Step 5: Commit**

```bash
git add internal/sessionrecorder internal/httpapi/work_permit.go internal/httpapi/work_permit_test.go internal/graph/graph.go internal/graph/session_callback_test.go main.go docs/superpowers/specs/2026-08-14-work-permit-session-json-design.md docs/superpowers/plans/2026-08-14-work-permit-session-json.md
git commit -m "feat: persist work permit sessions as json transcripts"
```
