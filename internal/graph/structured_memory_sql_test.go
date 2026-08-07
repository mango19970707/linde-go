package graph

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
)

func TestSQLiteStructuredMemoryStoreQueriesByTypeAndArea(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStructuredMemoryStore(db)
	if err != nil {
		t.Fatalf("NewSQLiteStructuredMemoryStore() error = %v", err)
	}

	insertStructuredMemoryEntry(t, db, StructuredMemoryEntry{
		WpTypeID:        23,
		WpTypeName:      "作业许可证",
		Area:            "空分装置",
		WorkDescription: "动火检修",
		FieldName:       "sqxx.6.value",
		FieldValue:      "空分装置北侧平台",
		Source:          "sample-a",
		UpdatedAt:       10,
	})
	insertStructuredMemoryEntry(t, db, StructuredMemoryEntry{
		WpTypeID:        23,
		WpTypeName:      "作业许可证",
		Area:            "空分装置",
		WorkDescription: "动火检修",
		FieldName:       "grfh.0.value",
		FieldValue:      []any{map[string]any{"id": 1, "name": "是"}},
		Source:          "sample-a",
		UpdatedAt:       11,
	})
	insertStructuredMemoryEntry(t, db, StructuredMemoryEntry{
		WpTypeID:        23,
		WpTypeName:      "作业许可证",
		Area:            "压缩机厂房",
		WorkDescription: "巡检",
		FieldName:       "sqxx.6.value",
		FieldValue:      "压缩机厂房东侧",
		Source:          "sample-b",
		UpdatedAt:       12,
	})

	entries, err := store.FindByTypeAndArea(context.Background(), 23, "空分装置")
	if err != nil {
		t.Fatalf("FindByTypeAndArea() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d", len(entries))
	}

	areas, err := store.ListAreasByType(context.Background(), 23)
	if err != nil {
		t.Fatalf("ListAreasByType() error = %v", err)
	}
	if len(areas) != 2 || areas[0] != "压缩机厂房" || areas[1] != "空分装置" {
		t.Fatalf("areas = %#v", areas)
	}

	allEntries, err := store.FindByType(context.Background(), 23)
	if err != nil {
		t.Fatalf("FindByType() error = %v", err)
	}
	if len(allEntries) != 3 {
		t.Fatalf("len(allEntries) = %d", len(allEntries))
	}
}

func insertStructuredMemoryEntry(t *testing.T, db *sql.DB, entry StructuredMemoryEntry) {
	t.Helper()

	fieldValueJSON, err := json.Marshal(entry.FieldValue)
	if err != nil {
		t.Fatalf("json.Marshal(field value) error = %v", err)
	}

	if _, err := db.Exec(`
INSERT INTO structured_memory_entries (
	wp_type_id, wp_type_name, area, work_description, field_name, field_value_json, source, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, entry.WpTypeID, entry.WpTypeName, entry.Area, entry.WorkDescription, entry.FieldName, string(fieldValueJSON), entry.Source, entry.UpdatedAt); err != nil {
		t.Fatalf("insert structured memory entry error = %v", err)
	}
}
