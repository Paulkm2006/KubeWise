package db

import (
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB is the global database connection pool handle.
// goroutine-safe, set once during Open().
var DB *sql.DB

type Wrapper struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at the given dir.
func Open(dir string) (*Wrapper, error) {
	path := filepath.Join(dir, "kubewise.db")
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	w := &Wrapper{conn}
	if err := w.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// Set global DB
	DB = conn
	return w, nil
}

func (w *Wrapper) migrate() error {
	// Run idempotent migrations (CREATE TABLE IF NOT EXISTS, CREATE INDEX IF NOT EXISTS)
	files := []string{"migrations/001_initial.sql", "migrations/002_diagnosis_events.sql"}
	for _, f := range files {
		data, err := migrations.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", f, err)
		}
		if _, err := w.Exec(string(data)); err != nil {
			return fmt.Errorf("exec migration %s: %w", f, err)
		}
	}

	// Non-idempotent migrations: SQLite doesn't support ALTER TABLE ADD COLUMN IF NOT EXISTS,
	// so we check column existence in Go first.
	return w.ensureDiagnosisStatusColumn()
}

// ensureDiagnosisStatusColumn adds the status column if it doesn't exist yet.
// Safe to call on every startup — the PRAGMA check + conditional ALTER is idempotent.
func (w *Wrapper) ensureDiagnosisStatusColumn() error {
	var count int
	if err := w.QueryRow("SELECT COUNT(*) FROM pragma_table_info('diagnoses') WHERE name='status'").Scan(&count); err != nil {
		return fmt.Errorf("check status column: %w", err)
	}
	if count == 0 {
		if _, err := w.Exec("ALTER TABLE diagnoses ADD COLUMN status TEXT NOT NULL DEFAULT 'pending'"); err != nil {
			return fmt.Errorf("add status column: %w", err)
		}
	}
	return nil
}
