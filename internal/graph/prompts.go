package graph

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const (
	PromptKeyWpInfoExtraction      = "wp_info_extraction"
	PromptKeyWpTypeRecognition     = "wp_type_recognition"
	PromptKeyWpFillAreaSelection   = "wp_fill_area_selection"
	PromptKeyWpFillFieldCompletion = "wp_fill_field_completion"
)

type PromptDefinition struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
	UpdatedAt   int64  `json:"updated_at"`
}

type PromptStore interface {
	List(ctx context.Context) ([]PromptDefinition, error)
	Get(ctx context.Context, key string) (PromptDefinition, error)
	Upsert(ctx context.Context, prompt PromptDefinition) error
	Reset(ctx context.Context, key string) error
}

type SQLPromptStore struct {
	db      *sql.DB
	dialect string
}

type memoryPromptStore struct{}

func LoadPromptStore(driver, dsn string) (PromptStore, error) {
	driver = strings.TrimSpace(driver)
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("prompt dsn is empty")
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
		return nil, fmt.Errorf("unsupported prompt driver %q", driverName)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open prompt db: %w", err)
	}

	store := SQLPromptStore{db: db, dialect: driverName}
	if err := store.ensureSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func newDefaultPromptStore() PromptStore {
	return memoryPromptStore{}
}

