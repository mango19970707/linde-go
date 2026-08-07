package graph

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStructuredMemoryStoreFiltersByWpType(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "structured_memory.json")
	data := `[
  {"wp_type_id":23,"area_id":101,"field_name":"wpplace","field_value":"A","updated_at":1},
  {"wp_type_id":23,"area_id":102,"field_name":"wpplace","field_value":"B","updated_at":2},
  {"wp_type_id":2,"area_id":101,"field_name":"wpplace","field_value":"C","updated_at":3}
]`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write memory file: %v", err)
	}

	store, err := LoadStructuredMemoryStore(path)
	if err != nil {
		t.Fatalf("LoadStructuredMemoryStore() error = %v", err)
	}

	entries, err := store.Lookup(context.Background(), 23)
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if len(entries) != 2 || entries[0].FieldValue != "A" || entries[1].FieldValue != "B" {
		t.Fatalf("entries = %#v", entries)
	}
}
