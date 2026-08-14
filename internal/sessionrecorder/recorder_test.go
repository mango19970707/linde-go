package sessionrecorder

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecorderPersistsTranscriptEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	recorder, err := NewRecorder(dir)
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	ctx := WithSessionID(context.Background(), "session-123")
	err = recorder.AppendEvent(ctx, Event{
		Role:    "user",
		Content: "start permit",
	})
	if err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	got := readSessionRecord(t, dir, "session-123")
	if got.SessionID != "session-123" {
		t.Fatalf("session_id = %q", got.SessionID)
	}
	if len(got.Transcript) != 1 {
		t.Fatalf("transcript = %#v", got.Transcript)
	}
	if got.Transcript[0].Role != "user" || got.Transcript[0].Content != "start permit" {
		t.Fatalf("transcript[0] = %#v", got.Transcript[0])
	}
}

func TestRecorderUpdatesStatusWithoutDroppingTranscript(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	recorder, err := NewRecorder(dir)
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	ctx := WithSessionID(context.Background(), "session-123")
	if err := recorder.AppendEvent(ctx, Event{
		Role:    "assistant",
		Content: "need more details",
	}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	if err := recorder.MarkStatus(ctx, "need_more_info"); err != nil {
		t.Fatalf("MarkStatus() error = %v", err)
	}

	got := readSessionRecord(t, dir, "session-123")
	if got.Status != "need_more_info" {
		t.Fatalf("status = %q", got.Status)
	}
	if len(got.Transcript) != 1 {
		t.Fatalf("transcript = %#v", got.Transcript)
	}
	if got.Transcript[0].Content != "need more details" {
		t.Fatalf("transcript[0] = %#v", got.Transcript[0])
	}
}

type persistedSessionRecord struct {
	SessionID  string  `json:"session_id"`
	Status     string  `json:"status"`
	Transcript []Event `json:"transcript"`
}

func readSessionRecord(t *testing.T, dir, sessionID string) persistedSessionRecord {
	t.Helper()

	path := filepath.Join(dir, sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	var record persistedSessionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", path, err)
	}
	return record
}
