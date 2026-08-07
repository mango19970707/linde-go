package graph

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type SQLStructuredMemoryStore struct {
	db      *sql.DB
	dialect string
}

func LoadStructuredMemoryStore(driver, dsn string) (StructuredMemoryStore, error) {
	driver = strings.TrimSpace(driver)
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("structured memory dsn is empty")
	}

	driverName := strings.ToLower(driver)
	if driverName == "" {
		if isPostgresDSN(dsn) {
			driverName = "pgx"
		} else {
			driverName = "sqlite"
		}
	}
	switch driverName {
	case "sqlite", "pgx":
	default:
		return nil, fmt.Errorf("unsupported structured memory driver %q", driverName)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open structured memory db: %w", err)
	}

	store := SQLStructuredMemoryStore{
		db:      db,
		dialect: driverName,
	}
	if err := store.ensureSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func NewSQLiteStructuredMemoryStore(db *sql.DB) (StructuredMemoryStore, error) {
	store := SQLStructuredMemoryStore{db: db, dialect: "sqlite"}
	if err := store.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func NewPostgresStructuredMemoryStore(db *sql.DB) (StructuredMemoryStore, error) {
	store := SQLStructuredMemoryStore{db: db, dialect: "pgx"}
	if err := store.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s SQLStructuredMemoryStore) ensureSchema(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("structured memory db is nil")
	}

	for _, statement := range schemaStatementsByDialect(s.dialect) {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("ensure structured memory schema: %w", err)
		}
	}
	return nil
}

func (s SQLStructuredMemoryStore) FindByTypeAndArea(ctx context.Context, wpTypeID int, area string) ([]StructuredMemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
SELECT wp_type_id, wp_type_name, area, work_description, field_name, field_value_json, source, updated_at
FROM structured_memory_entries
WHERE wp_type_id = ? AND area = ?
ORDER BY updated_at DESC, field_name ASC
`), wpTypeID, strings.TrimSpace(area))
	if err != nil {
		return nil, fmt.Errorf("query structured memory by type and area: %w", err)
	}
	defer rows.Close()

	return scanStructuredMemoryEntries(rows)
}

func (s SQLStructuredMemoryStore) ListAreasByType(ctx context.Context, wpTypeID int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
SELECT DISTINCT area
FROM structured_memory_entries
WHERE wp_type_id = ? AND area <> ''
ORDER BY area ASC
`), wpTypeID)
	if err != nil {
		return nil, fmt.Errorf("query structured memory areas by type: %w", err)
	}
	defer rows.Close()

	areas := make([]string, 0)
	for rows.Next() {
		var area string
		if err := rows.Scan(&area); err != nil {
			return nil, fmt.Errorf("scan structured memory area: %w", err)
		}
		areas = append(areas, strings.TrimSpace(area))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate structured memory areas: %w", err)
	}
	return areas, nil
}

func (s SQLStructuredMemoryStore) FindByType(ctx context.Context, wpTypeID int) ([]StructuredMemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
SELECT wp_type_id, wp_type_name, area, work_description, field_name, field_value_json, source, updated_at
FROM structured_memory_entries
WHERE wp_type_id = ?
ORDER BY updated_at DESC, area ASC, field_name ASC
`), wpTypeID)
	if err != nil {
		return nil, fmt.Errorf("query structured memory by type: %w", err)
	}
	defer rows.Close()

	return scanStructuredMemoryEntries(rows)
}

func scanStructuredMemoryEntries(rows *sql.Rows) ([]StructuredMemoryEntry, error) {
	entries := make([]StructuredMemoryEntry, 0)
	for rows.Next() {
		var entry StructuredMemoryEntry
		var fieldValueJSON string
		if err := rows.Scan(
			&entry.WpTypeID,
			&entry.WpTypeName,
			&entry.Area,
			&entry.WorkDescription,
			&entry.FieldName,
			&fieldValueJSON,
			&entry.Source,
			&entry.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan structured memory entry: %w", err)
		}
		if err := json.Unmarshal([]byte(fieldValueJSON), &entry.FieldValue); err != nil {
			return nil, fmt.Errorf("decode structured memory field value: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate structured memory entries: %w", err)
	}
	return entries, nil
}

func schemaStatementsByDialect(dialect string) []string {
	if dialect == "pgx" {
		return []string{
			`CREATE TABLE IF NOT EXISTS structured_memory_entries (
				wp_type_id BIGINT NOT NULL,
				wp_type_name TEXT NOT NULL DEFAULT '',
				area TEXT NOT NULL DEFAULT '',
				work_description TEXT NOT NULL DEFAULT '',
				field_name TEXT NOT NULL,
				field_value_json TEXT NOT NULL,
				source TEXT NOT NULL,
				updated_at BIGINT NOT NULL,
				PRIMARY KEY (wp_type_id, area, source, field_name)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_structured_memory_type_area ON structured_memory_entries (wp_type_id, area)`,
			`CREATE INDEX IF NOT EXISTS idx_structured_memory_type ON structured_memory_entries (wp_type_id)`,
			`CREATE INDEX IF NOT EXISTS idx_structured_memory_type_updated_at ON structured_memory_entries (wp_type_id, updated_at)`,
		}
	}

	return []string{
		`CREATE TABLE IF NOT EXISTS structured_memory_entries (
			wp_type_id INTEGER NOT NULL,
			wp_type_name TEXT NOT NULL DEFAULT '',
			area TEXT NOT NULL DEFAULT '',
			work_description TEXT NOT NULL DEFAULT '',
			field_name TEXT NOT NULL,
			field_value_json TEXT NOT NULL,
			source TEXT NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (wp_type_id, area, source, field_name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_structured_memory_type_area ON structured_memory_entries (wp_type_id, area)`,
		`CREATE INDEX IF NOT EXISTS idx_structured_memory_type ON structured_memory_entries (wp_type_id)`,
		`CREATE INDEX IF NOT EXISTS idx_structured_memory_type_updated_at ON structured_memory_entries (wp_type_id, updated_at)`,
	}
}

func isPostgresDSN(dsn string) bool {
	lower := strings.ToLower(strings.TrimSpace(dsn))
	return strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") || strings.Contains(lower, "host=")
}

func (s SQLStructuredMemoryStore) rebind(query string) string {
	if s.dialect != "pgx" {
		return query
	}
	result := strings.Builder{}
	result.Grow(len(query) + 16)
	index := 1
	for _, ch := range query {
		if ch == '?' {
			result.WriteString(fmt.Sprintf("$%d", index))
			index++
			continue
		}
		result.WriteRune(ch)
	}
	return result.String()
}
