# Work Permit Session JSON Transcript Design

## Goal

Persist each `/api/work-permits` session as a JSON transcript under `D:/code/linde-go-v2/session`, capturing the full session timeline across HTTP requests, graph interrupts and resumes, LLM messages, system messages, tool calls, and tool outputs.

## Scope

- Persist one JSON file per `session_id`.
- Incrementally update the JSON file whenever a new session event is recorded.
- Write a final snapshot when the session becomes `interrupted`, `completed`, or `failed`.
- Capture all message classes needed by the work permit flow:
  - `system_message`
  - `user_message`
  - `assistant_message`
  - `tool_call`
  - `tool_output`
  - `http_request`
  - `http_response`
  - `interrupt`
  - `resume`
  - `error`

## Non-Goals

- Do not add Markdown export.
- Do not persist checkpoint state for cross-process resume.
- Do not redesign the work permit graph flow itself.
- Do not add a transcript query API in this change.

## Architecture

### Session Recorder

Add a dedicated recorder component responsible for collecting ordered session events and persisting them to disk.

Suggested package layout:

- `internal/sessionrecorder/context.go`
- `internal/sessionrecorder/recorder.go`
- `internal/sessionrecorder/file_store.go`

Responsibilities:

- `context.go`
  - Attach and read `session_id` from `context.Context`.
- `recorder.go`
  - Define transcript and event types.
  - Append events in order.
  - Track transcript status and timestamps.
- `file_store.go`
  - Ensure the session directory exists.
  - Atomically write JSON transcripts using a temporary file plus rename.

### Transcript Model

Persist each session file as:

```json
{
  "session_id": "string",
  "status": "running|interrupted|completed|failed",
  "created_at": "RFC3339 timestamp",
  "updated_at": "RFC3339 timestamp",
  "events": [
    {
      "id": "string",
      "session_id": "string",
      "timestamp": "RFC3339 timestamp",
      "source": "http|graph|llm|tool|system",
      "type": "system_message|user_message|assistant_message|tool_call|tool_output|http_request|http_response|interrupt|resume|error",
      "name": "optional component name",
      "content": "structured payload or text",
      "metadata": {}
    }
  ]
}
```

The `content` field may contain text or structured JSON-safe values. `metadata` stores secondary details such as checkpoint ID, interrupt ID, operation name, model name, tool name, HTTP status code, or graph node.

## Integration Points

### HTTP Layer

Update [work_permit.go](D:/code/linde-go-v2/internal/httpapi/work_permit.go) so the server records:

- initial request payload as `http_request`
- resume request payload as `resume`
- interrupt payload as `interrupt`
- successful API response as `http_response`
- run failures and validation failures as `error`

Before invoking the runner, inject `session_id` into the request context so downstream callbacks can append to the same transcript.

### Graph / Model / Tool Layer

Attach Eino callbacks when constructing the work permit graph runner so downstream components emit transcript events for:

- system prompt messages
- user prompt messages
- model responses
- tool calls
- tool outputs

The callback layer should read `session_id` from `context.Context` and append events through the shared recorder. If no `session_id` is present, the callback should no-op instead of failing the work permit flow.

## Concurrency and Persistence

- Keep a top-level recorder map keyed by `session_id`.
- Use a per-session mutex so concurrent writes within one session remain ordered.
- Allow different sessions to write independently.
- On every appended event:
  - update transcript `updated_at`
  - persist the full transcript JSON to `D:/code/linde-go-v2/session/<session_id>.json`
- On session terminal states:
  - set transcript status to `interrupted`, `completed`, or `failed`
  - write one additional final snapshot

Atomic file write behavior:

1. Marshal the transcript to JSON.
2. Write to a temporary file in the same directory.
3. Rename the temporary file over the target JSON.

This avoids partially written JSON files.

## Status Rules

- Start a new session transcript in `running`.
- Move to `interrupted` when the graph returns an interrupt response.
- Move back to `running` when a resume request is accepted.
- Move to `completed` when the flow finishes successfully.
- Move to `failed` when the runner or request handling returns a non-interrupt error.

## Testing

### Recorder Unit Tests

- appending an event creates a valid transcript file
- repeated appends preserve event order
- status transitions update the file correctly
- the written file is always valid JSON

### HTTP Tests

Extend [work_permit_test.go](D:/code/linde-go-v2/internal/httpapi/work_permit_test.go) to verify:

- an interrupted session creates `session/<session_id>.json`
- the JSON contains `http_request`, `interrupt`, and `http_response`
- a resumed session updates the same file and adds `resume`
- a failed session is marked `failed`

### Graph / Callback Tests

Use fake model and fake tool components to verify the transcript file includes:

- `system_message`
- `user_message`
- `assistant_message`
- `tool_call`
- `tool_output`

## Error Handling

- If transcript persistence fails, return an internal error only for HTTP-layer recording steps that are required to produce the requested artifact.
- Callback-level recording failures must not break the work permit business flow; log or collect them internally and continue.
- Missing `session_id` in callback context must degrade safely to no-op recording.

## Acceptance Criteria

- Every `/api/work-permits` session produces exactly one JSON transcript file at `D:/code/linde-go-v2/session/<session_id>.json`.
- The transcript preserves ordered events from the same session across interrupt and resume cycles.
- The JSON includes all required message types: system, user, assistant/LLM, tool call, tool output, and HTTP/session control events.
- Files are updated incrementally during the session and refreshed again on terminal state transitions.
- The implementation does not add cross-process resume support.
