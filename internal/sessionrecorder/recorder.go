package sessionrecorder

import (
	"context"
	"errors"
)

var ErrMissingSessionID = errors.New("sessionrecorder: missing session ID")

type Event struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Transcript []Event

type Recorder struct {
	store *fileStore
}

func NewRecorder(dir string) (*Recorder, error) {
	store, err := newFileStore(dir)
	if err != nil {
		return nil, err
	}

	return &Recorder{store: store}, nil
}

func (r *Recorder) AppendEvent(ctx context.Context, event Event) error {
	sessionID, err := sessionIDFromContext(ctx)
	if err != nil {
		return err
	}

	record, err := r.store.load(sessionID)
	if err != nil {
		return err
	}

	record.Transcript = append(record.Transcript, event)
	return r.store.save(record)
}

func (r *Recorder) MarkStatus(ctx context.Context, status string) error {
	sessionID, err := sessionIDFromContext(ctx)
	if err != nil {
		return err
	}

	record, err := r.store.load(sessionID)
	if err != nil {
		return err
	}

	record.Status = status
	return r.store.save(record)
}

func sessionIDFromContext(ctx context.Context) (string, error) {
	sessionID, ok := SessionIDFromContext(ctx)
	if !ok {
		return "", ErrMissingSessionID
	}
	return sessionID, nil
}