func NewSQLitePromptStore(db *sql.DB) (PromptStore, error) {
	store := SQLPromptStore{db: db, dialect: "sqlite"}
	if err := store.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func NewPostgresPromptStore(db *sql.DB) (PromptStore, error) {
	store := SQLPromptStore{db: db, dialect: "pgx"}
	if err := store.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s SQLPromptStore) List(ctx context.Context) ([]PromptDefinition, error) {
	index := defaultPromptIndex()
	rows, err := s.db.QueryContext(ctx, `
SELECT prompt_key, title, description, content, updated_at
FROM llm_prompts
`)
	if err != nil {
		return nil, fmt.Errorf("query prompts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var prompt PromptDefinition
		if err := rows.Scan(&prompt.Key, &prompt.Title, &prompt.Description, &prompt.Content, &prompt.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan prompt: %w", err)
		}
		index[prompt.Key] = prompt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prompts: %w", err)
	}

	keys := make([]string, 0, len(index))
	for key := range index {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	prompts := make([]PromptDefinition, 0, len(keys))
	for _, key := range keys {
		prompts = append(prompts, index[key])
	}
	return prompts, nil
}

func (s SQLPromptStore) Get(ctx context.Context, key string) (PromptDefinition, error) {
	key = strings.TrimSpace(key)
	defaultPrompt, ok := defaultPromptByKey(key)
	if !ok {
		return PromptDefinition{}, fmt.Errorf("prompt %q not found", key)
	}

	var prompt PromptDefinition
	err := s.db.QueryRowContext(ctx, s.rebind(`
SELECT prompt_key, title, description, content, updated_at
FROM llm_prompts
WHERE prompt_key = ?
`), key).Scan(&prompt.Key, &prompt.Title, &prompt.Description, &prompt.Content, &prompt.UpdatedAt)
	if err == nil {
		return prompt, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return defaultPrompt, nil
	}
	return PromptDefinition{}, fmt.Errorf("query prompt %q: %w", key, err)
}

func (s SQLPromptStore) Upsert(ctx context.Context, prompt PromptDefinition) error {
	normalized, err := normalizePromptDefinition(prompt)
	if err != nil {
		return err
	}
	normalized.UpdatedAt = time.Now().Unix()

	_, err = s.db.ExecContext(ctx, s.rebind(`
INSERT INTO llm_prompts (prompt_key, title, description, content, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(prompt_key) DO UPDATE SET
	title = excluded.title,
	description = excluded.description,
	content = excluded.content,
	updated_at = excluded.updated_at
`), normalized.Key, normalized.Title, normalized.Description, normalized.Content, normalized.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert prompt %q: %w", normalized.Key, err)
	}
	return nil
}

func (s SQLPromptStore) Reset(ctx context.Context, key string) error {
	defaultPrompt, ok := defaultPromptByKey(strings.TrimSpace(key))
	if !ok {
		return fmt.Errorf("prompt %q not found", key)
	}
	return s.Upsert(ctx, defaultPrompt)
}

func (s SQLPromptStore) ensureSchema(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("prompt db is nil")
	}

	for _, statement := range promptSchemaStatementsByDialect(s.dialect) {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("ensure prompt schema: %w", err)
		}
	}
	for _, prompt := range defaultPromptDefinitions() {
		if err := s.Upsert(ctx, prompt); err != nil {
			return fmt.Errorf("seed prompt %q: %w", prompt.Key, err)
		}
	}
	return nil
}

func promptSchemaStatementsByDialect(dialect string) []string {
	if dialect == "pgx" {
		return []string{
			`CREATE TABLE IF NOT EXISTS llm_prompts (
				prompt_key TEXT PRIMARY KEY,
				title TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				content TEXT NOT NULL,
				updated_at BIGINT NOT NULL
			)`,
		}
	}

	return []string{
		`CREATE TABLE IF NOT EXISTS llm_prompts (
			prompt_key TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
	}
}

func normalizePromptDefinition(prompt PromptDefinition) (PromptDefinition, error) {
	key := strings.TrimSpace(prompt.Key)
	defaultPrompt, ok := defaultPromptByKey(key)
	if !ok {
		return PromptDefinition{}, fmt.Errorf("prompt %q not found", key)
	}

	content := strings.TrimSpace(prompt.Content)
	if content == "" {
		return PromptDefinition{}, fmt.Errorf("prompt %q content is empty", key)
	}

	title := strings.TrimSpace(prompt.Title)
	if title == "" {
		title = defaultPrompt.Title
	}
	description := strings.TrimSpace(prompt.Description)
	if description == "" {
		description = defaultPrompt.Description
	}

	return PromptDefinition{
		Key:         key,
		Title:       title,
		Description: description,
		Content:     content,
	}, nil
}

func defaultPromptDefinitions() []PromptDefinition {
	return []PromptDefinition{
		{
			Key:         PromptKeyWpInfoExtraction,
			Title:       "作业票信息提取",
			Description: "提取区域、时间、描述、设备和单位。",
			Content:     wpInfoExtractionPrompt,
		},
		{
			Key:         PromptKeyWpTypeRecognition,
			Title:       "作业票类型识别",
			Description: "从候选作业票中选择需要开具的类型。",
			Content:     wpTypeRecognitionPrompt,
		},
		{
			Key:         PromptKeyWpFillAreaSelection,
			Title:       "结构化记忆地点匹配",
			Description: "从候选地点中选择最可能对应的地点。",
			Content:     wpFillAreaSelectionPrompt,
		},
		{
			Key:         PromptKeyWpFillFieldCompletion,
			Title:       "作业票字段补全",
			Description: "根据描述和历史样本补全必填字段。",
			Content:     wpFillFieldCompletionPrompt,
		},
	}
}

func defaultPromptIndex() map[string]PromptDefinition {
	index := make(map[string]PromptDefinition, 4)
	for _, prompt := range defaultPromptDefinitions() {
		index[prompt.Key] = prompt
	}
	return index
}

func defaultPromptByKey(key string) (PromptDefinition, bool) {
	prompt, ok := defaultPromptIndex()[key]
	return prompt, ok
}

func (memoryPromptStore) List(ctx context.Context) ([]PromptDefinition, error) {
	prompts := defaultPromptDefinitions()
	return prompts, nil
}

func (memoryPromptStore) Get(ctx context.Context, key string) (PromptDefinition, error) {
	prompt, ok := defaultPromptByKey(key)
	if !ok {
		return PromptDefinition{}, fmt.Errorf("prompt %q not found", key)
	}
	return prompt, nil
}

func (memoryPromptStore) Upsert(ctx context.Context, prompt PromptDefinition) error {
	return nil
}

func (memoryPromptStore) Reset(ctx context.Context, key string) error {
	return nil
}

func (s SQLPromptStore) rebind(query string) string {
	if s.dialect != "pgx" {
		return query
	}
	var builder strings.Builder
	builder.Grow(len(query) + 16)
	index := 1
	for _, ch := range query {
		if ch == '?' {
			builder.WriteString(fmt.Sprintf("$%d", index))
			index++
			continue
		}
		builder.WriteRune(ch)
	}
	return builder.String()
}
