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
			if isIgnorableSchemaError(err, statement) {
				continue
			}
			return fmt.Errorf("ensure structured memory schema: %w", err)
		}
	}
	return nil
}

func (s SQLStructuredMemoryStore) FindByTypeAndArea(ctx context.Context, wpTypeID int, area string) ([]StructuredMemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
SELECT wp_type_id, wp_type_name, area, work_description, field_description, field_name, field_value_json, source, updated_at
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
SELECT wp_type_id, wp_type_name, area, work_description, field_description, field_name, field_value_json, source, updated_at
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
			&entry.FieldDescription,
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

func (s SQLStructuredMemoryStore) Upsert(ctx context.Context, entry StructuredMemoryEntry) error {
	if s.db == nil {
		return fmt.Errorf("structured memory db is nil")
	}

	fieldValueJSON, err := json.Marshal(entry.FieldValue)
	if err != nil {
		return fmt.Errorf("encode structured memory field value: %w", err)
	}

	_, err = s.db.ExecContext(ctx, s.rebind(`
INSERT INTO structured_memory_entries (
	wp_type_id,
	wp_type_name,
	area,
	work_description,
	field_description,
	field_name,
	field_value_json,
	source,
	updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(wp_type_id, area, source, field_name) DO UPDATE SET
	wp_type_name = excluded.wp_type_name,
	work_description = excluded.work_description,
	field_description = excluded.field_description,
	field_value_json = excluded.field_value_json,
	updated_at = excluded.updated_at
`),
		entry.WpTypeID,
		entry.WpTypeName,
		entry.Area,
		entry.WorkDescription,
		entry.FieldDescription,
		entry.FieldName,
		string(fieldValueJSON),
		entry.Source,
		entry.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert structured memory entry %q: %w", entry.FieldName, err)
	}
	return nil
}

func (s SQLStructuredMemoryStore) FindIntentMemoryBySite(ctx context.Context, siteID int) ([]StructuredIntentMemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
SELECT site_id, area, work_description, wp_type_ids_json, source, updated_at
FROM structured_intent_memory_entries
WHERE site_id = ?
ORDER BY updated_at DESC, work_description ASC
`), siteID)
	if err != nil {
		return nil, fmt.Errorf("query structured intent memory by site: %w", err)
	}
	defer rows.Close()

	entries := make([]StructuredIntentMemoryEntry, 0)
	for rows.Next() {
		var entry StructuredIntentMemoryEntry
		var wpTypeIDsJSON string
		if err := rows.Scan(
			&entry.SiteID,
			&entry.Area,
			&entry.WorkDescription,
			&wpTypeIDsJSON,
			&entry.Source,
			&entry.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan structured intent memory entry: %w", err)
		}
		if err := json.Unmarshal([]byte(wpTypeIDsJSON), &entry.WpTypeIDs); err != nil {
			return nil, fmt.Errorf("decode structured intent memory wp type ids: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate structured intent memory entries: %w", err)
	}
	return entries, nil
}

func (s SQLStructuredMemoryStore) UpsertIntentMemory(ctx context.Context, entry StructuredIntentMemoryEntry) error {
	if s.db == nil {
		return fmt.Errorf("structured memory db is nil")
	}

	wpTypeIDsJSON, err := json.Marshal(entry.WpTypeIDs)
	if err != nil {
		return fmt.Errorf("encode structured intent memory wp type ids: %w", err)
	}

	_, err = s.db.ExecContext(ctx, s.rebind(`
INSERT INTO structured_intent_memory_entries (
	site_id,
	area,
	work_description,
	wp_type_ids_json,
	source,
	updated_at
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(site_id, area, work_description, source) DO UPDATE SET
	wp_type_ids_json = excluded.wp_type_ids_json,
	updated_at = excluded.updated_at
`),
		entry.SiteID,
		entry.Area,
		entry.WorkDescription,
		string(wpTypeIDsJSON),
		entry.Source,
		entry.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert structured intent memory entry %q: %w", entry.WorkDescription, err)
	}
	return nil
}

func schemaStatementsByDialect(dialect string) []string {
	if dialect == "pgx" {
		return []string{
			`CREATE TABLE IF NOT EXISTS structured_memory_entries (
				wp_type_id BIGINT NOT NULL,
				wp_type_name TEXT NOT NULL DEFAULT '',
				area TEXT NOT NULL DEFAULT '',
				work_description TEXT NOT NULL DEFAULT '',
				field_description TEXT NOT NULL DEFAULT '',
				field_name TEXT NOT NULL,
				field_value_json TEXT NOT NULL,
				source TEXT NOT NULL,
				updated_at BIGINT NOT NULL,
				PRIMARY KEY (wp_type_id, area, source, field_name)
			)`,
			`ALTER TABLE structured_memory_entries ADD COLUMN IF NOT EXISTS field_description TEXT NOT NULL DEFAULT ''`,
			`CREATE INDEX IF NOT EXISTS idx_structured_memory_type_area ON structured_memory_entries (wp_type_id, area)`,
			`CREATE INDEX IF NOT EXISTS idx_structured_memory_type ON structured_memory_entries (wp_type_id)`,
			`CREATE INDEX IF NOT EXISTS idx_structured_memory_type_updated_at ON structured_memory_entries (wp_type_id, updated_at)`,
			`CREATE TABLE IF NOT EXISTS structured_intent_memory_entries (
				site_id BIGINT NOT NULL,
				area TEXT NOT NULL DEFAULT '',
				work_description TEXT NOT NULL DEFAULT '',
				wp_type_ids_json TEXT NOT NULL,
				source TEXT NOT NULL,
				updated_at BIGINT NOT NULL,
				PRIMARY KEY (site_id, area, work_description, source)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_structured_intent_memory_site ON structured_intent_memory_entries (site_id, updated_at)`,
			`CREATE INDEX IF NOT EXISTS idx_structured_intent_memory_desc ON structured_intent_memory_entries (site_id, work_description)`,
		}
	}

	return []string{
		`CREATE TABLE IF NOT EXISTS structured_memory_entries (
			wp_type_id INTEGER NOT NULL,
			wp_type_name TEXT NOT NULL DEFAULT '',
			area TEXT NOT NULL DEFAULT '',
			work_description TEXT NOT NULL DEFAULT '',
			field_description TEXT NOT NULL DEFAULT '',
			field_name TEXT NOT NULL,
			field_value_json TEXT NOT NULL,
			source TEXT NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (wp_type_id, area, source, field_name)
		)`,
		`ALTER TABLE structured_memory_entries ADD COLUMN field_description TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_structured_memory_type_area ON structured_memory_entries (wp_type_id, area)`,
		`CREATE INDEX IF NOT EXISTS idx_structured_memory_type ON structured_memory_entries (wp_type_id)`,
		`CREATE INDEX IF NOT EXISTS idx_structured_memory_type_updated_at ON structured_memory_entries (wp_type_id, updated_at)`,
		`CREATE TABLE IF NOT EXISTS structured_intent_memory_entries (
			site_id INTEGER NOT NULL,
			area TEXT NOT NULL DEFAULT '',
			work_description TEXT NOT NULL DEFAULT '',
			wp_type_ids_json TEXT NOT NULL,
			source TEXT NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (site_id, area, work_description, source)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_structured_intent_memory_site ON structured_intent_memory_entries (site_id, updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_structured_intent_memory_desc ON structured_intent_memory_entries (site_id, work_description)`,
	}
}

func isPostgresDSN(dsn string) bool {
	lower := strings.ToLower(strings.TrimSpace(dsn))
	return strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") || strings.Contains(lower, "host=")
}

func isIgnorableSchemaError(err error, statement string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	stmt := strings.ToLower(strings.TrimSpace(statement))
	return strings.HasPrefix(stmt, "alter table") && strings.Contains(message, "duplicate column")
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
