package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type FileStructuredMemoryStore struct {
	entries []StructuredMemoryEntry
}

func LoadStructuredMemoryStore(path string) (StructuredMemoryStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read structured memory %s: %w", path, err)
	}

	var entries []StructuredMemoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("unmarshal structured memory %s: %w", path, err)
	}

	return FileStructuredMemoryStore{entries: entries}, nil
}

func (s FileStructuredMemoryStore) Lookup(ctx context.Context, wpTypeID int) ([]StructuredMemoryEntry, error) {
	entries := make([]StructuredMemoryEntry, 0)
	for _, entry := range s.entries {
		if entry.WpTypeID == wpTypeID {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}
