package httpapi

import (
	"context"
	"sync"
)

type MemoryCheckPointStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func NewMemoryCheckPointStore() *MemoryCheckPointStore {
	return &MemoryCheckPointStore{data: make(map[string][]byte)}
}

func (s *MemoryCheckPointStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.data[checkPointID]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), data...), true, nil
}

func (s *MemoryCheckPointStore) Set(ctx context.Context, checkPointID string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[checkPointID] = append([]byte(nil), data...)
	return nil
}
