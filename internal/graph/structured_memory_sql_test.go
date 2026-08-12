package graph

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSQLiteIntentStructuredMemoryStoreQueriesByDescriptionSimilarity(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "intent_memory.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStructuredMemoryStore(db)
	if err != nil {
		t.Fatalf("NewSQLiteStructuredMemoryStore() error = %v", err)
	}

	if err := store.UpsertIntentMemory(context.Background(), StructuredIntentMemoryEntry{
		SiteID:          28,
		Area:            "A区",
		WorkDescription: "动火作业",
		WpTypeIDs:       []int{23, 1},
		Source:          "test",
		UpdatedAt:       1,
	}); err != nil {
		t.Fatalf("UpsertIntentMemory() error = %v", err)
	}

	entries, err := store.FindIntentMemoryBySite(context.Background(), 28)
	if err != nil {
		t.Fatalf("FindIntentMemoryBySite() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].WorkDescription != "动火作业" {
		t.Fatalf("WorkDescription = %q", entries[0].WorkDescription)
	}
	if len(entries[0].WpTypeIDs) != 2 || entries[0].WpTypeIDs[0] != 23 || entries[0].WpTypeIDs[1] != 1 {
		t.Fatalf("WpTypeIDs = %#v", entries[0].WpTypeIDs)
	}
}

func TestSQLiteStructuredMemoryStoreQueriesByTypeAndArea(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "structured_memory.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStructuredMemoryStore(db)
	if err != nil {
		t.Fatalf("NewSQLiteStructuredMemoryStore() error = %v", err)
	}

	area := "空分装置东侧冷箱平台"
	seedEntries := []StructuredMemoryEntry{
		{WpTypeID: 23, WpTypeName: "PTW800", Area: area, WorkDescription: "动火", FieldDescription: "地点", FieldName: "sqxx.0.value", FieldValue: "冷箱平台", Source: "seed-1", UpdatedAt: 1},
		{WpTypeID: 23, WpTypeName: "PTW800", Area: area, WorkDescription: "动火", FieldDescription: "电话", FieldName: "sqxx.1.value", FieldValue: "13800138000", Source: "seed-1", UpdatedAt: 1},
		{WpTypeID: 23, WpTypeName: "PTW800", Area: "空分装置西侧", WorkDescription: "受限空间", FieldDescription: "地点", FieldName: "sqxx.0.value", FieldValue: "塔下", Source: "seed-2", UpdatedAt: 2},
	}
	for _, entry := range seedEntries {
		if err := store.Upsert(context.Background(), entry); err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
	}

	entries, err := store.FindByTypeAndArea(context.Background(), 23, area)
	if err != nil {
		t.Fatalf("FindByTypeAndArea() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}

	areas, err := store.ListAreasByType(context.Background(), 23)
	if err != nil {
		t.Fatalf("ListAreasByType() error = %v", err)
	}
	if len(areas) != 2 {
		t.Fatalf("len(areas) = %d, want 2", len(areas))
	}
	if areas[0] != "空分装置东侧冷箱平台" || areas[1] != "空分装置西侧" {
		t.Fatalf("areas = %#v", areas)
	}

	allEntries, err := store.FindByType(context.Background(), 23)
	if err != nil {
		t.Fatalf("FindByType() error = %v", err)
	}
	if len(allEntries) != 3 {
		t.Fatalf("len(allEntries) = %d, want 3", len(allEntries))
	}
}
