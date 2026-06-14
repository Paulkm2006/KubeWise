package persistence

import (
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB wraps a SQLite connection opened by Open.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at the given dir.
func Open(dir string) (*DB, error) {
	path := filepath.Join(dir, "kubewise.db")
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	w := &DB{conn}
	if err := w.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return w, nil
}

func (w *DB) migrate() error {
	files := []string{
		"migrations/001_initial.sql",
		"migrations/002_diagnosis_events.sql",
		"migrations/003_diagnosis_structured.sql",
		"migrations/004_audits.sql",
	}
	for _, f := range files {
		data, err := migrations.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", f, err)
		}
		if _, err := w.Exec(string(data)); err != nil {
			return fmt.Errorf("exec migration %s: %w", f, err)
		}
	}
	if err := w.ensureDiagnosisStatusColumn(); err != nil {
		return err
	}
	return w.ensureDiagnosisStructuredColumns()
}

func (w *DB) ensureDiagnosisStatusColumn() error {
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

func (w *DB) ensureDiagnosisStructuredColumns() error {
	columns := map[string]string{
		"report_json": "TEXT",
	}
	for name, colType := range columns {
		var count int
		if err := w.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info('diagnoses') WHERE name=?",
			name,
		).Scan(&count); err != nil {
			return fmt.Errorf("check diagnoses.%s: %w", name, err)
		}
		if count == 0 {
			if _, err := w.Exec(fmt.Sprintf("ALTER TABLE diagnoses ADD COLUMN %s %s", name, colType)); err != nil {
				return fmt.Errorf("add diagnoses.%s: %w", name, err)
			}
		}
	}

	eventColumns := map[string]string{
		"summary":      "TEXT DEFAULT ''",
		"payload_kind": "TEXT DEFAULT ''",
		"payload_json": "TEXT DEFAULT ''",
	}
	for name, colType := range eventColumns {
		var count int
		if err := w.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info('diagnosis_events') WHERE name=?",
			name,
		).Scan(&count); err != nil {
			return fmt.Errorf("check diagnosis_events.%s: %w", name, err)
		}
		if count == 0 {
			if _, err := w.Exec(fmt.Sprintf("ALTER TABLE diagnosis_events ADD COLUMN %s %s", name, colType)); err != nil {
				return fmt.Errorf("add diagnosis_events.%s: %w", name, err)
			}
		}
	}
	return nil
}
