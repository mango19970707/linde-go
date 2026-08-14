package sessionrecorder

import (
	"context"
	"errors"
	"sync"
)

var ErrMissingSessionID = errors.New("sessionrecorder: missing session ID")
var ErrInvalidSessionID = errors.New("sessionrecorder: invalid session ID")

type Event struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Transcript []Event

type Recorder struct {
	store    *fileStore
	mu       sync.Mutex
	sessions map[string]*sessionSnapshot
}

type sessionSnapshot struct {
	mu     sync.Mutex
	record sessionRecord
	loaded bool
}

func NewRecorder(dir string) (*Recorder, error) {
	store, err := newFileStore(dir)
	if err != nil {
		return nil, err
	}

	return &Recorder{
		store:    store,
		sessions: make(map[string]*sessionSnapshot),
	}, nil
}

func (r *Recorder) AppendEvent(ctx context.Context, event Event) error {
	sessionID, err := sessionIDFromContext(ctx)
	if err != nil {
		return err
	}

	return r.updateSession(sessionID, func(record *sessionRecord) error {
		record.Transcript = append(record.Transcript, event)
		return nil
	})
}

func (r *Recorder) MarkStatus(ctx context.Context, status string) error {
	sessionID, err := sessionIDFromContext(ctx)
	if err != nil {
		return err
	}

	return r.updateSession(sessionID, func(record *sessionRecord) error {
		record.Status = status
		return nil
	})
}

func (r *Recorder) updateSession(sessionID string, update func(*sessionRecord) error) error {
	snapshot, err := r.getSessionSnapshot(sessionID)
	if err != nil {
		return err
	}

	snapshot.mu.Lock()
	defer snapshot.mu.Unlock()

	if !snapshot.loaded {
		record, err := r.store.load(sessionID)
		if err != nil {
			return err
		}
		snapshot.record = record
		snapshot.loaded = true
	}

	if err := update(&snapshot.record); err != nil {
		return err
	}

	return r.store.save(snapshot.record)
}

func (r *Recorder) getSessionSnapshot(sessionID string) (*sessionSnapshot, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	snapshot, ok := r.sessions[sessionID]
	if !ok {
		snapshot = &sessionSnapshot{}
		r.sessions[sessionID] = snapshot
	}
	return snapshot, nil
}

func sessionIDFromContext(ctx context.Context) (string, error) {
	sessionID, ok := SessionIDFromContext(ctx)
	if !ok {
		return "", ErrMissingSessionID
	}
	return sessionID, nil
}
