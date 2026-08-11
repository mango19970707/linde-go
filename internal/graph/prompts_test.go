package graph

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSQLitePromptStoreSeedsDefaultsAndSupportsCRUD(t *testing.T) {
	t.Parallel()

	db := mustOpenSQLiteDB(t, filepath.Join(t.TempDir(), "prompts.db"))
	store, err := NewSQLitePromptStore(db)
	if err != nil {
		t.Fatalf("NewSQLitePromptStore() error = %v", err)
	}

	ctx := context.Background()
	all, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("len(List()) = %d, want 4", len(all))
	}

	got, err := store.Get(ctx, "wp_info_extraction")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Key != "wp_info_extraction" || got.Content == "" {
		t.Fatalf("Get() = %#v", got)
	}

	original := got.Content
	if err := store.Upsert(ctx, PromptDefinition{
		Key:     "wp_info_extraction",
		Title:   "custom title",
		Content: "custom prompt",
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	updated, err := store.Get(ctx, "wp_info_extraction")
	if err != nil {
		t.Fatalf("Get() after Upsert error = %v", err)
	}
	if updated.Content != "custom prompt" {
		t.Fatalf("updated.Content = %q", updated.Content)
	}

	if err := store.Reset(ctx, "wp_info_extraction"); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	reset, err := store.Get(ctx, "wp_info_extraction")
	if err != nil {
		t.Fatalf("Get() after Reset error = %v", err)
	}
	if reset.Content != original {
		t.Fatalf("reset.Content = %q, want %q", reset.Content, original)
	}
}

func TestSQLitePromptStoreRejectsUnknownKey(t *testing.T) {
	t.Parallel()

	db := mustOpenSQLiteDB(t, filepath.Join(t.TempDir(), "prompts.db"))
	store, err := NewSQLitePromptStore(db)
	if err != nil {
		t.Fatalf("NewSQLitePromptStore() error = %v", err)
	}

	if _, err := store.Get(context.Background(), "missing"); err == nil {
		t.Fatal("Get() error = nil")
	}
	if err := store.Upsert(context.Background(), PromptDefinition{Key: "missing", Content: "x"}); err == nil {
		t.Fatal("Upsert() error = nil")
	}
	if err := store.Reset(context.Background(), "missing"); err == nil {
		t.Fatal("Reset() error = nil")
	}
}

func mustOpenSQLiteDB(t *testing.T, path string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}
