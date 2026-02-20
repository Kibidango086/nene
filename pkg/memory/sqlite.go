package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteMemory struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

func NewSQLiteMemory(dataDir string) (*SQLiteMemory, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "memory.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	mem := &SQLiteMemory{
		db:   db,
		path: dbPath,
	}

	if err := mem.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return mem, nil
}

func (m *SQLiteMemory) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS memories (
		id          TEXT PRIMARY KEY,
		key         TEXT NOT NULL UNIQUE,
		content     TEXT NOT NULL,
		category    TEXT NOT NULL DEFAULT 'core',
		session_id  TEXT,
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category);
	CREATE INDEX IF NOT EXISTS idx_memories_key ON memories(key);
	CREATE INDEX IF NOT EXISTS idx_memories_session ON memories(session_id);

	CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
		key, content, content=memories, content_rowid=rowid
	);

	CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
		INSERT INTO memories_fts(rowid, key, content)
		VALUES (new.rowid, new.key, new.content);
	END;

	CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
		INSERT INTO memories_fts(memories_fts, rowid, key, content)
		VALUES ('delete', old.rowid, old.key, old.content);
	END;

	CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
		INSERT INTO memories_fts(memories_fts, rowid, key, content)
		VALUES ('delete', old.rowid, old.key, old.content);
		INSERT INTO memories_fts(rowid, key, content)
		VALUES (new.rowid, new.key, new.content);
	END;
	`

	_, err := m.db.Exec(schema)
	return err
}

func (m *SQLiteMemory) Store(ctx context.Context, req *StoreRequest) (*Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	id := uuid.New().String()

	if req.Category == "" {
		req.Category = CategoryCore
	}

	stmt := `
	INSERT INTO memories (id, key, content, category, session_id, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(key) DO UPDATE SET
		content = excluded.content,
		category = excluded.category,
		session_id = excluded.session_id,
		updated_at = excluded.updated_at
	`

	_, err := m.db.ExecContext(ctx, stmt,
		id, req.Key, req.Content, string(req.Category), req.SessionID,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("store memory: %w", err)
	}

	return &Entry{
		ID:        id,
		Key:       req.Key,
		Content:   req.Content,
		Category:  req.Category,
		SessionID: req.SessionID,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (m *SQLiteMemory) Recall(ctx context.Context, req *RecallRequest) ([]*Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if req.Limit <= 0 {
		req.Limit = 5
	}

	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, nil
	}

	ftsQuery := buildFTSQuery(query)

	sql := `
	SELECT m.id, m.key, m.content, m.category, m.session_id, m.created_at, m.updated_at
	FROM memories m
	JOIN memories_fts f ON m.rowid = f.rowid
	WHERE memories_fts MATCH ?
	ORDER BY bm25(memories_fts)
	LIMIT ?
	`

	rows, err := m.db.QueryContext(ctx, sql, ftsQuery, req.Limit)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "fts") {
			return m.recallFallback(ctx, req)
		}
		return nil, fmt.Errorf("recall memory: %w", err)
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}

	if len(entries) == 0 {
		return m.recallFallback(ctx, req)
	}

	return entries, nil
}

func (m *SQLiteMemory) recallFallback(ctx context.Context, req *RecallRequest) ([]*Entry, error) {
	keywords := strings.Fields(req.Query)
	if len(keywords) == 0 {
		return nil, nil
	}

	var conditions []string
	var args []interface{}
	for _, kw := range keywords {
		conditions = append(conditions, "(content LIKE ? OR key LIKE ?)")
		args = append(args, "%"+kw+"%", "%"+kw+"%")
	}

	sql := fmt.Sprintf(`
		SELECT id, key, content, category, session_id, created_at, updated_at
		FROM memories
		WHERE %s
		ORDER BY updated_at DESC
		LIMIT ?
	`, strings.Join(conditions, " OR "))

	args = append(args, req.Limit)

	rows, err := m.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("recall fallback: %w", err)
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}

	return entries, nil
}

func (m *SQLiteMemory) Get(ctx context.Context, key string) (*Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
	SELECT id, key, content, category, session_id, created_at, updated_at
	FROM memories
	WHERE key = ?
	`

	row := m.db.QueryRowContext(ctx, query, key)
	e, err := scanEntryRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get memory: %w", err)
	}

	return e, nil
}

func (m *SQLiteMemory) List(ctx context.Context, req *ListRequest) ([]*Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if req.Limit <= 0 {
		req.Limit = 100
	}

	var query string
	var args []interface{}

	if req.Category != "" {
		query = `
		SELECT id, key, content, category, session_id, created_at, updated_at
		FROM memories
		WHERE category = ?
		ORDER BY updated_at DESC
		LIMIT ?
		`
		args = []interface{}{string(req.Category), req.Limit}
	} else {
		query = `
		SELECT id, key, content, category, session_id, created_at, updated_at
		FROM memories
		ORDER BY updated_at DESC
		LIMIT ?
		`
		args = []interface{}{req.Limit}
	}

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}

	return entries, nil
}

func (m *SQLiteMemory) Forget(ctx context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result, err := m.db.ExecContext(ctx, "DELETE FROM memories WHERE key = ?", key)
	if err != nil {
		return false, fmt.Errorf("forget memory: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return affected > 0, nil
}

func (m *SQLiteMemory) Count(ctx context.Context) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int
	err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memories").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count memories: %w", err)
	}

	return count, nil
}

func (m *SQLiteMemory) Close() error {
	return m.db.Close()
}

func buildFTSQuery(query string) string {
	words := strings.Fields(query)
	var parts []string
	for _, w := range words {
		parts = append(parts, fmt.Sprintf("\"%s\"", w))
	}
	return strings.Join(parts, " OR ")
}

func scanEntry(rows *sql.Rows) (*Entry, error) {
	var e Entry
	var createdAt, updatedAt string
	err := rows.Scan(
		&e.ID, &e.Key, &e.Content, &e.Category, &e.SessionID,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan entry: %w", err)
	}

	e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &e, nil
}

func scanEntryRow(row *sql.Row) (*Entry, error) {
	var e Entry
	var createdAt, updatedAt string
	err := row.Scan(
		&e.ID, &e.Key, &e.Content, &e.Category, &e.SessionID,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &e, nil
}
