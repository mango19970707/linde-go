package httpapi

import (
	"context"

	"linde-go-v2/internal/graph"
)

type EmptyStructuredMemoryStore struct{}

func (EmptyStructuredMemoryStore) Lookup(ctx context.Context, wpTypeID int) ([]graph.StructuredMemoryEntry, error) {
	return nil, nil
}
