package sessionrecorder

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
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
	if got.Transcript[0] != (persistedEvent{Role: "assistant", Content: "need more details"}) {
		t.Fatalf("transcript[0] = %#v", got.Transcript[0])
	}
}

func TestRecorderRejectsUnsafeSessionID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	recorder, err := NewRecorder(dir)
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	ctx := WithSessionID(context.Background(), "../escape")
	err = recorder.AppendEvent(ctx, Event{
		Role:    "user",
		Content: "start permit",
	})
	if err == nil {
		t.Fatal("AppendEvent() error = nil")
	}
}

func TestRecorderConcurrentAppendAndStatusDoNotDropEvents(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	recorder, err := NewRecorder(dir)
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	const eventCount = 16
	ctx := WithSessionID(context.Background(), "session-concurrent")

	var wg sync.WaitGroup
	wg.Add(eventCount + 1)

	for i := range eventCount {
		go func(index int) {
			defer wg.Done()
			if err := recorder.AppendEvent(ctx, Event{
				Role:    "user",
				Content: "event",
			}); err != nil {
				t.Errorf("AppendEvent(%d) error = %v", index, err)
			}
		}(i)
	}

	go func() {
		defer wg.Done()
		if err := recorder.MarkStatus(ctx, "completed"); err != nil {
			t.Errorf("MarkStatus() error = %v", err)
		}
	}()

	wg.Wait()

	got := readSessionRecord(t, dir, "session-concurrent")
	if got.Status != "completed" {
		t.Fatalf("status = %q", got.Status)
	}
	if len(got.Transcript) != eventCount {
		t.Fatalf("len(transcript) = %d", len(got.Transcript))
	}
}

type persistedSessionRecord struct {
	SessionID  string           `json:"session_id"`
	Status     string           `json:"status"`
	Transcript []persistedEvent `json:"transcript"`
}

type persistedEvent struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
