package graph

import (
	"context"
	"database/sql"
	"testing"
)

func TestSQLiteStructuredMemoryStoreQueriesByTypeAndArea(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", `D:\code\linde-go-v2\data\structured_memory.db`)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStructuredMemoryStore(db)
	if err != nil {
		t.Fatalf("NewSQLiteStructuredMemoryStore() error = %v", err)
	}

	area := "空分装置东侧冷箱平台"

	var wantAreaCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM structured_memory_entries WHERE wp_type_id = ? AND area = ?`, 23, area).Scan(&wantAreaCount); err != nil {
		t.Fatalf("count structured memory entries by type and area: %v", err)
	}

	entries, err := store.FindByTypeAndArea(context.Background(), 23, area)
	if err != nil {
		t.Fatalf("FindByTypeAndArea() error = %v", err)
	}
	if len(entries) != wantAreaCount {
		t.Fatalf("len(entries) = %d, want %d", len(entries), wantAreaCount)
	}

	wantAreas, err := loadDistinctAreas(db, 23)
	if err != nil {
		t.Fatalf("loadDistinctAreas() error = %v", err)
	}

	areas, err := store.ListAreasByType(context.Background(), 23)
	if err != nil {
		t.Fatalf("ListAreasByType() error = %v", err)
	}
	if len(areas) != len(wantAreas) {
		t.Fatalf("len(areas) = %d, want %d", len(areas), len(wantAreas))
	}
	for i := range wantAreas {
		if areas[i] != wantAreas[i] {
			t.Fatalf("areas = %#v, want %#v", areas, wantAreas)
		}
	}

	var wantTotal int
	if err := db.QueryRow(`SELECT COUNT(*) FROM structured_memory_entries WHERE wp_type_id = ?`, 23).Scan(&wantTotal); err != nil {
		t.Fatalf("count structured memory entries by type: %v", err)
	}

	allEntries, err := store.FindByType(context.Background(), 23)
	if err != nil {
		t.Fatalf("FindByType() error = %v", err)
	}
	if len(allEntries) != wantTotal {
		t.Fatalf("len(allEntries) = %d, want %d", len(allEntries), wantTotal)
	}
}

func loadDistinctAreas(db *sql.DB, wpTypeID int) ([]string, error) {
	rows, err := db.Query(`SELECT DISTINCT area FROM structured_memory_entries WHERE wp_type_id = ? AND area <> '' ORDER BY area ASC`, wpTypeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	areas := make([]string, 0)
	for rows.Next() {
		var area string
		if err := rows.Scan(&area); err != nil {
			return nil, err
		}
		areas = append(areas, area)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return areas, nil
}
